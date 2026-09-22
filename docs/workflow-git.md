# Workflow gita i pracy z Claude Code

## Gałęzie

- `master` — zawsze działający kod, chroniony. Nikt nie pushuje bezpośrednio.
- Gałęzie robocze: `feat/12-import-csv`, `fix/34-sms-parts`, `chore/…`, `docs/…`
  (numer issue + krótki slug).
- Gałąź żyje krótko: 1–3 dni. Dłuższa = rozbij zadanie.

## Commity

Conventional Commits, zakres = pakiet/domena:

```
feat(recipients): import CSV with per-row error report
fix(delivery): do not abort batch when one sms fails
chore(ci): cache go modules
```

Treść po angielsku, tryb rozkazujący, bez kropki na końcu tematu.

## Pull request

1. `make check` i `make test` lokalnie — zielone.
2. PR do `master`, opis z szablonu (`.github/pull_request_template.md`).
3. Review od jednej z pozostałych dwóch osób; CODEOWNERS przypisuje właściciela domeny.
4. Merge: **squash**, tytuł PR w formacie Conventional Commit.
5. Po merge usuń gałąź i odśwież swoją: `git switch master && git pull --ff-only`.

## Konflikty — jak ich nie mieć

- Rebase na `master` codziennie rano: `git fetch origin && git rebase origin/master`.
- Migracje: **nigdy** nie edytuj zastosowanego pliku. Dwie osoby dodały `0007_*`?
  Ta, która mergeuje później, przenumerowuje swój plik na `0008_*`.
- `openapi.yaml`, `platform/`, `go.mod`, `package.json`, `Makefile`: uprzedź na czacie,
  zrób osobny mały PR, zmergeuj szybko.
- Nie formatuj cudzych plików „przy okazji" — formatowanie w osobnym commicie.

## Praca z Claude Code w trzy osoby

- **Jeden agent = jeden worktree.** Równoległe sesje w tym samym katalogu roboczym nadpisują
  sobie pliki. Nowe zadanie:
  ```bash
  git worktree add ../DoYouSend-feat-12 -b feat/12-import-csv
  cd ../DoYouSend-feat-12 && claude
  ```
  (w Claude Code można też użyć trybu worktree bez ręcznego polecenia).
- `.claude/settings.json` jest **wspólny i commitowany** — zmiany uprawnień idą przez PR,
  żeby wszyscy mieli ten sam zestaw. Prywatne wyjątki trzymaj w
  `.claude/settings.local.json` (w `.gitignore`).
- `CLAUDE.md` w katalogu głównym oraz w `backend/` i `frontend/` to kontrakt zespołu z agentem.
  Gdy poprawiasz Claude'a w kółko tak samo — dopisz regułę do właściwego `CLAUDE.md`
  zamiast powtarzać ją w promptach.
- Wspólne komendy: `/zadanie`, `/pr`, `/adr`, `/status-projektu` (katalog `.claude/commands/`).
- Nie wklejaj do promptów prawdziwych danych osobowych ani kluczy API.
- Kod wygenerowany przez agenta przechodzi review jak każdy inny — autor PR odpowiada
  za to, co mergeuje.
