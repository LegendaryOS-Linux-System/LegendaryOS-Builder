package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/legendaryos/builder/src/config"
	"github.com/legendaryos/builder/src/ui"
)

// cmdSettings — interactive TUI settings editor for config.toml
func cmdSettings(_ []string) {
	ui.SmallBanner()
	ui.Section("Settings")

	root := cwd()
	cfg, err := config.Load(root)
	if err != nil {
		ui.Fatal("%v", err)
	}

	for {
		showSettingsMenu(cfg)
		choice := askField("Choice", "0")
		switch choice {
		case "0", "q", "quit", "exit":
			ui.OK("Settings closed (no changes saved)")
			ui.Newline()
			return
		case "1":
			editProject(cfg)
		case "2":
			editSystem(cfg)
		case "3":
			editDesktop(cfg)
		case "4":
			editContainer(cfg)
		case "5":
			editAnaconda(cfg)
		case "6":
			editBuild(cfg)
		case "s", "save":
			if err := saveSettings(root, cfg); err != nil {
				ui.Error("Save failed: %v", err)
			} else {
				ui.OK("Saved to config.toml")
			}
		default:
			ui.Warn("Unknown option: %q", choice)
		}
	}
}

func showSettingsMenu(cfg *config.Config) {
	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[96;1m%-4s %-22s %s\033[0m\n", "#", "Section", "Current values")
	fmt.Fprintf(ui.Out, "  \033[90m%s\033[0m\n", strings.Repeat("─", 60))

	row := func(n, section, val string) {
		fmt.Fprintf(ui.Out, "  \033[96m[%s]\033[0m \033[97;1m%-22s\033[0m \033[90m%s\033[0m\n", n, section, val)
	}

	// version shown as-is — symbolic or semver
	row("1", "Project",   fmt.Sprintf("%s %s  (Fedora %d)", cfg.Project.Name, cfg.Project.Version, cfg.Project.BaseVersion))
	row("2", "System",    fmt.Sprintf("host=%s  locale=%s  tz=%s", cfg.System.Hostname, cfg.System.Locale, cfg.System.Timezone))
	row("3", "Desktop",   fmt.Sprintf("env=%s  display=%s", cfg.Desktop.Environment, cfg.Desktop.DisplayServer))
	row("4", "Container", fmt.Sprintf("%s/%s:%s", cfg.Container.Registry, cfg.Container.Image, cfg.Container.Tag))
	row("5", "Anaconda",  fmt.Sprintf("enabled=%v  webui=%v", cfg.Anaconda.Enabled, cfg.Anaconda.WebUI))
	row("6", "Build",     fmt.Sprintf("compression=%s  jobs=%d", cfg.Build.Compression, cfg.Build.Jobs))

	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[96m[s]\033[0m \033[97;1mSave changes to config.toml\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[96m[0]\033[0m \033[97;1mExit without saving\033[0m\n")
	fmt.Fprintln(ui.Out)
}

// ── Section editors ───────────────────────────────────────────────────────────

func editProject(cfg *config.Config) {
	ui.Section("Project settings")
	cfg.Project.Name        = askField("Name",        cfg.Project.Name)
	cfg.Project.Description = askField("Description", cfg.Project.Description)
	cfg.Project.Author      = askField("Author",      cfg.Project.Author)
	cfg.Project.License     = askField("License",     cfg.Project.License)

	// Version — accept both semver and symbolic labels
	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[90mVersion can be a semver number (e.g. 1.2.3) or a symbolic label:\033[0m\n")
	fmt.Fprintf(ui.Out, "  \033[90msymbolic: stable | beta | alpha | nightly | latest | edge | dev\033[0m\n")
	cfg.Project.Version = askField("Version", cfg.Project.Version)
	if config.IsSymbolicVersion(cfg.Project.Version) {
		ui.Info("Using symbolic version label: %q", cfg.Project.Version)
	}

	v := askField("Fedora base version", fmt.Sprintf("%d", cfg.Project.BaseVersion))
	fmt.Sscanf(v, "%d", &cfg.Project.BaseVersion)
	ui.OK("Project updated (not saved yet — press [s])")
}

func editSystem(cfg *config.Config) {
	ui.Section("System settings")
	cfg.System.Hostname = askField("Hostname",  cfg.System.Hostname)
	cfg.System.Locale   = askField("Locale",    cfg.System.Locale)
	cfg.System.Timezone = askField("Timezone",  cfg.System.Timezone)
	cfg.System.Keyboard = askField("Keyboard",  cfg.System.Keyboard)
	cfg.System.SELinux  = askFieldChoice("SELinux", cfg.System.SELinux, []string{"enforcing", "permissive", "disabled"})
	cfg.System.Firewall = askFieldBool("Firewall", cfg.System.Firewall)
	ui.OK("System updated (not saved yet — press [s])")
}

func editDesktop(cfg *config.Config) {
	ui.Section("Desktop settings")
	cfg.Desktop.Environment   = askFieldChoice("Environment",    cfg.Desktop.Environment,   []string{"gnome", "kde", "xfce", "cinnamon", "mate", "lxqt", "cosmic", "none"})
	cfg.Desktop.DisplayServer = askFieldChoice("Display server", cfg.Desktop.DisplayServer, []string{"wayland", "x11", "both"})
	cfg.Desktop.AutoLogin     = askFieldBool("Auto login",       cfg.Desktop.AutoLogin)
	if cfg.Desktop.AutoLogin {
		cfg.Desktop.AutoLoginUser = askField("Auto login user", cfg.Desktop.AutoLoginUser)
	}
	ui.OK("Desktop updated (not saved yet — press [s])")
}

func editContainer(cfg *config.Config) {
	ui.Section("Container / bootc settings")
	cfg.Container.Enabled  = askFieldBool("Enabled",  cfg.Container.Enabled)
	cfg.Container.Registry = askField("Registry", cfg.Container.Registry)
	cfg.Container.Image    = askField("Image",    cfg.Container.Image)

	fmt.Fprintln(ui.Out)
	fmt.Fprintf(ui.Out, "  \033[90mTag can be a semver number or symbolic label (stable, nightly, …)\033[0m\n")
	cfg.Container.Tag = askField("Tag", cfg.Container.Tag)

	cfg.Container.Push      = askFieldBool("Auto push after build", cfg.Container.Push)
	cfg.Container.SignImage  = askFieldBool("Sign image with cosign", cfg.Container.SignImage)
	ui.OK("Container updated (not saved yet — press [s])")
}

func editAnaconda(cfg *config.Config) {
	ui.Section("Anaconda installer settings")
	cfg.Anaconda.Enabled          = askFieldBool("Enabled",          cfg.Anaconda.Enabled)
	cfg.Anaconda.ProductName      = askField("Product name",          cfg.Anaconda.ProductName)

	fmt.Fprintf(ui.Out, "  \033[90mProduct version — semver or symbolic (stable, beta, …)\033[0m\n")
	cfg.Anaconda.ProductVersion   = askField("Product version",       cfg.Anaconda.ProductVersion)

	cfg.Anaconda.WebUI            = askFieldBool("WebUI",             cfg.Anaconda.WebUI)
	cfg.Anaconda.DefaultLang      = askField("Default language",      cfg.Anaconda.DefaultLang)
	cfg.Anaconda.DefaultKeyboard  = askField("Default keyboard",      cfg.Anaconda.DefaultKeyboard)
	cfg.Anaconda.DefaultTimezone  = askField("Default timezone",      cfg.Anaconda.DefaultTimezone)
	cfg.Anaconda.RootPasswordLock = askFieldBool("Lock root password", cfg.Anaconda.RootPasswordLock)
	cfg.Anaconda.UserName         = askField("Default user",          cfg.Anaconda.UserName)
	ui.OK("Anaconda updated (not saved yet — press [s])")
}

func editBuild(cfg *config.Config) {
	ui.Section("Build settings")
	cfg.Build.Compression = askFieldChoice("Compression", cfg.Build.Compression, []string{"zstd", "xz", "gzip"})
	cfg.Build.Filesystem  = askFieldChoice("Filesystem (for ISO)", cfg.Build.Filesystem, []string{"ext4", "xfs", "btrfs"})
	j := askField("Parallel jobs", fmt.Sprintf("%d", cfg.Build.Jobs))
	fmt.Sscanf(j, "%d", &cfg.Build.Jobs)
	cfg.Build.ISOLabel    = askField("ISO label",                    cfg.Build.ISOLabel)
	cfg.Build.ISOFilename = askField("ISO filename (empty = auto)",  cfg.Build.ISOFilename)
	cfg.Build.CleanBuild  = askFieldBool("Clean build",              cfg.Build.CleanBuild)
	ui.OK("Build updated (not saved yet — press [s])")
}

// ── Save ──────────────────────────────────────────────────────────────────────

func saveSettings(root string, cfg *config.Config) error {
	cfgPath := filepath.Join(root, "config.toml")

	// Backup original
	orig, err := os.ReadFile(cfgPath)
	if err == nil {
		_ = os.WriteFile(cfgPath+".bak", orig, 0644)
	}

	content := renderConfigFromStruct(cfg)
	return os.WriteFile(cfgPath, []byte(content), 0644)
}

func renderConfigFromStruct(cfg *config.Config) string {
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	sl := func(ss []string) string {
		parts := make([]string, len(ss))
		for i, s := range ss {
			parts[i] = fmt.Sprintf("%q", s)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}

	// version channel comment — shown when a symbolic label is used
	verComment := ""
	if config.IsSymbolicVersion(cfg.Project.Version) {
		verComment = fmt.Sprintf("  # channel: %s → use git tags for semver releases", cfg.Project.Version)
	}

	return fmt.Sprintf(`# config.toml — saved by legendaryos-builder settings
# Backup of previous config saved to config.toml.bak
#
# version / tag fields accept:
#   • semver numbers : 1.2.3
#   • symbolic labels: stable | beta | alpha | nightly | latest | edge | dev

[project]
name         = %q
version      = %q%s
description  = %q
author       = %q
license      = %q
base_distro  = "fedora"
base_version = %d
arch         = "x86_64"

[system]
hostname         = %q
locale           = %q
timezone         = %q
keyboard         = %q
language         = "pl_PL:pl:en_US:en"
selinux          = %q
firewall         = %s
services_enable  = %s
services_disable = %s

[desktop]
environment     = %q
display_server  = %q
auto_login      = %s
auto_login_user = %q

[boot]
bootloader  = %q
kernel_args = %q
splash      = true
timeout     = %d

[anaconda]
enabled            = %s
kickstart_embed    = true
product_name       = %q
product_version    = %q
webui              = %s
hide_shell         = false
default_lang       = %q
default_keyboard   = %q
default_timezone   = %q
root_password_lock = %s
default_user       = %q
default_user_groups = %s

[build]
output_dir   = "build/output"
cache_dir    = "build/cache"
compression  = %q
iso_label    = %q
iso_filename = %q
jobs         = %d
clean_build  = %s
# System filesystem type used by bootc-image-builder
# Options: ext4 | xfs | btrfs   (default: ext4)
filesystem   = %q

[container]
enabled    = %s
registry   = %q
image      = %q
# tag accepts semver or symbolic label: stable | beta | nightly | latest | edge | dev
tag        = %q
push       = %s
sign_image = %s
bootc_mode = true
`,
		cfg.Project.Name, cfg.Project.Version, verComment,
		cfg.Project.Description, cfg.Project.Author, cfg.Project.License,
		cfg.Project.BaseVersion,

		cfg.System.Hostname, cfg.System.Locale, cfg.System.Timezone,
		cfg.System.Keyboard, cfg.System.SELinux, b(cfg.System.Firewall),
		sl(cfg.System.Services), sl(cfg.System.Disable),

		cfg.Desktop.Environment, cfg.Desktop.DisplayServer,
		b(cfg.Desktop.AutoLogin), cfg.Desktop.AutoLoginUser,

		cfg.Boot.Bootloader, cfg.Boot.KernelArgs, cfg.Boot.Timeout,

		b(cfg.Anaconda.Enabled), cfg.Anaconda.ProductName, cfg.Anaconda.ProductVersion,
		b(cfg.Anaconda.WebUI), cfg.Anaconda.DefaultLang, cfg.Anaconda.DefaultKeyboard,
		cfg.Anaconda.DefaultTimezone, b(cfg.Anaconda.RootPasswordLock),
		cfg.Anaconda.UserName, sl(cfg.Anaconda.UserGroups),

		cfg.Build.Compression, cfg.Build.ISOLabel, cfg.Build.ISOFilename,
		cfg.Build.Jobs, b(cfg.Build.CleanBuild), cfg.Build.Filesystem,

		b(cfg.Container.Enabled), cfg.Container.Registry, cfg.Container.Image,
		cfg.Container.Tag, b(cfg.Container.Push), b(cfg.Container.SignImage),
	)
}

// ── Input helpers ─────────────────────────────────────────────────────────────

func askField(label, current string) string {
	fmt.Fprintf(ui.Out, "  \033[96m%-26s\033[0m \033[90m[%s]\033[0m: ", label, current)
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		v := strings.TrimSpace(sc.Text())
		if v != "" {
			return v
		}
	}
	return current
}

func askFieldBool(label string, current bool) bool {
	cur := "y"
	if !current {
		cur = "n"
	}
	v := askField(label+" (y/n)", cur)
	return strings.ToLower(v) == "y" || strings.ToLower(v) == "yes" || v == "1" || v == "true"
}

func askFieldChoice(label, current string, choices []string) string {
	fmt.Fprintf(ui.Out, "  \033[96m%s\033[0m \033[90m(%s)\033[0m\n", label, strings.Join(choices, " | "))
	for i, c := range choices {
		marker := "  "
		if c == current {
			marker = "\033[92m›\033[0m "
		}
		fmt.Fprintf(ui.Out, "    %s\033[90m[%d]\033[0m %s\n", marker, i+1, c)
	}
	fmt.Fprintf(ui.Out, "  \033[90mChoice [current: %s]\033[0m: ", current)
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		v := strings.TrimSpace(sc.Text())
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1]
		}
		for _, c := range choices {
			if strings.EqualFold(v, c) {
				return c
			}
		}
	}
	return current
}
