---
description: Rozpoczyna pracę nad zadaniem — gałąź, kontekst domeny, plan
argument-hint: <numer issue lub opis zadania>
---

Zadanie: $ARGUMENTS

Wykonaj po kolei:

1. Jeśli podano numer issue — pobierz jego treść (`gh issue view <numer>`).
2. Ustal, do której domeny należy zadanie (`docs/podzial-pracy.md`) i przeczytaj
   `CLAUDE.md` z odpowiedniego katalogu (`backend/` lub `frontend/`) oraz komentarz
   pakietowy właściwej domeny.
3. Sprawdź, czy zadanie dotyka plików wspólnych (`backend/internal/platform/**`,
   `backend/migrations/**`, `docs/api/openapi.yaml`, `go.mod`, `frontend/package.json`,
   `Makefile`, `.github/**`). Jeśli tak — powiedz o tym wyraźnie na początku odpowiedzi,
   bo wymaga to uprzedzenia zespołu.
4. Załóż gałąź: `git switch -c feat/<numer>-<slug>` (albo `fix/`, `chore/`, `docs/`).
5. Przedstaw krótki plan: jakie pliki, jakie testy, czy potrzebna migracja, czy zmienia się
   kontrakt API. Poczekaj na potwierdzenie, zanim zaczniesz pisać kod.

Odpowiadaj po polsku. Kod, nazwy i commity po angielsku.
