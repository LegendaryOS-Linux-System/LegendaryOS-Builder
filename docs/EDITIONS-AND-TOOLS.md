# Edycje i /tools (builder 0.3.0)

## Edycje
`LegendaryOS/editions/<id>/edition.hk` (+ `package-list/<id>.list`, `files/`, `scripts/`).
Edycje: plasma (domyślna), xfce, gnome, cosmic, blue, server. Wspólne listy: base, hardware, legendary, graphical (nie dla server).

    builder editions                         # lista
    builder validate [--edition all]         # walidacja projektu, edycji i katalogu narzędzi
    builder plan  --edition plasma,xfce      # co zostanie wykonane (bez zmian)
    builder build --edition all [--allow-missing] [--offline] [--keep-going] [--cache DIR]
    builder tools / fetch-tools              # katalog narzędzi / samo pobranie z walidacją

Wyniki: `build/<id>/out/` (ISO `legendaryos-<id>-<ver>-<arch>.iso`, OCI, `tools.lock.hk`, `build-info.hk`, `build.log`).
Cache pobrań jest wspólny: `build/cache/tools/<id>/<tag>/<asset>`.

## /tools = jeden obraz squashfs
`tools.hk` opisuje narzędzia (pola m.in. `repo`, `asset`, `as`, `version`, `kind`, `editions`, `system`, `required`).
Wersja: `manifest:<Klucz>` (HackerOS manifest.hk), `bithk` (Bit.hk repo), `latest`, `pin:<v>`.
Pobrane binarki są walidowane (ELF/shebang, rozmiar, opcjonalny sha256), pakowane do `tools.sqfs`
(zstd 19) z `bin/`, `share/applications/*.desktop` i `TOOLS.hk`. Obraz leży w `/tools/tools.sqfs`
i jest montowany loop (ro) NA `/tools`; aktualizacja `lpm tools update` zapisuje `/state/tools/tools.sqfs`
(pierwszeństwo). Narzędzia `system=true` (lpm, zenitd) są dodatkowo w `/system/bin`.
Wykluczenia per edycja: `[tools] exclude` w `edition.hk`.

Zmienne środowiskowe (lokalne binarki zamiast GitHuba): `LEGENDARY_TOOLS_LOCAL=<dir z plikami wg id/as>`,
`LPM_BIN`, `ZENITD_BIN`, `LEGENDARY_MANIFEST_HK`.

## Znane ograniczenia / do weryfikacji
- Część repozytoriów LegendaryOS nie ma jeszcze wydań → `--allow-missing` lub `LEGENDARY_TOOLS_LOCAL`.
- `Z-Sharp/scripts/LegendaryOS/install.hlpp` jest pusty — krok zsharp tylko ostrzega.
- Nazwy pakietów Fedory w listach edycji, jednostki (cosmic-greeter, elogind, sshd) i kontrakt install.hlpp
  nie były weryfikowane (brak dostępu do mirrorów) — `builder check-packages` na maszynie z siecią.
- Blue dostaje instalator GUI (decyzja własna); zenitd jest „required: false” (fallback do wersji z rootfs).
- Montowanie loop /tools nie było testowane w sandboxie (tylko pakowanie i weryfikacja obrazu).
