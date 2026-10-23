# Baza Fedora (Silverblue) — LegendaryOS atomowy

`[release] -> base => fedora` (albo `--base fedora`) buduje *ten sam, klasyczny LegendaryOS*, ale na
**Fedora Silverblue** (system atomowy, ostree/bootc) zamiast na HackerOS/Debianie.

```
[release]
-> base => fedora

[fedora]
-> version     => 44                                  ;; wersja Fedory
;; -> image    => quay.io/fedora-ostree-desktops/silverblue
;; -> repos    => [https://example.org/blue-environment.repo]   ;; dodatkowe repozytoria .repo
;; -> copr     => [owner/projekt]
-> filesystem  => ext4                                ;; ext4 | xfs | btrfs
-> kernel-args => quiet splash mitigations=off

[container]
-> registry => ghcr.io/legendaryos-linux-system
-> image    => legendaryos
-> push     => false                                  ;; true = `podman push` po budowie
```

## Co robi builder

| Etap | Komenda | Efekt |
|---|---|---|
| `stage` | — | `build/lb/pc-<arch>/.staging/`: `Containerfile`, `bib-config.toml`, nakładka (branding, tożsamość, znaczniki `.live*`, `files/`) |
| `lb-config` | — | sprawdza `podman`, tworzy katalogi robocze |
| `lb-build` | `podman build` → `podman run bootc-image-builder --type anaconda-iso` | obraz OCI `ghcr.io/…/legendaryos:<tag>` + ISO instalacyjne |
| `finish` | — | `build/out/<iso-name>-<tag>-<arch>.iso` + `.SHA256SUMS` |

Wymaga **roota** i **podmana** (obraz musi trafić do magazynu roota — stamtąd czyta go bootc-image-builder).

`os-release`: `ID=legendaryos`, `ID_LIKE=fedora`, `VERSION_ID` pozostaje numerem Fedory (od niego zależy
`$releasever` w dnf/rpm-ostree); wersja LegendaryOS jest w `VERSION` i `LEGENDARYOS_VERSION`.

## Pakiety

`[packages] -> desktop`: `blue-environment` → pakiet `blue-environment` + `sdm`; `xfce`/`kde`/`cosmic` →
grupy `@xfce-desktop-environment` / `@kde-desktop-environment` / `@cosmic-desktop-environment`;
`gnome` jest już w obrazie Silverblue. `include`/`exclude` działają tak samo jak na bazie hackeros.
Pakiet `blue-environment` i `sdm` muszą być dostępne w repozytoriach obrazu — dodaj je przez
`[fedora] -> repos` / `copr`.

## Sesja live i Blue Installer — ograniczenia

* Znaczniki `.live`, `.live-legendaryos`, `.live-fedora-legendaryos` trafiają do `/etc/skel/.config/Blue-Environment/`.
  Uwaga: na bazie fedora są częścią obrazu OCI — jeśli ten sam obraz jest też *zainstalowanym* systemem,
  każdy nowy użytkownik dostanie `.live` i zobaczy Blue Installer. Dla obrazów produkcyjnych ustaw
  `[live] -> markers => false` albo zdejmij znaczniki w obrazie live.
* `bootc-image-builder` tworzy ISO *instalacyjne* (Anaconda), nie sesję live. Blue Installer (`engine.rs`)
  kopiuje system przez `rsync /` i woła `grub-install` w chroot — to model systemu zmiennego (Debian). Dla
  Silverblue potrzebna jest po stronie Blue Installer ścieżka ostree/bootc (`bootc install`).
