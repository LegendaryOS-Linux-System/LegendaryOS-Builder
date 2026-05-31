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
}

func runInitWizard(dir string) {
	ui.Section("Project Wizard")
	fmt.Fprintln(ui.Out)

	a := &initAnswers{}
	a.name        = ui.AskDefault("Project name", "MyOS")

	// Version — guide user to semver or symbolic label
	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[90mVersion — enter a semver number (e.g. 0.1.0) or a symbolic label:\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90m  stable | beta | alpha | nightly | latest | edge | dev\033[0m\n")
	a.version     = ui.AskDefault("Version", "0.1.0")
	if config.IsSymbolicVersion(a.version) {
		ui.Info("Symbolic version label %q — will be used as the OCI tag", a.version)
	}

	a.description = ui.AskDefault("Description", "A custom Fedora-based OS")
	a.author      = ui.AskDefault("Author", "")

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

	scaffold(dir, a)
}

func runInitDefault(dir string) {
	scaffold(dir, &initAnswers{
		name: "MyOS", version: "0.1.0",
		description: "A custom Fedora-based OS",
		desktop: "gnome", hostname: "myos",
		locale: "en_US.UTF-8", timezone: "UTC", keyboard: "us",
		registry: "ghcr.io/myorg", image: "myos",
		anaconda: true, bootc: true,
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
	ui.Info("1. Edit config.toml  (version can be semver or: stable | beta | nightly | …)")
	ui.Info("2. Add packages to packages/install.packages")
	ui.Info("3. Drop .rpm files into packages/")
	ui.Info("4. Add files to files/after/ and files/before/")
	ui.Info("5. Add hooks to scripts/before/ and scripts/after/")
	ui.Info("6. legendaryos build cloud")
	ui.Info("7. legendaryos build iso")
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
		"  │   ├── flatpak.packages        ← Flatpak apps to install (Flathub IDs)",
		"  │   ├── flatpak.remove.packages ← Flatpak apps to remove",
		"  │   └── *.rpm                   ← local .rpm files (optional)",
		"  ├── files/",
		"  │   ├── before/            ← overlay BEFORE package install",
		"  │   └── after/             ← overlay AFTER package install",
		"  ├── scripts/",
		"  │   ├── pre/               ← run on HOST before any build step",
		"  │   ├── before/            ← run inside container BEFORE dnf install",
		"  │   └── after/             ← run inside container AFTER dnf install",
		"  ├── repos/                 ← custom .repo files",
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
		if b {
			return "true"
		}
		return "false"
	}

	// Add a comment when a symbolic version label is used
	versionComment := ""
	if config.IsSymbolicVersion(a.version) {
		versionComment = fmt.Sprintf("  # channel label — use git tags for semver releases")
	}

	return fmt.Sprintf(`# ╔══════════════════════════════════════════════════════════════╗
# ║              LegendaryOS Builder — config.toml              ║
# ╚══════════════════════════════════════════════════════════════╝
#
# version / tag fields accept:
#   • semver numbers  : 1.2.3
#   • symbolic labels : stable | beta | alpha | nightly | latest | edge | dev
#
# Symbolic labels are passed through to OCI tags, ISO filenames and
# the Anaconda kickstart as-is.

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

# ── Boot ──────────────────────────────────────────────────────────────────────
[boot]
bootloader  = "grub2"
kernel_args = "quiet splash"
splash      = true
timeout     = 5

# ── Anaconda Installer ────────────────────────────────────────────────────────
[anaconda]
enabled            = %s
kickstart_embed    = true
product_name       = %q
# product_version mirrors project.version — can be semver or symbolic label
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
# System filesystem type used by bootc-image-builder
# Options: ext4 | xfs | btrfs   (default: ext4)
filesystem   = "ext4"

# ── Container / bootc ─────────────────────────────────────────────────────────
[container]
enabled    = %s
registry   = %q
image      = %q
# tag mirrors project.version — accepts symbolic labels (stable, nightly, …)
tag        = %q
push       = false
sign_image = false
bootc_mode = true
`,
		a.name,
		a.version, versionComment,
		a.description, a.author,
		a.hostname, a.locale, a.timezone, a.keyboard,
		a.desktop,
		boolStr(a.anaconda), a.name, a.version,
		a.locale, a.keyboard, a.timezone,
		strings.ToUpper(strings.ReplaceAll(a.image, "-", "_")),
		boolStr(a.bootc), a.registry, a.image, a.version,
	)
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
			sb.WriteString("# Requires repos/cosmic.repo — see notes below\n")
			for _, p := range []string{
				"cosmic-session",
				"cosmic-settings",
				"cosmic-files",
				"cosmic-terminal",
				"cosmic-launcher",
				"cosmic-panel",
				"cosmic-dock",
				"cosmic-applets",
				"cosmic-bg",
				"cosmic-notifications",
				"cosmic-screenshot",
				"cosmic-edit",
				"cosmic-store",
				"cosmic-greeter",
				"xdg-user-dirs",
				"pipewire",
				"pipewire-alsa",
				"pipewire-pulseaudio",
				"wireplumber",
			} {
				sb.WriteString(p + "\n")
			}
		default:
			sb.WriteString(fmt.Sprintf("\n# ── %s Desktop ──────────────────────────────────────────────────\n",
				strings.Title(a.desktop)))
			sb.WriteString(fmt.Sprintf("@%s-desktop-environment\n", a.desktop))
		}
	}
	return sb.String()
}

var defaultFlatpakPkgs = `# packages/flatpak.packages — Flatpak apps to install (system-wide, from Flathub)
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

var defaultFlatpakRemovePkgs = `# packages/flatpak.remove.packages — Flatpak apps to remove
# One Application ID per line, comments start with #
# Useful for removing pre-installed Flatpaks from the base image
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
# Runs BEFORE package installation.
# Use for: adding custom repos, pre-seeding config, setting up keys.
set -euo pipefail

echo "==> LegendaryOS [before] hook: ${0##*/}"

# Example: import a GPG key before dnf install
# rpm --import https://example.com/RPM-GPG-KEY-myrepo
`

var exampleScriptAfter = `#!/usr/bin/env bash
# scripts/after/00-example.sh
# Runs AFTER package installation.
# Use for: system config, enabling services, sysctl tweaks, Flatpak remotes.
set -euo pipefail

echo "==> LegendaryOS [after] hook: ${0##*/}"
echo "    Project : ${LEGENDARYOS_PROJECT:-unknown}"
echo "    Version : ${LEGENDARYOS_VERSION:-unknown}"

# Example: enable a systemd service
# systemctl enable my-service.service

# Example: add Flathub remote
# flatpak remote-add --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
`

var cosmicRepo = `# repos/cosmic.repo
# COSMIC Desktop — System76
# COPR: nickel-org/cosmic-desktop (community port for Fedora)

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
	w("#")
	w("# Builds a bootc container image and pushes it to ghcr.io.")
	w("# Trigger: push to main, version tags (v*), or workflow_dispatch.")
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
