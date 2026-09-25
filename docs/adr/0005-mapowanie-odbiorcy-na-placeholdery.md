# ADR-0005: Mapowanie Recipient → placeholdery w messaging.Render

- **Status:** propozycja — decyzja jeszcze nie podjęta, czeka na wspólną sesję A + B
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

Jeszcze nie podjęta. Do ustalenia na wspólnej sesji A + B przed startem M3.

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

Do uzupełnienia po decyzji. Na razie: `recipients` i `messaging` pozostają od siebie
niezależne (żadna z paczek nie importuje drugiej) — zgodnie z `backend/CLAUDE.md`
("zależności między pakietami domenowymi idą przez interfejsy definiowane przez konsumenta").
