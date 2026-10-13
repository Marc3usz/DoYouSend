-- 0003_recipient_classes: klasy odbiorcow (ADR-0009).
-- Uczen ma najwyzej jedna klase (pilnuje walidacja w backend/internal/recipients),
-- rodzic dowolnie wiele - klasy swoich dzieci. Grupy klasowe sa wyliczane z tej tabeli
-- w kodzie (backend/internal/groups), bez wierszy w groups ani group_members.

CREATE TABLE recipient_classes (
    recipient_id uuid NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    -- znormalizowana nazwa, np. 3A (recipients.NormalizeClass)
    class_name   text NOT NULL CONSTRAINT recipient_classes_name_check CHECK (class_name ~ '^[0-9][0-9A-Z]{0,5}$'),
    PRIMARY KEY (recipient_id, class_name)
);

-- grupy klasowe i filtr GET /recipients?class= szukaja po nazwie klasy
CREATE INDEX recipient_classes_class_name_idx ON recipient_classes (class_name);
