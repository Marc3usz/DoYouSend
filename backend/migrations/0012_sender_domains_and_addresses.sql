-- 0012_sender_domains_and_addresses: nadawcy e-maili (ADR-0021).
--   sender_domains        domeny uwierzytelnione u operatora (SendGrid Domain Authentication).
--                         Gdy domena jest "valid", kazdy adres w niej moze byc nadawca bez
--                         osobnej weryfikacji, wiec uzytkownicy z kontem w tej domenie wysylaja
--                         automatycznie ze swojego adresu.
--   sender_addresses      wspolne skrzynki (np. sekretariat@), ktore administrator deleguje
--   sender_address_users  wybranym uzytkownikom; wybieraja je przy wysylce.
-- Zadna tabela nie trzyma sekretow: tylko identyfikatory u operatora i rekordy DNS do ustawienia.

BEGIN;

CREATE TABLE sender_domains (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    domain             text NOT NULL UNIQUE CHECK (domain = lower(domain)),
    provider           text NOT NULL,          -- 'sendgrid' albo 'fake' (DRY_RUN / lokalnie)
    provider_domain_id text NOT NULL,
    -- pending: rekordy DNS jeszcze nie potwierdzone; valid: operator potwierdzil;
    -- missing: operator juz jej nie zna (usunieta poza aplikacja).
    status             text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'valid', 'missing')),
    -- [{"type": "cname", "host": "...", "data": "...", "valid": false}, ...]
    dns_records        jsonb NOT NULL DEFAULT '[]',
    created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    checked_at         timestamptz,
    validated_at       timestamptz,
    UNIQUE (provider, provider_domain_id)
);

CREATE TABLE sender_addresses (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text NOT NULL CHECK (email = lower(email)),
    name       text NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sender_addresses_email_key ON sender_addresses (email);

CREATE TABLE sender_address_users (
    address_id uuid NOT NULL REFERENCES sender_addresses(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (address_id, user_id)
);
CREATE INDEX sender_address_users_user ON sender_address_users (user_id);

COMMIT;
