# ADR-0016: Weryfikacja nadawców e-mail (SendGrid Single Sender) zakładana przez administratora

- **Status:** przyjęta (2026-11-13) — decyzje 1–6 wdrożone w #87 (kontrakt) i #90
  (implementacja). Punkt otwarty rozstrzygnięty w ADR-0021. MichalK252 odszedł z projektu przed
  decyzją w sprawie wysyłki; rozstrzygnął Marc3usz jako właściciel wszystkich domen (ADR-0020).
- **Data:** 2026-11-03
- **Uczestnicy:** Marc3usz (domeny A i D: `iam`, panel administratora) proponuje,
  MichalK252 (domeny B i C: `delivery`, `providers`) decyduje o punktach dotyczących wysyłki
- **Dotyczy:** ADR-0008 (SendGrid), ADR-0014 (SDK i region UE), ADR-0015 (konfiguracja w `.env`)

## Kontekst

- Dziś każdy e-mail wychodzi z jednego adresu `EMAIL_FROM`. SendGrid wysyła tylko z adresu,
  który jest zweryfikowaną tożsamością nadawcy albo należy do uwierzytelnionej domeny.
- Szkoła chce, żeby dyrektor czy nauczyciel mógł wysyłać „od siebie”, a administrator mógł
  to przygotować z panelu, bez logowania się do konsoli SendGrid.
- SendGrid ma do tego API **Single Sender Verification** (`/v3/verified_senders`), sprawdzone
  w dokumentacji 2026-11-03:
  - `POST /v3/verified_senders`: wymaga `nickname` (≤100), `from_email` (≤256) i `reply_to`.
    Pola opcjonalne: `from_name`, `reply_to_name`, `address`, `address2`, `city`, `state` (≤2),
    `zip` (≤10), `country`. Odpowiedź 201 zawiera `id`, `verified` i `locked`. SendGrid wysyła
    na `from_email` e-mail z linkiem potwierdzającym.
  - `GET /v3/verified_senders?id=` (lista; parametry `limit`, `lastSeenID`, `id`),
    `POST /v3/verified_senders/resend/{id}`, `DELETE /v3/verified_senders/{id}`.
  - `GET /v3/verified_senders/verify/{token}` potwierdza tożsamość tokenem z e-maila.
    Nie używamy go: token trafia tylko do skrzynki właściciela i tak ma zostać.
  - `GET /v3/verified_senders/domains` zwraca domeny z restrykcyjnym DMARC
    (`hard_failures`, np. yahoo.com), z których wysyłka nie dojdzie.
  - Region UE: te same ścieżki pod `api.eu.sendgrid.com` (ADR-0014).

## Decyzja

1. **Administrator zakłada tożsamość nadawcy dla istniejącego konta** (`POST /api/users/{id}/sender`).
   Adres nadawcy to adres logowania użytkownika, nazwa to imię i nazwisko, a odpowiedzi idą
   na ten sam adres. Administrator nie wpisuje innego adresu, więc nie da się zweryfikować
   cudzej skrzynki „przy okazji”. Dochodzą: podgląd statusu (`GET`, przy `pending` pytamy
   SendGrid), ponowna wysyłka linku (`POST …/resend`), usunięcie (`DELETE`) i lista
   (`GET /api/admin/senders`). Wszystko w `/api/users` i `/api/admin`, więc tylko dla administratora.
2. **Stan w bazie, bez sekretów:** tabela `user_sender_identities` (migracja 0009) trzyma
   operatora, id tożsamości u operatora, adres, status `pending|verified|missing` i czasy
   sprawdzeń. Klucz API nie trafia do tej tabeli. *Zmiana (ADR-0018):* klucz API SendGrid
   i konfigurację operatora ustawia się teraz w panelu (zaszyfrowane w `app_settings`), a `.env`
   jest tylko wartością zapasową; tożsamości nadal zakłada się na skonfigurowanym koncie.
3. **DRY_RUN obowiązuje także tu.** Przy `DRY_RUN=true` albo `EMAIL_PROVIDER` innym niż
   `sendgrid` działa atrapa w pamięci (`email.FakeSenders`, provider `fake`). Nic nie jest
   wysyłane, a pierwsze sprawdzenie statusu zwraca `verified`. Panel dostaje `live: false`
   i to pokazuje. Tożsamość założona przez innego operatora jest usuwana tylko lokalnie,
   bez pytania obecnego operatora.
4. **Bez nowej zależności.** Klient `email.SendGridSenders` używa `net/http` i tej samej
   logiki hosta/regionu co adapter wysyłki (`resolveHost`, ADR-0014). Domena `iam` zależy od
   interfejsu `iam.SenderVerifier`, który sama definiuje; adapter jest w `cmd/api`.
5. **Adres szkoły** pokazywany w e-mailu weryfikacyjnym pochodzi z `SENDGRID_SENDER_ADDRESS`,
   `_CITY`, `_ZIP` i `_COUNTRY` (opcjonalne w API).
6. **Audyt:** `sender_verification_requested`, `sender_verification_resent` i
   `sender_identity_removed` (encja `user`).

## Punkt otwarty — rozstrzygnięty w ADR-0021

Rozstrzygnięcie (2026-11-13, ADR-0021): adres nadawcy (`From`) e-maila wybiera się przy każdym
wsadzie spośród adresów, których autor może używać — wspólny `EMAIL_FROM`, własny adres autora
(gdy jest zweryfikowany albo należy do uwierzytelnionej domeny) albo adresy, które przydzielił
mu administrator. Uwierzytelnianie domeny szkoły w SendGrid (Domain Authentication) jest
zarządzane z panelu, więc dowolny adres w takiej domenie może wysyłać bez weryfikacji
pojedynczego nadawcy. Treść e-maila i SMS-a zostaje bajt w bajt identyczna (zasada 4);
nadawca SMS pozostaje wspólny (`SMS_SENDER_NAME`).

Pierwotny opis punktu (do decyzji MichalK252, domeny B i C):

- **Czy wysyłka ma używać tożsamości autora wsadu?** Ten ADR tylko zakłada i śledzi
  tożsamości. Wysyłka dalej idzie z `EMAIL_FROM`. Propozycja: `providers.Message` dostaje
  opcjonalne `From`. `delivery` ustawia je na zweryfikowany adres `batches.created_by`, a gdy
  autor nie ma statusu `verified`, zostaje `EMAIL_FROM`. Treść e-maila i SMS-a bez zmian
  (zasada 4). Nadawca SMS zostaje wspólny (`SMS_SENDER_NAME`).
- **Darmowe skrzynki (gmail.com, yahoo.com…):** przez DMARC SendGrid może nie doręczać
  poczty wysłanej z takich adresów (`/v3/verified_senders/domains`). Kont szkolnych w domenie
  szkoły to nie dotyczy. Docelowo lepsze jest uwierzytelnienie domeny szkoły w SendGrid
  (Domain Authentication) zamiast pojedynczych nadawców.

## Rozważane alternatywy

- **Zakładanie tożsamości automatycznie przy tworzeniu konta.** Odrzucone: nie każde konto
  wysyła (administratorzy), a błąd SendGrid nie powinien blokować założenia konta.
  Panel może wywołać oba endpointy jeden po drugim.
- **Wpisywanie dowolnego adresu nadawcy.** Odrzucone: link i tak trafia do właściciela
  adresu, ale wiązanie tożsamości z kontem logowania jest prostsze do audytu.
- **Uwierzytelnienie domeny przez API (`/v3/whitelabel/domains`).** Wymaga rekordów DNS
  szkoły, czyli osoby z dostępem do DNS. To lepsze rozwiązanie produkcyjne, ale osobny krok
  wdrożeniowy, nie funkcja panelu.

## Konsekwencje

- Administrator przygotowuje nadawców bez dostępu do konsoli SendGrid. Klucz API potrzebuje
  uprawnienia „Sender Verification”.
- Włączenie prawdziwego SendGrid (`DRY_RUN=false`, `EMAIL_PROVIDER=sendgrid`) sprawi, że
  przycisk w panelu wyśle prawdziwy e-mail do pracownika. To nadal wymaga zgody opiekuna
  (CLAUDE.md, zasada 3).
- Zweryfikowana tożsamość staje się jednym z adresów, które autor może wybrać jako nadawcę
  wsadu (ADR-0021). Bez niej wysyłka idzie z `EMAIL_FROM` albo z adresu przydzielonego przez
  administratora.
- Dla adresów w domenie szkoły zalecane jest uwierzytelnienie domeny w SendGrid zamiast
  weryfikowania każdej osoby osobno (ADR-0021).
