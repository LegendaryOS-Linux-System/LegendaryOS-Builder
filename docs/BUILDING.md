# Budowanie obrazu — wymagania i znane ryzyka

Host: Fedora 43 (zalecane; kontener `fedora:43` z `--privileged`). Pakiety: `dnf5 git curl squashfs-tools grub2-tools-extra xorriso mtools podman dracut`, `fasttree` (z ../libfasttree), opcjonalnie `z#` i `hl`.

## Znane ryzyka (nie rozwiązane w v0.1.0)
1. **systemd w Fedorze.** Wiele pakietów ma twarde `Requires: systemd` (udev, dbus, polkit, NetworkManager, sddm przez logind). Obraz będzie zawierał pliki systemd, ale PID 1 = zenitd. `systemd-udev` działa jako samodzielny demon pod zenitd. **logind**: SDDM/Wayland/Blue Environment potrzebują sesji logind lub `elogind`; Fedora nie ma `elogind` — wymaga Copr LegendaryOS albo `seatd` + patch SDDM. To jest ryzyko #1 całego projektu. Rozwiazaniem jest elogind ze specialnego copr lub jezeli nie bedzie ze zrodel natomaist problem w tym ze paczki wymagaja systemd jako pakietu ten problem bedzie rozwiazywal lpm
2. **install.sh zenitd** używa `HERE=<skrypt>/..` — sam pobrany skrypt (bez repo) nie zadziała; builder klonuje repo i podmienia `scripts/install.sh` pobraną wersją.
3. **Kernel + dracut + composefs**: initramfs musi zamontować `/system` (composefs) i `/state`; moduł dracut `legendary-root` jest do napisania (ROADMAP M3 w głównym planie).
4. **ISO live** używa `dmsquash-live`; layout non-FHS wymaga `init=/sbin/init` (symlink do zenitd — jest w install.sh).
5. **Blue Environment** (Tauri/Svelte) dostarczany jako narzędzie w `/tools` albo RPM — w szablonie jest tylko plik sesji; budowa BE nie jest częścią buildera.
