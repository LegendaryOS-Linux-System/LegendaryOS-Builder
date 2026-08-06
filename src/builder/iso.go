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
	mode := "immutable (bootc)"
	if b.cfg.Project.IsClassic() {
		mode = "classic (mutable)"
	}
	ui.OK("Project valid — Fedora %d / %s / %s", b.cfg.Project.BaseVersion, b.cfg.Project.Arch, mode)
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
	missing  := []string{}

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

// GenerateKickstart writes the Anaconda kickstart file.
// Flatpak entries from packages/flatpak.packages and
// packages/flatpak.remove.packages are embedded in the %post section.
func (b *ISOBuilder) GenerateKickstart(ksPath string) error {
	if ksPath == "" {
		ksPath = filepath.Join(b.paths.BuildDir, "anaconda.ks")
	}
	// Pass paths so the kickstart generator can read flatpak package lists.
	ks, err := generateKickstart(b.cfg, b.paths)
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

	ui.Info("Pre-pulling source image: %s", sourceImage)
	if err := b.run("podman", "pull", "--platform=linux/amd64", sourceImage); err != nil {
		return fmt.Errorf("cannot pull source image %s: %w", sourceImage, err)
	}

	ociDir := filepath.Join(outDir, "oci-image")
	_ = os.RemoveAll(ociDir)
	ui.Info("Exporting image to OCI dir: %s", ociDir)
	if err := b.run("podman", "save", "--format=oci-dir", "-o", ociDir, sourceImage); err != nil {
		return fmt.Errorf("cannot export image to OCI dir: %w", err)
	}
	ociRef := "oci:" + ociDir

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
	args = append(args, ociRef)

	ui.Info("bootc-image-builder %s", strings.Join(args, " "))
	if err := b.run("bootc-image-builder", args...); err != nil {
		return err
	}

	_ = os.RemoveAll(ociDir)
	return b.renameOutput(outDir, finalPath)
}

func (b *ISOBuilder) buildViaPodman(sourceImage, outDir, finalPath, label, kickstart string) error {
	bibImage := "quay.io/centos-bootc/bootc-image-builder:latest"
	ui.Info("Method: bootc-image-builder via podman (privileged container)")
	ui.Info("BIB image: %s", bibImage)

	// Root cause: GHCR recompresses layers server-side, changing the layer
	// sha256. The ostree.final-diffid label stays pointing to the old sha256,
	// so Anaconda fails with "Missing ostree.final-diffid".
	//
	// Fix: export to OCI dir, patch the config blob JSON directly to update
	// ostree.final-diffid to match the actual layer DiffID, then re-import.
	// This preserves the full ostree/bootc structure unlike buildah commit.

	ui.Info("Pulling source image: %s", sourceImage)
	if err := b.run("podman", "pull", "--platform=linux/amd64", sourceImage); err != nil {
		return fmt.Errorf("cannot pull source image %s: %w", sourceImage, err)
	}

	// Export to OCI dir
	ociDir := filepath.Join(outDir, "oci-image")
	_ = os.RemoveAll(ociDir)
	ui.Info("Exporting to OCI dir: %s", ociDir)
	if err := b.run("podman", "save", "--format=oci-dir", "-o", ociDir, sourceImage); err != nil {
		return fmt.Errorf("cannot export image to OCI dir: %w", err)
	}

	// Patch ostree.final-diffid in the OCI config blob to match actual layer
	ui.Info("Patching ostree.final-diffid in OCI config blob")
	patchScript := `
import json, os, sys, hashlib, gzip

oci_dir = sys.argv[1]

# Read index to find manifest
with open(os.path.join(oci_dir, 'index.json')) as f:
    index = json.load(f)

manifest_digest = index['manifests'][0]['digest'].replace('sha256:', '')
manifest_path = os.path.join(oci_dir, 'blobs', 'sha256', manifest_digest)

with open(manifest_path) as f:
    manifest = json.load(f)

# Get config blob
config_digest = manifest['config']['digest'].replace('sha256:', '')
config_path = os.path.join(oci_dir, 'blobs', 'sha256', config_digest)

with open(config_path) as f:
    config = json.load(f)

# Get the actual uncompressed layer DiffID from config
actual_diffid = config['rootfs']['diff_ids'][0]
print(f'Actual DiffID from config: {actual_diffid}')

# Get current ostree.final-diffid from labels
labels = config.get('config', {}).get('Labels', {})
current_diffid = labels.get('ostree.final-diffid', '')
print(f'Current ostree.final-diffid: {current_diffid}')

if actual_diffid == current_diffid:
    print('DiffIDs match — no patch needed')
    sys.exit(0)

# Patch the label
print(f'Patching: {current_diffid} -> {actual_diffid}')
config['config']['Labels']['ostree.final-diffid'] = actual_diffid

# Write patched config blob
new_config = json.dumps(config, separators=(',', ':')).encode()
new_config_digest = hashlib.sha256(new_config).hexdigest()
new_config_path = os.path.join(oci_dir, 'blobs', 'sha256', new_config_digest)

with open(new_config_path, 'wb') as f:
    f.write(new_config)

# Update manifest to point to new config blob
manifest['config']['digest'] = 'sha256:' + new_config_digest
manifest['config']['size'] = len(new_config)

new_manifest = json.dumps(manifest, separators=(',', ':')).encode()
new_manifest_digest = hashlib.sha256(new_manifest).hexdigest()
new_manifest_path = os.path.join(oci_dir, 'blobs', 'sha256', new_manifest_digest)

with open(new_manifest_path, 'wb') as f:
    f.write(new_manifest)

# Update index to point to new manifest
index['manifests'][0]['digest'] = 'sha256:' + new_manifest_digest
index['manifests'][0]['size'] = len(new_manifest)

with open(os.path.join(oci_dir, 'index.json'), 'w') as f:
    json.dump(index, f, separators=(',', ':'))

print('Patch applied successfully')
`
	patchOut, err := b.runOutput("python3", "-c", patchScript, ociDir)
	if err != nil {
		ui.Info("Warning: OCI patch failed (%v), proceeding anyway", err)
	} else {
		ui.Info("Patch result: %s", strings.TrimSpace(patchOut))
	}

	// Re-import patched OCI dir into containers-storage under original tag.
	// Use skopeo copy oci:dir -> containers-storage:tag which is unambiguous.
	// If skopeo is unavailable fall back to podman pull oci:dir + tag by digest.
	ui.Info("Re-importing patched image as: %s", sourceImage)
	_ = b.run("podman", "rmi", "-f", sourceImage)

	skopeoErr := b.run("skopeo", "copy",
		"oci:"+ociDir,
		"containers-storage:"+sourceImage,
	)
	if skopeoErr != nil {
		ui.Info("skopeo not available (%v), using podman pull + tag", skopeoErr)

		// podman pull from oci-dir, capture the image ID from output
		loadOut, err := b.runOutput("podman", "pull", "oci:"+ociDir)
		if err != nil {
			return fmt.Errorf("cannot re-import patched OCI dir: %w", err)
		}
		// loadOut last non-empty line is the image ID / digest
		imageID := ""
		for _, line := range strings.Split(strings.TrimSpace(loadOut), "\n") {
			if t := strings.TrimSpace(line); t != "" {
				imageID = t
			}
		}
		if imageID == "" {
			return fmt.Errorf("cannot determine loaded image ID after podman pull oci:")
		}
		ui.Info("Loaded image ID: %s", imageID)
		if err := b.run("podman", "tag", imageID, sourceImage); err != nil {
			return fmt.Errorf("cannot tag patched image as %s: %w", sourceImage, err)
		}
	}

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

	_ = os.RemoveAll(ociDir)
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
	sb.WriteString("# bootc-image-builder config — generated by LegendaryOS Builder\n\n")
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
func (b *ISOBuilder) renameOutput(outDir, finalPath string) error {
	candidates := []string{
		filepath.Join(outDir, "bootiso", "install.iso"),
		filepath.Join(outDir, "bootiso", "disk.iso"),
		filepath.Join(outDir, "disk.iso"),
		filepath.Join(outDir, "install.iso"),
		filepath.Join(outDir, "image.iso"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			ui.Info("Found ISO at: %s", c)
			if err := os.Rename(c, finalPath); err != nil {
				if err2 := copyFile(c, finalPath); err2 != nil {
					return fmt.Errorf("cannot move ISO: %w", err2)
				}
				os.Remove(c)
			}
			ui.OK("ISO → %s", finalPath)
			return nil
		}
	}
	entries, _ := os.ReadDir(outDir)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
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

func (b *ISOBuilder) runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}

func (b *ISOBuilder) rootfs() string {
	fs := b.cfg.Build.Filesystem
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
