# DoYouSend

System komunikacji e-mail i SMS dla szkoły. Jedna treść, jedno zatwierdzenie, dwa kanały:
ten sam komunikat trafia do rodzica/ucznia na adres e-mail **i** na numer telefonu.

Pełna specyfikacja wymagań: [`description.md`](description.md).

## Stack

- **Backend:** Go 1.24 (`net/http` + chi, pgx)
- **Frontend:** SvelteKit 2 + Svelte 5 + TypeScript
- **Baza:** PostgreSQL 16 (migracje SQL w `backend/migrations/`)
- **Kolejka wysyłki:** worker oparty o Redis
- **Lokalnie:** Docker Compose — Postgres, Redis, Mailpit (atrapa serwera SMTP)

## Wymagania

Go 1.24+, Node 22+, Docker Desktop, `make`, `git`. Sprawdź: `make doctor`.

## Uruchomienie krok po kroku

```bash
git clone git@github.com:Marc3usz/DoYouSend.git
cd DoYouSend
cp .env.example .env        # uzupełnij lokalnie, .env nie trafia do repo
make up                     # Postgres + Redis + Mailpit w kontenerach
make migrate                # struktura bazy
make seed                   # dane demonstracyjne (wyłącznie fikcyjne)
make dev-api                # API na http://localhost:8080
make dev-web                # UI na http://localhost:5173   (drugi terminal)
```

Podgląd „wysłanych" e-maili: Mailpit na <http://localhost:8025>.
SMS-y przy `SMS_PROVIDER=fake` nie wychodzą na zewnątrz — lądują w logu i w bazie,
widoczne w historii wysyłek.

## Bezpieczniki

- `DRY_RUN=true` w `.env` — domyślnie nic nie wychodzi poza maszynę dewelopera.
- Providerzy `fake`/Mailpit to domyślna konfiguracja. Podłączenie prawdziwej bramki SMS
  i prawdziwych odbiorców wymaga zgody opiekuna projektu.
- W repo nie ma i nie może być prawdziwych danych osobowych ani kluczy API.

## Praca zespołowa (3 osoby)

| Dokument | Zawartość |
|---|---|
| [`docs/podzial-pracy.md`](docs/podzial-pracy.md) | kto odpowiada za którą domenę, kolejność prac |
| [`docs/workflow-git.md`](docs/workflow-git.md) | branche, PR, review, rozwiązywanie konfliktów |
| [`docs/architektura.md`](docs/architektura.md) | warstwy, przepływ wysyłki, statusy |
| [`docs/model-danych.md`](docs/model-danych.md) | encje i relacje |
| [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md) | sekrety, dane testowe, uprawnienia |
| [`docs/adr/`](docs/adr/) | decyzje architektoniczne (ADR) |
| [`CLAUDE.md`](CLAUDE.md) | instrukcje dla Claude Code — wspólne dla całego zespołu |

## Przydatne komendy

```bash
make check     # lintery i typy (to samo co CI)
make test      # testy backendu i frontendu
make fmt       # formatowanie
make down      # zatrzymanie kontenerów
```
