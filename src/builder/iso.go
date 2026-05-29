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
	if err := os.Chmod(b.paths.BuildDir, 0755); err != nil {
		ui.Warn("cannot chmod build dir: %v", err)
	}
	if err := os.Chmod(b.paths.OutputDir, 0755); err != nil {
		ui.Warn("cannot chmod output dir: %v", err)
	}
	ui.OK("Build directories ready")
	return nil
}

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
	if !strings.Contains(image, "/") || strings.HasPrefix(image, "localhost/") {
		ui.Info("Using local image: %s", image)
		return nil
	}
	ui.Info("Pulling: %s", image)
	return b.run("podman", "pull", image)
}

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

func (b *ISOBuilder) BuildISO(sourceImage, output, label, kickstart string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return fmt.Errorf("cannot create output dir: %w", err)
	}

	outDir := filepath.Dir(output)

	if _, err := exec.LookPath("bootc-image-builder"); err == nil {
		return b.buildViaBinary(sourceImage, outDir, output, label, kickstart)
	}

	return b.buildViaPodman(sourceImage, outDir, output, label, kickstart)
}

func (b *ISOBuilder) buildViaBinary(sourceImage, outDir, finalPath, label, kickstart string) error {
	ui.Info("Method: bootc-image-builder binary")

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

func (b *ISOBuilder) buildViaPodman(sourceImage, outDir, finalPath, label, kickstart string) error {
	bibImage := "quay.io/centos-bootc/bootc-image-builder:latest"
	ui.Info("Method: bootc-image-builder via podman (privileged container)")
	ui.Info("BIB image: %s", bibImage)

	bibCfgPath, err := b.writeBIBConfig(outDir, label, kickstart)
	if err != nil {
		return fmt.Errorf("cannot write BIB config: %w", err)
	}

	args := []string{
		"run", "--rm",
		"--privileged",
		"--pull=newer",
		"--volume", "/var/lib/containers/storage:/var/lib/containers/storage",
		"--volume", outDir + ":/output",
		"--volume", "/dev:/dev",
	}

	if bibCfgPath != "" {
		args = append(args, "--volume", bibCfgPath+":/config.toml:ro")
	}

	args = append(args, bibImage)

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

// renameOutput finds the ISO produced by BIB and moves it to finalPath.
//
// BIB always writes to <outDir>/bootiso/install.iso (as of BIB >= 0.16).
// Older versions used disk.iso directly in outDir.
// We check all known locations so the builder works across BIB versions.
func (b *ISOBuilder) renameOutput(outDir, finalPath string) error {
	candidates := []string{
		// Current BIB default (>= 0.16): bootiso/install.iso
		filepath.Join(outDir, "bootiso", "install.iso"),
		// Older BIB: bootiso/disk.iso
		filepath.Join(outDir, "bootiso", "disk.iso"),
		// Even older: directly in outDir
		filepath.Join(outDir, "disk.iso"),
		filepath.Join(outDir, "install.iso"),
		filepath.Join(outDir, "image.iso"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			ui.Info("Found ISO at: %s", c)
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
		// also list one level deeper
		if e.IsDir() {
			sub, _ := os.ReadDir(filepath.Join(outDir, e.Name()))
			for _, s := range sub {
				names = append(names, e.Name()+"/"+s.Name())
			}
		}
	}
	ui.Warn("Could not find ISO in output dir. Contents: %s", strings.Join(names, ", "))
	return nil
}

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

func (b *ISOBuilder) rootfs() string {
	fs := b.cfg.Build.Filesystem
	if fs == "" {
		return "ext4"
	}
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
