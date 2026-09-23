# backend/ — instructions for Claude Code

Go 1.24 API + delivery worker. Read the root `CLAUDE.md` first; this file adds the local detail.

## Layout

- `cmd/api` — entrypoint, wiring only. No business logic here.
- `internal/platform/**` — config, database pool, HTTP helpers. **Shared by all four devs:
  announce changes, keep the surface small.**
- `internal/<domain>` — one package per domain (see the package doc comment in each for its
  scope and owner). A domain package owns its own storage access, HTTP handlers and tests.
- `migrations/` — numbered SQL. Append only; never edit an applied file.
- `testdata/` — fictional fixtures only (`@example.test`, `+48 500 100 1xx`).

## Rules

- Dependencies between domain packages go through **interfaces defined by the consumer**.
  `delivery` must not import a vendor SDK — it depends on the `providers` interfaces.
- No global state, no `init()` side effects, no panics outside `main`.
- Wrap errors: `fmt.Errorf("resolve group %s: %w", id, err)`. Compare with `errors.Is/As`.
- `context.Context` is the first parameter of anything doing I/O.
- Structured logging with `log/slog`. **Never log a full e-mail address, phone number or message
  body** — log recipient IDs instead.
- Tests: table-driven, `_test.go` next to the code, no network and no Docker in unit tests.
  Anything touching Postgres goes behind `testing.Short()` or a `//go:build integration` tag.

## Domain invariants worth re-reading before you touch delivery/messaging

1. The SMS body and the e-mail body for one recipient are byte-identical; only the e-mail
   subject is separate.
2. A recipient selected through several groups is messaged exactly once per batch.
3. One recipient's failure never aborts the batch; each `deliveries` row fails on its own.
4. A sent batch is immutable — reuse means copying it into a new draft.
5. `DRY_RUN=true` must short-circuit every real provider call.

## Commands

```bash
go run ./cmd/api          # API on :8080
go test ./...             # unit tests
go vet ./... && gofmt -l . && golangci-lint run
```
