# Integracje — e-mail i SMS

Ten dokument opisuje architekturę, konfigurację i działanie integracji z operatorami poczty
elektronicznej i bramkami SMS w systemie **DoYouSend** (domena C, `backend/internal/providers`;
zob. `docs/podzial-pracy.md`).

---

## 1. Architektura integracji

Zgodnie z `description.md`, ADR-0003 i ADR-0008 integracje działają za wspólnym interfejsem
`providers.Provider`:

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
│  e-mail (Provider)     │                     │   SMS (Provider)       │
├────────────────────────┤                     ├────────────────────────┤
│ • SMTP / Mailpit       │                     │ • Fake SMS (in-memory) │
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
                     └──────────────┬──────────────┘
                                    │ brak dopasowania (w trakcie dispatch)
                                    ▼
                     ┌─────────────────────────────┐
                     │ unmatched_delivery_reports  │
                     │ (bufor do chwili zapisu ID) │
                     └─────────────────────────────┘
```

> **Odporność na wyścigi DLR (ADR-0011):** gdy raport doręczenia przychodzi w trakcie wysyłki
> wsadu (zanim wiersz w `deliveries` dostanie `provider_message_id`), konsument buforuje go
> w tabeli `unmatched_delivery_reports`. Gdy dyspozytor zapisze identyfikator, wyzwalacz
> w bazie uzgadnia status (tylko „do przodu”), więc raporty nie giną.

**Skąd API bierze konfigurację.** Przy starcie `backend/cmd/api` wczytuje ustawienia przez
`settings.Service` (kolejność: wartość z panelu, zmienna środowiskowa, wartość domyślna z
`backend/internal/settings/defs.go`) i buduje providerów przez `setup.NewLive`. Zapis w panelu
podmienia providerów od razu, bez restartu. Gdy zapisane ustawienia nie dają się uruchomić,
API startuje z samych zmiennych środowiskowych z wymuszonym `DRY_RUN`. Funkcja
`setup.ConfigFromEnv()` służy już tylko testom.

---

## 2. Bezpiecznik `DRY_RUN`

Wszyscy providerzy są opakowani dekoratorem `providers.WrapDryRun`:

- Gdy `DRY_RUN=true` (domyślnie), żadne żądanie nie wychodzi poza aplikację. Provider zwraca
  syntetyczny identyfikator (`dryrun-email-…`, `dryrun-sms-…`), więc cały przepływ — wsad,
  statusy, historia — działa bez kosztów i bez ryzyka wysłania czegokolwiek prawdziwym osobom.
- Wyłączenie `DRY_RUN` z operatorem, który wysyła poza komputer (SendGrid, SMSAPI albo SMTP na
  host inny niż `localhost`, adres pętli zwrotnej czy `mailpit`), wymaga w panelu wpisania
  `WYSYŁAJ` oraz zgody opiekuna projektu. `DRY_RUN_LOCK=true` w `.env` całkowicie blokuje
  wyłączenie trybu testowego.
- Gdy jakiś wsad jest właśnie wysyłany, panel nie pozwala wyłączyć `DRY_RUN` ani zmienić
  operatora (ADR-0018), żeby jeden wsad nie poszedł częściowo na niby, a częściowo naprawdę.

---

## 3. Operatorzy e-mail

Operatora, nadawcę i klucze ustawia się w **Panelu administratora → Ustawienia wysyłki**
(rozdział 7). Nazwy zmiennych poniżej działają jako wartości zapasowe w `.env` — są
zakomentowane na końcu `.env.example`.

### 3.1. Lokalnie: Mailpit (`EMAIL_PROVIDER=mailpit`)

Domyślny adapter, uruchamiany przez `make up`:

- **SMTP:** `localhost:1025`, **podgląd poczty:** <http://localhost:8025>.
- Zmienne zapasowe: `EMAIL_PROVIDER=mailpit`, `SMTP_HOST=localhost`, `SMTP_PORT=1025`,
  `EMAIL_FROM="DoYouSend <no-reply@example.test>"`.
- Adapter jest zwykłym klientem SMTP (z opcjonalnym `SMTP_USERNAME`/`SMTP_PASSWORD`). Dlatego
  SMTP na host spoza komputera traktujemy jak prawdziwą wysyłkę (rozdział 2).
- Cechy implementacji: odrzucanie znaków CRLF w polach `To` i `Subject` (ochrona przed
  wstrzykiwaniem nagłówków), kodowanie RFC 2047 polskich znaków w temacie, unikalny
  nagłówek `Message-ID`, a koperta SMTP dostaje sam adres nadawcy, bez nazwy wyświetlanej.
- Mailpit nie wysyła raportów doręczeń: e-maile kończą na statusie `sent`.

### 3.2. Produkcyjnie: SendGrid (`EMAIL_PROVIDER=sendgrid`, ADR-0008, ADR-0014)

Wysyłka przez REST API SendGrid v3 (`/v3/mail/send`). Żądanie buduje oficjalne SDK
`sendgrid-go`, a wysyła nasz `http.Client` z limitem czasu (ADR-0014). Treść idzie jako
`text/plain` z wyłączonym śledzeniem kliknięć, więc nie zmienia się po drodze.

- Zmienne zapasowe:
  ```env
  EMAIL_PROVIDER=sendgrid
  SENDGRID_API_KEY=<klucz API z konsoli SendGrid>
  SENDGRID_SANDBOX=false
  SENDGRID_REGION=global
  EMAIL_FROM="DoYouSend <nadawca@example.test>"
  ```
- **Region danych:** `global` (domyślnie, `api.sendgrid.com`) albo `eu` (`api.eu.sendgrid.com`).
  Inna wartość jest odrzucana. Klucz API powinien pochodzić z konta w tym samym regionie
  (ADR-0014). Przed produkcją ustawiamy `eu`.
- **Sandbox:** `SENDGRID_SANDBOX=true` — SendGrid sprawdza żądanie, ale niczego nie wysyła.
- **Adres nadawcy:** SendGrid wysyła tylko z adresu zweryfikowanego albo z uwierzytelnionej
  domeny. Weryfikację pojedynczych nadawców zleca administrator z panelu (ADR-0016), a wybór
  adresu nadawcy dla wsadu i uwierzytelnianie domeny szkoły opisuje ADR-0021.
- **Webhook zdarzeń (Event Webhook):** `POST /providers/email/events`, podpis ECDSA
  w nagłówkach `X-Twilio-Email-Event-Webhook-Signature` i `X-Twilio-Email-Event-Webhook-Timestamp`
  (starsze niż 5 minut są odrzucane), klucz publiczny w ustawieniu „klucz webhooka SendGrid”
  (`SENDGRID_WEBHOOK_PUBLIC_KEY`). Bez klucza endpoint odpowiada 503.

---

## 4. Bramki SMS

### 4.1. Lokalnie: Fake SMS (`SMS_PROVIDER=fake`)

Domyślny adapter. Zapamiętuje ostatnie 1000 SMS-ów w pamięci procesu i zwraca identyfikatory
`fake-sms-N`. Do logu (poziom `INFO`) trafia tylko ID odbiorcy i ID wiadomości — nigdy numer
ani treść. Status wysyłki i treść widać w historii, bo zapisuje je baza.

### 4.2. Produkcyjnie: SMSAPI (`SMS_PROVIDER=smsapi`, ADR-0003)

Wysyłka przez REST API SMSAPI (`https://api.smsapi.pl/sms.do`):

- Zmienne zapasowe:
  ```env
  SMS_PROVIDER=smsapi
  SMS_API_KEY=<token OAuth z panelu SMSAPI>
  SMS_SENDER_NAME=SZKOLA-TEST
  SMS_TEST_MODE=false
  SMSAPI_DLR_TOKEN=<losowy tajny ciąg>
  ```
- **Tryb testowy:** `SMS_TEST_MODE=true` dodaje `test=1` — bramka sprawdza wiadomość, ale jej
  nie wysyła i nie pobiera opłaty.
- **Raporty doręczeń (DLR):** `GET` i `POST` na `/providers/sms/dlr` (token w `?token=`) albo
  `/providers/sms/dlr/{token}`. Token jest porównywany z ustawieniem `SMSAPI_DLR_TOKEN` w czasie
  stałym; bez tokenu endpoint odpowiada 503. Mapowanie statusów:
  `DELIVERED` → `delivered`; `UNDELIVERED`, `EXPIRED`, `REJECTED`, `FAILED` → `failed`;
  `SENT`, `ACCEPTED` → `sent`.

---

## 5. Statystyki wykorzystania i koszty

Panel administratora (`/admin`) korzysta z dwóch endpointów; oba wymagają roli administratora.

### 5.1. SMS (`GET /api/stats/sms`)

- Koszt liczony jest z ceny zapisanej przy każdym wsadzie w chwili wysyłki
  (`message_batches.estimated_cost`), więc późniejsza zmiana ceny w panelu nie przelicza
  historii.
- Wysyłki symulowane (`DRY_RUN`, provider `fake`) nie są liczone jako zużycie ani koszt.
- `plannedCostMilli` — koszt zaplanowanych części SMS w okresie; `billedCostMilli` — koszt
  części przyjętych przez operatora (`sent`, `delivered` oraz `failed` z `provider_message_id`).
  Wiadomości odrzucone przed przyjęciem nic nie kosztują.
- Rozbicie: `totalMessages`, `totalParts`, `deliveredMessages`, `deliveredParts`, `sentMessages`,
  `sentParts`, `failedMessages`, `failedParts`, `inFlightMessages`, `inFlightParts`.

### 5.2. E-mail (`GET /api/stats/email`)

Agregacja `totalMessages`, `deliveredMessages`, `sentMessages`, `failedMessages` i
`inFlightMessages`. Statusy `delivered` i `failed` przychodzą tylko z webhooka SendGrid; przy
Mailpicie e-maile zostają na `sent`.

### 5.3. Parametry

Oba endpointy przyjmują `batch_id` (UUID wsadu), `from` i `to` (RFC 3339 albo `YYYY-MM-DD`).
Data bez godziny jest liczona w strefie `Europe/Warsaw`, a `to` obejmuje cały dzień
(przedział lewostronnie domknięty).

---

## 6. Uruchomienie lokalne

Pełna instrukcja jest w [README](../README.md) (sekcje *Getting started* i *Sending a test
message*). W skrócie: `make up`, `make migrate`, `make seed`, `make create-admin`, `make dev-api`,
`make dev-web`. Przy domyślnym `DRY_RUN=true` nic nie trafia do Mailpita; aby zobaczyć e-maile
pod <http://localhost:8025>, wyłącz tryb testowy w panelu, zostawiając Mailpit na `localhost`
i SMS `fake`.

---

## 7. Ustawienia wysyłki w panelu (ADR-0018)

Operatorów, nadawców, klucze API, cenę SMS, tryb testowy i import z Notion ustawia się
w **Panelu administratora → Ustawienia wysyłki** (`/admin/ustawienia`). Zmiany działają od
razu, bez restartu.

- Kolejność: wartość z panelu, potem zmienna z `.env`, potem domyślna. „Przywróć” usuwa wartość
  z panelu. W `.env` zostają tylko wartości startowe (baza, porty, `APP_ENV`,
  `SESSION_COOKIE_SECURE`, `DRY_RUN`, `DRY_RUN_LOCK`, klucz szyfrujący); zmienne operatorów są
  w `.env.example` zakomentowane jako wartości zapasowe.
- Klucze API są w bazie zaszyfrowane (AES-256-GCM) kluczem z `SETTINGS_ENCRYPTION_KEY` albo
  z pliku `SETTINGS_KEY_FILE` (domyślnie `backend/settings.key`, tworzony przy pierwszym starcie).
  **Zrób kopię tego klucza osobno od bazy.** Bez niego klucze API trzeba wpisać ponownie.
- Wyłączenie trybu testowego z operatorem wysyłającym poza komputer wymaga wpisania `WYSYŁAJ`
  i zgody opiekuna (rozdział 2).

---

## 8. Testy integracyjne

Przepływ dyspozytor → PostgreSQL → raporty doręczeń → statystyki obejmuje test
`backend/internal/providers/setup/delivery_integration_test.go`:

```bash
make up
make test-integration     # TEST_DATABASE_URL z .env (zob. .env.example)
# albo tylko pakiety providerów:
cd backend && TEST_DATABASE_URL="postgres://doyousend:doyousend@localhost:5432/doyousend?sslmode=disable" \
  go test -tags integration -count=1 -v ./internal/providers/...
```

Każdy test tworzy i usuwa własny schemat, więc może używać tej samej bazy co `make dev-api`.
Test:

1. buduje providerów przez `setup.New` z włączonym `DRY_RUN`,
2. planuje wsad i wysyła e-maile oraz SMS-y przez `delivery.Dispatcher`,
3. zapisuje wyniki i identyfikatory wiadomości w bazie,
4. symuluje raporty doręczeń przez `PGDeliveryReportConsumer` (statusy `delivered`),
5. sprawdza statusy, części i koszty w `sms.PGUsageStore` i `email.PGUsageStore`.

---

## 9. Import odbiorców z Notion (ADR-0017)

Odbiorców można wczytać z bazy danych w Notion zamiast z pliku CSV/XLSX. Wiersze przechodzą
**tę samą walidację i wykrywanie duplikatów** co plik (`recipients.Importer.CheckTable` /
`ImportTable`), więc raport wygląda tak samo. Integracja tylko czyta z Notion; nic tam nie zapisuje.

### Konfiguracja (jednorazowo)

Wszystko ustawia się w **Panelu administratora → Ustawienia → Import odbiorców z Notion**
(`/admin/ustawienia`). Zmienne `NOTION_*` w `.env` działają jako wartości zapasowe i są
jedynym źródłem konfiguracji dla komendy `make import-notion`.

1. W Notion: **Settings → Connections → Develop or manage integrations → New integration**,
   typ **Internal**, uprawnienie tylko **Read content**. Skopiuj *Internal Integration Secret*
   do pola **Token integracji Notion** (zapisywany zaszyfrowany, nigdy do repozytorium).
2. Otwórz bazę z odbiorcami → menu **⋯ → Connections → Add connections** → wybierz integrację.
   Bez tego API odpowiada `object_not_found`.
3. Skopiuj link do bazy (**⋯ → Copy link**) i wklej go w pole **Baza Notion z odbiorcami** —
   panel sam wyciągnie z niego identyfikator. Jeśli baza ma kilka źródeł danych (*data sources*),
   import to zgłosi z ich nazwami; wtedy wypełnij **Źródło danych Notion**
   (**⋯ → Manage data sources → Copy ID**).

### Kolumny

Najprościej nazwać kolumny w Notion tak jak w szablonie CSV: `Imię`, `Nazwisko`, `E-mail`,
`Telefon`, `Typ` (`rodzic`/`uczeń`) i opcjonalnie `Klasa`. Inne kolumny są pomijane.
Gdy kolumny nazywają się inaczej, mapowanie podaje się w polu **Mapowanie kolumn Notion**
(albo `NOTION_PROPERTY_MAP`):

```env
NOTION_PROPERTY_MAP=Name=first_name,Surname=last_name,Parent e-mail=email,Mobile=phone,Role=type,Class=classes
```

Obsługiwane typy właściwości: tytuł, tekst, e-mail, telefon, wybór, wielokrotny wybór (łączony
przecinkami — format listy klas), status, liczba, pole wyboru, formuła (tekst/liczba) i unikalne ID.
Daty, osoby, pliki i relacje są czytane jako puste.

### Uruchomienie

W panelu: **Odbiorcy → Import → Sprawdź bazę Notion**, potem **Zapisz poprawne wiersze**.
Raport wygląda jak przy pliku, a przy każdym wierszu jest link „otwórz w Notion”. Zapis trafia
do dziennika (`recipients_imported`, same liczby).

Z wiersza poleceń (konfiguracja tylko z `.env`):

```bash
make import-notion              # tylko raport: ile poprawnych, błędnych i duplikatów
make import-notion ARGS=-apply  # zapisuje poprawne wiersze (wszystkie albo żaden)
```

Raport podaje numer wiersza (nagłówek to wiersz 1, strony posortowane od najstarszej) i ID strony
w Notion, nigdy imion, adresów ani numerów. Limit to `MaxImportRows` (5000) — większa baza jest
odrzucana w całości, a nie importowana częściowo.

Notion ogranicza ruch do ok. **3 zapytań na sekundę** na integrację; klient pobiera po 100 stron
na zapytanie i przy odpowiedzi `429` czeka tyle, ile wskazuje `Retry-After` (maks. 30 s, do 4 prób).
Używana wersja API: `Notion-Version: 2025-09-03` (zapytania idą do
`POST /v1/data_sources/{id}/query`).
