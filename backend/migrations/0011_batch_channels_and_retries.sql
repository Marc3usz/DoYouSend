-- 0011_batch_channels_and_retries: kanaly wybrane dla wysylki, nadawca e-maila i slad
-- ponowien (ADR-0019, ADR-0021).
--   channels       kanaly, ktorymi wysylajacy kazal wyslac wiadomosc ({email,sms}, {email}
--                  albo {sms}). Historia pokazuje dzieki temu wysylke jednym kanalem jako
--                  swiadomy wybor, a nie jako brak danych odbiorcow.
--   from_email,    nadawca e-maila wybrany przy zatwierdzeniu i adres do odpowiedzi.
--   from_name,     NULL (wysylki sprzed tej migracji) = adres z ustawien wysylki
--   reply_to       w chwili wysylania (EMAIL_FROM).
--   retry_count,   ile razy uruchomiono "ponow nieudane" i kiedy ostatnio. Licznik prob
--   last_retry_at  w deliveries.attempts nie jest juz zerowany przy ponowieniu.

BEGIN;

ALTER TABLE message_batches
    ADD COLUMN channels      text[] NOT NULL DEFAULT '{email,sms}'
        CONSTRAINT message_batches_channels_valid
        CHECK (cardinality(channels) > 0 AND channels <@ ARRAY['email', 'sms']),
    ADD COLUMN from_email    text,
    ADD COLUMN from_name     text,
    ADD COLUMN reply_to      text,
    ADD COLUMN retry_count   integer NOT NULL DEFAULT 0,
    ADD COLUMN last_retry_at timestamptz;

-- Wysylki sprzed tej migracji: jeden kanal byl wybrany swiadomie, gdy wszystkie dostawy
-- sa w jednym kanale i nikt nie jest oznaczony jako czesciowy (przy wysylce dwoma kanalami
-- brak drugiego kanalu u kazdego odbiorcy oznaczalby wszystkich jako czesciowych).
UPDATE message_batches b
SET channels = ARRAY[c.only_channel]
FROM (
    SELECT br.batch_id, min(d.channel::text) AS only_channel
    FROM batch_recipients br
    JOIN deliveries d ON d.batch_recipient_id = br.id
    GROUP BY br.batch_id
    HAVING count(DISTINCT d.channel) = 1 AND NOT bool_or(br.is_partial)
) c
WHERE c.batch_id = b.id;

COMMIT;
