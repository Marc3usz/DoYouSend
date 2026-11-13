# ADR-0021: Nadawca e-maila wybierany przy wysyłce, uwierzytelnianie domeny i delegowanie adresów

- **Status:** przyjęta (2026-11-13)
- **Data:** 2026-11-13
- **Uczestnicy:** Marc3usz (wszystkie domeny po ADR-0020); rozstrzyga punkt otwarty ADR-0016
- **Dotyczy:** ADR-0008, ADR-0014, ADR-0016, ADR-0018

## Kontekst

- Każdy e-mail wychodził z jednego `EMAIL_FROM`. ADR-0016 pozwolił administratorowi zweryfikować
  w SendGrid adres pracownika (Single Sender), ale wysyłka z niego nie korzystała. Punkt „czy
  wysyłać z adresu autora” czekał na decyzję właściciela domen B i C, który odszedł z projektu.
- Szkoła chce, żeby nauczyciel czy dyrektor wysyłał „od siebie”, a sekretariat mógł przekazać
  wybranym osobom wspólną skrzynkę (np. `sekretariat@`).
- SendGrid wysyła tylko z adresu, który jest zweryfikowanym pojedynczym nadawcą albo należy do
  uwierzytelnionej domeny (Domain Authentication, `/v3/whitelabel/domains`). Weryfikowanie
  każdej osoby osobno nie skaluje się na szkołę.

## Decyzja

1. **Nadawca jest wybierany przy każdym wsadzie** spośród opcji zalogowanego użytkownika
   (`GET /api/me/senders`):
   - `shared` — `EMAIL_FROM` z ustawień, z opcjonalnym `Reply-To` (`email_reply_to`);
   - `own` — adres logowania użytkownika;
   - `delegated` — skrzynki, które administrator mu przydzielił (`sender_addresses`,
     `sender_address_users`, migracja 0012).
   `POST /api/batches` przyjmuje `senderId`. Backend sprawdza, czy opcja należy do autora i czy
   operator ją teraz przyjmie, a adres zapisuje we wsadzie (`from_email`, `from_name`,
   `reply_to`, migracja 0011). Zmienia się tylko nagłówek nadawcy, treść e-maila i SMS-a jest
   nadal bajt w bajt ta sama (CLAUDE.md, zasada 4). Nadawca SMS zostaje wspólny.
2. **Kiedy adres jest użyteczny.** Gdy operator sam sprawdza nadawców (SendGrid przy wyłączonym
   DRY_RUN), adres jest użyteczny, jeśli jego domena ma status `valid` albo jest zweryfikowanym
   pojedynczym nadawcą (ADR-0016). W innych trybach (DRY_RUN, Mailpit/SMTP) każdy przydzielony
   adres jest użyteczny.
3. **Uwierzytelnianie domeny z panelu** (Panel administratora → Nadawcy): administrator dodaje
   domenę, backend zakłada ją w SendGrid z `automatic_security` i pokazuje trzy rekordy CNAME do
   dodania w DNS szkoły, a „Sprawdź DNS” prosi SendGrid o walidację. Przy DRY_RUN albo bez
   SendGrid działa atrapa w pamięci (`email.FakeDomains`): rekordy są przykładowe i nic nie
   trafia do SendGrid. Klucz API potrzebuje uprawnienia „Sender Authentication”.
4. **Domyślny nadawca:** `own`, gdy jest użyteczny i ustawienie `email_sender_default=own`
   (domyślne). W przeciwnym razie `shared`. Dzięki temu po uwierzytelnieniu domeny szkoły
   wszyscy jej pracownicy wysyłają automatycznie ze swoich adresów, bez dodatkowej konfiguracji.
5. **Audyt:** `sender_domain_added`, `sender_domain_removed`, `sender_address_added`,
   `sender_address_updated`, `sender_address_removed`. Adres nadawcy każdego wsadu jest w historii.

## Rozważane alternatywy

- **Zawsze wysyłać z adresu autora.** Odrzucone: konta administratorów i osób spoza domeny
  szkoły nie zawsze mają adres, z którego operator przyjmie pocztę, a sekretariat chce wysyłać
  ze wspólnej skrzynki.
- **Dowolny adres nadawcy wpisywany w kreatorze.** Odrzucone: każdy mógłby podszyć się pod
  dyrekcję. Delegowanie zostawia kontrolę administratorowi i ślad w dzienniku.
- **Tylko pojedynczy nadawcy (ADR-0016).** Zostają, ale jako wyjątek: każdy pracownik musiałby
  kliknąć link weryfikacyjny, a podpis DKIM domeny daje lepszą doręczalność.

## Konsekwencje

- Rekordy DNS dodaje osoba z dostępem do DNS szkoły; do tego czasu adresy w domenie są
  nieużyteczne (w kreatorze widać powód), a wysyłka idzie ze wspólnego adresu.
- Odpowiedzi na wysyłkę z adresu własnego lub delegowanego trafiają do tego adresu. Dla
  wspólnego adresu decyduje `email_reply_to`.
- Delegacja jest sprawdzana przy zatwierdzeniu wsadu, a ponowienie używa adresu zapisanego we
  wsadzie, więc cofnięcie delegacji nie zmienia historii.
