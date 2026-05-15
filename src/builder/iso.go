package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
)

// ISOBuilder converts a bootc container image into a bootable ISO.
//
// How it works:
//
//  1. You give it a bootc container image (local or from a registry).
//     The image contains a full Fedora OS — kernel, bootloader, everything.
//
//  2. bootc-image-builder is called. Internally it:
//     a) Pulls/loads the container image
//     b) Extracts the OSTree commit embedded in it
//     c) Uses osbuild pipelines to:
//        - create a squashfs LiveOS image from the container rootfs
//        - embed an Anaconda installer (if kickstart provided) OR
//          a direct live-boot environment
//        - wrap everything in an El Torito + hybrid-MBR ISO
//           so it boots from both USB and DVD/optical
//     d) Output: a standard .iso file
//
//  3. The resulting ISO can be:
//     - Written to USB: `dd if=my.iso of=/dev/sdX bs=4M`
//     - Used in QEMU/VirtualBox/VMware directly
//     - Served via PXE (with additional setup)
//
// Requirements on the build host:
//   - podman (to pull/run bootc-image-builder)
//   - bootc-image-builder runs as a privileged container itself
//     (needs root or `sudo` unless newuidmap is configured)
type ISOBuilder struct {
	cfg     *config.Config
	paths   *config.Paths
	verbose bool
	release bool
}

func NewISO(cfg *config.Config, paths *config.Paths, verbose, release bool) *ISOBuilder {
	return &ISOBuilder{cfg: cfg, paths: paths, verbose: verbose, release: release}
}

func (b *ISOBuilder) Validate() error {
	if b.cfg.Project.BaseDistro != "fedora" {
		return fmt.Errorf("unsupported distro %q", b.cfg.Project.BaseDistro)
	}
	ui.OK("Project valid — Fedora %d / %s", b.cfg.Project.BaseVersion, b.cfg.Project.Arch)
	return nil
}

func (b *ISOBuilder) PrepareDirs() error {
	dirs := []string{
		b.paths.BuildDir,
		b.paths.CacheDir,
		b.paths.OutputDir,
		filepath.Join(b.paths.BuildDir, "iso-work"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("cannot create %s: %w", d, err)
		}
	}
	ui.OK("Build directories ready")
	return nil
}

// CheckTools verifies that podman and bootc-image-builder are present.
// bootc-image-builder itself runs as a privileged podman container,
// so technically only podman is strictly required — but we also check
// if the bootc-image-builder binary is available for direct invocation.
func (b *ISOBuilder) CheckTools() error {
	required := []string{"podman"}
	optional := []string{"bootc-image-builder"}
	missing := []string{}

	for _, t := range required {
		if _, err := exec.LookPath(t); err != nil {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools: %s\n  Install: sudo dnf install %s",
			strings.Join(missing, ", "), strings.Join(missing, " "))
	}
	ui.OK("podman: available")

	for _, t := range optional {
		if _, err := exec.LookPath(t); err != nil {
			ui.Info("%s not found — will use podman-run mode", t)
		} else {
			ui.OK("%s: available", t)
		}
	}
	return nil
}

// RegistryLogin logs into the registry before pulling the source image.
func (b *ISOBuilder) RegistryLogin(registry, username, token string) error {
	if token == "" {
		ui.Info("No token — skipping registry login")
		return nil
	}
	if username == "" {
		username = "token"
	}
	ui.Info("Logging into %s as %s", registry, username)
	return b.run("podman", "login",
		"--username", username,
		"--password", token,
		registry,
	)
}


func (b *ISOBuilder) PullImage(image string) error {
	// If it looks like a local image name (no registry prefix), skip pull
	if !strings.Contains(image, "/") || strings.HasPrefix(image, "localhost/") {
		ui.Info("Using local image: %s", image)
		return nil
	}
	ui.Info("Pulling: %s", image)
	return b.run("podman", "pull", image)
}

// GenerateKickstart writes the Anaconda kickstart file to the build dir.
func (b *ISOBuilder) GenerateKickstart(ksPath string) error {
	if ksPath == "" {
		ksPath = filepath.Join(b.paths.BuildDir, "anaconda.ks")
	}
	ks, err := generateKickstart(b.cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(ksPath, []byte(ks), 0644); err != nil {
		return fmt.Errorf("cannot write kickstart: %w", err)
	}
	ui.OK("Kickstart → %s", ksPath)
	return nil
}

// BuildISO invokes bootc-image-builder to produce the final ISO.
//
// bootc-image-builder can be called two ways:
//
//  A) As a binary (if installed on the host):
//     bootc-image-builder build --type iso --output <dir> <image>
//
//  B) Via podman run (always works, no host install needed):
//     podman run --rm -it --privileged \
//       -v /var/lib/containers/storage:/var/lib/containers/storage \
//       -v $(pwd)/output:/output \
//       quay.io/centos-bootc/bootc-image-builder:latest \
//       build --type iso --output /output <image>
//
// We prefer (A) if available, fall back to (B).
func (b *ISOBuilder) BuildISO(sourceImage, output, label, kickstart string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return fmt.Errorf("cannot create output dir: %w", err)
	}

	outDir := filepath.Dir(output)

	// Prefer direct binary
	if _, err := exec.LookPath("bootc-image-builder"); err == nil {
		return b.buildViaBinary(sourceImage, outDir, output, label, kickstart)
	}

	// Fall back: run bootc-image-builder as a privileged podman container
	return b.buildViaPodman(sourceImage, outDir, output, label, kickstart)
}

// buildViaBinary calls bootc-image-builder directly.
func (b *ISOBuilder) buildViaBinary(sourceImage, outDir, finalPath, label, kickstart string) error {
	ui.Info("Method: bootc-image-builder binary")

	args := []string{
		"build",
		"--type", "iso",
		"--output", outDir,
	}
	if kickstart != "" {
		if _, err := os.Stat(kickstart); err == nil {
			args = append(args, "--config", kickstart)
		}
	}
	if label != "" {
		args = append(args, "--iso-label", label)
	}
	args = append(args, sourceImage)

	ui.Info("bootc-image-builder %s", strings.Join(args, " "))
	if err := b.run("bootc-image-builder", args...); err != nil {
		return err
	}

	return b.renameOutput(outDir, finalPath)
}

// buildViaPodman runs bootc-image-builder inside a privileged podman container.
// This is the standard recommended way — no host install of bootc-image-builder needed.
//
//	podman run --rm --privileged \
//	  --volume /var/lib/containers/storage:/var/lib/containers/storage \
//	  --volume <outDir>:/output \
//	  [--volume <kickstart>:/config.ks:ro] \
//	  quay.io/centos-bootc/bootc-image-builder:latest \
//	  build --type iso --output /output <sourceImage>
func (b *ISOBuilder) buildViaPodman(sourceImage, outDir, finalPath, label, kickstart string) error {
	bibImage := "quay.io/centos-bootc/bootc-image-builder:latest"
	ui.Info("Method: bootc-image-builder via podman (privileged container)")
	ui.Info("BIB image: %s", bibImage)

	args := []string{
		"run", "--rm",
		"--privileged",
		"--pull=newer",
		// Share the host container storage so BIB can access the source image
		// without pulling it again
		"--volume", "/var/lib/containers/storage:/var/lib/containers/storage",
		// Output directory
		"--volume", outDir + ":/output",
		// Needed for osbuild inside BIB
		"--volume", "/dev:/dev",
	}

	// Mount kickstart if present
	if kickstart != "" {
		if _, err := os.Stat(kickstart); err == nil {
			args = append(args, "--volume", kickstart+":/config.ks:ro")
		}
	}

	// The bootc-image-builder image
	args = append(args, bibImage)

	// bootc-image-builder sub-command
	args = append(args, "build", "--type", "iso", "--output", "/output")

	if kickstart != "" {
		if _, err := os.Stat(kickstart); err == nil {
			args = append(args, "--config", "/config.ks")
		}
	}
	if label != "" {
		args = append(args, "--iso-label", label)
	}

	// Source image — must be reachable from inside the BIB container.
	// Since we shared /var/lib/containers/storage the local image is visible.
	args = append(args, sourceImage)

	ui.Info("podman %s", strings.Join(args, " "))
	if err := b.run("podman", args...); err != nil {
		return fmt.Errorf(
			"bootc-image-builder failed\n\n"+
				"  Common fixes:\n"+
				"    • Run as root / with sudo\n"+
				"    • Make sure the source image exists locally:\n"+
				"        podman images | grep %s\n"+
				"    • Pull it first: legendaryos build iso --source <registry/img:tag>\n",
			sourceImage)
	}

	return b.renameOutput(outDir, finalPath)
}

// renameOutput finds the ISO produced by bootc-image-builder and moves it
// to the user-specified output path.
// BIB writes to <outDir>/bootiso/disk.iso by default.
func (b *ISOBuilder) renameOutput(outDir, finalPath string) error {
	candidates := []string{
		filepath.Join(outDir, "bootiso", "disk.iso"),
		filepath.Join(outDir, "disk.iso"),
		filepath.Join(outDir, "image.iso"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if err := os.Rename(c, finalPath); err != nil {
				// rename across devices fails; copy+delete instead
				if err2 := copyFile(c, finalPath); err2 != nil {
					return fmt.Errorf("cannot move ISO: %w", err2)
				}
				os.Remove(c)
			}
			ui.OK("ISO → %s", finalPath)
			return nil
		}
	}
	// BIB succeeded but we can't find the output — list what's there
	entries, _ := os.ReadDir(outDir)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	ui.Warn("Could not find ISO in output dir. Contents: %s", strings.Join(names, ", "))
	return nil
}

// VerifyISO checks the ISO exists and prints its size.
func (b *ISOBuilder) VerifyISO(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		ui.Warn("ISO not found at %s (bootc-image-builder may have named it differently)", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot stat ISO: %w", err)
	}
	sizeMB := float64(info.Size()) / 1024 / 1024
	ui.OK("ISO verified: %s  (%.0f MB)", filepath.Base(path), sizeMB)
	return nil
}

func (b *ISOBuilder) run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	if b.verbose {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	ui.OK("%s done", name)
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
