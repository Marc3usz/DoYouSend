# DoYouSend — instructions for Claude Code

School-wide notification system: one message is sent **simultaneously by e-mail and SMS**
to selected recipients or groups. Full product spec: `description.md` (Polish, authoritative).

Working language: **code, identifiers, commits, PR titles and `README.md` in English**;
user-facing UI text and `docs/**` in Polish. Write answers to the team in Polish.

## Stack

| Layer     | Choice                                                        |
|-----------|---------------------------------------------------------------|
| Backend   | Go 1.24, stdlib `net/http`, `pgx`                             |
| Frontend  | SvelteKit 2 + Svelte 5 + TypeScript (strict), Vite            |
| DB        | PostgreSQL 16, SQL migrations in `backend/migrations/`         |
| Delivery  | in-process worker (`backend/internal/delivery`); state is the `deliveries` table in Postgres, no separate queue |
| Local env | Docker Compose: Postgres, Mailpit (fake SMTP)                  |

Do not introduce a new framework, ORM or runtime dependency without an ADR
(`docs/adr/`) — dependency choices outlive whoever makes them.

## Repo map

```
backend/            Go API + delivery worker
  cmd/api/          entrypoint
  internal/
    platform/       config, database, httpx — shared plumbing (coordinate before changing)
    recipients/     recipients CRUD, CSV/XLSX/Notion import, validation  -> domain A
    groups/         groups, class groups, recipient resolution            -> domain A
    integrations/notion/  read-only Notion client for the import          -> domain A
    messaging/      message composition, preview, SMS part count          -> domain B
    delivery/       dispatch, per-channel status, retries, history        -> domain B
    providers/      email/ + sms/ adapters (incl. fake ones)              -> domain C
    iam/            users, roles, auth, audit log, sender identities      -> domain D
    settings/       sending settings edited in the admin panel            -> domain D
    senders/        e-mail sender options, domain auth, delegated addresses -> domains C/D
  migrations/       numbered SQL migrations (shared, see below)
frontend/           SvelteKit UI (routes mirror the same domain split)
docs/               architecture, work split, git workflow, ADRs
docs/api/openapi.yaml   backend<->frontend contract (shared, see below)
.claude/            shared team settings, slash commands, subagents
```

All four domains are maintained by Marc3usz since 2026-11-13 (ADR-0020). The domain split stays
so packages keep talking through consumer-defined interfaces. See `docs/podzial-pracy.md`.

## Commands

```bash
make up            # docker compose: postgres + mailpit
make migrate       # apply SQL migrations
make dev-api       # run the Go API with `go run` (no live reload)
make dev-web       # run SvelteKit dev server (http://localhost:5173)
make test          # go test ./... + frontend unit tests
make test-integration  # Postgres-backed tests (TEST_DATABASE_URL from .env)
make check         # gofmt + go vet + golangci-lint (if installed) + svelte-check + prettier + eslint
make fmt           # gofmt -w + prettier --write
```

Run `make check` before every commit. CI additionally runs `go build`, `go test -race`, the
integration tests, `npm run build`, golangci-lint and `scripts/check-secrets.sh`.

## Shared files — coordinate before touching

These cause the merge conflicts that actually hurt with parallel Claude sessions:

- `backend/migrations/*.sql` — **never renumber or edit an applied migration.** Always add a new
  file `NNNN_description.sql` with the next free number; if two people race, the later PR renumbers.
- `docs/api/openapi.yaml` — the contract between backend and frontend. Change it in its own small
  PR, announce it, then implement against it.
- `backend/internal/platform/**`, `go.mod`, `frontend/package.json`, `Makefile`, `.github/workflows/**`.

## Hard rules

1. **No secrets in the repo.** API keys, passwords and tokens live in `.env` (gitignored) or,
   encrypted, in the panel settings (ADR-0018). Add every new variable to `.env.example` with a
   placeholder value.
2. **No real personal data.** Tests, fixtures, seeds and demo imports use fictional names,
   `@example.test` addresses and `+48 500 100 1NN`-style numbers. Never commit a real export.
3. **No production sending.** `DRY_RUN=true` and the `fake`/Mailpit providers are the default;
   switching to a real provider requires the project supervisor's approval (`description.md`).
4. **E-mail and SMS bodies must be byte-identical** per recipient. Never truncate, summarise or
   reformat the SMS variant — only the e-mail subject is a separate field.
5. **A failed recipient must not abort the batch.** Delivery is per-recipient, per-channel,
   with its own status and retry.
6. **Deduplicate recipients** resolved from overlapping groups before dispatch.

## Conventions

- Branches: `feat/<issue>-short-slug`, `fix/…`, `chore/…`, `docs/…`. Never commit to `master`.
- Commits: Conventional Commits with a scope matching the package,
  e.g. `feat(delivery): retry failed sms parts`.
- Go: table-driven tests next to the code, errors wrapped with `fmt.Errorf("...: %w", err)`,
  no panics outside `main`, no global state.
- Svelte: TypeScript strict, server-side data loading in `+page.server.ts`, shared calls in
  `src/lib/api/`, no `any`.
- Every PR: `make check` and CI green, description filled from the template, and a review by the
  `recenzent` subagent with its findings answered in the PR (ADR-0020: one-person team; the
  one-approval rule returns when a second developer joins).

## Working style in this repo

- Prefer a git worktree (or a separate clone) per parallel Claude session so agents do not
  fight over the same working tree.
- Before starting a task, read the owning package's `CLAUDE.md` (`backend/CLAUDE.md`,
  `frontend/CLAUDE.md`) — they carry the local details this file deliberately omits.
- When a decision affects more than one domain, write an ADR in `docs/adr/` instead of
  encoding it silently in code.
- Before starting work that touches a cross-domain contact point (see "Punkty styku" in
  `docs/podzial-pracy.md`), check `docs/adr/` for an ADR whose status is still
  **propozycja** (open/undecided) on that point. Don't silently assume a shape for
  something an open ADR is tracking — either resolve it first (with the other domain owner,
  if there is one), or build against the status quo and flag the ADR again in the PR description. When you
  resolve one, update its `Status`/`Decyzja`/`Konsekwencje` in place instead of leaving it
  stale next to code that has moved on.
