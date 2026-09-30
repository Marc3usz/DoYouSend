# ADR-0008: Wysyłka e-maili przez SendGrid

- **Status:** propozycja — czeka na akceptację MichalK252 (DEV C, właściciel `providers`)
- **Data:** 2026-09-30
- **Uczestnicy:** Marc3usz (DEV A) proponuje, MichalK252 (DEV C) decyduje i implementuje
- **Zmienia:** część „E-mail” w ADR-0003. Część „SMS” (SMSAPI) zostaje bez zmian.

## Kontekst

ADR-0003 (w wersji z PR #11) zakłada dla e-maili dowolny serwer SMTP za adapterem
`email.Mailpit`. SMTP wystarcza, żeby wiadomość wysłać, ale nie spełnia dwóch wymagań
z `description.md`:

- **status każdego kanału dla każdego odbiorcy**, łącznie z „dostarczono, jeżeli operator
  udostępnia taką informację”. Przy zwykłym SMTP wiemy tylko, że serwer przyjął wiadomość.
  O odbiciu (bounce) dowiadujemy się później, z e-maila zwrotnego, którego nikt nie parsuje.
- **ponowna wysyłka nieudanych**. `Provider.Send` ma rozróżniać błąd trwały od błędu do
  ponowienia, a z kodów SMTP robi się to nieprecyzyjnie.

Chcemy jednego operatora e-mail z raportami doręczeń, tak jak SMSAPI daje je dla SMS-ów.

## Decyzja

E-maile wysyłamy przez **SendGrid (Twilio), Web API v3** (`POST /v3/mail/send`).

- **Bez SDK.** Adapter `internal/providers/email/sendgrid.go` używa zwykłego `net/http`,
  tak jak planowany adapter SMSAPI. `go.mod` się nie zmienia.
- **Jedno wywołanie na odbiorcę.** Wysyłamy pojedynczo (`description.md`: bez ujawniania
  danych innych osób). Każdy wiersz `deliveries` ma wtedy własny wynik i własne ponowienie
  (`CLAUDE.md`, zasada 5). Nie używamy `personalizations` dla wielu odbiorców naraz.
- **ID wiadomości.** Nagłówek `X-Message-Id` z odpowiedzi `202 Accepted` trafia do
  `deliveries.provider_message_id`. Dodatkowo w `custom_args` przekazujemy nasze
  `deliveries.id`, żeby raport dało się połączyć z wierszem nawet bez ID operatora.
- **Klasyfikacja błędów w `Send`:**
  - `400`, `401`, `403`, `413` → błąd trwały (zła treść, zły klucz, brak uprawnień),
  - `429` i `5xx` → błąd do ponowienia (z `Retry-After`, jeśli jest).
- **Raporty doręczeń przez Event Webhook**, z weryfikacją podpisu (Signed Event Webhook).
  Parser zwraca `providers.DeliveryReport`, tak jak `sms.ParseSMSAPIDLR`:

  | Zdarzenie SendGrid | `deliveries.status` |
  |---|---|
  | `processed`, `deferred` | `sent` (wiadomość jest u operatora, nie cofamy statusu) |
  | `delivered` | `delivered` |
  | `bounce`, `dropped` | `failed` (z powodem w `deliveries.error`) |
  | `open`, `click`, `spamreport`, `unsubscribe` i nieznane | ignorowane, logowane po ID |

- **Treść bez zmian (`CLAUDE.md`, zasada 4).** Wysyłamy `text/plain` z dokładnie tym samym
  `rendered_body`, które idzie SMS-em. Po stronie SendGrid **wyłączamy click tracking**,
  bo podmienia linki w treści na przekierowania: odbiorca dostałby w e-mailu inny link
  niż w SMS-ie. Open tracking też wyłączamy (i tak działa tylko dla HTML).
- **Lokalnie i w CI bez zmian:** Mailpit i `DRY_RUN=true`. Adapter SendGrid wybiera
  `EMAIL_PROVIDER=sendgrid`. Do testów integracyjnych służy `mail_settings.sandbox_mode`:
  SendGrid sprawdza żądanie, ale nic nie wysyła.

Nowe zmienne środowiskowe (dopisać do `.env.example` z placeholderami w PR z adapterem):
`SENDGRID_API_KEY`, `SENDGRID_WEBHOOK_PUBLIC_KEY`, `SENDGRID_SANDBOX`. `EMAIL_FROM` zostaje.

## Rozważane alternatywy

- **Zostać przy dowolnym SMTP (ADR-0003).** Najprościej i bez przywiązania do dostawcy, ale
  bez statusu „dostarczono” i z nieprecyzyjną klasyfikacją błędów. Odrzucone z powodów
  opisanych w „Kontekście”.
- **SendGrid przez SMTP Relay** (obecny adapter, zmiana tylko `SMTP_HOST`). Event Webhook
  działa także dla relaya, ale ID wiadomości i `custom_args` trzeba przemycać w nagłówku
  `X-SMTPAPI`, a błędy dalej przychodzą jako kody SMTP. Web API daje to samo prościej.
- **SendGrid także do SMS-ów.** Niemożliwe: SendGrid wysyła tylko e-maile. SMS-y w tej
  firmie to osobny produkt (Twilio Programmable Messaging), wyraźnie droższy do Polski niż
  SMSAPI. Dlatego SMS zostaje przy SMSAPI (ADR-0003).
- **Amazon SES, Mailgun.** Podobne możliwości. Nie widzimy przewagi, która uzasadniałaby
  zmianę propozycji. Wracamy do nich, jeśli warunki SendGrid (niżej) okażą się nie do
  przyjęcia.

## Do sprawdzenia przed akceptacją

1. **Plan i koszt.** Aktualne warunki darmowego lub testowego planu SendGrid i limit
   dzienny. Masowa wysyłka do całej szkoły to kilka tysięcy e-maili naraz.
2. **Dane osobowe.** Adresy e-mail rodziców i uczniów trafiają do operatora z USA. Trzeba
   sprawdzić z opiekunem projektu umowę powierzenia (DPA) i opcję przechowywania danych w UE.
3. **Domena nadawcy.** Uwierzytelnienie domeny szkoły (SPF, DKIM) w SendGrid, inaczej
   e-maile trafią do spamu.
4. **Publiczny endpoint webhooka.** Tak jak DLR z SMSAPI, wymaga routingu i zabezpieczenia
   po stronie DEV D.

## Konsekwencje

- `internal/providers` dostaje adapter `email/sendgrid.go` i parser Event Webhooka. Reszta
  kodu nadal zna tylko interfejs `providers` (ADR-0003).
- Mailpit zostaje adapterem lokalnym i domyślnym. Przejście na SendGrid w produkcji wymaga
  zgody opiekuna projektu (`CLAUDE.md`, zasada 3).
- Status `delivered` dla e-maili staje się osiągalny, więc historia wysyłki pokazuje oba
  kanały tak samo.
- ADR-0003 po akceptacji tego ADR-u powinien w części „E-mail” odsyłać tutaj.
