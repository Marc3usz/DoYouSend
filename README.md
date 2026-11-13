# DoYouSend

An e-mail and SMS messaging system for a school. One message, one approval, two channels:
the same notice reaches a parent or student at their e-mail address **and** their phone number.

Full requirements spec (in Polish): [`description.md`](description.md).

## Stack

- **Backend:** Go 1.24 (stdlib `net/http`, `pgx`)
- **Frontend:** SvelteKit 2 + Svelte 5 + TypeScript
- **Database:** PostgreSQL 16 (SQL migrations in `backend/migrations/`)
- **Delivery:** delivery worker inside the API process (`backend/internal/delivery`); every
  per-recipient, per-channel delivery is a row in PostgreSQL, so there is no separate queue service
- **Local environment:** Docker Compose — Postgres and Mailpit (fake SMTP server)

## Features

- **One message, two channels** — the same body goes out by e-mail and SMS. The composer lets the
  sender pick both, e-mail only or SMS only (ADR-0019), previews the rendered bodies (personalised
  per recipient, with the SMS part count and price) and asks for confirmation before sending.
- **Recipients and groups** — CRUD, CSV/XLSX import with an error report, and import straight from a
  Notion database ([`docs/integracje.md`](docs/integracje.md), ADR-0017). Built-in groups (all
  parents, all students, students and parents of each class) and custom groups. A data-quality
  panel lists incomplete or invalid contacts; senders can fix a missing contact directly in the
  composer (ADR-0013). Recipients from overlapping groups are deduplicated.
- **Delivery that survives failures** — per-recipient, per-channel status; a failed recipient never
  aborts the batch. Transient errors are retried automatically with backoff; any failed delivery
  can be re-sent from the history. A batch interrupted by an API restart is resumed on start-up.
- **History** — browse past batches with per-person statuses, errors, retries and excluded
  recipients. A sent batch never changes; "use as a basis" pre-fills a new message in the composer.
- **Admin panel** — users and roles, an audit log, SMS and e-mail usage with cost, sender
  verification in SendGrid (ADR-0016, ADR-0021) and **Ustawienia wysyłki** (sending settings,
  ADR-0018): `DRY_RUN`, providers and API keys are edited in the UI (keys are encrypted in the
  database), with an explicit confirmation before real sending is switched on.
- **Providers** — Mailpit and SendGrid (official SDK, EU region option, ADR-0014) for e-mail; Fake
  and SMSAPI for SMS, with delivery reports.

## Requirements

Go 1.24+, Node 22+, Docker Desktop (running), GNU `make`, `git`. Check with `make doctor`.

- An older Go is fine as long as it may download the 1.24 toolchain on first use (the default
  `GOTOOLCHAIN=auto`, needs network); `make doctor` tells you which case you are in.
- **Windows:** run every command in **Git Bash** (comes with Git for Windows); the Makefile and the
  scripts are bash. Git for Windows has no `make` — install it with `choco install make`,
  `scoop install make` or `winget install ezwinports.make`. Start Docker Desktop before `make up`.

## Getting started

```bash
git clone https://github.com/Marc3usz/DoYouSend.git   # or git@github.com:… with an SSH key
cd DoYouSend
cp .env.example .env        # works as is for local development; .env never goes into the repo
make doctor                 # checks the tools, Docker and .env
make install                # frontend dependencies (npm ci); dev-web also does this on its own
make up                     # Postgres + Mailpit in containers (waits until ready)
make migrate                # database schema
make seed                   # demo data (fictional only)
make create-admin EMAIL=admin@example.test NAME="Administrator"   # password for the admin account (asked in the terminal)
make dev-api                # API at http://localhost:8080   (keeps the terminal busy)
make dev-web                # UI at http://localhost:5173    (second terminal)
```

`make create-admin` reads the password from the first line of input — **the characters you type
are visible**. You can set `ADMIN_PASSWORD` in `.env` instead. The seed already creates the
`admin@example.test` account without a usable password, so the command sets its password (and
name) rather than creating a second account. The first `make dev-api` / `make create-admin`
downloads the Go modules and takes longer.

Sign in at <http://localhost:5173> with the e-mail and password from `make create-admin`. Further
accounts (including senders) are created in **Panel administratora → Użytkownicy** (Admin panel →
Users). Passwords have at least 12 characters. Login and role details:
`docs/adr/0010-logowanie-sesje-role.md`.

### Sending a test message

Open **Nowa wiadomość** (<http://localhost:5173/wiadomosci>), pick a group from the seed (e.g.
*Wszyscy rodzice*), type a subject and body, check the summary and confirm. The batch page in
**Historia** then shows the status of both channels for every recipient.

With the default `DRY_RUN=true` sending stops at a stub: the history fills in, but nothing reaches
Mailpit. To see the e-mails in Mailpit (<http://localhost:8025>), turn the test mode off in
**Panel administratora → Ustawienia wysyłki** (or set `DRY_RUN=false` in `.env` and restart
`make dev-api`; a value saved in the panel wins over `.env`) while keeping the Mailpit e-mail
provider on `localhost` and the `fake` SMS provider — this still sends nothing off your machine.
With `fake`, text messages never leave the system; they land in the database and the history.
Never turn `DRY_RUN` off with a real provider (SendGrid, SMSAPI, or SMTP to a non-local host)
without the project supervisor's approval; the panel asks for an explicit confirmation then.

### Configuration: `.env` vs. the panel

`.env` holds only what the API needs to start: `APP_ENV`, `API_PORT`, `PUBLIC_API_URL`,
`DATABASE_URL`, `TEST_DATABASE_URL`, `SESSION_COOKIE_SECURE`, `ADMIN_PASSWORD`, `DRY_RUN`,
`DRY_RUN_LOCK` and the settings encryption key. E-mail and SMS providers, API keys, the SMS price
and the Notion import are configured in **Panel administratora → Ustawienia wysyłki** (ADR-0018).
The same variables still work as fallbacks — they are listed, commented out, at the end of
`.env.example`; the order is panel, then `.env`, then the built-in default (Mailpit and `fake`).
The command-line `make import-notion` reads the `NOTION_*` variables from `.env` only.

API keys saved in the panel are encrypted in the database with `SETTINGS_ENCRYPTION_KEY` or, when
it is empty, with a key file the API creates on first start (`backend/settings.key`, gitignored).
**Back that key up separately from the database** — without it the API keys have to be entered
again. Providers, test modes and the Notion import are described in
[`docs/integracje.md`](docs/integracje.md).

## Troubleshooting

- **`failed SASL auth` / `28P01` on `make create-admin` or `make dev-api`** — port 5432 is taken by
  another Postgres (e.g. a Windows service). `make up` will not report it, because `migrate` and
  `seed` run inside the container. Stop that service or remap the port in a local
  `docker-compose.override.yml` (do not commit it):

  ```yaml
  services:
    db:
      ports: !override ["55432:5432"]
  ```

  and change the port in `DATABASE_URL` / `TEST_DATABASE_URL` in `.env`. Mailpit ports 1025/8025
  can be remapped the same way (then change the SMTP port in the panel). To move the API off 8080,
  change both `API_PORT` and `PUBLIC_API_URL` in `.env`; the UI dev server reads the latter.
- **`make doctor` reports `docker daemon`** — Docker Desktop is not running; start it and wait
  until it is ready.
- **`'vite' is not recognized` / `vite: command not found`** — `frontend/node_modules` is missing;
  run `make install`.

## Safety switches

- `DRY_RUN=true` — by default nothing leaves the developer's machine. Turning it off in the admin
  panel requires explicit confirmation when a provider would send outside the machine;
  `DRY_RUN_LOCK=true` in `.env` blocks turning it off at all (demo and test environments).
- The `fake`/Mailpit providers are the default configuration. Connecting a real SMS gateway and
  real recipients requires the project supervisor's approval.
- The repo does not and must not contain real personal data or API keys; CI scans for both.

## Documentation

The documents below are in Polish.

| Document | Contents |
|---|---|
| [`docs/podzial-pracy.md`](docs/podzial-pracy.md) | domains, ownership, order of work |
| [`docs/workflow-git.md`](docs/workflow-git.md) | branches, PRs, review, resolving conflicts |
| [`docs/architektura.md`](docs/architektura.md) | layers, delivery flow, statuses |
| [`docs/model-danych.md`](docs/model-danych.md) | tables, columns and the constraints behind the requirements |
| [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md) | secrets, test data, permissions |
| [`docs/integracje.md`](docs/integracje.md) | e-mail (Mailpit/SendGrid), SMS (Fake/SMSAPI), panel settings and Notion import |
| [`docs/api/openapi.yaml`](docs/api/openapi.yaml) | backend ↔ frontend API contract |
| [`docs/adr/`](docs/adr/) | architecture decision records (ADRs) |
| [`CLAUDE.md`](CLAUDE.md) | instructions for Claude Code |

## Useful commands

```bash
make check             # gofmt, go vet, golangci-lint (if installed), svelte-check, prettier, eslint
make test              # backend and frontend unit tests
make test-integration  # backend tests against Postgres (make up; TEST_DATABASE_URL from .env)
make build             # API binary (backend/bin/api) and the frontend build
make import-notion     # recipients from Notion: report only; ARGS=-apply saves (ADR-0017)
make fmt               # formatting
make down              # stop the containers
```

CI runs `make check`'s steps plus `go build`, `go test -race`, the integration tests on a Postgres
service, `npm run build`, golangci-lint on changed lines and a scan for secrets and contact data.
