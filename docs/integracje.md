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

## 5. Rozliczanie kosztów i statystyki SMS

System udostępnia endpoint `GET /api/stats/sms` agregujący dane z bazy danych (`deliveries` i `message_batches`):
* **Cena jednostkowa:** pobierana z `SMS_PRICE_PER_PART_PLN` (np. `0.08` zł / część).
* **`plannedCostMilli`:** koszt wszystkich zaplanowanych części SMS w danym okresie.
* **`billedCostMilli`:** koszt faktycznie rozliczonych SMS-ów przez bramkę:
  * Obejmuje statusy `sent`, `delivered` oraz wiadomości `failed`, które zostały przyjęte do wysyłki przez operatora (`provider_message_id IS NOT NULL`).
  * Wiadomości odrzucone przed przyjęciem do sieci (błędy walidacji, brak środków, `provider_message_id IS NULL`) nie generują kosztu.
* **Strefa czasowa zapytań:** parametry z samą datą (`?from=2026-10-01&to=2026-10-31`) interpretowane są w strefie `Europe/Warsaw`, obejmując pełną dobę w polskim czasie.

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
4. Uruchom serwer API:
   ```bash
   make dev-api
   ```
5. Przeglądaj maile w Mailpicie pod adresem: [http://localhost:8025](http://localhost:8025).
