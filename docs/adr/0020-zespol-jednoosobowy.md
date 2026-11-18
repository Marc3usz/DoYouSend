# ADR-0020: Zespół jednoosobowy — Marc3usz prowadzi wszystkie domeny

- **Status:** przyjęta (2026-11-13)
- **Data:** 2026-11-13
- **Uczestnicy:** Marc3usz (domeny A–D)

## Kontekst

MichalK252 (domeny B — wiadomości i wysyłka — oraz C — integracje) został przeniesiony do innego
projektu. Od ADR-0012 zespół liczył dwie osoby i każdą zmianę zatwierdzała druga osoba. Bez
drugiej osoby ta zasada blokuje każdy merge, a projekt jest przed odbiorem.

## Decyzja

- **Marc3usz** jest właścicielem wszystkich czterech domen (A, B, C, D), w tym pakietów, które nie
  miały przypisania: `backend/internal/settings` (ustawienia wysyłki w panelu, ADR-0018) i
  `backend/internal/integrations/notion` (import z Notion, ADR-0017).
- Podział na domeny A–D **zostaje**: pakiety dalej rozmawiają przez interfejsy po stronie
  konsumenta, żeby kolejna osoba mogła przejąć domenę bez przepisywania kodu.
- Zamiast zatwierdzenia przez drugą osobę każdy PR przed merge'em ma:
  1. zielone CI (`make check`, testy jednostkowe z `-race`, testy integracyjne na Postgresie,
     build frontendu, skan sekretów i danych kontaktowych),
  2. przegląd subagentem `recenzent` (`.claude/agents/recenzent.md`) — identyczność treści,
     brak duplikatów, odporność wysyłki, sekrety i dane osobowe — z odpowiedzią na jego uwagi
     w opisie PR-a,
  3. przeczytanie całego diffu przez autora (także kodu wygenerowanego przez Claude Code).
- Ochronę gałęzi `master` trzeba dostosować tak, żeby PR autora dało się zmergować po zielonym
  CI (bez wymaganego zatwierdzenia), a bezpośredni push dalej był zablokowany.
- Decyzje, które wcześniej czekały na drugą osobę (np. otwarty punkt ADR-0016), rozstrzyga
  Marc3usz i zapisuje w ADR.

## Rozważane alternatywy

- **Zatwierdzenia od opiekuna projektu** — odrzucone jako warunek każdego PR-a: opiekun ocenia
  rozwiązanie (`description.md`), a nie przegląda bieżących zmian. Zostaje przy odbiorze i przed
  podłączeniem produkcyjnej wysyłki.
- **Zamrożenie domen B i C** — odrzucone: kryteria odbioru dotyczą głównie wysyłki i historii.

## Konsekwencje

- `docs/podzial-pracy.md`, `CLAUDE.md`, `frontend/CLAUDE.md`, `backend/CLAUDE.md`,
  `docs/workflow-git.md`, `CONTRIBUTING.md` i `.github/CODEOWNERS` wskazują jednego właściciela.
- Mniej oczu na każdej zmianie: większy nacisk na testy (w tym integracyjne) i na skan sekretów
  w CI. Zmiany wspólnych plików (`migrations/`, `openapi.yaml`, `platform/`) dalej idą osobnymi,
  małymi PR-ami, żeby łatwo było je przejrzeć później.
- Gdy dołączy druga osoba, wraca zasada jednego zatwierdzenia od innej osoby (ADR-0012),
  a ten ADR zostaje zastąpiony.
- ADR-0012 zostaje jako historia; jego przypisania osób są nieaktualne.
