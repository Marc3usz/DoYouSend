# Model danych

Źródło prawdy: `backend/migrations/` (rdzeń w `0001_init.sql`). Ten dokument tłumaczy, dlaczego
tak. Numer w nawiasie to migracja, która dodała tabelę albo kolumnę.

## Tabele

| Tabela | Rola |
|---|---|
| `users` | konta z rolą `admin` / `sender`; autor wysyłki |
| `sessions` (0004) | sesje logowania; w bazie tylko skrót SHA-256 tokenu z cookie |
| `recipients` | baza odbiorców: dane kontaktowe i typ (`parent` / `student`) |
| `recipient_classes` (0003) | klasy odbiorcy: uczeń ma najwyżej jedną, rodzic klasy swoich dzieci |
| `groups`, `group_members` | grupy systemowe i własne, przynależność wiele-do-wielu (tylko własne) |
| `message_batches` | jedna wysyłka: temat, wspólna treść, status, wybór odbiorców, podsumowanie, autor |
| `batch_recipients` | jeden odbiorca w jednej wysyłce, z utrwaloną treścią po personalizacji |
| `deliveries` | jeden kanał dla jednego odbiorcy: status, części, próby, błąd, ID u operatora |
| `unmatched_delivery_reports` (0007) | raporty doręczeń, które przyszły przed zapisem ID wiadomości |
| `audit_log` | kto co zrobił i kiedy (wysyłka, ponowienia, użytkownicy, ustawienia, importy) |
| `user_sender_identities` (0009) | tożsamość nadawcy SendGrid użytkownika i jej stan (ADR-0016) |
| `app_settings` (0010) | ustawienia wysyłki z panelu; sekrety zaszyfrowane (ADR-0018) |
| `sender_domains` (0012) | domeny uwierzytelnione u operatora e-mail i ich rekordy DNS (ADR-0021) |
| `sender_addresses`, `sender_address_users` (0012) | skrzynki nadawcze delegowane użytkownikom (ADR-0021) |

## Kolumny

**`users`** — `id`, `email` (unikalny, także bez względu na wielkość liter — 0004),
`full_name`, `role`, `password_hash` (PBKDF2-SHA256), `created_at`, `disabled_at` (0004,
konto zablokowane), `last_login_at` (0004).

**`sessions`** (0004) — `token_hash` (PK), `user_id`, `created_at`, `expires_at`.

**`recipients`** — `id`, `first_name`, `last_name`, `email`, `phone` (E.164), `type`,
`created_at`, `updated_at`.

**`recipient_classes`** (0003) — `recipient_id`, `class_name` (znormalizowana, np. `3A`);
klucz główny na parze.

**`groups`** — `id`, `name`, `description`, `is_system`, `created_at`.
**`group_members`** — `group_id`, `recipient_id`.

**`message_batches`** —
- treść: `subject` (tylko e-mail; pusty przy wysyłce tylko SMS), `body`;
- stan: `status`, `created_at`, `confirmed_at`, `finished_at`;
- autor: `created_by`, `confirmed_by` (ta sama osoba — wysyłkę przygotowuje i zatwierdza jeden
  użytkownik);
- wybór: `selected_groups` (jsonb: id i nazwa grupy w chwili wysyłki),
  `selected_recipient_ids` (0005, osoby wybrane ręcznie), `excluded_recipient_ids` (0008),
  `channels` (0011, kanały wysyłki — oba albo jeden, ADR-0019);
- nadawca e-maila: `from_email`, `from_name`, `reply_to` (0011, ADR-0021; NULL we wsadach
  sprzed migracji — wtedy `EMAIL_FROM` z chwili wysyłki);
- podsumowanie: `recipient_count`, `sms_part_count`, `estimated_cost` (`numeric(12,3)` od 0005;
  koszt wyliczony ceną z chwili wysyłki);
- idempotencja: `idempotency_key`, `request_hash` (0006) — ten sam klucz od tego samego autora
  zwraca istniejący wsad;
- ponowienia: `retry_count`, `last_retry_at` (0011).

**`batch_recipients`** — `id`, `batch_id`, `recipient_id`, `rendered_body`, `email_snapshot`,
`phone_snapshot`, `is_partial`, `position` (0005, kolejność w historii).

**`deliveries`** — `id`, `batch_recipient_id`, `channel`, `status`, `parts`,
`provider_message_id`, `error`, `attempts`, `updated_at`.

**`unmatched_delivery_reports`** (0007) — `provider_message_id`, `channel`, `status`, `error`,
`created_at`, `updated_at`.

**`audit_log`** — `id`, `user_id`, `action`, `entity`, `entity_id`, `details` (jsonb, bez
danych kontaktowych i treści), `created_at`.

**`user_sender_identities`** (0009) — `user_id` (PK), `provider`, `provider_sender_id`,
`from_email`, `from_name`, `status` (`pending` / `verified` / `missing`), `created_at`,
`checked_at`, `verified_at`.

**`app_settings`** (0010) — `key`, `value` (przy `encrypted = true`: `v1:` + base64 nonce
i szyfrogramu), `encrypted`, `updated_at`, `updated_by`.

**`sender_domains`** (0012) — `id`, `domain` (unikalna, małymi literami), `provider`,
`provider_domain_id`, `status` (`pending` / `valid` / `missing`), `dns_records` (jsonb: typ,
host, wartość i wynik ostatniego sprawdzenia), `created_by`, `created_at`, `checked_at`,
`validated_at` (pierwsze potwierdzenie).

**`sender_addresses`** (0012) — `id`, `email` (unikalny, małymi literami), `name` (nazwa
nadawcy), `created_by`, `created_at`, `updated_at`.
**`sender_address_users`** (0012) — `address_id`, `user_id` (klucz główny na parze),
`granted_by`, `granted_at`; usunięcie skrzynki albo użytkownika usuwa delegację.

## Więzy i wyzwalacze, które pilnują wymagań

- `recipients_contact_present` — odbiorca musi mieć e-mail albo telefon.
- unikalne indeksy na `lower(email)` i `phone` — wykrywanie duplikatów przy imporcie.
- `recipient_classes_name_check` — nazwa klasy w znormalizowanej postaci.
- `UNIQUE (batch_id, recipient_id)` — osoba wybrana przez kilka grup dostaje wiadomość raz.
- `UNIQUE (batch_recipient_id, channel)` — dokładnie jeden wiersz statusu na kanał.
- `batch_recipients.rendered_body` — historia pokazuje dokładnie to, co poszło do danej osoby;
  ta sama kolumna zasila oba kanały, więc treści nie da się rozjechać.
- `is_partial` — wysyłka tylko jednym kanałem jest jawnie oznaczona.
- `groups_name_lower_key` (0002) — nazwa grupy unikalna bez względu na wielkość liter.
- `message_batches_idempotency_key` (0006) — unikalny `(created_by, idempotency_key)`: podwójne
  kliknięcie nie wyśle wiadomości dwa razy.
- `trg_apply_unmatched_delivery_report` (0007) — przy zapisie `provider_message_id` stosuje
  zbuforowany raport doręczenia, tylko „do przodu” (ADR-0011).
- `trg_delivery_failure_updates_batch` (0007) — spóźniony raport `failed` zmienia wsad z `done`
  na `done_with_errors`.

## Grupy systemowe i klasowe

„Wszyscy rodzice” i „Wszyscy uczniowie” mają stałe UUID i wiersze w `groups` z `is_system = true`
(migracja `0002_system_groups.sql`). Ich członkostwo **nie** jest zapisane w `group_members`:
wylicza je kod (`backend/internal/groups/system.go`) z `recipients.type`, więc nie trzeba go
synchronizować po imporcie (ADR-0007). Magazyn grup w Postgresie (`groups.PGStore`) w ogóle
nie widzi tych wierszy — nie da się ich zmienić ani dodać do nich osób.

Grupy klasowe („Uczniowie klasy 3A”, „Rodzice uczniów klasy 3A”) nie mają wierszy w `groups`:
powstają w kodzie z `recipient_classes`, a ich ID to stałe UUIDv5 z nazwy klasy (ADR-0009).

## Zmiany schematu

Nowy plik `backend/migrations/NNNN_opis.sql`, nigdy edycja zastosowanej migracji.
Zmiana dotykająca innej domeny — opisz ją w PR-ze i, gdy zmienia kontrakt, w ADR.
