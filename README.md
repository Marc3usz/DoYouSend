# DoYouSend

An e-mail and SMS messaging system for a school. One message, one approval, two channels:
the same notice reaches a parent or student at their e-mail address **and** their phone number.

Full requirements spec (in Polish): [`description.md`](description.md).

## Stack

- **Backend:** Go 1.24 (stdlib `net/http`, `pgx`)
- **Frontend:** SvelteKit 2 + Svelte 5 + TypeScript
- **Database:** PostgreSQL 16 (SQL migrations in `backend/migrations/`)
- **Delivery queue:** delivery worker (`backend/internal/delivery`) with its state persisted in PostgreSQL
- **Local environment:** Docker Compose — Postgres, Redis, Mailpit (fake SMTP server)

## Requirements

Go 1.24+, Node 22+, Docker Desktop, `make`, `git`. Check with `make doctor`.

## Getting started

```bash
git clone https://github.com/Marc3usz/DoYouSend.git   # or git@github.com:… with an SSH key
cd DoYouSend
cp .env.example .env        # fill in locally; .env never goes into the repo
make doctor                 # checks the tools and .env
make install                # frontend dependencies (npm ci); dev-web also does this on its own
make up                     # Postgres + Redis + Mailpit in containers (waits until ready)
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

To view "sent" e-mails, open Mailpit at <http://localhost:8025>. **Note:** with the default
`DRY_RUN=true`, sending stops at a stub and nothing reaches Mailpit. To see e-mails in Mailpit, set
`DRY_RUN=false` together with `EMAIL_PROVIDER=mailpit` and `SMS_PROVIDER=fake` — this still sends
nothing off your machine. Never combine `DRY_RUN=false` with a real provider without the project
supervisor's approval. With `SMS_PROVIDER=fake`, text messages never leave the system — they land
in the log and the database and show up in the sending history.
Production providers (SendGrid, SMSAPI) and their test modes are described in
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

  and change the port in `DATABASE_URL` / `TEST_DATABASE_URL` in `.env`. The same applies to ports
  6379 (Redis), 1025/8025 (Mailpit), 8080 (API) and 5173 (UI).
- **`'vite' is not recognized` / `vite: command not found`** — `frontend/node_modules` is missing;
  run `make install`.

## Safety switches

- `DRY_RUN=true` in `.env` — by default nothing leaves the developer's machine.
- The `fake`/Mailpit providers are the default configuration. Connecting a real SMS gateway and
  real recipients requires the project supervisor's approval.
- The repo does not and must not contain real personal data or API keys.

## Teamwork (2 people)

The documents below are in Polish.

| Document | Contents |
|---|---|
| [`docs/podzial-pracy.md`](docs/podzial-pracy.md) | who owns which domain, order of work |
| [`docs/workflow-git.md`](docs/workflow-git.md) | branches, PRs, review, resolving conflicts |
| [`docs/architektura.md`](docs/architektura.md) | layers, delivery flow, statuses |
| [`docs/model-danych.md`](docs/model-danych.md) | entities and relations |
| [`docs/bezpieczenstwo.md`](docs/bezpieczenstwo.md) | secrets, test data, permissions |
| [`docs/integracje.md`](docs/integracje.md) | setup and behaviour of e-mail (Mailpit/SendGrid) and SMS (Fake/SMSAPI) |
| [`docs/adr/`](docs/adr/) | architecture decision records (ADRs) |
| [`CLAUDE.md`](CLAUDE.md) | instructions for Claude Code — shared by the whole team |

## Useful commands

```bash
make check     # linters and type checks (same as CI)
make test      # backend and frontend tests
make test-integration  # tests against Postgres (TEST_DATABASE_URL, see .env.example)
make fmt       # formatting
make down      # stop the containers
```
