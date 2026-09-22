-- Dane demonstracyjne. WYLACZNIE fikcyjne: domena example.test, numery z puli testowej.
-- Nigdy nie wolno podmienic tego pliku na prawdziwy eksport ze szkoly.

INSERT INTO users (email, full_name, role, password_hash) VALUES
  ('dyrektor@example.test', 'Anna Testowa',  'sender', 'do-ustawienia-lokalnie'),
  ('admin@example.test',    'Piotr Przykladowy', 'admin',  'do-ustawienia-lokalnie')
ON CONFLICT (email) DO NOTHING;

INSERT INTO recipients (first_name, last_name, email, phone, type) VALUES
  ('Jan',      'Kowalski',  'jan.kowalski@example.test',      '+48500100101', 'parent'),
  ('Maria',    'Kowalska',  'maria.kowalska@example.test',    '+48500100102', 'parent'),
  ('Tomasz',   'Nowak',     'tomasz.nowak@example.test',      NULL,           'parent'),
  ('Zofia',    'Wisniewska', NULL,                            '+48500100104', 'parent'),
  ('Kacper',   'Kowalski',  'kacper.kowalski@example.test',   '+48500100105', 'student'),
  ('Lena',     'Nowak',     'lena.nowak@example.test',        '+48500100106', 'student')
ON CONFLICT DO NOTHING;

INSERT INTO groups (name, description, is_system) VALUES
  ('Wszyscy rodzice',  'Grupa systemowa', true),
  ('Wszyscy uczniowie','Grupa systemowa', true),
  ('Rodzice 3A',       'Rodzice uczniow klasy 3A', false)
ON CONFLICT (name) DO NOTHING;

INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g JOIN recipients r ON r.type = 'parent'
WHERE g.name = 'Wszyscy rodzice' ON CONFLICT DO NOTHING;

INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g JOIN recipients r ON r.type = 'student'
WHERE g.name = 'Wszyscy uczniowie' ON CONFLICT DO NOTHING;

-- Celowo nakladajaca sie grupa: sluzy do testu deduplikacji odbiorcow.
INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g JOIN recipients r ON r.last_name IN ('Kowalski','Kowalska') AND r.type = 'parent'
WHERE g.name = 'Rodzice 3A' ON CONFLICT DO NOTHING;
