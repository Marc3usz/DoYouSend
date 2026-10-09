# ADR-0010: Logowanie, sesje i role

- **Status:** propozycja — wdrożona w PR z panelem administratora; czeka na akceptację
  averithefox (DEV D, właściciel `iam`) i przegląd zespołu
- **Data:** 2026-10-09
- **Uczestnicy:** Marc3usz (DEV A) — napisał w zastępstwie DEV D; averithefox (DEV D) — właściciel
  `iam`; S1D0R-10 (DEV B) i MichalK252 (DEV C) — konsumenci middleware

## Kontekst

`description.md` („Bezpieczeństwo”) wymaga, żeby dostęp mieli wyłącznie upoważnieni użytkownicy,
każdy widział tylko dane potrzebne do pracy, a system rejestrował, kto przygotował i uruchomił
wysyłkę. Do 2026-10-09 pakiet `iam` był pusty. Wszystkie endpointy, także te z danymi osobowymi
odbiorców, były otwarte, a w kodzie wisiały `TODO(iam)`. Wsad z #30 potrzebuje `CreatedBy`, a panel
administratora („Panel administracyjny”) potrzebuje kont i ról.

Ograniczenia: zero nowych zależności bez ADR (`CLAUDE.md`), brak sekretów w repo, jedna aplikacja
SvelteKit przed jednym API w Go, na razie bez SSO szkoły.

## Decyzja

1. **Sesje po stronie serwera w Postgresie**, nie JWT. Losowy token (32 bajty) trafia do cookie
   `dys_session`, a w tabeli `sessions` (migracja `0004`) leży tylko jego skrót SHA-256. Sesja
   trwa 12 h. Wylogowanie, zablokowanie konta i nadanie nowego hasła kończą sesje natychmiast, bez
   listy unieważnionych tokenów.
2. **Cookie: `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure`** poza `APP_ENV=development`
   (`SESSION_COOKIE_SECURE`). `SameSite=Lax` jest ochroną przed CSRF. Przeglądarka nie dołącza
   cookie do żądań POST/PUT/PATCH/DELETE z obcej domeny, a API nie ma endpointów zmieniających
   stan przez GET.
3. **Hasła: PBKDF2-HMAC-SHA256, 600 000 iteracji, sól 16 B** (`crypto/pbkdf2` z biblioteki
   standardowej Go 1.24), zapis `pbkdf2-sha256$iteracje$sól$klucz`. Hasło ma 12–128 znaków.
   Logowanie na nieznany e-mail też liczy PBKDF2, więc czas odpowiedzi nie zdradza, kto ma konto.
   Każda nieudana próba daje tę samą odpowiedź (`invalid_credentials`). Po 5 nieudanych próbach na
   jeden e-mail w 15 minut odpowiedzią jest `429 too_many_attempts`. Licznik jest w pamięci
   procesu, co wystarcza przy jednej instancji API.
4. **Dwie role:** `admin` (odbiorcy, grupy, import, konta, konfiguracja, statystyki, dziennik) i
   `sender` (kreator wiadomości, podgląd, lista grup i rozwinięcie wyboru, wysyłka).
5. **Jedno miejsce decyzji o dostępie:** `iam.Middleware` owija cały mux i sprawdza tabelę reguł
   `iam/policy.go` (metoda + ścieżka, dopasowanie po segmentach, ścieżka wyczyszczona przez
   `path.Clean`). **Domyślnie wymagane jest logowanie.** Nowy endpoint bez reguły nie będzie
   publiczny, a co najwyżej dostępny dla każdego zalogowanego. Publiczne są tylko `/healthz`,
   `POST /api/auth/login` i webhooki operatorów (`/providers/*`, które mają własny token i podpis
   z #28). Handlery czytają użytkownika przez `iam.UserFrom(ctx)` i same o dostępie nie decydują.
6. **Pierwsze konto:** komenda `make create-admin` (`backend/cmd/create-admin`). Hasło pochodzi z
   `ADMIN_PASSWORD` albo ze standardowego wejścia, nigdy z flagi, żeby nie zostało w historii
   powłoki. Komenda ustawia też hasło istniejącym kontom z `seed.sql`, które mają hasło-zaślepkę i
   bez tego nie mogą się zalogować.
7. **Ochrona przed zablokowaniem panelu:** administrator nie zablokuje ani nie zdegraduje siebie
   (`self_lockout`). Ostatniego aktywnego administratora nie zdegraduje ani nie zablokuje nikt
   (`last_admin`).
8. **Dziennik:** `iam` zapisuje w istniejącej tabeli `audit_log` logowania (także nieudane),
   wylogowania, zakładanie i zmiany kont oraz nadanie hasła. W szczegółach nie ma danych
   kontaktowych ani treści. Inne domeny dopisują własne akcje (np. DEV B: kto zatwierdził wsad), a
   panel pokazuje nieznane akcje bez tłumaczenia.
9. **Frontend:** `hooks.server.ts` pyta `GET /api/auth/me` przy każdym żądaniu strony. Bez sesji
   przekierowuje na `/login?next=…`, a stron administratora nie renderuje dla roli `sender` (403).
   Reguły stron (`$lib/access.ts`) odzwierciedlają reguły API, ale to backend jest zabezpieczeniem.
   Ukrycie linku nim nie jest (`docs/bezpieczenstwo.md`).

## Rozważane alternatywy

- **JWT w cookie.** Odrzucone: nie da się go unieważnić przy zablokowaniu konta bez listy
  unieważnień, czyli i tak potrzebna jest tabela. Do tego dochodzi sekret do podpisu
  (`AUTH_SECRET` z `.env.example` usunięty jako nieużywany).
- **bcrypt/argon2 z `golang.org/x/crypto`.** Odrzucone na teraz: nowa zależność, a PBKDF2 z
  biblioteki standardowej z parametrami OWASP wystarcza dla kilkunastu kont pracowników. Format
  hasha ma prefiks schematu, więc migracja na argon2id przy następnym logowaniu jest możliwa
  później bez resetu haseł.
- **Middleware per handler (każda domena owija swoje trasy).** Odrzucone: cztery osoby i cztery
  miejsca, w których łatwo zapomnieć o jednej trasie. Jedna tabela reguł jest łatwa do przejrzenia
  w review i do przetestowania (`policy_test.go`).
- **Token CSRF w formularzach.** Odrzucone na teraz: `SameSite=Lax` plus brak zmian stanu przez
  GET pokrywa obecne przeglądarki. Do rozważenia, gdyby API miało przyjmować żądania z innej domeny.
- **Logowanie przez konta Google/Microsoft szkoły.** Poza zakresem. Wymaga konfiguracji po stronie
  szkoły. Interfejs `Service.Authenticate` da się później podpiąć pod OIDC.

## Konsekwencje

- **Wszystkie** endpointy z danymi wymagają teraz logowania. Lokalnie trzeba raz uruchomić
  `make migrate` i `make create-admin` (README), inaczej UI pokaże tylko ekran logowania.
- Wysyłający nie ma dostępu do listy odbiorców (`/api/recipients`). Wybór pojedynczych osób w
  kreatorze (DEV B, ADR-0007) będzie potrzebował osobnego, węższego endpointu (np. wyszukiwarki bez
  danych kontaktowych) albo zmiany reguły. Do ustalenia z DEV B.
- `delivery.Service.Send` (#30) może brać `CreatedBy` z `iam.UserFrom(ctx)`.
- Licznik nieudanych logowań działa per proces. Przy kilku instancjach API trzeba go przenieść do
  Redisa albo bazy.
- Migracja ma numer `0004`, bo `0003` jest zarezerwowane dla klas odbiorców (ADR-0009). Migracje
  są niezależne, a `scripts/migrate.sh` stosuje każdy brakujący plik.
- Właściciel `iam` to nadal DEV D. Ten ADR i kod są propozycją do jego przeglądu, a nie przejęciem
  domeny.
