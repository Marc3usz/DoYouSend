# Podział pracy

Podział **domenowy**: system ma cztery pionowe wycinki (baza → backend → UI), oznaczone A–D.
Od 2026-11-13 wszystkie domeny prowadzi jedna osoba — Marc3usz (ADR-0020). Podział zostaje,
bo wyznacza granice pakietów i interfejsów: kolejna osoba może przejąć domenę bez przepisywania
kodu.

| | Właściciel | Domena | Katalogi | Zakres z `description.md` |
|---|---|---|---|---|
| **A** | Marc3usz | Odbiorcy i grupy | `backend/internal/recipients`, `backend/internal/groups`, `backend/internal/integrations/notion`, `frontend/src/routes/odbiorcy`, `frontend/src/routes/grupy` | baza odbiorców, import z pliku i z Notion, walidacja i duplikaty, grupy systemowe, klasowe i własne, rozwinięcie wyboru do listy odbiorców bez duplikatów |
| **B** | Marc3usz | Wiadomości i wysyłka | `backend/internal/messaging`, `backend/internal/delivery`, `frontend/src/routes/wiadomosci`, `frontend/src/routes/historia` | tworzenie komunikatu, podgląd, liczenie części SMS i kosztu, wysyłka, statusy per kanał, ponowna wysyłka, historia |
| **C** | Marc3usz | Integracje (e-mail/SMS) | `backend/internal/providers` | bramka SMS, usługa e-mail, providerzy `fake`/Mailpit za wspólnym interfejsem, raporty doręczeń, klasyfikacja błędów trwałych vs. do ponowienia |
| **D** | Marc3usz | Admin, bezpieczeństwo, CI | `backend/internal/iam`, `backend/internal/settings`, `frontend/src/routes/admin`, `frontend/src/routes/login`, `.github/workflows` | logowanie i role, panel administratora, ustawienia wysyłki, audyt, pipeline CI, przegląd bezpieczeństwa |

Pliki wspólne (`backend/internal/platform`, `backend/migrations`, `docs/api/openapi.yaml`,
manifesty) nie należą do żadnej domeny — zmiana idzie osobnym, małym PR-em.

> Historia: do 2026-09-23 domena integracje+admin+CI była jedną osobą (DEV C), potem podzielona
> na C i D (ADR-0004). 2026-10-27 S1D0R-10 (B) i averithefox (D) odeszli; B przejął MichalK252,
> D Marc3usz (ADR-0012). 2026-11-13 MichalK252 został przeniesiony do innego projektu i domeny B
> oraz C przejął Marc3usz (ADR-0020).

## Kolejność prac (kamienie milowe)

**M1 — fundament.** Schemat bazy (`0001_init.sql`), kontrakt API (`docs/api/openapi.yaml`),
logowanie i role, szkielet UI.

**M2 — domeny równolegle.**
- A: CRUD odbiorców + import CSV z raportem błędów + grupy + `Resolve()` z deduplikacją.
- B: kreator wiadomości + podgląd + podsumowanie (liczba odbiorców, części SMS, koszt).
- C: providerzy `fake`/Mailpit za wspólnym interfejsem `providers`.
- D: logowanie i role + panel admina + audyt + szkielet CI.

**M3 — wysyłka end-to-end.** B spina wysyłkę i statusy z providerami C i listą odbiorców A.

**M4 — domknięcie kryteriów odbioru.** Częściowe wysyłki, ponowna wysyłka nieudanych,
historia, instrukcja uruchomienia, przegląd repo pod kątem sekretów i danych.

## Punkty styku

Wszystkie są teraz w rękach jednej osoby, ale nadal przechodzą przez interfejsy:

| Styk | Kierunek | Ustalenie |
|---|---|---|
| `Resolve(selection) -> []Recipient` | A → B | kształt struktury odbiorcy i sygnalizowanie braków danych (ADR-0007) |
| `Provider.Send(ctx, msg) -> (id, error)` | C → B | wspólny interfejs, klasyfikacja błędów (trwały vs. do ponowienia) |
| liczenie części SMS | B → C | jedna implementacja w `messaging`, provider jej używa, frontend tylko wyświetla |
| raporty doręczeń | C → B | webhook aktualizujący `deliveries.status` (ADR-0011) |
| ustawienia wysyłki | D → C | `settings.Service` buduje konfigurację providerów (ADR-0018) |
| logowanie i role | D → wszyscy | middleware auth, reguły w `iam/policy.go` (ADR-0010) |
| kontrakt API | wszyscy | `docs/api/openapi.yaml`, zmiana = osobny PR |

## Rytm pracy

- Zmiana w `platform/`, `migrations/`, `openapi.yaml`, `go.mod`, `package.json` — osobny, mały PR.
- PR mniejszy niż ~400 zmienionych linii, gdy to możliwe; przed merge'em zielone CI i przegląd
  subagentem `recenzent` (ADR-0020).
