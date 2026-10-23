# Cele budowy: komputery i telefony

Jeden projekt może budować wiele celów naraz. Cel = `(rodzaj, architektura, urządzenie)`:

```
[project]
-> targets => [pc:amd64, pc:arm64, phone:arm64:pinephone]
```

| Zapis | Wynik |
|---|---|
| `pc:amd64` | ISO hybrydowa (BIOS + UEFI), instalator Calamares |
| `pc:arm64` | ISO UEFI (GRUB-EFI), np. VM-y, serwery i laptopy ARM |
| `phone:arm64:<id>` | `rootfs.tar.xz` + obraz zależny od urządzenia (poniżej) |

Wstecznie działa też stary zapis: `-> arch => [amd64, arm64]` + `[target] -> kind` + `[mobile] -> device`.
Gdy jest `targets`, tamte klucze są ignorowane.

Każdy cel ma własny katalog roboczy `build/lb/<kind>-<arch>[-<urządzenie>]/`,
a artefakty trafiają do `build/out/<iso-name>-<tag>-<arch>[-<urządzenie>].*` wraz z `.SHA256SUMS`.

## Cross-build

Gdy architektura celu ≠ architektura hosta, builder dodaje do `lb config`
`--bootstrap-qemu-arch` i `--bootstrap-qemu-static` (np. `/usr/bin/qemu-aarch64-static`).
Wymaga to pakietów `qemu-user-static` i `binfmt-support`; `doctor` to sprawdza.
`amd64` buduje `i386` natywnie, `arm64` buduje `armhf` natywnie. Budowa przez qemu jest wielokrotnie wolniejsza.

## Telefony

Telefon to system *zainstalowany*, nie live: `lb config --system normal --binary-images tar`.
Rootfs bierzemy z katalogu `chroot/` live-build, a obraz składamy bez loop-device i bez `mount`:

1. `extlinux.conf` trafia do `/boot/extlinux/` rootfs-u (U-Boot distro boot),
2. `mke2fs -d <chroot>` buduje system plików ext4 (etykieta `LEGENDARY_ROOT`),
3. `truncate` + `parted` tworzą obraz z jedną partycją od 16 MiB,
4. `dd` wpisuje ext4 pod offset 16 MiB oraz bloby U-Boota na sektory z profilu urządzenia,
5. `xz` kompresuje obraz.

Tablica partycji: **MBR**, gdy któryś blob leży poniżej sektora 34 (kolizja z GPT — tak jest u Allwinnera/PinePhone), w przeciwnym razie GPT.

### Profile urządzeń (`src/devices.h#`)

| id | SoC | boot | bloby U-Boota |
|---|---|---|---|
| `pinephone` | Allwinner A64 | u-boot | `u-boot-sunxi-with-spl.bin` @ 16 |
| `pinephone-pro` | Rockchip RK3399S | u-boot | `idbloader.img` @ 64, `u-boot.itb` @ 16384 |
| `librem5` | NXP i.MX8M | u-boot | `flash.bin` @ 66 |
| `android-arm64` | (dowolny) | android | `boot.img` (mkbootimg) + `rootfs.img` — eksperymentalne |
| `generic-arm64` | SBSA / VM | uefi | ISO jak dla `pc:arm64` |

Dodanie urządzenia = dopisanie wpisu `Device { ... }` w `devices::all()`.
Ścieżki blobów są ścieżkami **w rootfs** (pliki z pakietów `u-boot-*`).

### Środowisko telefonu

`[mobile] -> shell => phosh` instaluje Phosh (phoc, squeekboard, feedbackd, gnome-calls, chatty…).
`blue-environment` — Blue Environment w wariancie telefonicznym (pakiet `blue-environment-phones`,
UI `phones`) + display manager `sdm` (`[packages] -> display-manager` nadpisuje).
`none` — bez środowiska graficznego (własne pakiety przez `[packages] -> include`).

Telefony buduje wyłącznie baza `hackeros`; baza `fedora` (Silverblue) buduje tylko komputery.

### Ograniczenia — przeczytaj przed wgraniem na sprzęt

* Kod składający obrazy telefonów **sprawdzono na atrapie rootfs-u** (układ partycji, offset i zawartość bloba, extlinux.conf), ale **nie na fizycznych urządzeniach**.
* Offsety blobów i ścieżki w pakietach `u-boot-*` pochodzą z dokumentacji upstreamów — zweryfikuj je dla swojej wersji Debiana (szczególnie `librem5`).
* Jądra z Debiana nie zawsze obsługują całość sprzętu telefonu (modem, aparat, GPU). Obrazy to baza do dalszej pracy, nie gotowy produkt.
* `android-arm64`: jądro, dtb i `--cmdline` trzeba dopasować do modelu.
