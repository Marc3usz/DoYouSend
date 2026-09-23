# Podział pracy — 4 programistów

Podział **domenowy**: każda osoba dostaje pionowy wycinek systemu (baza → backend → UI),
dzięki czemu pracuje w swoich katalogach i rzadko koliduje z innymi w gicie.

| | Osoba | Domena | Katalogi | Zakres z `description.md` |
|---|---|---|---|---|
| **DEV A** | Marc3usz | Odbiorcy i grupy | `backend/internal/recipients`, `backend/internal/groups`, `frontend/src/routes/odbiorcy`, `frontend/src/routes/grupy` | baza odbiorców, import z pliku, walidacja i duplikaty, grupy systemowe i własne, rozwinięcie wyboru do listy odbiorców bez duplikatów |
| **DEV B** | S1D0R-10 | Wiadomości i wysyłka | `backend/internal/messaging`, `backend/internal/delivery`, `frontend/src/routes/wiadomosci`, `frontend/src/routes/historia` | tworzenie komunikatu, podgląd, liczenie części SMS i kosztu, kolejka, statusy per kanał, ponowna wysyłka, historia |
| **DEV C** | MichalK252 | Integracje (e-mail/SMS) | `backend/internal/providers` | bramka SMS, usługa e-mail, providerzy `fake`/Mailpit za wspólnym interfejsem, klasyfikacja błędów trwałych vs. do ponowienia |
| **DEV D** | averithefox | Admin, bezpieczeństwo, CI | `backend/internal/iam`, `frontend/src/routes/admin`, `frontend/src/routes/login`, `.github/workflows` | logowanie i role, panel administracyjny, audyt, pipeline CI |

Własność = **odpowiedzialność za review**, nie blokada. Możesz zmienić cudzy pakiet —
oznacz właściciela w PR.

> Historia: do 2026-09-23 domena integracje+admin+CI była jedną osobą (DEV C). Przy
> dołączeniu czwartej osoby podzielona na integracje (C) oraz admin/bezpieczeństwo/CI (D).
> Zob. `docs/adr/0004-czwarty-deweloper.md`.

## Kolejność prac (kamienie milowe)

**M1 — fundament (wspólnie, pierwszy tydzień).** Uzgodnione: schemat bazy (`0001_init.sql`),
kontrakt API (`docs/api/openapi.yaml`), logowanie i role, szkielet UI. Bez tego pozostałe
prace będą się non stop rozjeżdżać.

**M2 — równolegle.**
- A: CRUD odbiorców + import CSV z raportem błędów + grupy + `Resolve()` z deduplikacją.
- B: kreator wiadomości + podgląd + podsumowanie (liczba odbiorców, części SMS, koszt).
- C: providerzy `fake`/Mailpit za wspólnym interfejsem `providers`.
- D: logowanie i role + panel admina + audyt + szkielet CI.

**M3 — wysyłka end-to-end.** B spina kolejkę i statusy z providerami C i listą odbiorców A.
Tu potrzebna jest największa koordynacja — najlepiej wspólna sesja.

**M4 — domknięcie kryteriów odbioru.** Częściowe wysyłki, ponowna wysyłka nieudanych,
historia, instrukcja uruchomienia, przegląd repo pod kątem sekretów i danych (D koordynuje
przegląd bezpieczeństwa).

## Punkty styku (ustalcie przed M2)

| Styk | Kto z kim | Ustalenie |
|---|---|---|
| `Resolve(selection) -> []Recipient` | A → B | kształt struktury odbiorcy i sygnalizowanie braków danych |
| `Provider.Send(ctx, msg) -> (id, error)` | C → B | wspólny interfejs, klasyfikacja błędów (trwały vs. do ponowienia) |
| liczenie części SMS | B → C | jedna implementacja w `messaging`, provider jej używa, frontend tylko wyświetla |
| raporty doręczeń | C → B | webhook/poll aktualizujący `deliveries.status` |
| logowanie i role | D → wszyscy | middleware auth, kształt sesji/tokenu, role wymagane na poszczególnych endpointach |
| kontrakt API | wszyscy | `docs/api/openapi.yaml`, zmiana = osobny PR |

## Rytm pracy

- Krótka synchronizacja na starcie dnia: co robię, co blokuje, czy ruszam wspólny plik.
- Zmiana w `platform/`, `migrations/`, `openapi.yaml`, `go.mod`, `package.json` — uprzedź zespół.
- PR mniejszy niż ~400 zmienionych linii; review tego samego dnia.
