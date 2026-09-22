# Struktura projektu LegendaryOS

`legendaryos-builder` buduje **wyłącznie** projekty o poniższej,
dokładnej strukturze. `legendaryos-builder validate <ścieżka>` sprawdza
każdy z poniższych punktów i wypisuje pełną listę braków na raz (nie
przerywa na pierwszym błędzie).

```
moj-projekt/
├── branding/
│   ├── banner.png        wymagany
│   ├── logo-bg.jpg       wymagany
│   ├── logo-bg.png       wymagany
│   ├── logo.png          wymagany
│   └── notext-bg.png     wymagany
├── scripts/
│   └── *.hl               WYŁĄCZNIE pliki .hl (Hacker Lang) -- żaden inny
│                           format (.sh, .bash, .py, ...) nie jest dozwolony
├── files/                  nakładka "after packages" -- kopiowana 1:1
│                           do systemu plików obrazu (odpowiednik
│                           includes.chroot_after_packages w live-build)
├── build.hl                opis kroków budowy, w Hacker Lang
├── config.hk                metadane projektu i wydania
└── user.hk                    organizacja + token uwierzytelniający
```

## `branding/`

Pięć plików, dokładnie tymi nazwami. Każdy ma swoje jedno, ustalone
przeznaczenie w finalnym obrazie (patrz `src/branding.h#`):

| Plik              | Gdzie trafia                                             | Uwagi |
|-------------------|-----------------------------------------------------------|-------|
| `logo.png`        | `usr/share/legendaryos/branding/logo.png`                  | z przezroczystością |
| `logo-bg.png`      | panel boczny Calamares (`etc/calamares/branding/...`)        | z pełnym tłem |
| `logo-bg.jpg`      | tło menu GRUB                                                 | JPEG celowo -- GRUB źle radzi sobie z przezroczystością PNG w tle |
| `notext-bg.png`    | znak wodny Plymouth (ekran startowy)                          | bez tekstu, żeby działał na dowolnym tle |
| `banner.png`       | ekran powitalny instalatora (Calamares `show.png`)              | szeroki format, patrz przykład |

`legendaryos-builder new` tworzy te pliki jako minimalne, ale
**prawdziwe i poprawne** obrazy 1×1 px (nie puste atrapy) -- projekt od
razu przechodzi walidację; podmień je na docelową grafikę przed
właściwym buildem.

## `scripts/*.hl`

Jedyny dozwolony język hooków after-packages to **Hacker Lang**.
Żadnego shella, żadnego Pythona. Powód jest ten sam, dla którego
Hacker Lang w ogóle istnieje w HackerOS: to "alternatywa do pisania
skryptów w shellu" -- ustandaryzowana kontrola błędów (`? ok` / `? err`),
bez niejawnego `set -e`, bez cichego kontynuowania po nieudanym poleceniu.

Skrypty uruchamiane są w kolejności alfabetycznej -- konwencja
`00-`, `10-`, `20-...` w nazwach plików porządkuje kolejność wykonania
w czytelny sposób (patrz `examples/legendaryos-demo/scripts/`).

## `files/`

Zwykłe drzewo katalogów kopiowane 1:1 do obrazu, np.
`files/etc/os-release` trafia jako `/etc/os-release` w gotowym systemie.
Może być puste (musi tylko istnieć).

## `build.hl`

Skrypt Hacker Lang opisujący pełny pipeline builda tego konkretnego
projektu: `stage` → `run-scripts` → `lb-config` → `lb-build` (te cztery
etapy odpowiadają czterem niskopoziomowym komendom
`legendaryos-builder`, patrz `README.adoc`). `legendaryos-builder new`
generuje gotowy, działający `build.hl` -- zwykle nie trzeba go ręcznie
edytować, chyba że chcesz dodać własne etapy pośrednie.

## `config.hk`

```
[project]
-> name    => moj-projekt
-> tag     => stable
-> arch    => amd64

[release]
-> base     => hackeros
-> base-ref => official
-> suite    => trixie

[branding]
-> distro-name => MojProjekt
-> theme       => legendary-gold

[build]
-> mode     => live-build
-> iso-name => MojProjekt
```

`[release] -> base` / `base-ref` mówią `legendaryos-builder sync-base`,
z której gałęzi HackerOS korzystać jako bazy (patrz README.adoc, sekcja
"Relacja z HackerOS").

## `user.hk`

```
[account]
-> type => organisation
-> name => MojaOrganizacja

[auth]
-> token =>
```

Celowo **osobny plik** od `config.hk` -- to jedyne miejsce z sekretem
(token), więc jedyne, które ma sens trzymać poza kontrolą wersji.
`legendaryos-builder new` od razu dopisuje `user.hk` do
wygenerowanego `.gitignore`. Pusty token nie blokuje builda lokalnego
(tylko operacji wymagających uwierzytelnienia wobec organizacji) --
`validate` zgłasza to jako ostrzeżenie, nie błąd.
