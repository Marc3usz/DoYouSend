# Jak pracujemy

Skrót zasad. Szczegóły w `docs/`, a pełna instrukcja uruchomienia w [README](README.md).

1. **Zanim zaczniesz:** `cp .env.example .env`, `make doctor`, `make install`, `make up`,
   `make migrate`, `make seed`, `make create-admin EMAIL=admin@example.test NAME="Administrator"`,
   potem `make dev-api` i `make dev-web`. Na Windowsie wszystko w Git Bash.
2. **Bierzesz zadanie:** issue z szablonu → gałąź `feat/<issue>-<slug>` → praca →
   `make check && make test` (a przy zmianach SQL także `make test-integration`).
3. **Zgłaszasz PR:** szablon wypełniony, zielone CI, przegląd zmian subagentem `recenzent`,
   merge przez squash. Projekt prowadzi teraz jedna osoba (ADR-0020); gdy dołączy druga,
   wraca zasada jednego zatwierdzenia od innej osoby.
4. **Domeny:** [`docs/podzial-pracy.md`](docs/podzial-pracy.md). Podział A–D zostaje, żeby
   zmiany i zależności między pakietami dalej szły przez interfejsy.
5. **Pliki wspólne** (`backend/internal/platform/**`, `backend/migrations/**`,
   `docs/api/openapi.yaml`, `go.mod`, `frontend/package.json`, `Makefile`, `.github/**`) —
   osobny, mały PR; zmiana kontraktu API przed implementacją.
6. **Decyzja architektoniczna** wykraczająca poza jedną domenę → ADR w
   [`docs/adr/`](docs/adr/), nie ciche założenie w kodzie.
7. **Nigdy:** sekret w repo, prawdziwe dane osobowe w testach lub promptach,
   wysyłka na prawdziwe numery bez zgody opiekuna projektu.
   Zob. [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md).
8. **Claude Code:** wspólne ustawienia w `.claude/`, zasady w `CLAUDE.md`
   (korzeń + `backend/` + `frontend/`). Jedna sesja agenta = jeden worktree.
   Za kod wygenerowany przez agenta odpowiada autor PR-a.
