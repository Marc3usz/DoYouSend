-- 0001_init: rdzen schematu. Punkt wyjscia uzgodniony na starcie projektu
-- (docs/model-danych.md). Zmiany wylacznie przez NOWE pliki migracji.

CREATE TYPE recipient_type AS ENUM ('parent', 'student');
CREATE TYPE channel        AS ENUM ('email', 'sms');
CREATE TYPE channel_status AS ENUM ('pending', 'sending', 'sent', 'delivered', 'failed');
CREATE TYPE batch_status   AS ENUM ('draft', 'scheduled', 'running', 'done', 'done_with_errors', 'cancelled');
CREATE TYPE user_role      AS ENUM ('admin', 'sender');

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    full_name     text NOT NULL,
    role          user_role NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recipients (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name text NOT NULL,
    last_name  text NOT NULL,
    email      text,
    phone      text,                       -- E.164, np. +48500100101
    type       recipient_type NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- odbiorca bez zadnego kanalu kontaktu nie ma sensu
    CONSTRAINT recipients_contact_present CHECK (email IS NOT NULL OR phone IS NOT NULL)
);
CREATE UNIQUE INDEX recipients_email_key ON recipients (lower(email)) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX recipients_phone_key ON recipients (phone) WHERE phone IS NOT NULL;

CREATE TABLE groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL UNIQUE,
    description text,
    -- grupy systemowe (wszyscy rodzice, klasa 3A ...) sa wyliczane, nie edytowalne recznie
    is_system   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE group_members (
    group_id     uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    recipient_id uuid NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, recipient_id)
);

CREATE TABLE message_batches (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject        text NOT NULL,          -- wylacznie temat e-maila
    body           text NOT NULL,          -- jedna wspolna tresc dla e-maila i SMS-a
    status         batch_status NOT NULL DEFAULT 'draft',
    created_by     uuid NOT NULL REFERENCES users(id),
    confirmed_by   uuid REFERENCES users(id),
    selected_groups jsonb NOT NULL DEFAULT '[]'::jsonb,
    recipient_count integer NOT NULL DEFAULT 0,
    sms_part_count  integer NOT NULL DEFAULT 0,
    estimated_cost  numeric(10,2) NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    confirmed_at   timestamptz,
    finished_at    timestamptz
);

-- Jeden wiersz = jeden odbiorca w jednej wysylce. Utrwala tresc po personalizacji,
-- zeby historia pokazywala dokladnie to, co poszlo do danej osoby.
CREATE TABLE batch_recipients (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id      uuid NOT NULL REFERENCES message_batches(id) ON DELETE CASCADE,
    recipient_id  uuid NOT NULL REFERENCES recipients(id),
    rendered_body text NOT NULL,
    email_snapshot text,
    phone_snapshot text,
    is_partial    boolean NOT NULL DEFAULT false,
    UNIQUE (batch_id, recipient_id)        -- gwarancja braku duplikatow miedzy grupami
);

CREATE TABLE deliveries (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_recipient_id uuid NOT NULL REFERENCES batch_recipients(id) ON DELETE CASCADE,
    channel            channel NOT NULL,
    status             channel_status NOT NULL DEFAULT 'pending',
    parts              integer NOT NULL DEFAULT 1,
    provider_message_id text,
    error              text,
    attempts           integer NOT NULL DEFAULT 0,
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (batch_recipient_id, channel)
);
CREATE INDEX deliveries_status_idx ON deliveries (status);

CREATE TABLE audit_log (
    id         bigserial PRIMARY KEY,
    user_id    uuid REFERENCES users(id),
    action     text NOT NULL,
    entity     text,
    entity_id  text,
    details    jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
