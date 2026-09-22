# Model danych

Źródło prawdy: `backend/migrations/0001_init.sql`. Ten dokument tłumaczy, dlaczego tak.

| Tabela | Rola |
|---|---|
| `users` | konta z rolą `admin` / `sender`; autor i zatwierdzający wysyłkę |
| `recipients` | baza odbiorców: dane kontaktowe i typ (`parent` / `student`) |
| `groups`, `group_members` | grupy systemowe i własne, przynależność wiele-do-wielu |
| `message_batches` | jedna wysyłka: temat, wspólna treść, status, podsumowanie, autor |
| `batch_recipients` | jeden odbiorca w jednej wysyłce, z utrwaloną treścią po personalizacji |
| `deliveries` | jeden kanał dla jednego odbiorcy: status, próby, błąd, ID u operatora |
| `audit_log` | kto co zrobił: kto przygotował i kto zatwierdził wysyłkę |

## Więzy, które pilnują wymagań

- `recipients_contact_present` — odbiorca musi mieć e-mail albo telefon.
- unikalne indeksy na `lower(email)` i `phone` — wykrywanie duplikatów przy imporcie.
- `UNIQUE (batch_id, recipient_id)` — osoba wybrana przez kilka grup dostaje wiadomość raz.
- `UNIQUE (batch_recipient_id, channel)` — dokładnie jeden wiersz statusu na kanał.
- `batch_recipients.rendered_body` — historia pokazuje dokładnie to, co poszło do danej osoby;
  ta sama kolumna zasila oba kanały, więc treści nie da się rozjechać.
- `is_partial` — wysyłka tylko jednym kanałem jest jawnie oznaczona.

## Zmiany schematu

Nowy plik `backend/migrations/NNNN_opis.sql`, nigdy edycja zastosowanej migracji.
Zmiana dotykająca cudzej domeny — uprzedź właściciela (`docs/podzial-pracy.md`).
