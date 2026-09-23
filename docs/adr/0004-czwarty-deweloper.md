# ADR-0004: Czwarty deweloper — podział domeny integracje/admin

- **Status:** przyjęta
- **Data:** 2026-09-23
- **Uczestnicy:** zespół (4 osoby)

## Kontekst

Do zespołu dołączyła czwarta osoba. Dotychczasowa domena DEV C (`docs/adr/0002-podzial-domenowy.md`)
łączyła dwa niezwiązane obszary: providerów e-mail/SMS (`backend/internal/providers`) oraz
IAM, panel admina i CI (`backend/internal/iam`, `frontend/src/routes/admin`,
`frontend/src/routes/login`, `.github/workflows`). Te obszary rzadko się przecinają, więc
nadają się do rozdzielenia bez zwiększania liczby punktów styku.

## Decyzja

Domenę DEV C dzielimy na dwie:

- **DEV C** zostaje przy `backend/internal/providers` (integracje e-mail/SMS).
- **DEV D** przejmuje `backend/internal/iam`, `frontend/src/routes/admin`,
  `frontend/src/routes/login` oraz `.github/workflows` (IAM, admin, bezpieczeństwo, CI).

Aktualne przypisania: DEV A — Marc3usz, DEV B — S1D0R-10, DEV C — MichalK252,
DEV D — averithefox. Pełny opis w `docs/podzial-pracy.md`, egzekwowane przez
`.github/CODEOWNERS`.

## Rozważane alternatywy

- **Podział delivery/messaging (B) na dwie osoby** — odrzucone: to jedna spójna ścieżka
  (kompozycja wiadomości → kolejka → status), rozdzielenie zwiększyłoby liczbę punktów styku
  zamiast je zmniejszyć.
- **Podział recipients/groups (A) na dwie osoby** — odrzucone: domena już jest najmniejsza,
  a import CSV i grupy współdzielą tę samą logikę deduplikacji.

## Konsekwencje

- Provider interface (`Provider.Send(ctx, msg) -> (id, error)`) pozostaje punktem styku
  C → B bez zmian.
- Nowy punkt styku: logowanie i role (D → wszyscy) — middleware auth i wymagane role na
  endpointach muszą być ustalone przed M2.
- `.github/workflows` (CI) przechodzi pod review DEV D zamiast DEV C.
