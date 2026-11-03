-- 0010_app_settings: konfiguracja wysylki edytowana w panelu administratora (ADR-0018).
-- Wartosc z panelu wygrywa ze zmienna srodowiskowa, a ta z wartoscia domyslna.
-- Sekrety (klucze API, haslo SMTP, token DLR) sa zaszyfrowane AES-256-GCM kluczem spoza
-- bazy (SETTINGS_ENCRYPTION_KEY albo plik settings.key), wiec zrzut tej tabeli ich nie ujawnia.

BEGIN;

CREATE TABLE app_settings (
    key        text PRIMARY KEY,
    value      text NOT NULL,              -- przy encrypted = true: "v1:" + base64(nonce|szyfrogram)
    encrypted  boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL
);

COMMIT;
