-- 0007_unmatched_delivery_reports: buforowanie raportow doreczen przychodzacych
-- przed zapisaniem provider_message_id (np. w trakcie trwania wysylki wsadu)
-- oraz przeliczanie statusu wsadu z 'done' na 'done_with_errors' po spoznionym raporcie.
-- Issue #42, ADR-0011.

BEGIN;

CREATE TABLE unmatched_delivery_reports (
    provider_message_id text NOT NULL,
    channel             channel NOT NULL,
    status              channel_status NOT NULL,
    error               text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_message_id, channel)
);

CREATE INDEX unmatched_delivery_reports_lookup_idx
    ON unmatched_delivery_reports (provider_message_id, channel);

CREATE OR REPLACE FUNCTION apply_unmatched_delivery_report()
RETURNS TRIGGER AS $$
DECLARE
    unmatched RECORD;
BEGIN
    IF NEW.provider_message_id IS NOT NULL AND NEW.provider_message_id <> '' THEN
        SELECT status, error INTO unmatched
        FROM unmatched_delivery_reports
        WHERE provider_message_id = NEW.provider_message_id AND channel = NEW.channel;

        IF FOUND THEN
            IF (NEW.status = 'pending' AND unmatched.status IN ('sending', 'sent', 'delivered', 'failed')) OR
               (NEW.status = 'sending' AND unmatched.status IN ('sent', 'delivered', 'failed')) OR
               (NEW.status = 'sent'    AND unmatched.status IN ('delivered', 'failed')) THEN
                NEW.status := unmatched.status;
                IF unmatched.error IS NOT NULL THEN
                    NEW.error := unmatched.error;
                END IF;
            END IF;
            DELETE FROM unmatched_delivery_reports
            WHERE provider_message_id = NEW.provider_message_id AND channel = NEW.channel;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_apply_unmatched_delivery_report
BEFORE INSERT OR UPDATE OF provider_message_id ON deliveries
FOR EACH ROW
EXECUTE FUNCTION apply_unmatched_delivery_report();

CREATE OR REPLACE FUNCTION update_batch_status_on_delivery_failure()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'failed' AND (OLD.status IS NULL OR OLD.status <> 'failed') THEN
        UPDATE message_batches
        SET status = 'done_with_errors'
        WHERE id = (
            SELECT batch_id FROM batch_recipients WHERE id = NEW.batch_recipient_id
        )
        AND status = 'done';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_delivery_failure_updates_batch
AFTER UPDATE OF status ON deliveries
FOR EACH ROW
EXECUTE FUNCTION update_batch_status_on_delivery_failure();

COMMIT;
