# DoYouSend — instructions for Claude Code

School-wide notification system: one message is sent **simultaneously by e-mail and SMS**
to selected recipients or groups. Full product spec: `description.md` (Polish, authoritative).

Working language: **code, identifiers, commits, PR titles in English**; user-facing UI text,
`README.md` and `docs/**` in Polish. Write answers to the team in Polish.

## Stack

| Layer     | Choice                                                        |
|-----------|---------------------------------------------------------------|
| Backend   | Go 1.24, stdlib `net/http` (routing lib only if needed), `pgx` planned |
| Frontend  | SvelteKit 2 + Svelte 5 + TypeScript (strict), Vite            |
| DB        | PostgreSQL 16, SQL migrations in `backend/migrations/`         |
| Queue     | Redis-backed worker (`backend/internal/delivery`)              |
| Local env | Docker Compose: Postgres, Redis, Mailpit (fake SMTP)           |

Do not introduce a new framework, ORM or runtime dependency without an ADR
(`docs/adr/`) — four people share this repo and dependency choices are not local decisions.

## Repo map

```
backend/            Go API + delivery worker
  cmd/api/          entrypoint
  internal/
    platform/       config, database, httpx — shared plumbing (coordinate before changing)
    recipients/     recipients CRUD, CSV import, validation      -> owner: DEV A (Marc3usz)
    groups/         groups, membership, recipient resolution     -> owner: DEV A (Marc3usz)
    messaging/      message composition, preview, SMS part count -> owner: DEV B (S1D0R-10)
    delivery/       dispatch, queue, per-channel status, retries -> owner: DEV B (S1D0R-10)
    providers/      email/ + sms/ adapters (incl. fake ones)     -> owner: DEV C (MichalK252)
    iam/            users, roles, auth, audit log                -> owner: DEV D (averithefox)
  migrations/       numbered SQL migrations (shared, see below)
frontend/           SvelteKit UI (routes mirror the same domain split)
docs/               architecture, work split, git workflow, ADRs
docs/api/openapi.yaml   backend<->frontend contract (shared, see below)
.claude/            shared team settings, slash commands, subagents
```

Ownership is about *review responsibility*, not a lock. Editing someone else's package is fine;
tag them on the PR. See `docs/podzial-pracy.md`.

## Commands

```bash
make up            # docker compose: postgres + redis + mailpit
make migrate       # apply SQL migrations
make dev-api       # run Go API with live reload fallback to `go run`
make dev-web       # run SvelteKit dev server (http://localhost:5173)
make test          # go test ./... + frontend unit tests
make check         # go vet + golangci-lint + svelte-check + prettier --check
make fmt           # gofmt -w + prettier --write
```

Run `make check` before every commit; CI runs the same targets.

## Shared files — coordinate before touching

These cause the merge conflicts that actually hurt with three parallel Claude sessions:

- `backend/migrations/*.sql` — **never renumber or edit an applied migration.** Always add a new
  file `NNNN_description.sql` with the next free number; if two people race, the later PR renumbers.
- `docs/api/openapi.yaml` — the contract between backend and frontend. Change it in its own small
  PR, announce it, then implement against it.
- `backend/internal/platform/**`, `go.mod`, `frontend/package.json`, `Makefile`, `.github/workflows/**`.

## Hard rules

1. **No secrets in the repo.** API keys, passwords and tokens live in `.env` (gitignored).
   Add every new variable to `.env.example` with a placeholder value.
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
- Every PR: `make check` green, description filled from the template, one approval from another dev.

## Working style in this repo

- Prefer a git worktree (or a separate clone) per parallel Claude session so three agents do not
  fight over the same working tree.
- Before starting a task, read the owning package's `CLAUDE.md` (`backend/CLAUDE.md`,
  `frontend/CLAUDE.md`) — they carry the local details this file deliberately omits.
- When a decision affects more than one domain, write an ADR in `docs/adr/` instead of
  encoding it silently in code.
