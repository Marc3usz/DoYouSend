-- 0002_system_groups: wiersze grup systemowych o stalych UUID (ADR-0007, otwarte pytanie 2)
-- oraz nazwy grup unikalne bez wzgledu na wielkosc liter.
--
-- Czlonkostwo grup systemowych jest wyliczane w kodzie (backend/internal/groups/system.go)
-- z recipients.type, wiec te grupy nie maja wierszy w group_members.

-- scripts/migrate.sh nie owija pliku w transakcje: robimy to tutaj, zeby nieudany
-- CREATE INDEX nie zostawil skasowanych grup bez wpisu w schema_migrations.
BEGIN;

-- Starsze seedy wstawialy grupy systemowe z losowym UUID i materializowanym czlonkostwem.
-- Usuwamy je (group_members znika kaskadowo), zeby zwolnic nazwy dla wierszy ze stalym ID.
DELETE FROM groups
WHERE is_system
  AND id NOT IN ('00000000-0000-4000-a000-000000000001', '00000000-0000-4000-a000-000000000002');

INSERT INTO groups (id, name, description, is_system) VALUES
  ('00000000-0000-4000-a000-000000000001', 'Wszyscy rodzice',   'Każdy odbiorca typu rodzic.', true),
  ('00000000-0000-4000-a000-000000000002', 'Wszyscy uczniowie', 'Każdy odbiorca typu uczeń.',  true)
ON CONFLICT (id) DO UPDATE
  SET name = EXCLUDED.name, description = EXCLUDED.description, is_system = true;

-- Serwis porownuje nazwy bez wzgledu na wielkosc liter ("Rada rodzicow" = "rada Rodzicow");
-- baza pilnuje tego samego, takze przy wyscigu dwoch administratorow.
CREATE UNIQUE INDEX groups_name_lower_key ON groups (lower(name));

COMMIT;
