-- 0006_batch_idempotency: Idempotency-Key z POST /batches (docs/api/openapi.yaml).
-- Ten sam klucz od tego samego nadawcy zwraca istniejacy wsad zamiast wysylac drugi raz;
-- request_hash odroznia powtorzenie tego samego zadania od klucza uzytego z inna trescia.

BEGIN;

ALTER TABLE message_batches
    ADD COLUMN idempotency_key uuid,
    ADD COLUMN request_hash    text;

-- Unikalnosc pilnuje wyscigu dwoch jednoczesnych klikniec, nie tylko kolejnych zadan.
CREATE UNIQUE INDEX message_batches_idempotency_key
    ON message_batches (created_by, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Historia: od najnowszej, filtrowana po nadawcy.
CREATE INDEX message_batches_created_at_idx ON message_batches (created_at DESC, id DESC);

COMMIT;
