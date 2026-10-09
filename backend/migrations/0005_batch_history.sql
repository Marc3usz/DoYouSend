-- 0005_batch_history: brakujace kolumny, zeby wsad z historii mial wszystko, co pokazuje
-- GET /batches/{id} (docs/api/openapi.yaml, BatchDetail).
-- Numer 0003 jest zarezerwowany dla recipient_classes (ADR-0009, PR w toku).

BEGIN;

ALTER TABLE message_batches
    -- osoby wybrane recznie, poza grupami (BatchDetail.recipientIds); tablica nie ma FK,
    -- ale odbiorcy z wysylki i tak nie da sie usunac (batch_recipients.recipient_id)
    ADD COLUMN selected_recipient_ids uuid[] NOT NULL DEFAULT '{}',
    -- cena czesci SMS ma do 3 miejsc po przecinku (messaging.ParsePrice), wiec koszt tez
    ALTER COLUMN estimated_cost TYPE numeric(12,3);

-- Kolejnosc odbiorcow w planie wysylki (po nazwisku i imieniu w chwili wysylki), zeby
-- historia nie zmieniala porzadku, gdy ktos pozniej zmieni nazwisko.
ALTER TABLE batch_recipients
    ADD COLUMN position integer NOT NULL DEFAULT 0;

COMMIT;
