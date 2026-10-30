# ADR-0014: Oficjalne SDK SendGrid za adapterem i region UE

- **Status:** propozycja
- **Data:** 2026-10-30
- **Uczestnicy:** Marc3usz (DEV A) proponuje, MichalK252 (DEV C, właściciel `providers`) decyduje
- **Zmienia:** punkt „Bez SDK” w ADR-0008. Reszta ADR-0008 (Web API v3, jedno wywołanie na
  odbiorcę, klasyfikacja błędów, Event Webhook, wyłączony tracking) zostaje bez zmian.

## Kontekst

- ADR-0008 wybrał SendGrid Web API v3 i **świadomie bez SDK**: adapter `email/sendgrid.go`
  składa JSON ręcznie (własne struktury `sgMailSend`, `sgPersonalization` itd.) i woła
  `net/http`. Działa i ma ok. 600 linii testów.
- Aplikacja będzie działać w UE, więc adresy e-mail rodziców i uczniów mają trafiać do
  regionu UE SendGrid (`api.eu.sendgrid.com`). Dziś host jest na sztywno
  (`sendgridAPIURL = "https://api.sendgrid.com"`), a `setup.go` nie pozwala go zmienić.
  To jeden z warunków produkcyjnych z ADR-0008 (punkt „Dane osobowe”).
- Oficjalne SDK (`github.com/sendgrid/sendgrid-go`, wersja `v3.16.1+incompatible`) ma gotowe
  typy żądania `mail.SGMailV3` oraz `SetDataResidency` z mapą hostów `eu` / `global`.
- `CLAUDE.md` wymaga ADR-u dla każdej nowej zależności uruchomieniowej, więc zmiana
  z ADR-0008 musi być zapisana.

Sprawdzone w kodzie SDK, bo zmienia to sposób użycia:

- `sendgrid.Client.SendWithContext` wysyła przez **globalny** `DefaultClient`
  (`rest.DefaultClient = &Client{HTTPClient: &http.Client{}}`), czyli bez timeoutu i ze
  stanem globalnym. `CLAUDE.md` zabrania stanu globalnego, a adapter dziś ma timeout 15 s
  i wstrzykiwalnego klienta (testy).
- SDK ciągnie drugą zależność, `github.com/sendgrid/rest` (`v2.6.9+incompatible`).
  Oba moduły są bez `go.mod` (stąd `+incompatible`).
- Ostatnie stabilne wydanie to `v3.16.1` z 2025-05-29. Nowsza linia `v4` istnieje tylko jako
  kandydaci (`v4.0.0-rc.10`), więc przypinamy stabilną `v3`. `SetDataResidency` jest opisane
  w dokumentacji pakietu tak samo, jak użyte w adapterze: tylko `eu` i `global`, a dokumentacja
  nic nie mówi o kluczach ani kontach per region.
- `rest.Client` pozwala podać własny `*http.Client`, a `rest.Response` zwraca nagłówki
  (potrzebny `X-Message-Id`). Kody 4xx/5xx nie są błędem Go, więc klasyfikację
  trwały/do ponowienia zostawiamy po naszej stronie.

## Decyzja

Budowę żądania `mail/send` oraz wybór hosta regionu przenosimy na oficjalne SDK, ale
**wysyłamy własnym klientem**: `rest.Client{HTTPClient: <nasz klient z timeoutem>}`,
nigdy przez `sendgrid.DefaultClient`. Region wybiera nowa zmienna `SENDGRID_REGION`
(`global` | `eu`).

Zakres jest wąski: tylko ścieżka `Send` w `email/sendgrid.go`. Interfejs `providers.Provider`,
klasyfikacja błędów, `X-Message-Id`, tracking wyłączony, sandbox, `custom_args`, Event Webhook
i weryfikacja podpisu zostają bez zmian. Istniejące testy adaptera muszą przechodzić bez
modyfikacji asercji. To nasza siatka bezpieczeństwa, że treść e-maila nadal jest bajt w bajt
taka jak SMS (`CLAUDE.md`, zasada 4).

## Rozważane alternatywy

- **Zostać przy `net/http` (ADR-0008).** Zero nowych zależności, pełna kontrola i działające
  testy. Odpada tylko dlatego, że struktury żądania trzeba utrzymywać ręcznie, a region UE
  wymagałby własnej tablicy hostów. To jest przewaga niewielka i przyznajemy to wprost:
  ta alternatywa jest akceptowalna, jeśli właściciel domeny C uzna dwie zależności
  `+incompatible` za zbyt drogie.
- **SDK w pełni, z `sendgrid.NewSendClient(key).SendWithContext`.** Najkrótszy kod, ale
  globalny `DefaultClient` bez timeoutu i bez możliwości wstrzyknięcia w testach. Odrzucone.
- **SDK także do `GET /v3/user/credits` i statystyk.** SDK nie ma dla nich typowanych
  pomocników, dałby tylko `rest.Request`, więc zysku nie ma. Zostają przy `net/http`.
- **Osobny `SENDGRID_BASE_URL` zamiast `SENDGRID_REGION`.** Elastyczniejszy, ale pozwala wpisać
  dowolny host (np. pomyłkowo produkcyjny `global`). Dwie wartości są bezpieczniejsze.
  `SendGridConfig.BaseURL` zostaje wyłącznie do testów (`httptest.Server`) i ma pierwszeństwo.

## Konsekwencje

- **Dobre:** typy żądania utrzymuje producent, region UE to jedna zmienna, a host dla
  `mail/send` i `credits` jest wybierany w jednym miejscu.
- **Złe:** dwie nowe zależności uruchomieniowe spoza stdlib, obie `+incompatible` i bez
  gwarancji semver. Ostatnie stabilne wydanie ma ponad rok, a następna linia (`v4`) jest
  w wersjach `rc`, więc za jakiś czas możemy stanąć przed migracją. Rośnie powierzchnia łańcucha dostaw, a aktualizacje trzeba świadomie
  przeglądać. Adapter nadal ma własny kod HTTP dla `credits`, więc nie znika całkowicie.
- **Domyślnie `SENDGRID_REGION=global`** (zachowanie sprzed zmiany), żeby PR nie zmieniał
  niczego po cichu. Przed produkcją trzeba ustawić `eu` (warunek z ADR-0008).
- **Do sprawdzenia przed ustawieniem `eu` (nie zweryfikowane w tym ADR-ze):** czy klucz API
  i konto muszą być założone w regionie UE (klucz z `api.sendgrid.com` może nie działać na
  `api.eu.sendgrid.com`), jaki plan to umożliwia oraz czy Event Webhook i uwierzytelnienie
  domeny nadawcy też są per region. Zweryfikować w dokumentacji i w panelu SendGrid.
- **Poza zakresem:** rezydencja danych bazy, hostingu i logów (osobna decyzja, dotyka
  `platform/` i wdrożenia) oraz region SMSAPI (`api.smsapi.pl`).
- Do zrobienia po akceptacji: zmienna `SENDGRID_REGION` w `.env.example` i
  `docs/integracje.md`, odsyłacz w ADR-0008 (zrobiony w tym PR-ze).
