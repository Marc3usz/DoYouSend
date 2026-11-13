# Bezpieczeństwo i dane

## Sekrety

- W repo nie ma żadnego klucza, hasła ani tokenu. Jest wyłącznie `.env.example` z pustymi
  wartościami i placeholderami; `.env` jest w `.gitignore`.
- Sekrety żyją w jednym z dwóch miejsc:
  - **w `.env`** — wartości startowe (np. `DATABASE_URL`, `ADMIN_PASSWORD`) i opcjonalne
    wartości zapasowe dla panelu;
  - **w panelu** (Panel administratora → Ustawienia wysyłki, ADR-0018) — klucze API operatorów,
    hasło SMTP, token DLR i token Notion. W tabeli `app_settings` są zaszyfrowane AES-256-GCM
    kluczem spoza bazy: `SETTINGS_ENCRYPTION_KEY` albo plik `SETTINGS_KEY_FILE` (domyślnie
    `backend/settings.key`, tworzony przy pierwszym starcie, objęty `.gitignore` przez `*.key`).
    Zrzut bazy bez tego klucza nie ujawnia sekretów.
- **Kopia klucza szyfrującego** — trzymaj ją osobno od kopii bazy. Bez klucza zapisane klucze
  API trzeba wpisać w panelu od nowa; z kluczem i bazą razem da się je odczytać.
- Dodajesz nową zmienną — dopisz ją do `.env.example` w tym samym PR.
- Klucze produkcyjne trzyma opiekun projektu; podczas budowy pracujemy na kontach testowych.
- CI uruchamia `scripts/check-secrets.sh` na całym repo (także `docs/` i Markdown): `.env`
  w repo, klucze w formacie SendGrid i Notion, klucze prywatne, adresy spoza `example.*`,
  numery spoza puli testowej.
- Jeśli sekret jednak trafi do repo: **unieważnij go u dostawcy**, dopiero potem czyść
  historię. Sam `git rm` nie wystarczy, klucz zostaje w commitach.

## Dane testowe

- Wyłącznie dane fikcyjne: domena `@example.test`, numery `+48 500 100 1xx`, zmyślone nazwiska.
  Także w placeholderach UI i w dokumentacji.
- Prawdziwy eksport odbiorców ze szkoły nie trafia do repo ani do promptów Claude Code,
  nawet do jednorazowego testu importu. Do testów służy `backend/testdata/`.
- `.gitignore` blokuje `*.csv` poza `backend/testdata/` — to bezpiecznik, nie zwolnienie z uwagi.

## Wysyłka

- `DRY_RUN=true` jest domyślne: system rejestruje wiadomości, nic nie wychodzi na zewnątrz.
- Lokalnie e-mail idzie do Mailpita, SMS do providera `fake`.
- Wyłączenie `DRY_RUN` z operatorem wysyłającym poza komputer (SendGrid, SMSAPI, SMTP na host
  inny niż lokalny) wymaga w panelu wpisania `WYSYŁAJ`; `DRY_RUN_LOCK=true` blokuje to całkiem.
  Gdy trwa wysyłka wsadu, panel nie przełączy trybu ani operatora.
- Podłączenie prawdziwej bramki SMS, prawdziwej usługi e-mail albo prawdziwych odbiorców
  wymaga przeglądu rozwiązania i zgody opiekuna projektu (`description.md`).
- Masowa wysyłka wymaga końcowego potwierdzenia w UI. Wysyłkę przygotowuje i zatwierdza ten sam
  użytkownik: `message_batches.created_by` i `confirmed_by` wskazują tę samą osobę, a
  `audit_log` zapisuje `batch_sent` (kto, kiedy, ilu odbiorców) i każde `batch_retried`.
- Każdy odbiorca dostaje osobną wiadomość — nikt nie widzi adresów ani numerów innych osób.

## Uprawnienia

- Dostęp tylko dla zalogowanych. Role: `admin` (odbiorcy, grupy, użytkownicy, ustawienia,
  historia wszystkich wysyłek) i `sender` (tworzenie i wysyłka komunikatów, podgląd swoich
  wysyłek).
- Wysyłający nie widzi danych kontaktowych odbiorców: wyszukiwarka zwraca tylko imię, nazwisko,
  typ, klasy i informację o brakach. Może uzupełnić brakujący kontakt (ADR-0013), ale nie może
  odczytać istniejącego.
- Sprawdzanie uprawnień po stronie serwera. Ukrycie przycisku w UI nie jest zabezpieczeniem.
- Reguły dostępu do API są w jednej tabeli: `backend/internal/iam/policy.go`. Nowy endpoint
  domyślnie wymaga zalogowania; jeśli ma być tylko dla administratora albo publiczny, dopisz
  regułę i test w `policy_test.go` w tym samym PR (ADR-0010). Publiczne są tylko `/healthz`,
  logowanie i webhooki operatorów (`/providers/*`, chronione tokenem albo podpisem).
- Sesja to cookie `dys_session` (HttpOnly, SameSite=Lax, Secure poza `APP_ENV=development`);
  w bazie jest tylko skrót tokenu. Hasła: PBKDF2-SHA256, min. 12 znaków. Pierwsze konto:
  `make create-admin`.
- Logi nie zawierają adresów e-mail, numerów telefonów ani treści wiadomości, tylko ID.
