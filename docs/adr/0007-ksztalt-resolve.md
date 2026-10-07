# ADR-0007: Kształt `Resolve()` i sygnalizowanie braków danych kontaktowych

- **Status:** propozycja — czeka na akceptację S1D0R-10 (DEV B) przed spięciem wysyłki w M3
- **Data:** 2026-09-30
- **Uczestnicy:** Marc3usz (DEV A) proponuje, S1D0R-10 (DEV B) konsumuje wynik

## Kontekst

`docs/podzial-pracy.md` wymienia punkt styku A → B: `Resolve(selection) -> []Recipient`,
czyli ustalenie kształtu struktury odbiorcy i sposobu sygnalizowania braków danych.
`description.md` wymaga, żeby przed wysłaniem system:

- usuwał duplikaty wynikające z przynależności do kilku grup,
- pokazywał osoby bez kompletu danych, żeby wysyłający mógł poprawić dane, wykluczyć
  odbiorcę albo wysłać tylko dostępnym kanałem (wysyłka częściowa),
- pozwalał wskazać jedną osobę albo ręcznie wybrać kilku odbiorców.

Pakiet `backend/internal/groups` implementuje teraz grupy i `Resolve`. Ten ADR opisuje
kształt, na który DEV B może pisać `delivery` i podsumowanie w `messaging`.

## Decyzja

**Wejście:** `groups.Selection{GroupIDs, RecipientIDs, ExcludedRecipientIDs}`.
`ExcludedRecipientIDs` to nowe pole w stosunku do `RecipientSelection` z `openapi.yaml`.
Wykluczenie ma pierwszeństwo przed grupą i przed wyborem ręcznym. ID są porównywane
bez względu na wielkość liter i spacje.

**Wyjście:** `groups.Resolution`:

| Pole | Znaczenie |
|---|---|
| `Recipients []Resolved` | każdy odbiorca raz (deduplikacja po ID), posortowani po nazwisku, imieniu i ID |
| `Resolved.Recipient` | pełny `recipients.Recipient` (kształt z PR #4, bez nowego typu) |
| `Resolved.ViaGroupIDs`, `Direct` | skąd osoba trafiła na listę, żeby UI mógł to wyjaśnić |
| `Resolved.Issues []ContactIssue` | kanał (`email` / `sms`) i powód (`missing` / `invalid`) |
| `Resolved.Partial()` / `Unreachable()` | dokładnie jeden kanał albo żaden; `Partial()` ↔ `batch_recipients.is_partial` |
| `UnknownGroupIDs`, `UnknownRecipientIDs` | wybrane ID, które nie istnieją; **nie przerywają** rozwinięcia wyboru |
| `ExcludedIDs`, `MergedDuplicates` | do komunikatu „pominięto X osób, scalono Y powtórzeń”; powtórzenia osób wykluczonych nie są liczone |

Braki danych są **raportowane, nie filtrowane**. `Resolve` nie wyrzuca nikogo z listy
z powodu braku kanału. Decyzję (poprawić dane, wykluczyć, wysłać częściowo) podejmuje
wysyłający w kreatorze. `delivery` tworzy wiersz `deliveries` tylko dla kanałów z
`Channels()`.

Wartość zapisana w bazie, która nie przechodzi walidacji z `recipients` (np. numer spoza
E.164), jest zgłaszana jako `invalid`. `Resolve` nie normalizuje jej po cichu, bo `delivery`
wysłałby dokładnie to, co jest w bazie.

**Grupy systemowe** („Wszyscy rodzice”, „Wszyscy uczniowie”) są zdefiniowane w kodzie
(`groups/system.go`). Mają stałe UUID (`AllParentsID`, `AllStudentsID`), a członkostwo
wyliczane jest na bieżąco z `recipients.type`, bez trzymania go w `group_members`. Dzięki
stałym ID wybór zapisany w `message_batches.selected_groups` wskazuje zawsze tę samą grupę.
Grup systemowych nie da się edytować ani usunąć, a ich nazwy są zarezerwowane.

## Rozważane alternatywy

- **`Resolve` zwraca samo `[]recipients.Recipient`**, a braki liczy konsument. Odrzucone:
  każdy konsument (podsumowanie, delivery, UI) liczyłby braki po swojemu, a reguła
  walidacji jest w domenie A.
- **Odfiltrowanie nieosiągalnych odbiorców w `Resolve`.** Odrzucone: `description.md`
  wymaga, żeby wysyłający te osoby zobaczył i sam zdecydował.
- **Błąd przy nieznanym ID.** Odrzucone: grupa albo osoba usunięta w trakcie otwartego
  kreatora zablokowałaby całą wysyłkę. Zgłaszamy ją, a decyzję podejmuje wywołujący.
- **Grupy systemowe jako materializowane wiersze w `group_members`.** Odrzucone: każdy
  import musiałby je przeliczać, a rozjazd byłby niewidoczny.

## Otwarte pytania

1. **Grupy klasowe** („rodzice uczniów klasy 3A”, „uczniowie klasy 3A”). Schemat nie ma
   klasy ucznia ani powiązania rodzic–uczeń. Potrzebna jest migracja (np. `recipients.class`
   i tabela `guardianships(parent_id, student_id)`) oraz kolumny w imporcie. Do ustalenia
   osobnym ADR-em, bo to zmienia `migrations/` i format pliku importu.
   **Propozycja:** ADR-0009 (grupy klasowe jako grupy systemowe z wyliczanym członkostwem).
2. ~~**Wiersze grup systemowych w tabeli `groups`.**~~ **Rozwiązane** migracją
   `0002_system_groups.sql`: wiersze ze stałymi UUID i `is_system = true`, bez wpisów w
   `group_members` (członkostwo nadal wyliczane z `recipients.type`). `groups.PGStore` nie widzi
   tych wierszy, więc nie da się ich zmienić ani dodać do nich osób. Migracja usuwa też stare
   wiersze systemowe z losowym UUID, które wstawiał dawny seed.
3. ~~**`ExcludedRecipientIDs` w `openapi.yaml`** i kształt odpowiedzi `POST /groups/resolve`.~~
   **Rozwiązane** w PR #14: `RecipientSelection.excludedRecipientIds` i odpowiedź
   `POST /groups/resolve` są w kontrakcie.

## Konsekwencje

- `groups` importuje `recipients` (obie domeny należą do DEV A). `messaging` i `delivery`
  nie muszą importować `recipients`; wystarczy im `groups.Resolution`, albo mogą
  zdefiniować własny wąski interfejs.
- `messaging.RenderedMessage{HasEmail, HasPhone}` wypełnia się z `Resolved.CanReach`.
  Mapowanie pól na placeholdery nadal rozstrzyga ADR-0005.
- Deduplikacja jest po ID odbiorcy. Dwie różne osoby z tym samym e-mailem blokują unikalne
  indeksy w bazie (`0001_init.sql`), więc `Resolve` tego przypadku nie obsługuje.
