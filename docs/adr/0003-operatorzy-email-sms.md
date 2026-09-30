# ADR-0003: Operator e-mail i bramka SMS

- **Status:** przyjęta
- **Data:** 2026-09-22 (propozycja), 2026-09-30 (decyzja)
- **Uczestnicy:** DEV C (MichalK252) prowadzi, decyzja zespołowa

## Kontekst

`description.md` zostawia wybór operatorów zespołowi i dopuszcza konsultację z programistami
Techni. Wybór wpływa na koszty, raporty doręczeń i formalności (nazwa nadawcy SMS w Polsce
wymaga rejestracji u operatora).

## Decyzja

### E-mail: SMTP (Mailpit → dowolny serwer SMTP)

Adapter `email.Mailpit` działa przez standardowy protokół SMTP, więc nie zależy od żadnego
dostawcy. Lokalnie łączy się z Mailpitem (port 1025, bez auth, nic nie wychodzi na zewnątrz).
W środowisku produkcyjnym wystarczy podmienić `SMTP_HOST`, `SMTP_PORT` i dane uwierzytelniające
na dowolny serwer SMTP (szkolny serwer pocztowy, SendGrid SMTP Relay, Amazon SES SMTP itp.).
Nie potrzebujemy dedykowanego SDK.

### SMS: SMSAPI.pl

Wybieramy **SMSAPI** (https://www.smsapi.pl/) z następujących powodów:

1. **Masowa wysyłka i raporty doręczeń (DLR)** — callback URL (webhook) ze statusami
   `DELIVERED`, `UNDELIVERED`, `EXPIRED`, `REJECTED`. Pozwala aktualizować `deliveries.status`
   niemal w czasie rzeczywistym.
2. **Konto testowe/sandbox** — dostępne bez kosztów, z limitem 50 SMS-ów i dedykowanym
   numerem testowym. Wystarczające do developmentu i CI.
3. **Polskie znaki i UCS-2** — SMSAPI poprawnie liczy części przy znakach spoza GSM-7.
   Nasze `messaging.MeasureSMS` liczy części po stronie backendu, SMSAPI weryfikuje.
4. **Koszt** — ok. 0.07-0.09 PLN/SMS w prepaidzie, zgodne z `SMS_PRICE_PER_PART_PLN=0.08`
   w `.env.example`.
5. **API** — proste REST API z kluczem Bearer, bez ciężkiego SDK.

Adapter SMSAPI będzie w `internal/providers/sms/smsapi.go`, a parser webhooków DLR
w `internal/providers/sms/smsapi_dlr.go`. Lokalnie i z `DRY_RUN=true` nadal używamy
providera `fake`.

## Konsekwencje

- Wybór operatora nie wycieknie poza `internal/providers`; reszta kodu zna tylko interfejs.
- Klucze trafiają wyłącznie do `.env` (`SMS_API_KEY`, `SMS_API_SECRET`); przejście na
  wysyłkę produkcyjną wymaga zgody opiekuna projektu.
- Webhook DLR musi trafić na publiczny endpoint (`POST /providers/sms/dlr`), który DEV D
  uwzględni w routingu i zabezpieczy walidacją podpisu lub IP allowlist od SMSAPI.
- Jeśli SMSAPI przestanie działać lub zmieni warunki, podmiana na innego operatora wymaga
  jedynie nowego adaptera w `internal/providers/sms/` i wpisu w fabryce
  (`providers/setup`). Reszta kodu się nie zmienia.
