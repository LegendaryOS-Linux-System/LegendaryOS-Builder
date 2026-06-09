package builder

import (
	"fmt"
	"strings"

	"github.com/legendaryos/builder/src/config"
)

// bootloaderPostScript returns %post shell commands for alternative bootloaders.
func bootloaderPostScript(cfg *config.Config) string {
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

	bl := cfg.Bootloader.Type
	comment("╔══════════════════════════════════════════════════════════════╗")
	comment(fmt.Sprintf("║  Bootloader post-install: %-34s║", bl))
	comment("╚══════════════════════════════════════════════════════════════╝")
	blank()

	efi := cfg.Bootloader.EFIDir
	if efi == "" {
		efi = "/boot/efi"
	}

	switch strings.ToLower(bl) {
	case config.BootloaderRefind:
		comment("── rEFInd install ────────────────────────────────────────────────")
		line("refind-install --usedefault %s 2>/dev/null || true", efi)
		blank()

	case config.BootloaderSystemd:
		comment("── systemd-boot install ──────────────────────────────────────────")
		line("bootctl install --path=%s 2>/dev/null || true", efi)
		line("systemctl enable systemd-boot-update.service 2>/dev/null || true")
		blank()

	case config.BootloaderLimine:
		comment("── Limine install ────────────────────────────────────────────────")
		line("limine-install /dev/sda 2>/dev/null || true")
		blank()

	case config.BootloaderSyslinux:
		comment("── SYSLINUX install ──────────────────────────────────────────────")
		line("syslinux --install /dev/sda1 2>/dev/null || true")
		blank()
	}

	return sb.String()
}

// flatpakPostScript returns %post shell commands that install/remove Flatpaks
// on the live installed system via Anaconda's %post chroot environment.
// flatpak.packages and flatpak.remove.packages are applied here — NOT during
// the container/OCI image build.
func flatpakPostScript(install, remove []string) string {
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
	comment("║  Flatpak — applied at install time (packages/flatpak.*)      ║")
	comment("╚══════════════════════════════════════════════════════════════╝")
	blank()

	comment("── Ensure Flatpak + Flathub remote ──────────────────────────────")
	line("dnf install -y flatpak 2>/dev/null || true")
	line("flatpak remote-add --if-not-exists --system flathub https://dl.flathub.org/repo/flathub.flatpakrepo 2>/dev/null || true")
	blank()

	if len(install) > 0 {
		comment("── Install Flatpak apps (packages/flatpak.packages) ─────────────")
		line("flatpak install --system --noninteractive flathub \\")
		for i, app := range install {
			if i < len(install)-1 {
				line("    %s \\", app)
			} else {
				line("    %s", app)
			}
		}
		blank()
	}

	if len(remove) > 0 {
		comment("── Remove Flatpak apps (packages/flatpak.remove.packages) ───────")
		line("flatpak uninstall --system --noninteractive \\")
		for i, app := range remove {
			if i < len(remove)-1 {
				line("    %s \\", app)
			} else {
				line("    %s", app)
			}
		}
		blank()
	}

	return sb.String()
}
