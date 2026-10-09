# ADR-0005: Mapowanie Recipient → placeholdery w messaging.Render

- **Status:** przyjęta (2026-10-07) — zaproponowana przez S1D0R-10 (DEV B), zaakceptowana przez
  Marc3usz (DEV A) w review PR #25
- **Data:** 2026-09-25
- **Uczestnicy:** Marc3usz (DEV A), S1D0R-10 (DEV B)

## Kontekst

`backend/internal/recipients` (PR #4) udostępnia typ `Recipient` (imię, nazwisko, e-mail,
telefon, typ). `backend/internal/messaging.Render` (PR #2) personalizuje treść na podstawie
`map[string]string` z nazwami placeholderów (np. `imie`, `nazwisko`).

Nie ustalono jeszcze:

- kto mapuje `Recipient` → `map[string]string` — `recipients`, `messaging`, czy `delivery`
  jako spoiwo między nimi,
- jakie są kanoniczne nazwy placeholderów (`imie` czy `first_name`? czy dopuszczamy oba?),
- co się dzieje, gdy odbiorca nie ma wartości dla użytego w szablonie placeholdera (błąd
  importu/wysyłki czy puste pole) — musi to być spójne z regułą, że treść e-mail/SMS jest
  identyczna per odbiorca (`CLAUDE.md`, zasada 4).

Zgłoszone podczas review PR #4 przez S1D0R-10 (zob. komentarz na
https://github.com/Marc3usz/DoYouSend/pull/4) jako punkt do ustalenia, zanim ktokolwiek
zacznie spinać `recipients`/`groups` z `messaging`/`delivery` (M3 z `docs/podzial-pracy.md`).

## Decyzja

Wdrożona w `POST /api/messages/preview` (`messaging/placeholders.go`, PR #25):

- **Mapowanie po stronie `messaging`.** `messaging.RecipientFields(firstName, lastName)`
  zwraca `map[string]string` dla `Render`. Nazwy placeholderów są częścią treści wiadomości,
  więc zostają w domenie B. `messaging` i tak zależy już od `groups` (wynik `Resolve`), a
  `groups` od `recipients`.
- **Kanoniczne nazwy:** `imie`, `nazwisko`. Wielkość liter ma znaczenie, spacje wewnątrz
  `{{ imie }}` są ignorowane. Innych nazw (np. `first_name`) nie przyjmujemy, żeby jedna
  treść nie miała dwóch zapisów.
- **Nieznana nazwa** (`{{klasa}}`) jest zgłaszana w podglądzie (`unknownPlaceholders`)
  jeszcze przed wysyłką. Treść z taką nazwą nie zostanie wysłana nikomu.
- **Pusta wartość u odbiorcy** oznacza błąd tylko tego odbiorcy (`renderFailedIds`). Reszta
  wsadu idzie dalej. Nie podstawiamy pustego tekstu, bo e-mail i SMS muszą być identyczne
  i kompletne.

## Rozważane alternatywy

- **Mapowanie po stronie `recipients`** — `recipients` eksportuje
  `func (r Recipient) Placeholders() map[string]string`. Plus: jedno źródło prawdy o nazwach
  pól blisko definicji `Recipient`. Minus: `recipients` musiałby znać nazewnictwo
  placeholderów, które jest domeną `messaging`.
- **Mapowanie po stronie `messaging`** — `messaging` przyjmuje `Recipient` (albo węższy
  interfejs) i sam czyta pola. Plus: nazwy placeholderów zostają w domenie B. Minus:
  `messaging` zyskuje zależność od typu z `recipients`.
- **Osobna funkcja mapująca w `delivery`, na styku obu domen** — plus: żadna z domen A/B nie
  zależy od drugiej bezpośrednio. Minus: kolejny plik na styku, kolejny punkt koordynacji przy
  zmianie pól.

## Konsekwencje

- `messaging` importuje `groups` (typy `Selection`, `Resolution`, `Resolved`), a przez nie
  pośrednio `recipients.Recipient`. `recipients` nie zna nazw placeholderów i nie importuje
  `messaging`. Kierunek zależności jest jeden: B → A.
- `delivery` używa tego samego `messaging.RecipientFields` przy budowie planu wysyłki (PR #29, w review),
  więc podgląd i faktyczna wysyłka personalizują treść identycznie.
- Nowy placeholder (np. `{{klasa}}` po ADR-0009) to zmiana w `messaging/placeholders.go`
  i w argumentach `RecipientFields`, bez zmian w `recipients`.
- Pusta wartość jest rzadka, bo walidacja w `recipients` wymaga imienia i nazwiska. Dotyczy
  głównie danych wpisanych z pominięciem walidacji.
