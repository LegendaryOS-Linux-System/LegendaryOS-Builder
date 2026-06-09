package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
)

type initAnswers struct {
	name        string
	version     string
	description string
	author      string
	desktop     string
	hostname    string
	locale      string
	timezone    string
	keyboard    string
	registry    string
	image       string
	anaconda    bool
	bootc       bool
	specialType string // "default" or "classic"
	bootloader  string // "grub2", "refind", "limine", "systemd-boot", "syslinux", ""
}

func runInitWizard(dir string) {
	ui.Section("Project Wizard")
	fmt.Fprintln(ui.Out)

	a := &initAnswers{}
	a.name = ui.AskDefault("Project name", "MyOS")

	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[90mVersion — semver (e.g. 0.1.0) or symbolic label:\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90m  stable | beta | alpha | nightly | latest | edge | dev\033[0m\n")
	a.version = ui.AskDefault("Version", "0.1.0")
	if config.IsSymbolicVersion(a.version) {
		ui.Info("Symbolic version label %q — will be used as the OCI tag", a.version)
	}

	a.description = ui.AskDefault("Description", "A custom Fedora-based OS")
	a.author      = ui.AskDefault("Author", "")

	// Build mode
	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[90mBuild mode:\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90m  default → immutable Fedora (bootc/ostree)\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90m  classic → plain mutable Fedora\033[0m\n")
	modes    := []string{"default", "classic"}
	modeIdx  := ui.AskChoice("Build mode (special_type)", modes, 0)
	a.specialType = modes[modeIdx]

	desktops := []string{"gnome", "kde", "xfce", "cinnamon", "mate", "lxqt", "cosmic", "none"}
	idx := ui.AskChoice("Desktop environment", desktops, 0)
	a.desktop = desktops[idx]

	defHost := strings.ToLower(strings.ReplaceAll(a.name, " ", "-"))
	a.hostname  = ui.AskDefault("Default hostname", defHost)
	a.locale    = ui.AskDefault("Locale", "en_US.UTF-8")
	a.timezone  = ui.AskDefault("Timezone", "UTC")
	a.keyboard  = ui.AskDefault("Keyboard layout", "us")
	a.registry  = ui.AskDefault("Container registry (for build cloud)", "ghcr.io/myorg")
	a.image     = ui.AskDefault("Image name", strings.ToLower(strings.ReplaceAll(a.name, " ", "-")))
	a.anaconda  = ui.AskYN("Generate Anaconda installer?", true)
	a.bootc     = ui.AskYN("Enable bootc container build?", true)

	// Custom bootloader
	fmt.Fprintln(ui.Out)
	useCustomBL := ui.AskYN("Use custom bootloader? (default: grub2)", false)
	if useCustomBL {
		bls    := []string{"grub2", "refind", "limine", "systemd-boot", "syslinux"}
		blIdx  := ui.AskChoice("Bootloader type", bls, 0)
		a.bootloader = bls[blIdx]
	}

	scaffold(dir, a)
}

func runInitDefault(dir string) {
	scaffold(dir, &initAnswers{
		name:        "MyOS",
	  version:     "0.1.0",
	  description: "A custom Fedora-based OS",
	  desktop:     "gnome",
	  hostname:    "myos",
	  locale:      "en_US.UTF-8",
	  timezone:    "UTC",
	  keyboard:    "us",
	  registry:    "ghcr.io/myorg",
	  image:       "myos",
	  anaconda:    true,
	  bootc:       true,
	  specialType: "default",
	})
}

func scaffold(dir string, a *initAnswers) {
	ui.Section("Initializing Project")
	ui.Info("Target directory: %s", dir)
	ui.Newline()

	paths := config.GetPaths(dir)

	dirs := []string{
		dir,
		paths.PackagesDir,
		paths.FilesAfter + "/etc",
		paths.FilesAfter + "/usr/local/bin",
		paths.FilesBefore + "/etc",
		paths.ScriptsDir,
		paths.ScriptsPre,
		paths.ScriptsBefore,
		paths.ScriptsAfter,
		paths.ReposDir,
		paths.BuildDir,
		paths.CacheDir,
		paths.OutputDir,
		filepath.Join(dir, ".github", "workflows"),
	}

	bar := ui.NewProgressBar(len(dirs)+8, "scaffolding")
	for i, d := range dirs {
		bar.Set(i)
		if err := os.MkdirAll(d, 0755); err != nil {
			ui.Fatal("Cannot create %s: %v", d, err)
		}
	}

	write := func(path, content string) {
		if err := writeNew(path, content); err != nil {
			ui.Fatal("Cannot write %s: %v", path, err)
		}
	}

	bar.Set(len(dirs) + 1)
	write(paths.Config, renderConfig(a))
	ui.OK("config.toml")

	bar.Set(len(dirs) + 2)
	write(paths.InstallPkgs, renderInstallPackages(a))
	ui.OK("packages/install.packages")

	bar.Set(len(dirs) + 3)
	write(paths.RemovePkgs, defaultRemovePkgs)
	ui.OK("packages/remove.packages")

	bar.Set(len(dirs) + 4)
	write(paths.FlatpakPkgs, defaultFlatpakPkgs)
	ui.OK("packages/flatpak.packages")

	bar.Set(len(dirs) + 5)
	write(paths.FlatpakRemovePkgs, defaultFlatpakRemovePkgs)
	ui.OK("packages/flatpak.remove.packages")

	bar.Set(len(dirs) + 4)
	write(filepath.Join(paths.ScriptsBefore, "00-example.sh"), exampleScriptBefore)
	ui.OK("scripts/before/00-example.sh")

	bar.Set(len(dirs) + 5)
	write(filepath.Join(paths.ScriptsAfter, "00-example.sh"), exampleScriptAfter)
	ui.OK("scripts/after/00-example.sh")

	bar.Set(len(dirs) + 5)
	write(filepath.Join(paths.ReposDir, "example.repo"), exampleRepo)
	ui.OK("repos/example.repo")

	if a.desktop == "cosmic" {
		write(filepath.Join(paths.ReposDir, "cosmic.repo"), cosmicRepo)
		ui.OK("repos/cosmic.repo  (COSMIC desktop repo)")
	}

	bar.Set(len(dirs) + 6)
	write(filepath.Join(dir, ".gitignore"), gitignore)
	write(filepath.Join(dir, ".github", "workflows", "build-cloud.yml"), renderGHAWorkflow(a))
	ui.OK(".github/workflows/build-cloud.yml")

	bar.Done()

	fmt.Fprintln(ui.Out)
	ui.Section("Project layout")
	printTree(dir)

	fmt.Fprintln(ui.Out)
	ui.Section("Next steps")
	ui.Info("1.  Edit config.toml")
	ui.Info("2.  Add packages to packages/install.packages")
	ui.Info("3.  Add Flatpak IDs to packages/flatpak.packages  (applied at install time)")
	ui.Info("4.  Drop .rpm files into packages/")
	ui.Info("5.  Add files to files/after/ and files/before/")
	ui.Info("6.  Add hooks to scripts/before/ and scripts/after/")
	ui.Info("7.  legendaryos build cloud")
	ui.Info("8.  legendaryos build iso")
	fmt.Fprintln(ui.Out)
}

func printTree(dir string) {
	base := filepath.Base(dir)
	if base == "." {
		base = "project"
	}
	lines := []string{
		fmt.Sprintf("  %s/", base),
		"  ├── config.toml            ← main configuration",
		"  ├── packages/",
		"  │   ├── install.packages        ← DNF packages to install",
		"  │   ├── remove.packages         ← DNF packages to remove",
		"  │   ├── flatpak.packages        ← Flatpak apps (applied at install time)",
		"  │   ├── flatpak.remove.packages ← Flatpak apps to remove (install time)",
		"  │   └── *.rpm                   ← local .rpm files (optional)",
		"  ├── files/",
		"  │   ├── before/            ← overlay BEFORE package install",
		"  │   └── after/             ← overlay AFTER package install",
		"  ├── scripts/",
		"  │   ├── pre/               ← run on HOST before any build step",
		"  │   ├── before/            ← run inside container BEFORE dnf install",
		"  │   └── after/             ← run inside container AFTER dnf install",
		"  ├── repos/                 ← custom .repo files",
		"  ├── build.rb               ← optional custom build script",
		"  ├── .github/workflows/",
		"  │   └── build-cloud.yml    ← GitHub Actions CI",
		"  └── build/",
		"      ├── cache/",
		"      └── output/            ← final .iso",
	}
	for _, l := range lines {
		fmt.Fprintln(ui.Out, l)
	}
}

func writeNew(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — don't overwrite
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// ── Templates ─────────────────────────────────────────────────────────────────

func renderConfig(a *initAnswers) string {
	boolStr := func(b bool) string {
		if b { return "true" }
		return "false"
	}

	versionComment := ""
	if config.IsSymbolicVersion(a.version) {
		versionComment = "  # channel label — use git tags for semver releases"
	}

	specialTypeComment := ""
	switch a.specialType {
		case "classic":
			specialTypeComment = "  # classic → plain mutable Fedora (no bootc/ostree)"
		default:
			specialTypeComment = "  # default → Fedora immutable (bootc/ostree)"
	}

	// Build the [bootloader] section
	bootloaderSection := renderBootloaderSection(a.bootloader)

	return fmt.Sprintf(`# ╔══════════════════════════════════════════════════════════════╗
	# ║              LegendaryOS Builder — config.toml              ║
	# ╚══════════════════════════════════════════════════════════════╝
	#
	# version / tag fields accept:
	#   • semver numbers  : 1.2.3
	#   • symbolic labels : stable | beta | alpha | nightly | latest | edge | dev
	#
	# special_type:
	#   "default" → Fedora immutable (bootc/ostree) — default when absent
	#   "classic" → plain mutable Fedora (traditional RPM system)

	# ── Project ───────────────────────────────────────────────────────────────────
	[project]
	name         = %q
	version      = %q%s
	description  = %q
	author       = %q
	license      = "GPL-2.0"
	base_distro  = "fedora"
	base_version = 44
	arch         = "x86_64"
	special_type = %q%s

	# ── System ────────────────────────────────────────────────────────────────────
	[system]
	hostname         = %q
	locale           = %q
	timezone         = %q
	keyboard         = %q
	language         = "en_US"
	selinux          = "enforcing"
	firewall         = true
	services_enable  = ["sshd"]
	services_disable = []

	# ── Desktop ───────────────────────────────────────────────────────────────────
	[desktop]
	environment     = %q
	display_server  = "wayland"
	auto_login      = false
	auto_login_user = ""

	# ── Boot (legacy — overridden when [bootloader] enabled = true) ───────────────
	[boot]
	bootloader  = "grub2"
	kernel_args = "quiet splash"
	splash      = true
	timeout     = 5
	%s
	# ── Anaconda Installer ────────────────────────────────────────────────────────
	[anaconda]
	enabled            = %s
	kickstart_embed    = true
	product_name       = %q
	product_version    = %q
	webui              = true
	hide_shell         = false
	default_lang       = %q
	default_keyboard   = %q
	default_timezone   = %q
	root_password_lock = true
	default_user       = "user"
	default_user_groups = ["wheel", "audio", "video"]

	# ── Build ─────────────────────────────────────────────────────────────────────
	[build]
	output_dir   = "build/output"
	cache_dir    = "build/cache"
	compression  = "xz"
	iso_label    = %q
	iso_filename = ""
	jobs         = 4
	clean_build  = false
	filesystem   = "ext4"

	# ── Container / bootc ─────────────────────────────────────────────────────────
	[container]
	enabled    = %s
	registry   = %q
	image      = %q
	tag        = %q
	push       = false
	sign_image = false
	bootc_mode = true
	`,
	a.name,
	a.version, versionComment,
	a.description, a.author,
	a.specialType, specialTypeComment,
	a.hostname, a.locale, a.timezone, a.keyboard,
	a.desktop,
	bootloaderSection,
	boolStr(a.anaconda), a.name, a.version,
			   a.locale, a.keyboard, a.timezone,
		    strings.ToUpper(strings.ReplaceAll(a.image, "-", "_")),
			   boolStr(a.bootc), a.registry, a.image, a.version,
	)
}

// renderBootloaderSection generates the [bootloader] config block.
// When bootloader is empty the section is present but disabled (comments only).
func renderBootloaderSection(bl string) string {
	if bl == "" {
		return `# ── Bootloader (optional) ────────────────────────────────────────────────────
		# Uncomment and set enabled = true to use an alternative bootloader.
		# When disabled (default), boot.bootloader above governs behaviour.
		#
		# [bootloader]
		# enabled          = false
		# type             = "grub2"    # grub2 | refind | limine | systemd-boot | syslinux
		# extra_args       = ""         # extra kernel args appended when this bootloader is active
		# install_packages = []         # extra DNF packages to install for this bootloader
		# efi_dir          = "/boot/efi"
		`
	}

	return fmt.Sprintf(`# ── Bootloader ───────────────────────────────────────────────────────────────
	# type: grub2 | refind | limine | systemd-boot | syslinux
	[bootloader]
	enabled          = true
	type             = %q
	extra_args       = ""
	install_packages = []
	efi_dir          = "/boot/efi"

	`, bl)
}

func renderInstallPackages(a *initAnswers) string {
	var sb strings.Builder
	sb.WriteString("# packages/install.packages — one package per line\n")
	sb.WriteString("# Comments start with #\n")
	sb.WriteString("# Groups: prefix with @  e.g. @base-x\n\n")
	sb.WriteString("# ── Base tools ───────────────────────────────────────────────────\n")
	for _, p := range []string{"bash-completion", "curl", "wget", "git", "vim-enhanced", "htop", "rsync", "unzip", "zip", "tar"} {
		sb.WriteString(p + "\n")
	}
	if a.desktop != "none" && a.desktop != "" {
		switch a.desktop {
			case "cosmic":
				sb.WriteString("\n# ── COSMIC Desktop (System76) ──────────────────────────────────\n")
				for _, p := range []string{
					"cosmic-session", "cosmic-settings", "cosmic-files", "cosmic-terminal",
					"cosmic-launcher", "cosmic-panel", "cosmic-dock", "cosmic-applets",
					"cosmic-bg", "cosmic-notifications", "cosmic-screenshot", "cosmic-edit",
					"cosmic-store", "cosmic-greeter", "xdg-user-dirs",
					"pipewire", "pipewire-alsa", "pipewire-pulseaudio", "wireplumber",
				} {
					sb.WriteString(p + "\n")
				}
			default:
				sb.WriteString(fmt.Sprintf("\n# ── %s Desktop ──────────────────────────────────────────────────\n",
							   strings.Title(a.desktop))) //nolint
				sb.WriteString(fmt.Sprintf("@%s-desktop-environment\n", a.desktop))
		}
	}
	return sb.String()
}

var defaultFlatpakPkgs = `# packages/flatpak.packages — Flatpak apps to install at system install time
# These are applied via the Anaconda %post section, NOT during the container build.
# One Application ID per line, comments start with #
# Find IDs at: https://flathub.org
#
# Examples:
# com.visualstudio.code
# com.spotify.Client
# io.github.zen_browser.zen
# com.discordapp.Discord
# com.heroicgameslauncher.hgl
`

var defaultFlatpakRemovePkgs = `# packages/flatpak.remove.packages — Flatpak apps to remove at install time
# Applied via Anaconda %post, NOT during the container build.
# One Application ID per line, comments start with #
#
# Examples:
# org.gnome.Maps
# org.gnome.Weather
`

var defaultRemovePkgs = `# packages/remove.packages — packages to remove after install
# One per line, # = comment

# Slim down the image:
# nano
# gedit
`

var exampleScriptBefore = `#!/usr/bin/env bash
# scripts/before/00-example.sh
# Runs BEFORE package installation (inside container).
set -euo pipefail
echo "==> LegendaryOS [before] hook: ${0##*/}"
`

var exampleScriptAfter = `#!/usr/bin/env bash
# scripts/after/00-example.sh
# Runs AFTER package installation (inside container).
set -euo pipefail
echo "==> LegendaryOS [after] hook: ${0##*/}"
echo "    Project : ${LEGENDARYOS_PROJECT:-unknown}"
echo "    Version : ${LEGENDARYOS_VERSION:-unknown}"
`

var cosmicRepo = `# repos/cosmic.repo — COSMIC Desktop (System76) via COPR
[copr:copr.fedorainfracloud.org:nickel-org:cosmic-desktop]
name=COSMIC Desktop (Fedora $releasever)
baseurl=https://download.copr.fedorainfracloud.org/results/nickel-org/cosmic-desktop/fedora-$releasever-x86_64/
type=rpm-md
enabled=1
gpgcheck=1
gpgkey=https://download.copr.fedorainfracloud.org/results/nickel-org/cosmic-desktop/pubkey.gpg
repo_gpgcheck=0
priority=80
`

var exampleRepo = `# repos/example.repo
# Drop .repo files here to add custom DNF repositories.
# They are copied into the container during 'build cloud'.
#
# [my-repo]
# name=My Custom Repo
# baseurl=https://example.com/repo/fedora/$releasever/
# enabled=1
# gpgcheck=1
# gpgkey=https://example.com/repo/RPM-GPG-KEY
`

var gitignore = `# LegendaryOS Builder
build/output/
build/work/
build/iso-work/
build/rootfs/
build/context/
build/podman-storage/
*.iso
*.img
config.toml.bak

# Personal credentials — NEVER commit this
user.toml
`

func renderGHAWorkflow(a *initAnswers) string {
	imgName := a.image
	if imgName == "" {
		imgName = strings.ToLower(strings.ReplaceAll(a.name, " ", "-"))
	}

	ex := func(s string) string { return "${{" + " " + s + " }}" }

	notPR     := ex("github.event_name") + " != 'pull_request'"
	actor     := ex("github.actor")
	token     := ex("secrets.GITHUB_TOKEN")
	githubRef := ex("GITHUB_REF")
	refName   := ex("GITHUB_REF_NAME")
	tagFull   := ex("steps.tag.outputs.full")
	ifNotPR   := "if: " + notPR

	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }

	w("# .github/workflows/build-cloud.yml")
	w("# Generated by LegendaryOS Builder")
	w("")
	w("name: Build Cloud Image")
	w("")
	w("on:")
	w("  push:")
	w("    branches: [main]")
	w("    tags:     ['v*']")
	w("  pull_request:")
	w("    branches: [main]")
	w("  workflow_dispatch:")
	w("")
	w("env:")
	w("  IMAGE: ghcr.io/" + ex("github.repository_owner") + "/" + imgName)
	w("")
	w("jobs:")
	w("  build:")
	w("    name: Build bootc container image")
	w("    runs-on: ubuntu-latest")
	w("    permissions:")
	w("      contents: read")
	w("      packages: write")
	w("      id-token: write")
	w("")
	w("    steps:")
	w("      - name: Checkout")
	w("        uses: actions/checkout@v4")
	w("")
	w("      - name: Download legendaryos-builder")
	w("        run: |")
	w("          curl -fsSL \\")
	w("            https://github.com/legendaryos/builder/releases/latest/download/legendaryos-linux-amd64.tar.gz \\")
	w("            -o /tmp/legendaryos.tar.gz")
	w("          tar -xzf /tmp/legendaryos.tar.gz -C /tmp")
	w("          chmod +x /tmp/legendaryos")
	w("          sudo mv /tmp/legendaryos /usr/local/bin/legendaryos")
	w("          legendaryos version")
	w("")
	w("      - name: Install Podman")
	w("        run: |")
	w("          sudo apt-get update -qq")
	w("          sudo apt-get install -y podman buildah")
	w("")
	w("      - name: Log in to ghcr.io")
	w("        " + ifNotPR)
	w("        run: |")
	w("          echo \"" + token + "\" \\")
	w("            | podman login ghcr.io -u \"" + actor + "\" --password-stdin")
	w("")
	w("      - name: Compute image tag")
	w("        id: tag")
	w("        run: |")
	w("          if [[ \"" + githubRef + "\" == refs/tags/v* ]]; then")
	w("            TAG=\"" + refName + "\"")
	w("          else")
	w("            TAG=\"latest\"")
	w("          fi")
	w("          echo \"value=${TAG}\" >> \"$GITHUB_OUTPUT\"")
	w("          echo \"full=${IMAGE}:${TAG}\" >> \"$GITHUB_OUTPUT\"")
	w("")
	w("      - name: Validate")
	w("        run: legendaryos validate")
	w("")
	w("      - name: Build cloud image")
	w("        run: |")
	w("          PUSH_FLAG=\"\"")
	w("          if [ \"" + ex("github.event_name") + "\" != \"pull_request\" ]; then")
	w("            PUSH_FLAG=\"--push\"")
	w("          fi")
	w("          legendaryos build cloud \\")
	w("            --platform linux/amd64 \\")
	w("            $PUSH_FLAG \\")
	w("            --verbose")
	w("")
	w("      - name: Install cosign")
	w("        " + ifNotPR)
	w("        uses: sigstore/cosign-installer@v3")
	w("")
	w("      - name: Sign image")
	w("        " + ifNotPR)
	w("        run: cosign sign --yes \"" + tagFull + "\"")
	w("        env:")
	w("          COSIGN_EXPERIMENTAL: \"true\"")
	w("")
	w("      - name: Summary")
	w("        run: |")
	w("          {")
	w("            echo \"## ⬡ LegendaryOS Cloud Build\"")
	w("            echo \"\"")
	w("            echo \"| Field | Value |\"")
	w("            echo \"|-------|-------|\"")
	w("            echo \"| Image | \\`" + tagFull + "\\` |\"")
	w("            echo \"| Base  | Fedora 44 |\"")
	w("          } >> \"$GITHUB_STEP_SUMMARY\"")

	return b.String()
}
