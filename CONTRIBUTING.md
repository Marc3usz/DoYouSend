# Jak pracujemy

Skrót dla trzyosobowego zespołu. Szczegóły w `docs/`.

1. **Zanim zaczniesz:** `make doctor`, potem `cp .env.example .env`, `make up`, `make migrate`.
   Pełna instrukcja w [README](README.md).
2. **Bierzesz zadanie:** issue z szablonu → gałąź `feat/<issue>-<slug>` → praca →
   `make check && make test`.
3. **Zgłaszasz PR:** szablon wypełniony, jedno review od innej osoby, merge przez squash.
4. **Twoja domena:** [`docs/podzial-pracy.md`](docs/podzial-pracy.md). Możesz wejść w cudzy
   kod, ale oznacz właściciela w PR.
5. **Pliki wspólne** (`backend/internal/platform/**`, `backend/migrations/**`,
   `docs/api/openapi.yaml`, `go.mod`, `frontend/package.json`, `Makefile`, `.github/**`) —
   uprzedź zespół, zrób mały osobny PR.
6. **Decyzja architektoniczna** wykraczająca poza twoją domenę → ADR w
   [`docs/adr/`](docs/adr/), nie ciche założenie w kodzie.
7. **Nigdy:** sekret w repo, prawdziwe dane osobowe w testach lub promptach,
   wysyłka na prawdziwe numery bez zgody opiekuna projektu.
   Zob. [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md).
8. **Claude Code:** wspólne ustawienia w `.claude/`, zasady w `CLAUDE.md`
   (korzeń + `backend/` + `frontend/`). Jedna sesja agenta = jeden worktree.
   Za kod wygenerowany przez agenta odpowiada autor PR-a.
