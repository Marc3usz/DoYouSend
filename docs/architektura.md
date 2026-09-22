# Architektura

## Widok ogólny

```
SvelteKit (TS)  --/api/*-->  Go API  --SQL-->  PostgreSQL
                                 |
                                 +--> kolejka (Redis) --> worker wysyłki
                                                              |
                                                +-------------+-------------+
                                                |                           |
                                        provider e-mail              bramka SMS
                                        (Mailpit lokalnie)        (fake lokalnie)
```

Backend jest podzielony na pakiety domenowe (`internal/<domena>`), a nie na warstwy
techniczne. Pakiet domenowy ma własne handlery HTTP, logikę i dostęp do bazy. Zależności
między domenami idą przez interfejsy deklarowane po stronie konsumenta.

## Przepływ wysyłki

1. **Kompozycja** (`messaging`) — użytkownik wybiera odbiorców/grupy, wpisuje temat e-maila
   i jedną wspólną treść.
2. **Rozwinięcie odbiorców** (`groups.Resolve`) — z wybranych grup i osób powstaje lista
   bez duplikatów; osoby bez kompletu danych są oznaczone.
3. **Podsumowanie** (`messaging`) — długość treści, liczba części SMS na odbiorcę, liczba
   odbiorców, łączna liczba SMS-ów, przewidywany koszt.
4. **Potwierdzenie** — zapis `message_batches` + `batch_recipients` z utrwaloną, już
   spersonalizowaną treścią. Od tego momentu treść jest niezmienna.
5. **Kolejkowanie** (`delivery`) — po jednym zadaniu na (odbiorca x kanał), czyli po jednym
   wierszu `deliveries`. Oba kanały startują w tym samym momencie.
6. **Wysyłka** — worker bierze zadanie, woła providera, zapisuje status i identyfikator
   wiadomości. Błąd jednego zadania nie dotyka pozostałych.
7. **Raporty doręczeń** — provider oddaje statusy (webhook lub odpytywanie), worker
   aktualizuje `deliveries.status` na `delivered` albo `failed`.
8. **Domknięcie** — gdy wszystkie zadania wsadu się skończą, wsad dostaje status
   `done` albo `done_with_errors`.

## Statusy

- Kanał (per odbiorca, per kanał): `pending` -> `sending` -> `sent` -> `delivered`, albo `failed`.
- Wsad: `draft` -> `scheduled` -> `running` -> `done` | `done_with_errors` | `cancelled`.

Statusy obu kanałów są niezależne. Odbiorca, do którego poszedł tylko jeden kanał,
ma `batch_recipients.is_partial = true`.

## Kluczowe decyzje projektowe

- **Identyczność treści** wymuszona strukturą: jedna kolumna `body` we wsadzie i jedna
  `rendered_body` per odbiorca; oba kanały czytają to samo pole. Temat istnieje wyłącznie
  dla e-maila.
- **Brak duplikatów** wymuszony bazą: `UNIQUE (batch_id, recipient_id)`.
- **Odporność na błędy** wymuszona ziarnistością: jednostką pracy jest pojedyncza wiadomość,
  nie wsad; ponowna wysyłka to reset pojedynczych wierszy `deliveries` ze statusem `failed`.
- **Wymienność providerów**: `delivery` zna tylko interfejs, nigdy SDK dostawcy — dzięki temu
  wybór operatora (ADR-0003) można zmienić bez ruszania logiki.
- **Historia niezmienna**: wysłanego wsadu się nie edytuje, tworzy się kopię jako nowy `draft`.
