# Architektura

## Widok ogólny

```
SvelteKit (TS)  --/api/*-->  Go API  --SQL-->  PostgreSQL
                               |                  ^
                               |                  | statusy (deliveries)
                               +--> worker wysyłki (w procesie API)
                                              |
                                +-------------+-------------+
                                |                           |
                        provider e-mail              bramka SMS
                   (Mailpit lokalnie, SendGrid)  (fake lokalnie, SMSAPI)
                                |                           |
                                +---- webhooki raportów ----+
                                        doręczeń -> API
```

Nie ma osobnej kolejki: stan wysyłki to wiersze `deliveries` w PostgreSQL, a worker działa
w procesie API (`backend/internal/delivery`). Redis z pierwotnego planu (ADR-0001) nigdy nie
został użyty.

Backend jest podzielony na pakiety domenowe (`internal/<domena>`), a nie na warstwy
techniczne. Pakiet domenowy ma własne handlery HTTP, logikę i dostęp do bazy. Zależności
między domenami idą przez interfejsy deklarowane po stronie konsumenta. Konfiguracja
operatorów pochodzi z ustawień w panelu (`internal/settings`, ADR-0018), z `.env` jako
wartością zapasową.

## Przepływ wysyłki

1. **Kompozycja** (frontend `wiadomosci/`, `messaging`) — użytkownik wybiera odbiorców i grupy,
   kanały (oba, tylko e-mail albo tylko SMS; ADR-0019), wpisuje temat e-maila i jedną wspólną
   treść.
2. **Rozwinięcie odbiorców** (`groups.Resolve`) — z wybranych grup i osób powstaje lista
   bez duplikatów; osoby bez kompletu danych są oznaczone. Wysyłający może poprawić dane albo
   wykluczyć osobę z wysyłki (ADR-0013).
3. **Podsumowanie** (`messaging`) — długość treści, liczba części SMS na odbiorcę, liczba
   odbiorców, łączna liczba SMS-ów, przewidywany koszt.
4. **Potwierdzenie** (`POST /api/batches` z `Idempotency-Key`) — zapis `message_batches` +
   `batch_recipients` z utrwaloną, już spersonalizowaną treścią i wierszy `deliveries`
   (odbiorca × kanał). Od tego momentu treść jest niezmienna, a wsad ma status `running`.
5. **Wysyłka** (`delivery.Dispatcher`) — oba kanały odbiorcy startują razem; każda dostawa
   woła providera i zapisuje status, liczbę prób, błąd i identyfikator wiadomości. Błędy
   przejściowe są ponawiane automatycznie z odstępem. Błąd jednej dostawy nie dotyka pozostałych.
6. **Raporty doręczeń** — webhooki SendGrid i SMSAPI aktualizują `deliveries.status` na
   `delivered` albo `failed` (ADR-0011).
7. **Domknięcie** — gdy wszystkie dostawy się skończą, wsad dostaje status `done` albo
   `done_with_errors`.
8. **Ponowna wysyłka** (`POST /api/batches/{id}/retry`) — ponownie wysyła nieudane dostawy,
   o których wiadomo, że nie wyszły, na aktualny adres odbiorcy. Liczba prób się sumuje,
   ostatni błąd zostaje do udanej próby, a wsad zapisuje liczbę i czas ponowień.
9. **Wznowienie po restarcie** — przy starcie API wsady, które zostały w `running` po awarii,
   są wznawiane: dostawy `pending` idą do wysyłki, a te zawieszone w `sending` dostają status
   `failed` z informacją o przerwaniu, więc da się je ponowić.

## Statusy

- Kanał (per odbiorca, per kanał): `pending` -> `sending` -> `sent` -> `delivered`, albo `failed`.
- Wsad: `running` -> `done` | `done_with_errors`. Typ `batch_status` w bazie zna też `draft`,
  `scheduled` i `cancelled`, ale MVP ich nie używa — wysyłka startuje od razu po
  potwierdzeniu (ADR-0015).

Statusy obu kanałów są niezależne. Odbiorca, do którego poszedł tylko jeden kanał,
ma `batch_recipients.is_partial = true`.

## Kluczowe decyzje projektowe

- **Identyczność treści** wymuszona strukturą: jedna kolumna `body` we wsadzie i jedna
  `rendered_body` per odbiorca; oba kanały czytają to samo pole. Temat istnieje wyłącznie
  dla e-maila.
- **Brak duplikatów** wymuszony bazą: `UNIQUE (batch_id, recipient_id)`.
- **Odporność na błędy** wymuszona ziarnistością: jednostką pracy jest pojedyncza dostawa,
  nie wsad; ponowna wysyłka to reset pojedynczych wierszy `deliveries` ze statusem `failed`.
- **Wymienność providerów**: `delivery` zna tylko interfejs, nigdy SDK dostawcy — dzięki temu
  wybór operatora (ADR-0003) można zmienić bez ruszania logiki.
- **Historia niezmienna**: wysłanego wsadu się nie edytuje. „Użyj jako podstawy” wypełnia
  nową wiadomość w kreatorze treścią, grupami i odbiorcami starej; szkiców nie zapisujemy.
