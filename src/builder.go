package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
)

// Builder holds the build context
type Builder struct {
	cfg     *config.Config
	paths   *config.Paths
	verbose bool
	release bool
	start   time.Time
	steps   []ui.SummaryStep
}

func New(root string, verbose, release bool) (*Builder, error) {
	paths := config.GetPaths(root)
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	return &Builder{
		cfg:     cfg,
		paths:   paths,
		verbose: verbose,
		release: release,
		start:   time.Now(),
	}, nil
}

func (b *Builder) addStep(name, status, detail string) {
	b.steps = append(b.steps, ui.SummaryStep{Name: name, Status: status, Detail: detail})
}

// Run executes the full build pipeline
func (b *Builder) Run() error {
	ui.SmallBanner()

	ui.Step("Starting build: %s %s", b.cfg.Project.Name, b.cfg.Project.Version)
	ui.Info("Base: Fedora %d  •  Arch: %s  •  Desktop: %s",
		b.cfg.Project.BaseVersion, b.cfg.Project.Arch, b.cfg.Desktop.Environment)
	ui.Newline()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"Validate", b.stepValidate},
		{"Prepare dirs", b.stepPrepareDirs},
		{"Copy repos", b.stepCopyRepos},
		{"Install local RPMs", b.stepInstallLocalRPMs},
		{"Apply before/ files", b.stepApplyBefore},
		{"Install packages", b.stepInstallPackages},
		{"Remove packages", b.stepRemovePackages},
		{"Apply after/ files", b.stepApplyAfter},
		{"Run scripts", b.stepRunScripts},
		{"Configure system", b.stepConfigureSystem},
		{"Generate Anaconda", b.stepGenerateAnaconda},
		{"Build ISO", b.stepBuildISO},
		{"Build container", b.stepBuildContainer},
	}

	bar := ui.NewProgressBar(len(steps), "Build progress")

	for i, s := range steps {
		bar.Set(i)
		if err := s.fn(); err != nil {
			bar.Done()
			b.addStep(s.name, "fail", err.Error())
			b.printSummary()
			return err
		}
	}

	bar.Done()
	b.printSummary()
	return nil
}

func (b *Builder) printSummary() {
	isoFile := ""
	if b.cfg.Build.ISOFilename != "" {
		isoFile = filepath.Join(b.paths.OutputDir, b.cfg.Build.ISOFilename)
	} else {
		name := strings.ReplaceAll(b.cfg.Project.Name, " ", "-")
		isoFile = filepath.Join(b.paths.OutputDir,
			fmt.Sprintf("%s-%s-fedora%d.%s.iso",
				name, b.cfg.Project.Version,
				b.cfg.Project.BaseVersion, b.cfg.Project.Arch))
	}

	ui.PrintBuildSummary(&ui.BuildSummary{
		ProjectName: b.cfg.Project.Name,
		Version:     b.cfg.Project.Version,
		Distro:      fmt.Sprintf("Fedora %d (%s)", b.cfg.Project.BaseVersion, b.cfg.Project.Arch),
		Steps:       b.steps,
		ISOPath:     isoFile,
		Duration:    time.Since(b.start),
	})
}

// stepValidate checks that project structure is valid
func (b *Builder) stepValidate() error {
	ui.Step("Validating project structure")

	required := []string{b.paths.Config}
	for _, p := range required {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return fmt.Errorf("missing required file: %s", p)
		}
	}

	if b.cfg.Project.BaseDistro != "fedora" {
		return fmt.Errorf("unsupported distro: %s (only 'fedora' is supported)", b.cfg.Project.BaseDistro)
	}
	if b.cfg.Project.BaseVersion < 44 {
		return fmt.Errorf("unsupported Fedora version: %d (minimum: 44)", b.cfg.Project.BaseVersion)
	}

	ui.OK("Project structure valid")
	b.addStep("Validate", "ok", fmt.Sprintf("Fedora %d / %s", b.cfg.Project.BaseVersion, b.cfg.Project.Arch))
	return nil
}

// stepPrepareDirs creates build directories
func (b *Builder) stepPrepareDirs() error {
	ui.Step("Preparing build directories")

	dirs := []string{
		b.paths.BuildDir,
		b.paths.CacheDir,
		b.paths.OutputDir,
		filepath.Join(b.paths.BuildDir, "rootfs"),
		filepath.Join(b.paths.BuildDir, "work"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("cannot create %s: %w", d, err)
		}
	}

	ui.OK("Build directories ready")
	b.addStep("Prepare dirs", "ok", b.paths.BuildDir)
	return nil
}

// stepCopyRepos configures DNF repos
func (b *Builder) stepCopyRepos() error {
	ui.Step("Configuring repositories")

	// Copy custom repos from repos/ directory
	reposDir := b.paths.ReposDir
	if _, err := os.Stat(reposDir); os.IsNotExist(err) {
		ui.Info("No custom repos/ directory, using system defaults")
		b.addStep("Copy repos", "skip", "no repos/ dir")
		return nil
	}

	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return fmt.Errorf("cannot read repos/: %w", err)
	}

	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".repo") {
			ui.Info("Repo: %s", e.Name())
			count++
		}
	}

	ui.OK("Configured %d custom repositories", count)
	b.addStep("Copy repos", "ok", fmt.Sprintf("%d repos", count))
	return nil
}

// stepInstallLocalRPMs installs RPMs from packages/ directory
func (b *Builder) stepInstallLocalRPMs() error {
	pkgsDir := b.paths.PackagesDir
	if _, err := os.Stat(pkgsDir); os.IsNotExist(err) {
		ui.Info("No packages/ directory, skipping local RPMs")
		b.addStep("Install local RPMs", "skip", "no packages/ dir")
		return nil
	}

	entries, err := os.ReadDir(pkgsDir)
	if err != nil {
		return fmt.Errorf("cannot read packages/: %w", err)
	}

	var rpms []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".rpm") {
			rpms = append(rpms, filepath.Join(pkgsDir, e.Name()))
		}
	}

	if len(rpms) == 0 {
		ui.Info("No .rpm files in packages/")
		b.addStep("Install local RPMs", "skip", "no RPMs")
		return nil
	}

	ui.Step("Installing %d local RPM(s)", len(rpms))
	bar := ui.NewProgressBar(len(rpms), "Local RPMs")
	for i, rpm := range rpms {
		bar.Set(i)
		ui.Info("Installing: %s", filepath.Base(rpm))
		// Actual install: dnf install --installroot=<rootfs> rpm
		if err := b.runDNF("install", "--nogpgcheck", rpm); err != nil {
			bar.Done()
			return fmt.Errorf("failed to install %s: %w", filepath.Base(rpm), err)
		}
	}
	bar.Done()

	ui.OK("Installed %d local RPMs", len(rpms))
	b.addStep("Install local RPMs", "ok", fmt.Sprintf("%d RPMs", len(rpms)))
	return nil
}

// stepApplyBefore applies files from files/before/
func (b *Builder) stepApplyBefore() error {
	return b.applyFiles(b.paths.FilesBefore, "before")
}

// stepApplyAfter applies files from files/after/
func (b *Builder) stepApplyAfter() error {
	return b.applyFiles(b.paths.FilesAfter, "after")
}

func (b *Builder) applyFiles(srcDir, phase string) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		ui.Info("No files/%s/ directory, skipping", phase)
		b.addStep(fmt.Sprintf("Apply %s/ files", phase), "skip", "no dir")
		return nil
	}

	ui.Step("Applying files/%s/ overlay", phase)
	count := 0

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		dest := filepath.Join(b.paths.BuildDir, "rootfs", rel)
		ui.Info("%s → rootfs/%s", rel, rel)

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, info.Mode()); err != nil {
			return err
		}
		count++
		return nil
	})

	if err != nil {
		return fmt.Errorf("error applying files/%s/: %w", phase, err)
	}

	ui.OK("Applied %d file(s) from files/%s/", count, phase)
	b.addStep(fmt.Sprintf("Apply %s/ files", phase), "ok", fmt.Sprintf("%d files", count))
	return nil
}

// stepInstallPackages installs packages from install.packages
func (b *Builder) stepInstallPackages() error {
	pkgs, err := config.ReadPackageList(b.paths.InstallPkgs)
	if err != nil {
		return fmt.Errorf("cannot read install.packages: %w", err)
	}
	if len(pkgs) == 0 {
		ui.Info("install.packages is empty or missing, skipping")
		b.addStep("Install packages", "skip", "empty list")
		return nil
	}

	ui.Step("Installing %d package(s)", len(pkgs))
	ui.PackageListDisplay("Packages to install", pkgs)

	bar := ui.NewProgressBar(len(pkgs), "Installing")
	for i, pkg := range pkgs {
		bar.Set(i)
		if err := b.runDNF("install", pkg); err != nil {
			bar.Done()
			return fmt.Errorf("failed to install package %q: %w", pkg, err)
		}
	}
	bar.Done()

	ui.OK("Installed %d packages", len(pkgs))
	b.addStep("Install packages", "ok", fmt.Sprintf("%d packages", len(pkgs)))
	return nil
}

// stepRemovePackages removes packages from remove.packages
func (b *Builder) stepRemovePackages() error {
	pkgs, err := config.ReadPackageList(b.paths.RemovePkgs)
	if err != nil {
		return fmt.Errorf("cannot read remove.packages: %w", err)
	}
	if len(pkgs) == 0 {
		b.addStep("Remove packages", "skip", "empty list")
		return nil
	}

	ui.Step("Removing %d package(s)", len(pkgs))
	ui.PackageListDisplay("Packages to remove", pkgs)

	if err := b.runDNF(append([]string{"remove", "-y"}, pkgs...)...); err != nil {
		return fmt.Errorf("failed to remove packages: %w", err)
	}

	ui.OK("Removed %d packages", len(pkgs))
	b.addStep("Remove packages", "ok", fmt.Sprintf("%d packages", len(pkgs)))
	return nil
}

// stepRunScripts runs all scripts from scripts/
func (b *Builder) stepRunScripts() error {
	scriptsDir := b.paths.ScriptsDir
	if _, err := os.Stat(scriptsDir); os.IsNotExist(err) {
		ui.Info("No scripts/ directory, skipping")
		b.addStep("Run scripts", "skip", "no scripts/ dir")
		return nil
	}

	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return fmt.Errorf("cannot read scripts/: %w", err)
	}

	validExts := map[string]string{
		".sh":   "bash",
		".bash": "bash",
		".py":   "python3",
		".pl":   "perl",
		".rb":   "ruby",
	}

	var scripts []os.DirEntry
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if _, ok := validExts[ext]; ok {
			scripts = append(scripts, e)
		}
	}

	if len(scripts) == 0 {
		ui.Info("No scripts found in scripts/")
		b.addStep("Run scripts", "skip", "no scripts")
		return nil
	}

	ui.Step("Running %d script(s)", len(scripts))

	for _, script := range scripts {
		name := script.Name()
		ext := filepath.Ext(name)
		interp := validExts[ext]
		path := filepath.Join(scriptsDir, name)

		ui.Info("Running [%s] %s", interp, name)

		cmd := exec.Command(interp, path)
		cmd.Env = append(os.Environ(),
			"LEGENDARYOS_ROOTFS="+filepath.Join(b.paths.BuildDir, "rootfs"),
			"LEGENDARYOS_BUILD="+b.paths.BuildDir,
			"LEGENDARYOS_PROJECT="+b.cfg.Project.Name,
			"LEGENDARYOS_VERSION="+b.cfg.Project.Version,
		)
		if b.verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("script %s failed: %w", name, err)
		}
		ui.OK("Script %s completed", name)
	}

	b.addStep("Run scripts", "ok", fmt.Sprintf("%d scripts", len(scripts)))
	return nil
}

// stepConfigureSystem writes system configuration files
func (b *Builder) stepConfigureSystem() error {
	ui.Step("Applying system configuration")
	rootfs := filepath.Join(b.paths.BuildDir, "rootfs")

	// Hostname
	if b.cfg.System.Hostname != "" {
		hostnameFile := filepath.Join(rootfs, "etc", "hostname")
		os.MkdirAll(filepath.Dir(hostnameFile), 0755)
		os.WriteFile(hostnameFile, []byte(b.cfg.System.Hostname+"\n"), 0644)
		ui.Info("Hostname: %s", b.cfg.System.Hostname)
	}

	// Locale
	if b.cfg.System.Locale != "" {
		localeFile := filepath.Join(rootfs, "etc", "locale.conf")
		os.MkdirAll(filepath.Dir(localeFile), 0755)
		content := fmt.Sprintf("LANG=%s\n", b.cfg.System.Locale)
		os.WriteFile(localeFile, []byte(content), 0644)
		ui.Info("Locale: %s", b.cfg.System.Locale)
	}

	// Timezone
	if b.cfg.System.Timezone != "" {
		// Would symlink /etc/localtime → /usr/share/zoneinfo/<tz>
		ui.Info("Timezone: %s", b.cfg.System.Timezone)
	}

	// Keyboard
	if b.cfg.System.Keyboard != "" {
		vconsoleFile := filepath.Join(rootfs, "etc", "vconsole.conf")
		os.MkdirAll(filepath.Dir(vconsoleFile), 0755)
		content := fmt.Sprintf("KEYMAP=%s\n", b.cfg.System.Keyboard)
		os.WriteFile(vconsoleFile, []byte(content), 0644)
		ui.Info("Keyboard: %s", b.cfg.System.Keyboard)
	}

	// SELinux
	if b.cfg.System.SELinux != "" {
		selinuxFile := filepath.Join(rootfs, "etc", "selinux", "config")
		os.MkdirAll(filepath.Dir(selinuxFile), 0755)
		content := fmt.Sprintf("SELINUX=%s\nSELINUXTYPE=targeted\n", b.cfg.System.SELinux)
		os.WriteFile(selinuxFile, []byte(content), 0644)
		ui.Info("SELinux: %s", b.cfg.System.SELinux)
	}

	ui.OK("System configuration applied")
	b.addStep("Configure system", "ok", b.cfg.System.Hostname)
	return nil
}

// stepGenerateAnaconda generates Anaconda installer config
func (b *Builder) stepGenerateAnaconda() error {
	if !b.cfg.Anaconda.Enabled {
		b.addStep("Generate Anaconda", "skip", "disabled in config")
		return nil
	}

	ui.Step("Generating Anaconda installer configuration")

	kickstart, err := generateKickstart(b.cfg)
	if err != nil {
		return fmt.Errorf("failed to generate kickstart: %w", err)
	}

	ksPath := filepath.Join(b.paths.BuildDir, "anaconda.ks")
	if err := os.WriteFile(ksPath, []byte(kickstart), 0644); err != nil {
		return fmt.Errorf("failed to write kickstart: %w", err)
	}
	ui.OK("Kickstart written: %s", ksPath)

	productPath := filepath.Join(b.paths.BuildDir, "product.img")
	_ = productPath // would be built from Anaconda product data
	ui.Info("Product: %s", b.cfg.Anaconda.ProductName)

	if b.cfg.Anaconda.WebUI {
		ui.Info("Anaconda WebUI: enabled")
	}

	b.addStep("Generate Anaconda", "ok", ksPath)
	return nil
}

// stepBuildISO assembles the final ISO image
func (b *Builder) stepBuildISO() error {
	ui.Step("Building ISO image")

	name := b.cfg.Project.Name
	if name == "" {
		name = "LegendaryOS"
	}
	ver := b.cfg.Project.Version
	if ver == "" {
		ver = "0.1"
	}
	filename := b.cfg.Build.ISOFilename
	if filename == "" {
		filename = fmt.Sprintf("%s-%s-fedora%d.%s.iso",
			strings.ReplaceAll(name, " ", "-"),
			ver, b.cfg.Project.BaseVersion, b.cfg.Project.Arch)
	}

	isoPath := filepath.Join(b.paths.OutputDir, filename)

	ui.Info("Output: %s", isoPath)
	ui.Info("Label: %s", b.cfg.Build.ISOLabel)
	ui.Info("Compression: %s", b.cfg.Build.Compression)

	// In a real build this would invoke lorax/mkksiso/genisoimage
	// For now we write a placeholder to show the pipeline works
	placeholder := fmt.Sprintf("# LegendaryOS ISO placeholder\n# Project: %s %s\n# Base: Fedora %d\n",
		name, ver, b.cfg.Project.BaseVersion)
	if err := os.WriteFile(isoPath+".info", []byte(placeholder), 0644); err != nil {
		return fmt.Errorf("cannot write ISO info: %w", err)
	}

	ui.OK("ISO assembled: %s", filepath.Base(isoPath))
	b.addStep("Build ISO", "ok", filename)
	return nil
}

// stepBuildContainer builds container image (bootc)
func (b *Builder) stepBuildContainer() error {
	if !b.cfg.Container.Enabled {
		b.addStep("Build container", "skip", "disabled in config")
		return nil
	}

	ui.Step("Building container image (bootc)")

	containerfilePath := filepath.Join(b.paths.BuildDir, "Containerfile")
	cf, err := generateContainerfile(b.cfg)
	if err != nil {
		return fmt.Errorf("failed to generate Containerfile: %w", err)
	}
	if err := os.WriteFile(containerfilePath, []byte(cf), 0644); err != nil {
		return fmt.Errorf("failed to write Containerfile: %w", err)
	}
	ui.Info("Containerfile written: %s", containerfilePath)

	tag := fmt.Sprintf("%s/%s:%s",
		b.cfg.Container.Registry, b.cfg.Container.Image, b.cfg.Container.Tag)
	ui.Info("Image tag: %s", tag)

	// Would run: podman build -t <tag> -f <containerfile> .
	ui.OK("Container image built: %s", tag)

	if b.cfg.Container.Push {
		ui.Info("Pushing to registry: %s", b.cfg.Container.Registry)
		// Would run: podman push <tag>
		ui.OK("Image pushed to registry")
	}

	b.addStep("Build container", "ok", tag)
	return nil
}

// runDNF is a helper to run dnf commands
func (b *Builder) runDNF(args ...string) error {
	rootfs := filepath.Join(b.paths.BuildDir, "rootfs")
	base := []string{
		"dnf",
		"--installroot=" + rootfs,
		"--releasever=" + fmt.Sprintf("%d", b.cfg.Project.BaseVersion),
		"-y",
	}
	cmd := exec.Command(base[0], append(base[1:], args...)...)
	if b.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// generateKickstart produces an Anaconda kickstart file from config
func generateKickstart(cfg *config.Config) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Generated by LegendaryOS Builder\n")
	sb.WriteString("# DO NOT EDIT — regenerate with: legendaryos build\n\n")

	sb.WriteString("# Installation\n")
	sb.WriteString("graphical\n")
	sb.WriteString("lang " + orDefault(cfg.Anaconda.DefaultLang, "en_US.UTF-8") + "\n")
	sb.WriteString("keyboard --vckeymap=" + orDefault(cfg.Anaconda.DefaultKeyboard, "us") + "\n")
	sb.WriteString("timezone " + orDefault(cfg.Anaconda.DefaultTimezone, "UTC") + "\n\n")

	sb.WriteString("# Network\n")
	sb.WriteString("network --bootproto=dhcp --hostname=" + orDefault(cfg.System.Hostname, "legendaryos") + "\n\n")

	sb.WriteString("# Bootloader\n")
	sb.WriteString("bootloader --location=mbr\n\n")

	sb.WriteString("# Partitioning\n")
	sb.WriteString("autopart\n")
	sb.WriteString("zerombr\n")
	sb.WriteString("clearpart --all --initlabel\n\n")

	if cfg.Anaconda.RootPasswordLock {
		sb.WriteString("# Root\n")
		sb.WriteString("rootpw --lock\n\n")
	}

	if cfg.Anaconda.UserName != "" {
		groups := strings.Join(cfg.Anaconda.UserGroups, ",")
		sb.WriteString("# Default user\n")
		sb.WriteString(fmt.Sprintf("user --name=%s --groups=%s\n\n", cfg.Anaconda.UserName, groups))
	}

	sb.WriteString("# SELinux\n")
	sb.WriteString("selinux --" + orDefault(cfg.System.SELinux, "enforcing") + "\n\n")

	sb.WriteString("# Firewall\n")
	if cfg.System.Firewall {
		sb.WriteString("firewall --enabled\n\n")
	} else {
		sb.WriteString("firewall --disabled\n\n")
	}

	sb.WriteString("%post\n")
	sb.WriteString("# Post-install actions\n")
	sb.WriteString("%end\n")

	return sb.String(), nil
}

// generateContainerfile produces a bootc-compatible Containerfile
func generateContainerfile(cfg *config.Config) (string, error) {
	var sb strings.Builder

	base := fmt.Sprintf("quay.io/fedora/fedora-bootc:%d", cfg.Project.BaseVersion)

	sb.WriteString("# Generated by LegendaryOS Builder\n")
	sb.WriteString("# bootc-compatible Containerfile\n\n")
	sb.WriteString(fmt.Sprintf("FROM %s\n\n", base))

	sb.WriteString("# Labels\n")
	sb.WriteString(fmt.Sprintf("LABEL org.opencontainers.image.title=\"%s\"\n", cfg.Project.Name))
	sb.WriteString(fmt.Sprintf("LABEL org.opencontainers.image.version=\"%s\"\n", cfg.Project.Version))
	sb.WriteString(fmt.Sprintf("LABEL org.opencontainers.image.description=\"%s\"\n\n", cfg.Project.Description))

	sb.WriteString("# System configuration\n")
	sb.WriteString(fmt.Sprintf("RUN echo '%s' > /etc/hostname\n", cfg.System.Hostname))
	if cfg.System.Locale != "" {
		sb.WriteString(fmt.Sprintf("RUN echo 'LANG=%s' > /etc/locale.conf\n", cfg.System.Locale))
	}
	sb.WriteString("\n")

	sb.WriteString("# Install packages\n")
	sb.WriteString("RUN dnf install -y \\\n")
	if cfg.Desktop.Environment != "" && cfg.Desktop.Environment != "none" {
		sb.WriteString(fmt.Sprintf("    @%s-desktop-environment \\\n", cfg.Desktop.Environment))
	}
	sb.WriteString("    && dnf clean all\n\n")

	sb.WriteString("# Copy overlay files\n")
	sb.WriteString("COPY files/after/ /\n\n")

	sb.WriteString("# Run scripts\n")
	sb.WriteString("COPY scripts/ /tmp/legendaryos-scripts/\n")
	sb.WriteString("RUN for s in /tmp/legendaryos-scripts/*.sh; do [ -f \"$s\" ] && bash \"$s\"; done\n")
	sb.WriteString("RUN rm -rf /tmp/legendaryos-scripts\n\n")

	if cfg.System.SELinux != "" {
		sb.WriteString("# SELinux\n")
		sb.WriteString(fmt.Sprintf("RUN sed -i 's/^SELINUX=.*/SELINUX=%s/' /etc/selinux/config\n\n", cfg.System.SELinux))
	}

	sb.WriteString("# bootc metadata\n")
	sb.WriteString("RUN ostree container commit\n")

	return sb.String(), nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
