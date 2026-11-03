# ADR-0015: Otwarte decyzje architektoniczne domen B, C i D (konfiguracja operatorów, statusy wsadów, DRY_RUN)

- **Status:** przyjęta
- **Data:** 2026-10-30
- **Uczestnicy:** MichalK252 (domena B: kreator, domena C: dostawcy), Marc3usz (domena A: odbiorcy, domena D: uprawnienia i audyt)

## Kontekst

W miarę rozwoju systemu DoYouSend i integracji pomiędzy domenami B (kreator i historia wysyłek), C (dostawcy e-mail i SMS, przetwarzanie wsadów) oraz D (administracja, uprawnienia i konfiguracja systemowa), pojawiły się trzy istotne zagadnienia architektoniczne wymagające formalnego rozstrzygnięcia i udokumentowania:

1. **Konfiguracja operatorów w `.env` zamiast dynamicznej edycji w panelu:**
   Endpoint `GET /api/admin/config` udostępnia stan dostawców (m.in. czy ustawiono klucze uwierzytelniające, nazwę nadawcy, stawkę SMS, tryby testowe), jednak nie udostępnia metod zapisu (`POST`/`PUT`/`PATCH`). Należało sformalizować, dlaczego konfiguracja operatorów nie podlega modyfikacji „w locie” z poziomu interfejsu WWW.
2. **Statusy wsadu (`draft`, `scheduled`, `cancelled`) w kontekście MVP:**
   W specyfikacji `description.md` (sekcja „Statusy wiadomości”) wskazano: *„Dla całej masowej wysyłki można zastosować statusy: wersja robocza, zaplanowana, w trakcie, zakończona, zakończona z błędami, anulowana.”* Typ enum `batch_status` w bazie PostgreSQL zawiera wszystkie te wartości, natomiast obecny proces realizuje wyłącznie wysyłkę natychmiastową.
3. **Relacja bezpiecznika `DRY_RUN` z lokalnym serwerem Mailpit:**
   Domyślny tryb deweloperski `DRY_RUN=true` całkowicie tłumi wywołania sieciowe i symuluje wysyłkę w logach. Wymagało sprecyzowania, w jaki sposób i w jakich warunkach testerzy oraz deweloperzy mogą przekierować e-maile do interfejsu Mailpit w celu inspekcji wyrenderowanych szablonów.

## Decyzje

### 1. Konfiguracja operatorów wyłącznie przez zmienne środowiskowe (`.env`) i restart serwera

- **Endpoint `GET /api/admin/config` pozostaje ściśle tylko do odczytu (read-only):**
  Zwraca aktualny stan konfiguracji dostawców bez ujawniania haseł i kluczy API (pola `CredentialsSet` mają wartość boolean). Służy administratorowi wyłącznie do weryfikacji poprawności wdrożenia.
- **Brak dynamicznej edycji w panelu:**
  - W placówkach oświatowych przypadkowa lub nieautoryzowana zmiana bramki e-mail/SMS niesie ryzyko natychmiastowych kosztów finansowych lub wysyłki wrażliwych informacji z niewłaściwego adresu.
  - Zgodnie z zasadą projektu (`CLAUDE.md`, reguła 3): *„Przełączenie na prawdziwego operatora wymaga zgody przełożonego i wpisania kluczy do .env”*. Zmiana operatora jest decyzją organizacyjno-prawną (np. rejestracja nazwy nadawcy SMS, umowa powierzenia przetwarzania danych RODO).
  - Zgodnie z konwencją Twelve-Factor App konfiguracja bramek stanowi część środowiska wykonawczego. Zmiana parametrów wymaga aktualizacji pliku `.env` (lub sekretów środowiska produkcyjnego) oraz celowego zrestartowania usługi backendowej.

### 2. Potwierdzenie wyłączenia statusów `draft`, `scheduled` i `cancelled` z obecnego zakresu MVP

- **Zachowanie spójności typów:**
  Wartości `'draft'`, `'scheduled'`, `'cancelled'` pozostają zdefiniowane w typie PostgreSQL `batch_status` (`0001_init.sql`) oraz w strukturach Go (`delivery.BatchStatus`) jako przygotowanie pod przyszły rozwój systemu.
- **Zakres MVP i stabilność:**
  - Sformułowanie w specyfikacji `description.md` jest fakultatywne (*„można zastosować statusy”*).
  - W codziennej pracy szkoły najważniejszym i krytycznym scenariuszem jest natychmiastowa, pewna wysyłka komunikatów (np. nagłe odwołanie lekcji, pilne zebranie, ostrzeżenia pogodowe).
  - Planowanie wysyłek na określony termin (`scheduled`) wymagałoby niezależnego procesu harmonogramującego (cron/scheduler odporny na restarty węzłów), kolejki z opóźnieniem oraz obsługi unieważniania zadań.
  - Wersje robocze (`draft`) zapisałyby w bazie stan odbiorców i grup, który mógłby ulec dezaktualizacji przed faktyczną wysyłką (np. zmiana numeru telefonu rodzica w międzyczasie).
  - W MVP cały proces kreatora wiadomości (wybór grup, treść ze zmiennymi, weryfikacja i poprawa braków kontaktowych, natychmiastowa wysyłka) przebiega w jednym spójnym cyklu. Wsad przechodzi bezpośrednio ze stanu tworzenia do przetwarzania (`running`), a następnie do `done` lub `done_with_errors`.

### 3. Precyzyjne rozgraniczenie `DRY_RUN` a `EMAIL_PROVIDER=mailpit`

- **Domyślna ochrona (`DRY_RUN=true`):**
  - Wrapper `WrapDryRun` (`internal/providers/dryrun.go`) przechwytuje każde wywołanie `Send()` przed dotarciem do adaptera bazowego.
  - Żadne pakiety nie są wysyłane do sieci ani na port SMTP Mailpita. Wysyłka jest symulowana, generowany jest syntetyczny identyfikator wiadomości (`dryrun-...`), a zdarzenie trafia do logów aplikacji. Gwarantuje to zerowe ryzyko przypadkowego wysłania wiadomości i brak konieczności uruchamiania Mailpita do testów jednostkowych.
- **Podgląd wiadomości w Mailpicie (`DRY_RUN=false` + `EMAIL_PROVIDER=mailpit`):**
  - Gdy zachodzi potrzeba zweryfikowania wyglądu, kodowania polskich znaków i szablonów HTML w przeglądarce ([http://localhost:8025](http://localhost:8025)), w pliku `.env` należy jawnie ustawić:
    ```env
    DRY_RUN=false
    EMAIL_PROVIDER=mailpit
    SMTP_HOST=localhost
    SMTP_PORT=1025
    ```
  - W tym wariancie adapter `email.Mailpit` wysyła wiadomości przez protokół SMTP bezpośrednio do kontenera Mailpit.
  - Ponieważ Mailpit działa lokalnie wewnątrz sieci Dockera i nie posiada zewnętrznego mechanizmu przekazywania poczty dalej (relay), żadna wiadomość nie opuści maszyny dewelopera.

## Rozważane alternatywy

- **Zapisywanie konfiguracji operatorów w bazie danych z formularzem w panelu:**
  - *Odrzucone:* Wymagałoby wdrożenia mechanizmu bezpiecznego szyfrowania kluczy w bazie (encryption at rest), dynamicznego przeładowywania klientów HTTP/SMTP w trakcie obsługi żądań oraz audytowania zmian haseł; nie chroni przed przypadkową zmianą dokonaną przez mniej technicznego administratora.
- **Pełna implementacja harmonogramu i szkiców w MVP:**
  - *Odrzucone:* Nieuzasadniony koszt implementacji i ryzyko błędów synchronizacji danych odbiorców w fazie wdrożenia pilotażowego.
- **Kierowanie e-maili do Mailpita nawet przy włączonym `DRY_RUN=true`:**
  - *Odrzucone:* Naruszyłoby to zasadę deterministycznego działania bezpiecznika `DRY_RUN`, który ma gwarantować całkowite odcięcie komunikacji sieciowej dla wszystkich kanałów (zarówno e-mail, jak i SMS).

## Konsekwencje

- Panel administratora w domenie D pozostaje bezpiecznym narzędziem podglądu stanu systemu (`GET /api/admin/config`), a odpowiedzialność za sekrety spoczywa na środowisku uruchomieniowym.
- Proces wysyłki wsadu w domenach B i C skupia się na niezawodnym doręczeniu natychmiastowym; ewentualne rozszerzenie o szkice i harmonogram może nastąpić w kolejnych wersjach bez zmian w schemacie bazy danych.
- Zespół posiada jednoznaczną dokumentację dotyczącą testowania e-maili lokalnie przy użyciu Mailpita i flagi `DRY_RUN`.
