# Bezpieczeństwo i dane

## Sekrety

- Wszystkie klucze, hasła i tokeny żyją w `.env` (w `.gitignore`). W repo jest wyłącznie
  `.env.example` z placeholderami.
- Dodajesz nową zmienną — dopisz ją do `.env.example` w tym samym PR.
- Klucze produkcyjne trzyma opiekun projektu; deweloperzy pracują na kontach testowych.
- Jeśli sekret jednak trafi do repo: **unieważnij go u dostawcy**, dopiero potem czyść
  historię. Sam `git rm` nie wystarczy, klucz zostaje w commitach.

## Dane testowe

- Wyłącznie dane fikcyjne: domena `@example.test`, numery `+48 500 100 1xx`, zmyślone nazwiska.
- Prawdziwy eksport odbiorców ze szkoły nie trafia do repo ani do promptów Claude Code,
  nawet do jednorazowego testu importu. Do testów służy `backend/testdata/`.
- `.gitignore` blokuje `*.csv` poza `backend/testdata/` — to bezpiecznik, nie zwolnienie z uwagi.

## Wysyłka

- `DRY_RUN=true` jest domyślne: system rejestruje wiadomości, nic nie wychodzi na zewnątrz.
- Lokalnie e-mail idzie do Mailpita, SMS do providera `fake`.
- Podłączenie prawdziwej bramki SMS, prawdziwej usługi e-mail albo prawdziwych odbiorców
  wymaga przeglądu rozwiązania i zgody opiekuna projektu (`description.md`).
- Masowa wysyłka wymaga końcowego potwierdzenia w UI i zapisu w `audit_log`
  (kto przygotował, kto zatwierdził).

## Uprawnienia

- Dostęp tylko dla zalogowanych. Role: `admin` (odbiorcy, grupy, użytkownicy, konfiguracja)
  i `sender` (tworzenie i wysyłka komunikatów, podgląd swoich wysyłek).
- Sprawdzanie uprawnień po stronie serwera. Ukrycie przycisku w UI nie jest zabezpieczeniem.
- Logi nie zawierają adresów e-mail, numerów telefonów ani treści wiadomości, tylko ID.
