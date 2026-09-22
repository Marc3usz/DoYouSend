# ADR-0002: Podział domenowy zamiast warstwowego

- **Status:** przyjęta
- **Data:** 2026-09-22
- **Uczestnicy:** zespół (3 osoby)

## Kontekst

Trzy osoby pracują równolegle, każda z własną sesją Claude Code. Główny koszt przy takim
trybie pracy to konflikty w gicie i wzajemne blokowanie się.

## Decyzja

Kod dzielimy po domenach (`internal/recipients`, `internal/messaging`, ...), a każdej domenie
przypisujemy właściciela (`docs/podzial-pracy.md`, `.github/CODEOWNERS`). Zależności między
domenami wyłącznie przez interfejsy deklarowane przez konsumenta.

## Rozważane alternatywy

- **Podział warstwowy (front / back / infra)** — każda funkcja wymagałaby zmian u dwóch osób
  i ciągłej synchronizacji.
- **Brak własności** — najszybszy start, ale przy trzech agentach kończy się konfliktami
  w tych samych plikach.

## Konsekwencje

- Mało konfliktów: pliki wspólne to `platform/`, `migrations/`, `openapi.yaml` i manifesty.
  Te wymagają uprzedzenia zespołu (zasada zapisana w `CLAUDE.md`).
- Właściciel domeny jest domyślnym recenzentem, więc review trafia do osoby z kontekstem.
- Ryzyko silosów; przeciwdziałanie: review krzyżowe i wspólna sesja na M3 (wysyłka end-to-end).
