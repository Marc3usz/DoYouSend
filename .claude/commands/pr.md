---
description: Przygotowuje zmiany do review i tworzy pull request
argument-hint: [dodatkowy kontekst]
---

Dodatkowy kontekst: $ARGUMENTS

1. `git status` i `git diff origin/master...HEAD` — zbierz, co faktycznie się zmieniło.
2. Uruchom `make check` i `make test`. Jeśli coś jest czerwone — napraw, zanim pójdziesz dalej.
3. Przejdź checklistę z `.github/pull_request_template.md`:
   - nowe zmienne środowiskowe są w `.env.example`,
   - brak sekretów i prawdziwych danych osobowych w diffie,
   - migracja to nowy plik z kolejnym wolnym numerem,
   - zmiana kontraktu API / `platform/` jest wyraźnie zaznaczona.
4. Zacommituj w formacie Conventional Commits z zakresem pakietu.
5. Utwórz PR (`gh pr create`) z opisem wypełnionym wg szablonu, w tym sekcją
   "Wpływ na inne domeny" i informacją, kogo z zespołu warto dopisać do review
   (`docs/podzial-pracy.md`, `.github/CODEOWNERS`).

Push i utworzenie PR-a wymagają potwierdzenia — zapytaj przed wykonaniem.
