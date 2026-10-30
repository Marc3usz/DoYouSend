# ADR-0013: Poprawianie brakującego kontaktu przez wysyłającego

- **Status:** propozycja — czeka na akceptację MichalK252 (domena B, kreator)
- **Data:** 2026-10-28
- **Uczestnicy:** Marc3usz (domeny A i D: odbiorcy, uprawnienia), MichalK252 (domena B: kreator)

## Kontekst

`description.md` („Brak danych kontaktowych”): gdy odbiorca nie ma e-maila albo telefonu,
wysyłający powinien móc **poprawić dane**, wykluczyć osobę albo wysłać tylko dostępnym kanałem.
Wykluczanie i wysyłka częściowa działają (#61, #66). Poprawić danych wysyłający nie może: cała
edycja odbiorców (`PUT /recipients/{id}`) jest tylko dla administratora (ADR-0010), a wysyłający
nie widzi danych kontaktowych w ogóle (`/recipients/lookup`, #68).

Jednocześnie „Bezpieczeństwo” wymaga, żeby użytkownik widział tylko dane potrzebne do pracy.
Pełna edycja odbiorcy przez wysyłającego oznaczałaby wgląd w całą bazę kontaktów.

## Decyzja

Osobny, wąski endpoint `PATCH /recipients/{id}/contact` dla każdego zalogowanego:

- ustawia e-mail i/lub telefon **tylko w kanale, który ma problem** (`issues`: `missing` albo
  `invalid`); działającego kontaktu nie da się nim nadpisać — `409 contact_not_fixable`;
- niczego nie odczytuje: odpowiedź to `RecipientLookup` (bez e-maila i telefonu), więc po
  zapisie widać tylko, czy problem zniknął;
- walidacja, normalizacja i wykrywanie duplikatów jak przy edycji przez administratora, ale
  `409 duplicate_contact` dla wysyłającego nie zawiera `ids` — nie dowie się, czyj to kontakt;
- każda poprawka trafia do dziennika jako `recipient_contact_fixed` z nazwami kanałów, bez
  wartości (dziennik nie zawiera danych kontaktowych — `docs/bezpieczenstwo.md`).

Kreator (domena B) pokazuje przy osobie z brakiem przycisk „Popraw” obok „Wyklucz z wysyłki”.

## Rozważane alternatywy

- **Pełna edycja odbiorcy dla wysyłającego** — odrzucone: wymaga pokazania danych
  kontaktowych całej bazy, sprzeczne z „widzi tylko dane potrzebne do pracy”.
- **Prośba do administratora (zgłoszenie, bez edycji)** — odrzucone jako jedyna droga: przed
  pilną wysyłką administrator może być nieosiągalny, a wymaganie mówi wprost „poprawić dane”.
  Administrator nadal może poprawić wszystko w `/odbiorcy`.
- **Zastąpienie działającego kontaktu** — odrzucone: wysyłający mógłby przekierować
  wiadomości do rodzica na dowolny adres; tu wolno tylko uzupełnić to, co nie działa.

## Konsekwencje

- `iam/policy.go`: reguła `PATCH /api/recipients/{id}/contact` → `AccessUser` przed ogólną
  regułą `/api/recipients` (admin). Ścieżka ma zmienne `{id}`, więc reguła dopasowuje sufiks.
- `recipients.Service.FixContact` — nowa metoda; `iam` dostaje akcję `recipient_contact_fixed`,
  podpiętą w `cmd/api` tak jak audyt wsadów.
- Ryzyko: wysyłający może wpisać błędny adres w miejsce brakującego. Ślad zostaje w dzienniku,
  a administrator widzi i poprawia pełne dane w `/odbiorcy`.
- Kreator: przycisk „Popraw” w liście osób z brakami (#66) — do zrobienia po scaleniu kontraktu.
