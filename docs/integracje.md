# Integracje — E-mail i SMS

Niniejszy dokument opisuje architekturę, konfigurację oraz sposób działania integracji z dostawcami poczty elektronicznej (e-mail) oraz bramek SMS w systemie **DoYouSend** (domena **DEV C**).

---

## 1. Architektura integracji

Zgodnie z wymaganiami projektowymi (`description.md` oraz ADR-0003 i ADR-0008), integracje realizowane są za wspólnym interfejsem `providers.Provider`:

```go
type Provider interface {
    Send(ctx context.Context, msg Message) (Result, error)
    Channel() Channel
}
```

```
                        ┌────────────────────────┐
                        │   delivery.Dispatcher  │
                        └───────────┬────────────┘
                                    │ Send(ctx, msg)
            ┌───────────────────────┴───────────────────────┐
            ▼                                               ▼
┌────────────────────────┐                     ┌────────────────────────┐
│  p.Email (Provider)   │                     │   p.SMS (Provider)     │
├────────────────────────┤                     ├────────────────────────┤
│ • Mailpit (SMTP:1025)  │                     │ • Fake SMS (in-memory) │
│ • SendGrid (REST API)  │                     │ • SMSAPI (REST API)    │
└───────────┬────────────┘                     └───────────┬────────────┘
            │                                               │
            │ (WrapDryRun gdy DRY_RUN=true)                 │
            ▼                                               ▼
┌────────────────────────┐                     ┌────────────────────────┐
│ Webhook: SendGrid Evt  │                     │ Webhook: SMSAPI DLR    │
│ POST /providers/email/ │                     │ GET/POST /providers/   │
│ events                 │                     │ sms/dlr[/{token}]      │
└───────────┬────────────┘                     └───────────┬────────────┘
            │                                               │
            └───────────────────────┬───────────────────────┘
                                    ▼
                     ┌─────────────────────────────┐
                     │ PGDeliveryReportConsumer    │
                     │ (UPDATE deliveries status)  │
                     └─────────────────────────────┘
```

Fabryka providerów znajduje się w pakiecie `backend/internal/providers/setup` i jest inicjalizowana automatycznie na podstawie konfiguracji ze zmiennych środowiskowych (`setup.ConfigFromEnv()`).

---

## 2. Bezpiecznik `DRY_RUN`

Wszystkie providery są domyślnie zabezpieczone dekoratorem `providers.WrapDryRun`:
* Gdy `DRY_RUN=true` (wartość domyślna w `.env.example`), żadne żądanie sieciowe nie wychodzi poza aplikację.
* Zwracany jest syntetyczny identyfikator `ProviderMessageID` (np. `dryrun-email-...`), co pozwala na pełne przetestowanie przepływu wysyłki, kolejki oraz zapisu w bazie danych bez ponoszenia kosztów i bez ryzyka wysłania wiadomości do prawdziwych odbiorców.
* Wyłączenie trybu Dry Run (`DRY_RUN=false`) wymaga zgody opiekuna projektu.

---

## 3. Dostawcy E-mail

### 3.1. Lokalnie: Mailpit (`EMAIL_PROVIDER=mailpit`)
Domyślny adapter dla środowiska deweloperskiego i testowego, uruchamiany przez `docker compose up`:
* **Port SMTP:** `1025` (nasłuch serwera SMTP)
* **Web UI:** `http://localhost:8025` (interfejs do podglądu odebranych maili)
* **Zmienne w `.env`:**
  ```env
  EMAIL_PROVIDER=mailpit
  SMTP_HOST=localhost
  SMTP_PORT=1025
  EMAIL_FROM="DoYouSend <no-reply@example.test>"
  ```
* **Cechy implementacji:**
  * Ochrona przed wstrzykiwaniem nagłówków (odrzucanie znaków CRLF w polach `To` i `Subject`).
  * Kodowanie Q-encoding (RFC 2047) dla polskich znaków diakrytycznych w temacie.
  * Generowanie unikalnego nagłówka RFC 5322 `Message-ID`.

### 3.2. Produkcyjnie: SendGrid (`EMAIL_PROVIDER=sendgrid`, ADR-0008)
Adapter wysyłający wiadomości przez oficjalne REST API SendGrid v3 (`/v3/mail/send`):
* **Zmienne w `.env`:**
  ```env
  EMAIL_PROVIDER=sendgrid
  SENDGRID_API_KEY=SG.twoj-klucz-api
  SENDGRID_SANDBOX=false
  EMAIL_FROM="DoYouSend <zweryfikowany-nadawca@twojadomena.pl>"
  ```
* **Tryb Sandbox:** Ustawienie `SENDGRID_SANDBOX=true` waliduje żądanie po stronie API SendGrid bez wysyłania e-maila.
* **Webhook zdarzeń (Event Webhook):**
  * Ścieżka: `POST /providers/email/events`
  * Weryfikacja kryptograficzna ECDSA: nagłówki `X-Twilio-Email-Event-Webhook-Signature` oraz `X-Twilio-Email-Event-Webhook-Timestamp`.
  * Klucz publiczny weryfikacji: `SENDGRID_WEBHOOK_PUBLIC_KEY`.

---

## 4. Bramki SMS

### 4.1. Lokalnie: Fake SMS (`SMS_PROVIDER=fake`)
Domyślny adapter dla środowiska deweloperskiego:
* **Zmienne w `.env`:**
  ```env
  SMS_PROVIDER=fake
  SMS_SENDER_NAME=SZKOLA-TEST
  ```
* **Działanie:** Rejestruje wysłane SMS-y w pamięci w buforze pierścieniowym (ring buffer 1000 wpisów), logując treść na poziomie `DEBUG`. Zwraca deterministyczne identyfikatory `fake-sms-...`.

### 4.2. Produkcyjnie: SMSAPI (`SMS_PROVIDER=smsapi`, ADR-0003)
Adapter integrujący się z bramką SMSAPI przez REST API v2 (`https://api.smsapi.pl/sms.do`):
* **Zmienne w `.env`:**
  ```env
  SMS_PROVIDER=smsapi
  SMS_API_KEY=twoj-token-oauth-smsapi
  SMS_SENDER_NAME=SZKOLA-TEST
  SMS_TEST_MODE=false
  SMSAPI_DLR_TOKEN=twoj-tajny-token-dlr
  ```
* **Tryb testowy:** `SMS_TEST_MODE=true` dodaje parametr `test=1` do żądania SMSAPI — wiadomość jest sprawdzana i walidowana przez bramkę bez obciążania konta i wysyłki do sieci GSM.
* **Raporty doręczeń (DLR callback):**
  * Ścieżki: `GET /providers/sms/dlr`, `POST /providers/sms/dlr` oraz warianty z tokenem w ścieżce `/providers/sms/dlr/{token}`.
  * Zabezpieczenie: porównanie tokenu z `SMSAPI_DLR_TOKEN` w czasie stałym (`subtle.ConstantTimeCompare`) zapobiega atakom timingowym.
  * Mapowanie statusów DLR:
    * `DELIVERED` → `delivered`
    * `UNDELIVERED`, `EXPIRED`, `REJECTED`, `FAILED` → `failed`
    * `SENT`, `ACCEPTED` → `sent`

---

## 5. Statystyki wykorzystania kanałów i rozliczanie kosztów

Dla panelu administratora (`/admin`, domena DEV D) system udostępnia dedykowane endpointy agregujące metryki bezpośrednio z tabel `deliveries`, `batch_recipients` oraz `message_batches`. Oba endpointy podlegają kontroli dostępu IAM i wymagają uprawnień administratora (`AccessAdmin`).

### 5.1. Statystyki i koszty SMS (`GET /api/stats/sms`)
* **Cena jednostkowa:** pobierana z `SMS_PRICE_PER_PART_PLN` (np. `0.08` zł / część = `80` milli-PLN).
* **`plannedCostMilli`:** koszt wszystkich zaplanowanych części SMS w danym okresie.
* **`billedCostMilli`:** koszt faktycznie rozliczonych SMS-ów przez bramkę:
  * Obejmuje statusy `sent`, `delivered` oraz wiadomości `failed`, które zostały przyjęte do wysyłki przez operatora (`provider_message_id IS NOT NULL`).
  * Wiadomości odrzucone przed przyjęciem do sieci (błędy walidacji, brak środków, `provider_message_id IS NULL`) nie generują kosztu.
* **Części i wiadomości:** zwraca rozbicie na `totalMessages`, `totalParts`, `deliveredMessages`, `deliveredParts`, `sentMessages`, `sentParts`, `failedMessages`, `failedParts`, `inFlightMessages` oraz `inFlightParts`.

### 5.2. Statystyki wiadomości E-mail (`GET /api/stats/email`)
* **Struktura odpowiedzi:** zwraca agregację `totalMessages`, `deliveredMessages`, `sentMessages`, `failedMessages` oraz `inFlightMessages` dla kanału `email`.
* **Aktualizacja statusów:** statusy doręczeń `delivered` i `failed` aktualizowane są w czasie rzeczywistym przez konsumenta raportów (`PGDeliveryReportConsumer`) na podstawie webhooków SendGrid Event Webhook lub Mailpit.

### 5.3. Filtry czasowe i zakresy
Oba endpointy statystyk obsługują identyczny zestaw parametrów zapytania:
* `batch_id`: opcjonalny filtr po identyfikatorze UUID wsadu.
* `from`: początek zakresu czasu utworzenia/potwierdzenia wsadu (RFC3339 lub format daty `YYYY-MM-DD`).
* `to`: koniec zakresu czasu. W przypadku formatu `YYYY-MM-DD` data interpretowana jest w strefie czasowej `Europe/Warsaw` z domknięciem do końca doby (następna północ, przedział lewostronnie domknięty, prawostronnie otwarty).

---

## 6. Szybki start (uruchomienie lokalne)

1. Skopiuj plik środowiskowy:
   ```bash
   cp .env.example .env
   ```
2. Uruchom usługi pomocnicze (PostgreSQL, Redis, Mailpit):
   ```bash
   make up
   ```
3. Zastosuj migracje bazy danych:
   ```bash
   make migrate
   ```
4. Utwórz konto administratora:
   ```bash
   make create-admin EMAIL=admin@example.test NAME="Administrator"
   ```
5. Uruchom serwer API:
   ```bash
   make dev-api
   ```
6. Przeglądaj maile w Mailpicie pod adresem: [http://localhost:8025](http://localhost:8025).

---

## 7. Weryfikacja i testy integracyjne E2E (M3)

Kompletny przepływ integracji wysyłki end-to-end (dyspozytor $\rightarrow$ baza PostgreSQL $\rightarrow$ raporty doręczeń $\rightarrow$ agregacja kosztów) jest objęty testem integracyjnym w `backend/internal/providers/setup/delivery_integration_test.go`:

```bash
# Uruchomienie testów integracyjnych na bazie PostgreSQL:
make test-integration
# Lub bezpośrednio dla pakietu providerów:
cd backend && TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/doyousend_test?sslmode=disable" go test -tags integration -v ./internal/providers/...
```

Test automatycznie:
1. Inicjalizuje providerów przez fabrykę `setup.New` z włączonym bezpiecznikiem `DRY_RUN`.
2. Buduje plan wysyłki i dyspaczuje wiadomości e-mail oraz SMS przez `delivery.Dispatcher`.
3. Zapisuje wyniki i identyfikatory wiadomości w bazie danych.
4. Symuluje napływ raportów doręczeń przez `PGDeliveryReportConsumer` (zmiana statusów na `delivered`).
5. Weryfikuje prawidłową agregację statusów, części i kosztów przez `sms.PGUsageStore` oraz `email.PGUsageStore`.
