-- Dane demonstracyjne. WYLACZNIE fikcyjne: domena example.test, numery z puli testowej.
-- Nigdy nie wolno podmienic tego pliku na prawdziwy eksport ze szkoly.
-- Wymaga migracji (make migrate); mozna uruchamiac wielokrotnie.

INSERT INTO users (email, full_name, role, password_hash) VALUES
  ('dyrektor@example.test', 'Anna Testowa',  'sender', 'do-ustawienia-lokalnie'),
  ('admin@example.test',    'Piotr Przykladowy', 'admin',  'do-ustawienia-lokalnie')
ON CONFLICT (email) DO NOTHING;

-- Celowo sa tu osoby bez e-maila, bez telefonu i z blednym numerem: ekrany odbiorcow
-- i podglad wyboru musza takie braki pokazac (description.md). Bledny numer ('500-100')
-- nie normalizuje sie do zadnego poprawnego E.164, wiec import nie moze utworzyc
-- drugiej osoby z "tym samym" numerem, ktorej deduplikacja po ID by nie scalila.
INSERT INTO recipients (first_name, last_name, email, phone, type) VALUES
  ('Jan',       'Kowalski',    'jan.kowalski@example.test',      '+48500100101', 'parent'),
  ('Maria',     'Kowalska',    'maria.kowalska@example.test',    '+48500100102', 'parent'),
  ('Tomasz',    'Nowak',       'tomasz.nowak@example.test',      NULL,           'parent'),
  ('Zofia',     'Wisniewska',  NULL,                             '+48500100104', 'parent'),
  ('Ewa',       'Zielinska',   'ewa.zielinska@example.test',     '+48500100107', 'parent'),
  ('Marek',     'Wojcik',      'marek.wojcik@example.test',      '+48500100108', 'parent'),
  ('Agnieszka', 'Kaminska',    'agnieszka.kaminska@example.test','+48500100109', 'parent'),
  ('Pawel',     'Lewandowski', 'pawel.lewandowski@example.test', '500-100',      'parent'),
  ('Katarzyna', 'Dabrowska',   'katarzyna.dabrowska@example.test','+48500100111','parent'),
  ('Kacper',    'Kowalski',    'kacper.kowalski@example.test',   '+48500100105', 'student'),
  ('Lena',      'Nowak',       'lena.nowak@example.test',        '+48500100106', 'student'),
  ('Antoni',    'Zielinski',   'antoni.zielinski@example.test',  '+48500100112', 'student'),
  ('Julia',     'Wojcik',      NULL,                             '+48500100113', 'student'),
  ('Szymon',    'Kaminski',    'szymon.kaminski@example.test',   '+48500100114', 'student'),
  ('Hanna',     'Lewandowska', 'hanna.lewandowska@example.test', NULL,           'student'),
  ('Filip',     'Dabrowski',   'filip.dabrowski@example.test',   '+48500100116', 'student')
ON CONFLICT DO NOTHING;

-- Grupy systemowe ("Wszyscy rodzice", "Wszyscy uczniowie") wstawia migracja 0002, a ich
-- czlonkostwo wynika z recipients.type (ADR-0007) - nie dopisujemy ich do group_members.
INSERT INTO groups (name, description, is_system) VALUES
  ('Rodzice 3A',    'Rodzice uczniow klasy 3A', false),
  ('Rada rodzicow', 'Przedstawiciele rodzicow w radzie szkoly', false),
  ('Wycieczka 2B',  'Uczniowie i opiekunowie na wycieczce', false)
ON CONFLICT DO NOTHING;

-- Grupy celowo sie nakladaja: Jan Kowalski jest w "Rodzice 3A" i "Rada rodzicow",
-- a obie zawieraja sie we "Wszyscy rodzice" - test deduplikacji w podgladzie wyboru.
INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g
JOIN recipients r ON r.type = 'parent' AND r.last_name IN ('Kowalski', 'Kowalska', 'Nowak', 'Wisniewska')
WHERE g.name = 'Rodzice 3A'
ON CONFLICT DO NOTHING;

INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g
JOIN recipients r ON r.type = 'parent' AND r.email IN ('jan.kowalski@example.test', 'ewa.zielinska@example.test', 'pawel.lewandowski@example.test')
WHERE g.name = 'Rada rodzicow'
ON CONFLICT DO NOTHING;

INSERT INTO group_members (group_id, recipient_id)
SELECT g.id, r.id FROM groups g
JOIN recipients r ON (r.type = 'student' AND r.last_name IN ('Zielinski', 'Wojcik', 'Kaminski'))
                  OR r.email = 'marek.wojcik@example.test'
WHERE g.name = 'Wycieczka 2B'
ON CONFLICT DO NOTHING;
