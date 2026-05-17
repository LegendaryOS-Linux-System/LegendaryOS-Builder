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
	storageDir := b.paths.PodmanStorage
	dirs := []string{
		b.paths.BuildDir,
		b.paths.CacheDir,
		b.paths.OutputDir,
		filepath.Join(b.paths.BuildDir, "iso-work"),
		filepath.Join(storageDir, "graph"),
		filepath.Join(storageDir, "run"),
		filepath.Join(storageDir, "tmp"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("cannot create %s: %w", d, err)
		}
	}

	// Write storage.conf — same isolated storage as cloud build
	storageCfg := filepath.Join(b.paths.BuildDir, "storage.conf")
	content := fmt.Sprintf("[storage]\ndriver = \"overlay\"\ngraphRoot = \"%s/graph\"\nrunRoot = \"%s/run\"\n",
		storageDir, storageDir)
	if err := os.WriteFile(storageCfg, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write storage.conf: %w", err)
	}

	ui.OK("Build directories ready")
	ui.Info("Podman storage: %s", storageDir)
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

	// Write BIB config.toml (different from project config.toml!)
	// BIB uses its own config format for ISO customization
	bibCfgPath, err := b.writeBIBConfig(outDir, label, kickstart)
	if err != nil {
		return err
	}

	args := []string{
		"build",
		"--type", "iso",
		"--output", outDir,
	}
	if bibCfgPath != "" {
		args = append(args, "--config", bibCfgPath)
	}
	args = append(args, sourceImage)

	ui.Info("bootc-image-builder %s", strings.Join(args, " "))
	if err := b.run("bootc-image-builder", args...); err != nil {
		return err
	}

	return b.renameOutput(outDir, finalPath)
}

// buildViaPodman runs bootc-image-builder inside a privileged podman container.
//
// Key design decisions:
//  1. --iso-label does NOT exist in BIB — removed
//  2. Storage: mount project's podman-storage (not /var/lib/containers)
//     so everything stays on the external disk, not the system disk
//  3. Kickstart: embedded via BIB config.toml (--config flag)
func (b *ISOBuilder) buildViaPodman(sourceImage, outDir, finalPath, label, kickstart string) error {
	bibImage := "quay.io/centos-bootc/bootc-image-builder:latest"
	ui.Info("Method: bootc-image-builder via podman (privileged container)")
	ui.Info("BIB image: %s", bibImage)

	// Write BIB's own config.toml if we have a kickstart to embed
	bibCfgPath, err := b.writeBIBConfig(outDir, label, kickstart)
	if err != nil {
		return fmt.Errorf("cannot write BIB config: %w", err)
	}

	// Mount project storage so BIB can find the already-pulled source image.
	// PullImage() stored it in build/podman-storage/graph.
	storageDir := b.paths.PodmanStorage

	args := []string{
		"run", "--rm",
		"--privileged",
		"--pull=newer",
		// Project's isolated storage → BIB can find the source image here
		"--volume", storageDir + ":/var/lib/containers/storage",
		// Output goes here
		"--volume", outDir + ":/output",
		// osbuild needs device access
		"--volume", "/dev:/dev",
	}

	// Mount BIB config if we generated one
	if bibCfgPath != "" {
		args = append(args, "--volume", bibCfgPath+":/config.toml:ro")
	}

	// BIB container image
	args = append(args, bibImage)

	// BIB sub-command — no --iso-label, configured via config.toml
	args = append(args, "build", "--type", "iso", "--output", "/output")
	if bibCfgPath != "" {
		args = append(args, "--config", "/config.toml")
	}

	args = append(args, sourceImage)

	ui.Info("podman %s", strings.Join(args, " "))
	if err := b.run("podman", args...); err != nil {
		return fmt.Errorf(
			"bootc-image-builder failed\n\n"+
				"  Common fixes:\n"+
				"    • Run as root / with sudo (BIB needs privileged access)\n"+
				"    • On Debian/Ubuntu: podman must be installed and functional\n"+
				"    • Check pulled image: podman images | grep %s\n",
			sourceImage)
	}

	return b.renameOutput(outDir, finalPath)
}

// writeBIBConfig generates bootc-image-builder's own config.toml.
// This is NOT the project config.toml — BIB uses a different schema.
// Handles: kickstart embedding.
// Label is NOT supported by BIB config — it's set by the ISO build process itself.
func (b *ISOBuilder) writeBIBConfig(outDir, label, kickstart string) (string, error) {
	hasKS := kickstart != "" && func() bool {
		_, err := os.Stat(kickstart)
		return err == nil
	}()

	if !hasKS {
		return "", nil // nothing to configure
	}

	var sb strings.Builder
	sb.WriteString("# bootc-image-builder config — generated by LegendaryOS Builder\n")
	sb.WriteString("# Schema: https://github.com/osbuild/bootc-image-builder\n\n")

	if hasKS {
		ksData, err := os.ReadFile(kickstart)
		if err != nil {
			return "", fmt.Errorf("cannot read kickstart %s: %w", kickstart, err)
		}
		// BIB kickstart embedding format
		ksStr := strings.ReplaceAll(string(ksData), `"""`, `\"\"\"`)
		sb.WriteString("[customizations]\n")
		sb.WriteString("[customizations.installer]\n")
		sb.WriteString("[customizations.installer.kickstart]\n")
		sb.WriteString(fmt.Sprintf("contents = \"\"\"\n%s\"\"\"\n", ksStr))
	}

	cfgPath := filepath.Join(outDir, "bib-config.toml")
	if err := os.WriteFile(cfgPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("cannot write bib config: %w", err)
	}
	ui.Info("BIB config → %s", cfgPath)
	return cfgPath, nil
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
	return b.runEnv(b.podmanEnv(), name, args...)
}

func (b *ISOBuilder) runEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
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

// podmanEnv returns environment with isolated storage.conf set
func (b *ISOBuilder) podmanEnv() []string {
	storageCfg := filepath.Join(b.paths.BuildDir, "storage.conf")
	storageDir := b.paths.PodmanStorage
	return append(os.Environ(),
		"CONTAINERS_STORAGE_CONF="+storageCfg,
		"TMPDIR="+filepath.Join(storageDir, "tmp"),
	)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
