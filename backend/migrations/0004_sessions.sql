-- 0004_sessions: logowanie i sesje (ADR-0010).
-- Numer 0003 jest zarezerwowany dla recipient_classes (ADR-0009, PR w toku); migracje sa
-- niezalezne, a scripts/migrate.sh stosuje kazdy brakujacy plik, wiec kolejnosc wdrozenia
-- nie ma znaczenia.

BEGIN;

ALTER TABLE users
    ADD COLUMN disabled_at   timestamptz,   -- konto zablokowane przez administratora
    ADD COLUMN last_login_at timestamptz;

-- E-mail logowania porownywany bez wzgledu na wielkosc liter (iam.normalizeEmail).
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

-- Sesje po stronie serwera. W bazie jest tylko skrot SHA-256 tokenu z cookie, wiec wyciek
-- tej tabeli nie pozwala sie zalogowac.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE INDEX audit_log_created_at_idx ON audit_log (created_at DESC, id DESC);

COMMIT;
