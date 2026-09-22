# frontend/ — instructions for Claude Code

SvelteKit 2 + Svelte 5 (runes) + TypeScript strict. Read the root `CLAUDE.md` first.

## Layout

- `src/routes/` — pages, mirroring the domain split:
  `odbiorcy/` and `grupy/` (DEV A), `wiadomosci/` (DEV B), `historia/` (DEV B),
  `admin/` (DEV C), `login/` (DEV C).
- `src/lib/api/` — the only place that calls the backend. Add one typed function per endpoint.
- `src/lib/components/` — shared components. Anything used by two domains lives here;
  domain-only components stay under the route folder.
- `src/lib/stores/` — shared client state, kept small.

## Rules

- Svelte 5 runes (`$state`, `$derived`, `$props`), not the Svelte 4 store-in-component style.
- Load data in `+page.server.ts` / `+page.ts`, not in `onMount`, and pass the SvelteKit `fetch`
  into `api()`.
- No `any`, no non-null `!` to silence the compiler. `npm run check` must be clean.
- UI text is Polish; identifiers, props and comments are English.
- Never render another recipient's data in a shared view — a sender sees their own batches,
  an admin sees the recipient base. Enforce it server-side too.
- The message composer must never alter the body the user typed. Length, SMS part count and
  cost come from the backend (`/api/messages/preview`) — do not reimplement the count in TS.

## Commands

```bash
npm install
npm run dev      # http://localhost:5173, /api proxied to :8080
npm run check    # svelte-check + tsc
npm run lint     # prettier + eslint
npm run test     # vitest
```
