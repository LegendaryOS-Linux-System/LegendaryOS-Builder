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

// CloudBuilder builds an OCI bootc container image using podman.
//
// Pipeline (immutable / special_type = "default"):
//  1. Validate project
//  2. Prepare build/ dirs
//  3. Copy repos/ into build context
//  4. Generate Containerfile
//  5. podman build --tag <registry/image:tag>
//  6. (optional) podman push
//  7. (optional) cosign sign
//
// For special_type = "classic" the Containerfile omits the bootc ostree commit
// and uses a plain Fedora base image instead of fedora-bootc.
//
// Note on Flatpak:
//   flatpak.packages and flatpak.remove.packages are NOT executed during the
//   container/OCI build. They are consumed by the Anaconda kickstart generator
//   so that Flatpaks are installed/removed on the target system at install time.
type CloudBuilder struct {
	cfg     *config.Config
	paths   *config.Paths
	verbose bool
	release bool
}

func NewCloud(cfg *config.Config, paths *config.Paths, verbose, release bool) *CloudBuilder {
	return &CloudBuilder{cfg: cfg, paths: paths, verbose: verbose, release: release}
}

func (b *CloudBuilder) Validate() error {
	if b.cfg.Project.BaseDistro != "fedora" {
		return fmt.Errorf("unsupported distro %q — only 'fedora' is supported", b.cfg.Project.BaseDistro)
	}
	if b.cfg.Project.BaseVersion < 44 {
		return fmt.Errorf("Fedora %d is not supported — minimum is Fedora 44", b.cfg.Project.BaseVersion)
	}
	mode := "immutable (bootc)"
	if b.cfg.Project.IsClassic() {
		mode = "classic (mutable)"
	}
	ui.OK("Project valid — Fedora %d / %s / %s", b.cfg.Project.BaseVersion, b.cfg.Project.Arch, mode)
	return nil
}

func (b *CloudBuilder) PrepareDirs() error {
	storageDir := b.paths.PodmanStorage
	dirs := []string{
		b.paths.BuildDir,
		b.paths.CacheDir,
		b.paths.OutputDir,
		filepath.Join(b.paths.BuildDir, "context", "repos"),
		filepath.Join(storageDir, "graph"),
		filepath.Join(storageDir, "run"),
		filepath.Join(storageDir, "tmp"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("cannot create %s: %w", d, err)
		}
	}

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

// CopyRepos copies *.repo files from repos/ into the build context.
func (b *CloudBuilder) CopyRepos() error {
	if _, err := os.Stat(b.paths.ReposDir); os.IsNotExist(err) {
		ui.Info("No repos/ directory — using base image defaults")
		return nil
	}
	entries, err := os.ReadDir(b.paths.ReposDir)
	if err != nil {
		return fmt.Errorf("cannot read repos/: %w", err)
	}
	destDir := filepath.Join(b.paths.BuildDir, "context", "repos")
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".repo") {
			data, err := os.ReadFile(filepath.Join(b.paths.ReposDir, e.Name()))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(destDir, e.Name()), data, 0644); err != nil {
				return err
			}
			ui.Info("Repo: %s", e.Name())
			count++
		}
	}
	ui.OK("Copied %d repo file(s) into build context", count)
	return nil
}

// GenerateContainerfile renders the Containerfile and writes it to build/Containerfile.
// The Containerfile differs depending on special_type:
//   - "default" (immutable): uses fedora-bootc base, ends with `ostree container commit`
//   - "classic":             uses plain fedora base, no ostree commit
func (b *CloudBuilder) GenerateContainerfile(platform string) error {
	installPkgs, err := config.ReadPackageList(b.paths.InstallPkgs)
	if err != nil {
		return fmt.Errorf("cannot read install.packages: %w", err)
	}
	removePkgs, err := config.ReadPackageList(b.paths.RemovePkgs)
	if err != nil {
		return fmt.Errorf("cannot read remove.packages: %w", err)
	}

	// Note: flatpak.packages and flatpak.remove.packages are intentionally
	// NOT processed here — they belong to the Anaconda kickstart (install-time),
	// not to the container image build.

	var cf string
	if b.cfg.Project.IsClassic() {
		cf = b.renderClassicContainerfile(installPkgs, removePkgs, platform)
	} else {
		cf = b.renderImmutableContainerfile(installPkgs, removePkgs, platform)
	}

	cfPath := filepath.Join(b.paths.BuildDir, "Containerfile")
	if err := os.WriteFile(cfPath, []byte(cf), 0644); err != nil {
		return fmt.Errorf("cannot write Containerfile: %w", err)
	}
	ui.OK("Containerfile → %s", cfPath)

	if b.verbose {
		ui.Divider()
		fmt.Fprint(ui.Out, cf)
		ui.Divider()
	}
	if len(installPkgs) > 0 {
		ui.PackageListDisplay("Will install (DNF)", installPkgs)
	}
	if len(removePkgs) > 0 {
		ui.PackageListDisplay("Will remove (DNF)", removePkgs)
	}

	// Inform user that flatpak entries are handled at install time.
	flatpakInstall, _ := config.ReadPackageList(b.paths.FlatpakPkgs)
	flatpakRemove, _  := config.ReadPackageList(b.paths.FlatpakRemovePkgs)
	if len(flatpakInstall) > 0 {
		ui.Info("Flatpak install (%d apps) → will be applied by Anaconda at install time", len(flatpakInstall))
	}
	if len(flatpakRemove) > 0 {
		ui.Info("Flatpak remove  (%d apps) → will be applied by Anaconda at install time", len(flatpakRemove))
	}

	return nil
}

// ── Containerfile renderers ───────────────────────────────────────────────────

// renderImmutableContainerfile generates a Containerfile for a bootc/ostree image.
func (b *CloudBuilder) renderImmutableContainerfile(install, remove []string, platform string) string {
	cfg := b.cfg
	base := fmt.Sprintf("quay.io/fedora/fedora-bootc:%d", cfg.Project.BaseVersion)
	return b.renderContainerfileBody(install, remove, platform, base, true)
}

// renderClassicContainerfile generates a Containerfile for a plain Fedora image.
// No ostree commit at the end — this is a standard mutable container/OS image.
func (b *CloudBuilder) renderClassicContainerfile(install, remove []string, platform string) string {
	cfg := b.cfg
	base := fmt.Sprintf("registry.fedoraproject.org/fedora:%d", cfg.Project.BaseVersion)
	return b.renderContainerfileBody(install, remove, platform, base, false)
}

func (b *CloudBuilder) renderContainerfileBody(install, remove []string, platform, base string, immutable bool) string {
	cfg := b.cfg

	var sb strings.Builder
	line := func(s string, a ...interface{}) {
		if len(a) > 0 {
			fmt.Fprintf(&sb, s+"\n", a...)
		} else {
			sb.WriteString(s + "\n")
		}
	}
	comment := func(s string) { line("# " + s) }
	blank   := func()         { line("") }

	comment("╔══════════════════════════════════════════════════════════════╗")
	comment("║  Generated by LegendaryOS Builder — DO NOT EDIT MANUALLY   ║")
	comment("║  Regenerate with: legendaryos build cloud                   ║")
	if immutable {
		comment("║  Mode: immutable (bootc / ostree)                           ║")
	} else {
		comment("║  Mode: classic (plain Fedora, mutable)                      ║")
	}
	comment("╚══════════════════════════════════════════════════════════════╝")
	blank()

	line("FROM --platform=%s %s", platform, base)
	blank()

	comment("── Labels ───────────────────────────────────────────────────────")
	line("LABEL org.opencontainers.image.title=%q", cfg.Project.Name)
	line("LABEL org.opencontainers.image.version=%q", cfg.Project.Version)
	line("LABEL org.opencontainers.image.description=%q", cfg.Project.Description)
	line("LABEL org.opencontainers.image.licenses=%q", cfg.Project.License)
	line("LABEL com.legendaryos.builder.version=\"1.0.0\"")
	if immutable {
		line("LABEL com.legendaryos.special_type=\"default\"")
	} else {
		line("LABEL com.legendaryos.special_type=\"classic\"")
	}
	blank()

	// Custom repos
	if hasDir(b.paths.ReposDir) {
		comment("── Custom repositories ────────────────────────────────────────────")
		line("COPY build/context/repos/ /etc/yum.repos.d/")
		blank()
	}

	// Local RPMs
	if hasDir(b.paths.PackagesDir) {
		comment("── Local RPM packages (packages/) ─────────────────────────────────")
		line("COPY packages/ /tmp/los-rpms/")
		line("RUN dnf install -y /tmp/los-rpms/*.rpm 2>/dev/null || true \\")
		line("    && rm -rf /tmp/los-rpms")
		blank()
	}

	// scripts/before/
	if hasDir(b.paths.ScriptsBefore) {
		comment("── scripts/before/ — run BEFORE package install ───────────────────")
		line("COPY scripts/before/ /tmp/los-scripts-before/")
		line("RUN set -eux \\")
		line("    && for f in $(ls /tmp/los-scripts-before/*.sh  2>/dev/null | sort); do bash    \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-before/*.py  2>/dev/null | sort); do python3 \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-before/*.pl  2>/dev/null | sort); do perl    \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-before/*.rb  2>/dev/null | sort); do ruby    \"$f\"; done \\")
		line("    && rm -rf /tmp/los-scripts-before")
		blank()
	}

	// files/before/ overlay
	if hasDir(b.paths.FilesBefore) {
		comment("── Pre-install overlay (files/before/) ────────────────────────────")
		line("COPY files/before/ /")
		blank()
	}

	// Install packages
	if len(install) > 0 {
		comment("── Install packages (packages/install.packages) ───────────────────")
		line("RUN dnf install -y --skip-unavailable \\")
		for _, pkg := range install {
			line("    %s \\", pkg)
		}
		line("    && dnf clean all \\")
		line("    && rm -rf /var/cache/dnf")
		blank()
	}

	// Bootloader packages (when [bootloader] section is enabled)
	if cfg.Bootloader.Enabled && len(cfg.Bootloader.InstallPackages) > 0 {
		comment("── Bootloader packages ([bootloader] section) ─────────────────────")
		line("RUN dnf install -y --skip-unavailable \\")
		for _, pkg := range cfg.Bootloader.InstallPackages {
			line("    %s \\", pkg)
		}
		line("    && dnf clean all \\")
		line("    && rm -rf /var/cache/dnf")
		blank()
	}

	// Remove packages
	if len(remove) > 0 {
		comment("── Remove packages (packages/remove.packages) ─────────────────────")
		line("RUN dnf remove -y \\")
		for i, pkg := range remove {
			if i < len(remove)-1 {
				line("    %s \\", pkg)
			} else {
				line("    %s", pkg)
			}
		}
		blank()
	}

	// files/after/ overlay
	if hasDir(b.paths.FilesAfter) {
		comment("── Post-install overlay (files/after/) ────────────────────────────")
		line("COPY files/after/ /")
		blank()
	}

	// scripts/after/
	if hasDir(b.paths.ScriptsAfter) {
		comment("── scripts/after/ — run AFTER package install ─────────────────────")
		line("COPY scripts/after/ /tmp/los-scripts-after/")
		line("RUN set -eux \\")
		line("    && for f in $(ls /tmp/los-scripts-after/*.sh  2>/dev/null | sort); do bash    \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-after/*.py  2>/dev/null | sort); do python3 \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-after/*.pl  2>/dev/null | sort); do perl    \"$f\"; done \\")
		line("    && for f in $(ls /tmp/los-scripts-after/*.rb  2>/dev/null | sort); do ruby    \"$f\"; done \\")
		line("    && rm -rf /tmp/los-scripts-after")
		blank()
	}

	// NOTE: Flatpak packages are intentionally omitted here.
	// They are handled by the Anaconda kickstart at system install time.
	comment("── NOTE: Flatpak packages are applied at install time via Anaconda ──")
	comment("        See packages/flatpak.packages and packages/flatpak.remove.packages")
	blank()

	// System configuration
	comment("── System configuration ────────────────────────────────────────────")
	if cfg.System.Hostname != "" {
		line("RUN echo %q > /etc/hostname", cfg.System.Hostname)
	}
	if cfg.System.Locale != "" {
		line("RUN echo 'LANG=%s' > /etc/locale.conf", cfg.System.Locale)
	}
	if cfg.System.Keyboard != "" {
		line("RUN echo 'KEYMAP=%s' > /etc/vconsole.conf", cfg.System.Keyboard)
	}
	if cfg.System.Timezone != "" {
		line("RUN ln -sf /usr/share/zoneinfo/%s /etc/localtime || true", cfg.System.Timezone)
	}
	if cfg.System.SELinux != "" {
		line("RUN sed -i 's/^SELINUX=.*/SELINUX=%s/' /etc/selinux/config 2>/dev/null || true", cfg.System.SELinux)
	}
	for _, svc := range cfg.System.Services {
		line("RUN systemctl enable  %s 2>/dev/null || true", svc)
	}
	for _, svc := range cfg.System.Disable {
		line("RUN systemctl disable %s 2>/dev/null || true", svc)
	}
	if cfg.System.Firewall {
		line("RUN systemctl enable firewalld.service 2>/dev/null || true")
	}

	// Bootloader configuration inside the image
	if cfg.Bootloader.Enabled {
		b.renderBootloaderSteps(&sb, line, comment, blank)
	}

	// os-release
	comment("── os-release ────────────────────────────────────────────────────────")
	comment("  Ensure ID= and VERSION_ID= are present (required by bootc-image-builder)")
	line("RUN grep -q '^ID=' /etc/os-release || echo 'ID=fedora' >> /etc/os-release")
	line("RUN grep -q '^VERSION_ID=' /etc/os-release || echo 'VERSION_ID=%d' >> /etc/os-release", cfg.Project.BaseVersion)
	if cfg.Project.Name != "" {
		variantID := strings.ToLower(strings.ReplaceAll(cfg.Project.Name, " ", "-"))
		line("RUN grep -q '^VARIANT=' /etc/os-release    || echo 'VARIANT=%s'    >> /etc/os-release", cfg.Project.Name)
		line("RUN grep -q '^VARIANT_ID=' /etc/os-release || echo 'VARIANT_ID=%s' >> /etc/os-release", variantID)
	}
	// Classic mode marker
	if !immutable {
		line("RUN grep -q '^VARIANT_PLATFORM=' /etc/os-release || echo 'VARIANT_PLATFORM=classic' >> /etc/os-release")
	}
	blank()

	// Release mode
	if b.release {
		comment("── Release: strip debug symbols ────────────────────────────────────")
		line("RUN dnf remove -y gdb-minimal 2>/dev/null || true \\")
		line("    && find /usr/lib/debug -type f -delete 2>/dev/null || true \\")
		line("    && find /usr/src/debug -type f -delete 2>/dev/null || true")
		blank()
	}

	// bootc commit only for immutable builds
	if immutable {
		comment("── bootc: commit ostree layer — MUST be the last RUN ───────────────")
		line("RUN ostree container commit")
	} else {
		comment("── classic build: no ostree commit ─────────────────────────────────")
		comment("  This image is a standard mutable Fedora container.")
		comment("  Use it as a base for ISO builds or as a container image directly.")
	}

	return sb.String()
}

// renderBootloaderSteps appends bootloader-specific RUN instructions.
func (b *CloudBuilder) renderBootloaderSteps(
	_ *strings.Builder,
	line func(string, ...interface{}),
					     comment func(string),
					     blank func(),
) {
	cfg := b.cfg
	bl := cfg.Bootloader.Type
	comment(fmt.Sprintf("── Bootloader: %s ──────────────────────────────────────────────────", bl))

	switch strings.ToLower(bl) {
		case config.BootloaderRefind:
			comment("  rEFInd — EFI boot manager")
			line("RUN dnf install -y refind 2>/dev/null || true")
			efi := cfg.Bootloader.EFIDir
			if efi == "" {
				efi = "/boot/efi"
			}
			line("RUN refind-install --usedefault %s 2>/dev/null || true", efi)

		case config.BootloaderLimine:
			comment("  Limine — modern bootloader")
			line("RUN dnf install -y limine 2>/dev/null || true")

		case config.BootloaderSystemd:
			comment("  systemd-boot (sd-boot)")
			line("RUN dnf install -y systemd-boot-unsigned efi-filesystem 2>/dev/null || true")
			line("RUN bootctl install 2>/dev/null || true")

		case config.BootloaderSyslinux:
			comment("  SYSLINUX / ISOLINUX")
			line("RUN dnf install -y syslinux syslinux-extlinux 2>/dev/null || true")

		case config.BootloaderGRUB2, "grub", "":
			comment("  GRUB 2 (default)")
			line("RUN dnf install -y grub2 grub2-efi-x64 shim 2>/dev/null || true")
			line("RUN grub2-mkconfig -o /boot/grub2/grub.cfg 2>/dev/null || true")

		default:
			comment(fmt.Sprintf("  Custom bootloader: %s — no automatic setup steps", bl))
	}

	if cfg.Bootloader.ExtraArgs != "" {
		comment("  Extra kernel args for this bootloader")
		line("RUN sed -i 's/^GRUB_CMDLINE_LINUX=\"/GRUB_CMDLINE_LINUX=\"%s /' /etc/default/grub 2>/dev/null || true", cfg.Bootloader.ExtraArgs)
	}
	blank()
}

// RegistryLogin logs into the OCI registry using podman login.
func (b *CloudBuilder) RegistryLogin(registry, username, token string) error {
	if token == "" {
		ui.Info("No token — skipping registry login")
		return nil
	}
	if username == "" {
		username = "token"
	}
	ui.Info("Logging into %s as %s", registry, username)
	storageCfg := filepath.Join(b.paths.BuildDir, "storage.conf")
	env := append(os.Environ(), "CONTAINERS_STORAGE_CONF="+storageCfg)
	return b.runEnv(env, "podman", "login",
			"--username", username,
		 "--password", token,
		 registry,
	)
}

func (b *CloudBuilder) PodmanBuild(tag string, noCache bool) error {
	cfPath := filepath.Join(b.paths.BuildDir, "Containerfile")
	storageCfg := filepath.Join(b.paths.BuildDir, "storage.conf")

	args := []string{"build", "--tag", tag, "--file", cfPath}
	if noCache {
		args = append(args, "--no-cache")
	}
	if b.release {
		args = append(args, "--squash-all")
	}
	args = append(args, b.paths.Root)

	ui.Info("podman %s", strings.Join(args, " "))

	storageDir := b.paths.PodmanStorage
	env := os.Environ()
	env = append(env,
		     "CONTAINERS_STORAGE_CONF="+storageCfg,
	      "TMPDIR="+filepath.Join(storageDir, "tmp"),
	)

	return b.runEnv(env, "podman", args...)
}

func (b *CloudBuilder) PodmanPush(tag string) error {
	ui.Info("Pushing: %s", tag)
	storageCfg := filepath.Join(b.paths.BuildDir, "storage.conf")
	env := append(os.Environ(), "CONTAINERS_STORAGE_CONF="+storageCfg)
	return b.runEnv(env, "podman", "push", tag)
}

func (b *CloudBuilder) CosignSign(tag string) error {
	ui.Info("Signing with cosign: %s", tag)
	return b.runEnv(os.Environ(), "cosign", "sign", "--yes", tag)
}

func (b *CloudBuilder) run(name string, args ...string) error {
	return b.runEnv(os.Environ(), name, args...)
}

func (b *CloudBuilder) runEnv(env []string, name string, args ...string) error {
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

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// RunPreScripts executes scripts/pre/ on the HOST before any build step.
func (b *CloudBuilder) RunPreScripts() error {
	if !hasDir(b.paths.ScriptsPre) {
		ui.Info("No scripts/pre/ directory — skipping")
		return nil
	}
	entries, err := os.ReadDir(b.paths.ScriptsPre)
	if err != nil {
		return fmt.Errorf("cannot read scripts/pre/: %w", err)
	}
	interps := map[string]string{
		".sh": "bash", ".bash": "bash",
		".py": "python3",
		".pl": "perl",
		".rb": "ruby",
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		interp, ok := interps[ext]
		if !ok {
			continue
		}
		scriptPath := filepath.Join(b.paths.ScriptsPre, e.Name())
		ui.Info("[pre] %s  (%s)", e.Name(), interp)
		cmd := exec.Command(interp, scriptPath)
		cmd.Env = append(os.Environ(),
				 "LEGENDARYOS_PROJECT="+b.cfg.Project.Name,
		   "LEGENDARYOS_VERSION="+b.cfg.Project.Version,
		   "LEGENDARYOS_BUILD="+b.paths.BuildDir,
		   "LEGENDARYOS_ROOT="+b.paths.Root,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pre-script %s failed: %w", e.Name(), err)
		}
		ran++
	}
	if ran > 0 {
		ui.OK("Pre-scripts done (%d ran)", ran)
	}
	return nil
}
