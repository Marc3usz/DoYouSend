-- 0009_user_sender_identities: tozsamosc nadawcy e-mail kazdego uzytkownika (ADR-0016).
-- Administrator zaklada uzytkownikowi "Single Sender" w SendGrid; SendGrid wysyla na jego
-- adres link potwierdzajacy, a status sprawdzamy przez API. Tabela nie trzyma zadnego
-- sekretu: tylko identyfikator tozsamosci u operatora i ostatnio znany stan.

BEGIN;

CREATE TABLE user_sender_identities (
    user_id            uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    provider           text NOT NULL,          -- 'sendgrid' albo 'fake' (DRY_RUN / lokalnie)
    provider_sender_id text NOT NULL,          -- id tozsamosci u operatora
    from_email         text NOT NULL,
    from_name          text NOT NULL,
    -- pending: czeka na klikniecie linku; verified: potwierdzona;
    -- missing: operator jej juz nie zna (usunieta poza aplikacja).
    status             text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'verified', 'missing')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    checked_at         timestamptz,            -- ostatnie zapytanie do operatora
    verified_at        timestamptz,            -- pierwsze zobaczone potwierdzenie
    UNIQUE (provider, provider_sender_id)
);

COMMIT;
