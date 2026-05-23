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
	// ISO builder runs as root and uses /var/lib/containers/storage natively.
	// No isolated storage.conf needed — that's only for cloud (rootless) builds.
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

	// Fix permissions on build/ so the regular user can still access it after
	// this root run (e.g. to check output, run clean, etc.)
	if err := os.Chmod(b.paths.BuildDir, 0755); err != nil {
		ui.Warn("cannot chmod build dir: %v", err)
	}
	if err := os.Chmod(b.paths.OutputDir, 0755); err != nil {
		ui.Warn("cannot chmod output dir: %v", err)
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

	// BIB requires rootful podman
	if os.Getuid() != 0 {
		ui.Newline()
		ui.Warn("bootc-image-builder requires root — run with sudo:")
		fmt.Fprintf(ui.Out, "\n  \033[96m  sudo legendaryos-builder build iso\033[0m\n\n")
		return fmt.Errorf("must be root — run: sudo legendaryos-builder build iso")
	}
	ui.OK("Running as root")

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
		"--rootfs", b.rootfs(),
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

	// As root (rootful podman), BIB needs access to /var/lib/containers/storage
	// where the source image was pulled to by PullImage().
	// This mount is REQUIRED — BIB explicitly checks for it.
	args := []string{
		"run", "--rm",
		"--privileged",
		"--pull=newer",
		// REQUIRED: BIB needs to find the source image here
		"--volume", "/var/lib/containers/storage:/var/lib/containers/storage",
		// Output directory on external disk
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

	// BIB sub-command
	// --rootfs xfs: required when BIB cannot auto-detect filesystem type from
	// the bootc image metadata. Fedora uses XFS by default.
	args = append(args, "build",
		"--type", "iso",
		"--output", "/output",
		"--rootfs", b.rootfs(),
	)
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
// 
// IMPORTANT: We do NOT embed the kickstart inside bib-config.toml because
// TOML has strict escape rules and kickstart contains backslashes (udev rules,
// regex patterns etc.) that break TOML parsing.
//
// Instead we write a minimal BIB config and pass the kickstart as a separate
// file mounted into the BIB container at /kickstart.ks.
// BIB picks it up via [customizations.installer.kickstart] contents = ""
// pointing to the mounted file path.
//
// Actually the cleanest approach: pass kickstart path directly to BIB via
// the --kickstart flag (supported in BIB >= 0.16) or just mount the .ks file
// and set the path in config. We use the file reference approach.
func (b *ISOBuilder) writeBIBConfig(outDir, label, kickstart string) (string, error) {
	if kickstart == "" {
		return "", nil
	}
	if _, err := os.Stat(kickstart); err != nil {
		return "", nil
	}

	ksData, err := os.ReadFile(kickstart)
	if err != nil {
		return "", fmt.Errorf("cannot read kickstart: %w", err)
	}

	// TOML literal multi-line strings use ''' delimiters and do NOT
	// process backslash escapes — safe for kickstart containing \|, \n, etc.
	// The only thing we need to handle is ''' appearing in the kickstart
	// (extremely unlikely but replace just in case).
	tripleQ := "'''"
	ksContent := strings.ReplaceAll(string(ksData), tripleQ, "''\\''")

	var sb strings.Builder
	sb.WriteString("# bootc-image-builder config — generated by LegendaryOS Builder\n")
	sb.WriteString("# https://github.com/osbuild/bootc-image-builder\n\n")
	sb.WriteString("[customizations.installer.kickstart]\n")
	sb.WriteString("contents = '''\n")
	sb.WriteString(ksContent)
	if len(ksContent) == 0 || ksContent[len(ksContent)-1] != '\n' {
		sb.WriteString("\n")
	}
	sb.WriteString("'''\n")

	cfgPath := filepath.Join(outDir, "bib-config.toml")
	if err := os.WriteFile(cfgPath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("cannot write bib config: %w", err)
	}
	ui.Info("BIB config -> %s", cfgPath)
	return cfgPath, nil
}


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
	// As root: use native podman storage, no custom env needed
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

// rootfs returns the filesystem type for BIB --rootfs flag.
// Falls back to ext4 if not configured (most compatible).
func (b *ISOBuilder) rootfs() string {
	fs := b.cfg.Build.Filesystem
	if fs == "" {
		return "ext4"
	}
	// Validate — BIB supports: ext4, xfs, btrfs
	switch fs {
	case "ext4", "xfs", "btrfs":
		return fs
	default:
		return "ext4"
	}
}


func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
