# ADR-0009: Grupy klasowe — „uczniowie klasy X” i „rodzice uczniów klasy X”

- **Status:** przyjęta (2026-10-07) — S1D0R-10 (DEV B) zaakceptował w komentarzu do PR #27
  po dopisaniu migawki `selected_groups`, kolejności grup i przestrzeni nazw UUIDv5;
  MichalK252 (DEV C) zatwierdził i zmergował PR #27. Szczegóły kontraktu przejdą osobnym PR-em
- **Data:** 2026-10-07
- **Uczestnicy:** Marc3usz (DEV A) proponuje; S1D0R-10 (DEV B) konsumuje grupy w kreatorze i
  wsadzie; cały zespół — zmiana `openapi.yaml` i nowa migracja

## Kontekst

`description.md` („Grupy odbiorców”) wymaga wysyłki do grup **„rodzice uczniów wybranej
klasy”** i **„uczniowie wybranej klasy”**. Dziś schemat (`0001_init.sql`) nie zna klasy ucznia
ani powiązania rodzic–uczeń, a import przyjmuje tylko kolumny imię, nazwisko, e-mail, telefon,
typ. To otwarte pytanie 1 z ADR-0007, odłożone, bo wymaga migracji i zmiany formatu importu.

Ograniczenia, które już obowiązują:

- Wybór odbiorców to `groups.Selection` z ID grup (UUID). Kreator (#26), `POST /groups/resolve`
  i przyszły `message_batches.selected_groups` operują wyłącznie na ID grup. Zapisany wybór ma
  wskazywać tę samą grupę także później (ADR-0007).
- Grupy systemowe mają członkostwo **wyliczane** z danych odbiorcy, bez wierszy w
  `group_members`. Materializację odrzuciliśmy, bo rozjeżdżała się po każdym imporcie (ADR-0007).
- Szkoła ma dane w arkuszu (eksport z dziennika), więc klasa musi dać się wpisać w import.

## Decyzja

**Klasa to atrybut odbiorcy, a grupy klasowe to kolejne grupy systemowe o wyliczanym
członkostwie i stałym, deterministycznym ID.** Kreator i `Resolve` nie dostają nowego pojęcia,
tylko więcej grup na liście.

1. **Model.** Nowa tabela `recipient_classes(recipient_id → recipients ON DELETE CASCADE,
   class_name text, PK (recipient_id, class_name))` (migracja `0003_recipient_classes.sql`).
   - Uczeń: **co najwyżej jedna** klasa — jego własna.
   - Rodzic: **dowolnie wiele** klas — klasy jego dzieci. Bez osobnego powiązania z konkretnym
     uczniem.
   - Regułę „uczeń ≤ 1 klasa” pilnuje walidacja w `recipients`. Baza pilnuje formatu nazwy
     (`CHECK`) i unikalności pary.
2. **Nazwa klasy** jest normalizowana przy zapisie i imporcie: przycięcie, usunięcie spacji,
   wielkie litery (`" 3 a "` → `3A`). Format: cyfra na początku, potem litery/cyfry, łącznie
   1–6 znaków (`3A`, `1TI`, `2BG`). Inna wartość to błąd walidacji pola `classes`.
3. **Grupy.** Dla każdej klasy, w której jest co najmniej jeden odbiorca, `groups` pokazuje dwie
   grupy systemowe:
   - „Uczniowie klasy 3A” = odbiorcy typu uczeń z klasą 3A;
   - „Rodzice uczniów klasy 3A” = odbiorcy typu rodzic z klasą 3A.

   **ID** to UUIDv5 (RFC 9562) z przestrzeni nazw
   **`733b2535-1e27-428a-a23d-c275f4eab3fa`** i napisu `students:3A` / `parents:3A`
   (znormalizowana nazwa klasy). Implementacja na `crypto/sha1` ze stdlib, bez nowej
   zależności; stała `groups.ClassNamespace` w `groups/system.go`. **Tej wartości ani formatu
   napisu nie wolno zmienić po pierwszej wysyłce** — zmieniłyby się ID wszystkich grup klasowych
   i zapisane wybory przestałyby się rozwijać. Ta sama klasa daje zawsze
   to samo ID, więc zapisany wybór i historia działają jak dla „Wszyscy rodzice”. Nie ma
   wierszy w `groups` ani w `group_members`, więc nie ma czego synchronizować po imporcie.
4. **Kolejność na liście.** `GET /groups` zwraca najpierw „Wszyscy rodzice”, „Wszyscy
   uczniowie”, potem grupy klasowe posortowane po numerze klasy (liczbowo), następnie po reszcie
   nazwy (`1A, 1B, 2A, …, 10A`) i w każdej klasie najpierw uczniów, potem rodziców; na końcu
   grupy własne. Kreator może grupować wizualnie po `className`.
5. **Nazwy zarezerwowane.** Grupy własnej nie można nazwać „Uczniowie klasy …” ani „Rodzice
   uczniów klasy …” (rozszerzenie `isSystemName`).
6. **Import.** Nowa **opcjonalna** kolumna `klasa`. Plik bez niej działa jak dziś. Kilka klas
   rodzica wpisujemy w jednej komórce po przecinku: `3A, 1B` (w CSV z przecinkiem jako
   separatorem komórkę trzeba ująć w cudzysłów — Excel robi to sam). Dla ucznia więcej niż jedna
   klasa to błąd wiersza.
7. **Kontrakt** (osobny PR do `openapi.yaml`, po akceptacji tego ADR):
   - `Recipient.classes: string[]` (zawsze obecne, może być puste), to samo w `RecipientInput`
     i `ImportedRecipient`;
   - `Group.className: string | null` — wypełnione dla grup klasowych, `kind` zostaje `system`.
     Enum się nie zmienia, więc obecny kreator (#26) pokaże grupy klasowe bez zmian w kodzie;
   - nowy kod błędu pola importu `classes`.

## Rozważane alternatywy

- **Tabela `guardianships(parent_id, student_id)` i klasa tylko u ucznia** (wersja z ADR-0007).
  Rodzica dopasowujemy do klasy przez jego dzieci. Plus: model zgodny z rzeczywistością, możliwe
  „rodzice konkretnego ucznia”. Odrzucone **na teraz**: import musiałby łączyć wiersze rodzica z
  wierszami dziecka (po czym? imię+nazwisko nie są unikalne, uczeń często nie ma e-maila), a
  błędne dopasowanie to wiadomość do złego rodzica. `description.md` wymaga tylko grupy
  „rodzice uczniów klasy”, nie relacji z konkretnym dzieckiem. Tabelę można dodać później bez
  zmiany ID grup.
- **Tabela `classes` z UUID i grupy klasowe jako wiersze w `groups`**, tworzone razem z klasą.
  Plus: zwykły klucz obcy, nazwy grup w bazie. Odrzucone: każdy import i edycja odbiorcy
  musiałyby zakładać i sprzątać klasy oraz ich grupy. To ta sama synchronizacja, którą ADR-0007
  odrzucił dla grup systemowych.
- **Nowe pole `classIds` w `Selection`** zamiast grup. Odrzucone: zmienia kontrakt kreatora,
  `Resolve`, wsadu i historii (domena B) dla czegoś, co dla wysyłającego jest zwykłą grupą.
- **Kolumna `recipients.class_name`** (jedna wartość). Odrzucone: rodzic z dziećmi w dwóch
  klasach musiałby być dwoma rekordami, a to łamie unikalność e-maila i telefonu.

## Konsekwencje

- **Dobre:** kreator, `POST /groups/resolve` i deduplikacja działają bez zmian. Rodzic z dziećmi
  w 3A i 1B wybrany przez obie grupy dostanie wiadomość raz (zasada 6). Zapisany wybór pozostaje
  stabilny. Brak nowej zależności i brak synchronizacji grup.
- **Złe:**
  - Grupa znika z listy, gdy w klasie nie ma już nikogo. Zapisany w historii wybór wskazuje wtedy
    nieistniejące ID. Dlatego **`message_batches.selected_groups` (jsonb) przechowuje migawkę
    `[{"id": "<uuid>", "name": "Rodzice uczniów klasy 3A"}]`, a nie `string[]`** — historia nie
    zależy od tego, czy grupa nadal istnieje. Bez nowej migracji (kolumna już jest `jsonb`);
    zapisuje DEV B w `delivery` (uzgodnione w review #27).
  - Grupa może zniknąć między podglądem a zatwierdzeniem. `Resolve` zwraca ją w
    `UnknownGroupIDs`, a zatwierdzenie wsadu z niepustą listą jest odrzucane z komunikatem
    „grupa już nie istnieje, odśwież wybór” — nie wysyłamy po cichu do mniejszej listy (DEV B).
  - Promocja do następnej klasy (3A → 4A) to zmiana danych odbiorców: ponowny import albo edycja.
    Nie ma operacji „przenieś klasę” — można ją dodać później jako zbiorczą zmianę nazwy.
  - Rodzic przypisany do klasy ręcznie, a nie przez dziecko, może się rozjechać z faktycznymi
    klasami dzieci. Świadomie akceptujemy to jako koszt prostego importu.
- **Do zrobienia (DEV A):** migracja `0003`; `recipients` (pole `Classes`, walidacja, import
  kolumny `klasa`, `PGStore`); `groups` (grupy klasowe w `SystemGroups`/`List`/`Resolve`, nazwy
  zarezerwowane, UUIDv5); osobny PR z kontraktem; UI odbiorców (pole klas w formularzu, kolumna
  w imporcie).
- **ADR-0007:** otwarte pytanie 1 rozstrzyga ten ADR po przyjęciu.
