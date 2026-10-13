# Testy

Testy są wbudowane w narzędzie: `legendaryos-builder selftest` (kod
`src/selftest.h#`). Sprawdzają czyste funkcje — parser `.hk` (z pozycjami),
walidację, rozwiązywanie celów PC/telefon, listy pakietów, argumenty
`lb config`, tożsamość systemu, branding, układ obrazów telefonu,
dopełnianie powłok — bez sieci, live-build i roota.

```sh
bit run -- selftest        # albo: ./cache/build/release/legendaryos-builder selftest
```

CI uruchamia je po każdym buildzie (`.github/workflows/build.yml`).
