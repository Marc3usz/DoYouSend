# DoYouSend

System komunikacji e-mail i SMS dla szkoły. Jedna treść, jedno zatwierdzenie, dwa kanały:
ten sam komunikat trafia do rodzica/ucznia na adres e-mail **i** na numer telefonu.

Pełna specyfikacja wymagań: [`description.md`](description.md).

## Stack

- **Backend:** Go 1.24 (stdlib `net/http`, `pgx`)
- **Frontend:** SvelteKit 2 + Svelte 5 + TypeScript
- **Baza:** PostgreSQL 16 (migracje SQL w `backend/migrations/`)
- **Kolejka wysyłki:** worker wysyłki (`backend/internal/delivery`) z utrwalaniem stanu w PostgreSQL
- **Lokalnie:** Docker Compose — Postgres, Redis, Mailpit (atrapa serwera SMTP)

## Wymagania

Go 1.24+, Node 22+, Docker Desktop, `make`, `git`. Sprawdź: `make doctor`.

## Uruchomienie krok po kroku

```bash
git clone https://github.com/Marc3usz/DoYouSend.git   # albo git@github.com:… z kluczem SSH
cd DoYouSend
cp .env.example .env        # uzupełnij lokalnie, .env nie trafia do repo
make doctor                 # sprawdza narzędzia i .env
make install                # zależności frontendu (npm ci); dev-web zrobi to też sam
make up                     # Postgres + Redis + Mailpit w kontenerach (czeka na gotowość)
make migrate                # struktura bazy
make seed                   # dane demonstracyjne (wyłącznie fikcyjne)
make create-admin EMAIL=admin@example.test NAME="Administrator"   # hasło dla konta admina (zapyta o nie w terminalu)
make dev-api                # API na http://localhost:8080   (zajmuje terminal)
make dev-web                # UI na http://localhost:5173    (drugi terminal)
```

`make create-admin` czyta hasło z pierwszej linii wejścia — **wpisywane znaki są widoczne**. Zamiast
tego możesz ustawić `ADMIN_PASSWORD` w `.env`. Seed zakłada już konto `admin@example.test` bez
używalnego hasła, więc komenda ustawia mu hasło (i imię), a nie tworzy drugiego konta. Pierwsze
`make dev-api` / `make create-admin` pobiera moduły Go i trwa dłużej.

Zaloguj się na <http://localhost:5173> adresem i hasłem z `make create-admin`. Kolejne konta
(także dla wysyłających) zakłada się w **Panelu administratora → Użytkownicy**. Hasło ma co najmniej
12 znaków. Szczegóły logowania i ról: `docs/adr/0010-logowanie-sesje-role.md`.

Podgląd „wysłanych" e-maili: Mailpit na <http://localhost:8025>. **Uwaga:** przy domyślnym
`DRY_RUN=true` wysyłka kończy się na atrapie i do Mailpita nic nie trafia. Żeby zobaczyć e-maile w
Mailpicie, ustaw `DRY_RUN=false` przy `EMAIL_PROVIDER=mailpit` i `SMS_PROVIDER=fake` — to nadal nic
nie wysyła poza Twój komputer. Nie łącz `DRY_RUN=false` z prawdziwym dostawcą bez zgody opiekuna.
SMS-y przy `SMS_PROVIDER=fake` nie wychodzą na zewnątrz — lądują w logu i w bazie,
widoczne w historii wysyłek.
Konfiguracja dostawców produkcyjnych (SendGrid, SMSAPI) oraz trybów testowych: zobacz [`docs/integracje.md`](docs/integracje.md).

## Gdy coś nie działa

- **`failed SASL auth` / `28P01` przy `make create-admin` lub `make dev-api`** — port 5432 zajmuje
  inny Postgres (np. usługa Windows). `make up` tego nie zgłosi, bo `migrate` i `seed` działają
  wewnątrz kontenera. Zatrzymaj tamtą usługę albo przemapuj port lokalnym
  `docker-compose.override.yml` (nie commituj go):

  ```yaml
  services:
    db:
      ports: !override ["55432:5432"]
  ```

  i zmień port w `DATABASE_URL` / `TEST_DATABASE_URL` w `.env`. To samo dotyczy portów 6379
  (Redis), 1025/8025 (Mailpit), 8080 (API) i 5173 (UI).
- **`'vite' is not recognized` / `vite: command not found`** — brak `frontend/node_modules`; uruchom
  `make install`.

## Bezpieczniki

- `DRY_RUN=true` w `.env` — domyślnie nic nie wychodzi poza maszynę dewelopera.
- Providerzy `fake`/Mailpit to domyślna konfiguracja. Podłączenie prawdziwej bramki SMS
  i prawdziwych odbiorców wymaga zgody opiekuna projektu.
- W repo nie ma i nie może być prawdziwych danych osobowych ani kluczy API.

## Praca zespołowa (2 osoby)

| Dokument | Zawartość |
|---|---|
| [`docs/podzial-pracy.md`](docs/podzial-pracy.md) | kto odpowiada za którą domenę, kolejność prac |
| [`docs/workflow-git.md`](docs/workflow-git.md) | branche, PR, review, rozwiązywanie konfliktów |
| [`docs/architektura.md`](docs/architektura.md) | warstwy, przepływ wysyłki, statusy |
| [`docs/model-danych.md`](docs/model-danych.md) | encje i relacje |
| [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md) | sekrety, dane testowe, uprawnienia |
| [`docs/integracje.md`](docs/integracje.md) | konfiguracja i działanie e-mail (Mailpit/SendGrid) oraz SMS (Fake/SMSAPI) |
| [`docs/adr/`](docs/adr/) | decyzje architektoniczne (ADR) |
| [`CLAUDE.md`](CLAUDE.md) | instrukcje dla Claude Code — wspólne dla całego zespołu |

## Przydatne komendy

```bash
make check     # lintery i typy (to samo co CI)
make test      # testy backendu i frontendu
make test-integration  # testy na Postgresie (TEST_DATABASE_URL, patrz .env.example)
make fmt       # formatowanie
make down      # zatrzymanie kontenerów
```
