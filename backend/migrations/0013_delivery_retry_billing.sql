-- 0013_delivery_retry_billing: koszt prob sprzed ponowienia (ADR-0019).
-- Ponowienie czysci deliveries.provider_message_id, zeby spozniony raport o poprzedniej
-- probie nie nadpisal statusu nowej. Czesci SMS, ktore operator juz rozliczyl za tamta
-- probe (nieudana, ale przyjeta przez operatora), zostaja zapamietane tutaj, wiec
-- statystyka kosztow SMS ich nie gubi.

BEGIN;

ALTER TABLE deliveries
    ADD COLUMN billed_parts_before_retry integer NOT NULL DEFAULT 0
        CHECK (billed_parts_before_retry >= 0);

COMMIT;
