# Struktura projektu LegendaryOS

`legendaryos-builder` buduje **wyłącznie** projekty o poniższej strukturze.
`legendaryos-builder validate <ścieżka>` sprawdza każdy punkt i wypisuje pełną
listę problemów na raz (nie przerywa na pierwszym błędzie). Problemy w
`config.hk`/`user.hk` są pokazywane przez **nidus** — z fragmentem pliku i
podświetlonym miejscem błędu.

```
moj-projekt/
├── branding/
│   ├── banner.png        wymagany
│   ├── logo-bg.jpg       wymagany
│   ├── logo-bg.png       wymagany
│   ├── logo.png          wymagany
│   └── notext-bg.png     wymagany
├── scripts/
│   └── *.hl               WYŁĄCZNIE pliki .hl (Hacker Lang)
├── files/                  nakładka "after packages", kopiowana 1:1 do obrazu
├── package-lists/          (opcjonalnie) własne *.list.chroot / *.list.binary
├── build.hl                opis kroków budowy, w Hacker Lang
├── config.hk               metadane projektu i wydania
└── user.hk                 organizacja + token uwierzytelniający
```

## `branding/`

Pięć plików, dokładnie tymi nazwami. Walidacja sprawdza nagłówek (PNG:
sygnatura, JPEG: `ffd8ff`) i ostrzega przy grafikach mniejszych niż 64 px
(placeholdery).

| Plik | Gdzie trafia (komputer) | Uwagi |
|---|---|---|
| `logo.png` | `usr/share/legendaryos/branding/` | z przezroczystością |
| `logo-bg.png` | panel boczny Calamares; splash isolinux/grub-pc | z pełnym tłem |
| `logo-bg.jpg` | tło menu GRUB (`/etc/default/grub.d/legendaryos.cfg`) | JPEG — GRUB źle radzi sobie z przezroczystością PNG |
| `notext-bg.png` | znak wodny Plymouth | bez tekstu |
| `banner.png` | ekran powitalny Calamares (`show.png`, slideshow) | szeroki format |

Dla telefonów: Plymouth + surowe kopie w `usr/share/legendaryos/branding/`
(Calamares i GRUB nie są instalowane).

Builder **generuje** też pliki aktywujące branding: `branding.desc` i
`show.qml` (Calamares), `legendaryos.plymouth` + `.script` (kolor tła z
`[branding] -> background`), konfigurację GRUB oraz hook chroot
`hooks/live/0900-legendaryos-branding.hook.chroot` (natywny format
live-build; jedyny plik z shellem i jest generowany — skrypty projektu
nadal muszą być `.hl`).

## `scripts/*.hl`

Jedyny dozwolony język hooków to **Hacker Lang**. Skrypty uruchamiane są
**przed** złożeniem nakładki, w kolejności alfabetycznej (`00-`, `10-`,
`20-`…), z korzeniem projektu jako katalogiem roboczym — więc to, co
skrypt zapisze w `files/`, trafi do obrazu. Dostają zmienne
`LEGENDARYOS_ROOT` i `LEGENDARYOS_BUILD`. Sukces = kod wyjścia `hl` 0;
`--keep-going` nie przerywa po pierwszym błędzie.

## `files/`

Zwykłe drzewo katalogów kopiowane 1:1 do obrazu (`files/etc/motd` →
`/etc/motd`). Kopiowane **ostatnie**, więc nadpisuje wszystko, co
wygenerował builder (os-release, motd…) i zaimportowało z bazy.

## `package-lists/`

Opcjonalne własne listy live-build. Trafiają obok wygenerowanej
`legendaryos-base.list.chroot` i list zaimportowanych z bazy HackerOS.

## `config.hk`

```
[project]
-> name        => moj-projekt
-> tag         => stable
-> version     => 1.0
-> targets     => [pc:amd64, pc:arm64, phone:arm64:pinephone]

[release]
-> base        => hackeros          ;; hackeros | fedora
-> base-ref    => official          ;; gałąź/tag bazy HackerOS (tylko hackeros)
-> base-url    => https://github.com/HackerOS-Linux-System/HackerOS.git
-> suite       => trixie

[branding]
-> distro-name => MojProjekt
-> accent      => #6236B0
-> background  => #120C1E
-> home-url    => https://example.org
-> support-url => https://example.org/support
-> bug-url     => https://example.org/bugs

[build]
-> mode         => live-build
-> iso-name     => MojProjekt       ;; bez spacji i '/'
-> volume-label => MojProjekt       ;; max 32 znaki
-> publisher    => Moja Organizacja
-> bootappend   => boot=live components quiet splash
-> firmware     => true

[packages]
-> desktop    => xfce               ;; none | xfce | gnome | kde | cosmic (tylko fedora) | blue-environment
-> display-manager => auto          ;; auto (blue-environment => sdm) | none | sdm | gdm | sddm | lightdm
-> include    => [vim, htop]
-> exclude    => [os-prober]
-> recommends => true

[system]
-> motd              => Witaj!\nDruga linia
-> hostname          => legendary
-> generate-identity => true         ;; os-release, lsb-release, issue...
-> locale            => pl_PL.UTF-8
-> timezone          => Europe/Warsaw
-> keyboard          => pl
-> services-enable   => [sshd, NetworkManager]
-> services-disable  => [ModemManager]

[base]
-> import => [package-lists, hooks, includes]
-> helper => desktop                 ;; zawęża do helpers/desktop/ w repo bazy

[live]                               ;; znaczniki sesji live dla Blue Installer
-> markers     => auto               ;; auto | true | false

[fedora]                             ;; tylko base => fedora (patrz FEDORA.md)
-> version     => 44
-> filesystem  => ext4

[container]                          ;; tylko base => fedora
-> registry    => ghcr.io/org
-> image       => legendaryos
-> push        => false

[mobile]                             ;; tylko dla celów phone:*
-> shell       => phosh              ;; phosh | blue-environment | none
-> rootfs-size => 4096               ;; MiB (domyślnie liczone automatycznie)
-> cmdline     => quiet
-> root-label  => LEGENDARY_ROOT
```

Starszy zapis (`-> arch => [amd64, arm64]`, `[target] -> kind`, `[mobile] -> device`)
nadal działa, gdy nie ma `targets` — patrz [TARGETS.md](TARGETS.md).

Składnia `.hk`: `-> klucz => wartość`, listy `[a, b]`, komentarze `;;`, `#`
albo `!`, pod-klucze `-->`. `;;` w cudzysłowie nie jest komentarzem.

## `user.hk`

```
[account]
-> type => organisation
-> name => MojaOrganizacja

[auth]
-> token =>
```

Osobny plik, bo to jedyne miejsce z sekretem. `new` dopisuje go do
`.gitignore`. Pusty token to ostrzeżenie (nie błąd); alternatywa:
zmienna `LEGENDARYOS_TOKEN`. Token trafia do `git` wyłącznie przez
zmienne środowiskowe.

## Co powstaje w `build/`

```
build/
├── lb/<kind>-<arch>[-<urządzenie>]/   katalog roboczy live-build (config/, chroot/, build.log)
└── out/                                 gotowe artefakty
    ├── <iso-name>-<tag>-<arch>.iso
    ├── <iso-name>-<tag>-<arch>-<urządzenie>.img.xz
    ├── <iso-name>-<tag>-<arch>-<urządzenie>.rootfs.tar.xz
    ├── *.SHA256SUMS
    └── <iso-name>-<tag>-RELEASE-NOTES.md
```
