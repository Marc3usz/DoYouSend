# ADR-0003: Operator e-mail i bramka SMS

- **Status:** przyjęta
- **Data:** 2026-09-22 (propozycja), 2026-09-30 (decyzja)
- **Uczestnicy:** DEV C (MichalK252) prowadzi, decyzja zespołowa

## Kontekst

`description.md` zostawia wybór operatorów zespołowi i dopuszcza konsultację z programistami
Techni. Wybór wpływa na koszty, raporty doręczeń i formalności (nazwa nadawcy SMS w Polsce
wymaga rejestracji u operatora).

## Decyzja

### E-mail: Mailpit lokalnie, docelowo SendGrid (ADR-0008)

- **Lokalnie i w testach:** Adapter `email.Mailpit` działa przez standardowy protokół SMTP
  (port 1025 w `docker compose`, bez auth, `DRY_RUN=true`, nic nie wychodzi na zewnątrz).
- **Środowisko produkcyjne:** Wstępnie planowano dowolny serwer SMTP, jednak ze względu na
  wymóg raportów doręczeń per odbiorca (status `delivered`) oraz konieczność wyłączenia
  click trackingu (zasada identyczności treści e-mail/SMS z `CLAUDE.md`), wybór docelowego
  dostawcy e-mail reguluje **ADR-0008** (SendGrid Web API v3).

### SMS: SMSAPI.pl

Wybieramy **SMSAPI** (https://www.smsapi.pl/) z następujących powodów:

1. **Raporty doręczeń (DLR):**
   - Callback URL (webhook) wywoływany przez SMSAPI metodą GET z parametrami w URL:
     `MsgId` (ID wiadomości), `status` (kod liczbowy statusu, np. 404 = DELIVERED, 405 = UNDELIVERED),
     `status_name`, `donedate` (unixtime) oraz opcjonalny `idx`.
   - SMSAPI obsługuje paczkowanie: przy wielu wiadomościach wartości w parametrach
     są rozdzielone przecinkami (np. `MsgId=id1,id2&status=404,405`).
   - Endpoint odbiorczy musi odpowiedzieć zwykłym tekstem `OK`, inaczej SMSAPI ponawia
     wysyłkę raportu cyklicznie.
2. **Konto testowe/sandbox:** Dostępne bez kosztów, z limitem testowym i dedykowanym
   środowiskiem. Wystarczające do developmentu i CI.
3. **Polskie znaki i UCS-2:** SMSAPI poprawnie liczy części przy znakach spoza GSM-7.
   Nasze `messaging.MeasureSMS` liczy części po stronie backendu, SMSAPI weryfikuje.
4. **Koszt:** Ok. 0.07-0.09 PLN/SMS w prepaidzie, zgodne z `SMS_PRICE_PER_PART_PLN=0.08`
   w `.env.example`.
5. **API:** Proste REST API z kluczem Bearer, bez konieczności instalowania zewnętrznych SDK.

Adapter SMSAPI znajduje się w `internal/providers/sms/smsapi.go` (za wspólnym interfejsem
`providers.Provider`), a parser callbacków DLR w `internal/providers/sms/smsapi_dlr.go`.
Lokalnie i z `DRY_RUN=true` nadal domyślnie używamy providera `fake`. Wybór adaptera produkcyjnego
sterowany jest przez `SMS_PROVIDER=smsapi`.

## Konsekwencje

- Wybór operatora nie wycieka poza `internal/providers`; reszta kodu (zwłaszcza `delivery`)
  zna wyłącznie wspólny interfejs `providers.Provider` oraz kanoniczny model `providers.DeliveryReport`.
- Klucze trafiają wyłącznie do `.env` (`SMS_API_KEY`, `SMS_API_SECRET`).
- Webhook DLR SMSAPI wymaga publicznego endpointu (`GET /providers/sms/dlr`), który:
  - parsuje parametry `MsgId`, `status`, `donedate` (obsługując wartości pojedyncze i listy po przecinku),
  - aktualizuje `deliveries.status`,
  - zwraca odpowiedź HTTP 200 z treścią `OK`.
- Endpoint callbacku SMSAPI powinien zostać zabezpieczony przez DEV D na poziomie routingu / reverse proxy
  poprzez weryfikację adresów IP serwerów SMSAPI (`89.174.81.98, 91.185.187.219, 213.189.53.211, 31.186.83.18, 212.91.26.253`).
