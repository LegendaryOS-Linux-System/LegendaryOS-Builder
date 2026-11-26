# Budowanie obrazu — wymagania i znane ryzyka

Host: Fedora 43 (zalecane; kontener `fedora:43` z `--privileged`).
Narzędzia hosta: `git curl gawk bsdtar zstd rpm squashfs-tools grub2-tools-extra xorriso mtools podman dracut` oraz
**`lepm`** (z `../lepm`, `bit build`). **Nie potrzeba** `dnf5` ani programu `fasttree`:

* pakiety do rootfs składa **lepm** (rpm-md → satsolv → pobranie → rozpakowanie; biblioteka `lrpm`) — zob. `../lepm/README.md`;
* FastTree (snapshot, układ, manifest, `/tools`) jest wołany **jako biblioteka** (`use "bit -> libfasttree" from "ft"`,
  fasada `ft::api::*`) — zob. `../libfasttree/docs/USING-AS-LIBRARY.md`;
* opcjonalnie `z#` (kompilacja zenitd; albo `ZENITD_BIN`) i `hl` (hooki `.hl`); `LPM_BIN` = binarka lpm do obrazu.

## Dlaczego lepm, a nie dnf ani lpm
Przy budowie nie ma jeszcze systemu, więc nie ma czym instalować pakietów atomowo (Fedora Silverblue używa tu dnf w buildzie
bootc). LegendaryOS ma własne narzędzie **tylko do budowy obrazów** (`lepm`, nula + Z# + H#). W gotowym systemie pakiety obsługuje
**lpm** (niezmienny, FastTree) — też bez dnf. Oba korzystają z wspólnej biblioteki **lrpm** (satsolv), tej samej polityki
`compat.hk` i tych samych repozytoriów `repos.hk`. Baza zainstalowanych pakietów (`installed.xml`) powstaje w lepm i jedzie w obrazie
(`/system/share/legendaryos/installed.xml`), więc `lpm install` liczy zależności względem prawdziwej bazy.

## Znane ryzyka (nie rozwiązane w v0.2)
1. **systemd w Fedorze.** Wiele pakietów ma twarde `Requires: systemd` (udev, dbus, polkit, NetworkManager, sddm przez logind).
   **Rozwiązanie po stronie menedżera pakietów (lepm/lpm), nie dnf:** `compat.hk` — atrapa `Provides: systemd = <wersja repo>`
   spełnia wymaganie bez instalowania pakietu `systemd`, a `[exclude]` blokuje jego instalację (PID 1 = zenitd).
   `systemd-udev` działa jako samodzielny demon pod zenitd; `systemd-libs` zostaje (biblioteka, nie init).
   **logind**: SDDM/Wayland/Blue Environment potrzebują sesji logind lub `elogind`; Fedora nie ma `elogind`.
   Plan: **elogind z osobnego Copr LegendaryOS** (`copr-legendaryos` w `repos.hk`, domyślnie wyłączony), a jeśli Copr nie
   wystarczy — **budowa ze źródeł** (jako narzędzie w `/tools` albo własny RPM). To jest ryzyko #1 całego projektu.
2. **install.sh zenitd** używa `HERE=<skrypt>/..` — sam pobrany skrypt (bez repo) nie zadziała; builder klonuje repo i podmienia
   `scripts/install.sh` pobraną wersją.
3. **Kernel + dracut + composefs**: initramfs musi zamontować `/system` (composefs) i `/state`; moduł dracut `legendary-root`
   jest do napisania (ROADMAP M3 w głównym planie).
4. **ISO live** używa `dmsquash-live`; layout non-FHS wymaga `init=/sbin/init` (symlink do zenitd — jest w install.sh).
5. **Blue Environment** (Tauri/Svelte) dostarczany jako narzędzie w `/tools` albo RPM — w szablonie jest tylko plik sesji;
   budowa BE nie jest częścią buildera.
6. **Skryptlety RPM nie są wykonywane** (ani przez lepm, ani przez lpm). Skutki odtwarzają: `sysusers.d` (wbudowana,
   minimalna implementacja), hooki z `compat.hk` (`ldconfig`, `update-ca-trust`, `fc-cache`) oraz `scripts/*.sh`.
   Luki: `tmpfiles.d`, `alternatives`, `authselect`, triggery. Pierwszy obraz trzeba sprawdzić pod kątem brakujących kont/katalogów.
7. **Kod niezweryfikowany kompilatorem**: sesja, w której powstał lepm/lrpm i przepisany builder/lpm, nie miała `h#`, `bit`,
   libsolv ani Fedory. Przed pierwszą budową: `bit build` w `lrpm`, `lepm`, `lpm`, `builder`; `lrpm/scripts/run-tests.sh`.

## Przepływ
```
validate → prepare → packages(lepm) → zenitd → lpm → overlay → hooks → layout → identity → tools(ft::api) → fasttree(ft::api) → iso|oci → checksums
```
Podgląd bez wykonywania: `legendaryos-builder plan ../LegendaryOS` (pokazuje m.in. pełne polecenie `lepm bootstrap`).
