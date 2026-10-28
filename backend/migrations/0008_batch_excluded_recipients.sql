-- 0008_batch_excluded_recipients: utrwalenie osob wykluczonych z danej wysylki
-- (BatchDetail.excludedRecipientIds, description.md "Wykluczyc odbiorce z danej wysylki").

BEGIN;

ALTER TABLE message_batches
    ADD COLUMN excluded_recipient_ids uuid[] NOT NULL DEFAULT '{}';

COMMIT;
