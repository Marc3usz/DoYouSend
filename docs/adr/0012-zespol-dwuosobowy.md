# ADR-0012: Zespół dwuosobowy — przejęcie domen B i D

- **Status:** propozycja — czeka na akceptację MichalK252
- **Data:** 2026-10-27
- **Uczestnicy:** Marc3usz (domeny A, D), MichalK252 (domeny B, C)

## Kontekst

S1D0R-10 (domena B — wiadomości i wysyłka) oraz averithefox (domena D — IAM, admin, CI) od kilku
tygodni nie dostarczali zmian ani recenzji. Ich prace i tak wykonywali pozostali: Marc3usz napisał
`iam` i panel admina (PR #36, ADR-0010), kreator z wysyłką i historią (#55); MichalK252 —
odporność wysyłki i ponowienia (#44, #54, #57, #61). Decyzje czekające na tę dwójkę (ADR-0010,
ADR-0011) blokowały odbiór projektu. 2026-10-27 obie osoby zostały usunięte z repozytorium.

## Decyzja

Domeny zostają bez zmian (A–D, `docs/podzial-pracy.md`), zmieniają się właściciele, po równo:

- **Marc3usz:** A (odbiorcy, grupy) + D (`iam`, `admin/`, `login/`, `.github/`, przegląd
  bezpieczeństwa).
- **MichalK252:** C (providers) + B (`messaging`, `delivery`, `wiadomosci/`, `historia/`).

Każdy PR recenzuje druga osoba (jedno zatwierdzenie, jak dotąd).

## Rozważane alternatywy

- **Podział B na pół (messaging / delivery)** — odrzucone: to jedna ścieżka kompozycja → kolejka →
  status, rozdzielenie dodałoby punkt styku między dwiema osobami.
- **B do Marc3usz, D do MichalK252** — odrzucone: Marc3usz napisał `iam`, a MichalK252 od tygodni
  pracuje w `delivery`; zamiana oznaczałaby wdrażanie się obu osób w cudzy kod.

## Konsekwencje

- ADR-0010 i ADR-0011 przechodzą na **przyjęte** — ich właściciele są teraz autorami.
- `.github/CODEOWNERS`, `CLAUDE.md`, `frontend/CLAUDE.md` i komentarze właścicieli w kodzie
  wskazują nowe osoby.
- Styk B ↔ C (interfejs `providers`, raporty doręczeń) jest teraz wewnątrz domeny jednej osoby —
  interfejsy zostają, żeby kolejna zmiana składu nie wymagała przepisywania kodu.
- Ochrona gałęzi `master` (1 zatwierdzenie, `require_last_push_approval`) nadal działa: autor nie
  może zatwierdzić własnego PR-a, więc każda zmiana przechodzi przez drugą osobę.
- ADR-0004 zostaje jako historia; jego przypisania osób są nieaktualne.
