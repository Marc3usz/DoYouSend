# ADR-0001: Stack technologiczny

- **Status:** przyjęta
- **Data:** 2026-09-22
- **Uczestnicy:** zespół (3 osoby)

## Kontekst

`description.md` zostawia wybór technologii zespołowi. Potrzebujemy stacku, w którym trzy
osoby pracują równolegle, a system realizuje masową wysyłkę dwoma kanałami z osobnymi
statusami i ponawianiem.

## Decyzja

- Backend: **Go 1.24** (`net/http`, `pgx`), worker wysyłki w tym samym module.
- Frontend: **SvelteKit 2 + Svelte 5 + TypeScript strict**.
- Baza: **PostgreSQL 16**, migracje jako numerowane pliki SQL bez zewnętrznego narzędzia.
- Kolejka: **Redis** jako broker zadań wysyłki. *(Nie wprowadzone — zob. uwagę poniżej.)*
- Lokalne środowisko: Docker Compose (Postgres, Redis, Mailpit).

> **Uwaga (2026-11-13):** Redis nigdy nie został użyty. Stan wysyłki żyje w PostgreSQL: każda
> dostawa (odbiorca × kanał) to wiersz `deliveries`, a worker wysyłki działa w procesie API
> (`backend/internal/delivery`). Zadania wsadu przerwanego restartem API są wznawiane przy
> starcie. Usługa `redis`, `REDIS_URL` i `Config.RedisURL` zostały usunięte; Docker Compose
> uruchamia tylko Postgres i Mailpit.

## Rozważane alternatywy

- **Jedna aplikacja Next.js** — mniej ruchomych części, ale gorzej znosi długo działającego
  workera i współbieżną wysyłkę.
- **Wysyłka synchroniczna w requeście** — prostsza, ale łamie wymaganie "błąd jednego
  odbiorcy nie zatrzymuje wysyłki" przy większych wsadach.
- **ORM zamiast SQL** — odrzucone: schemat jest niewielki, a kluczowe więzy (deduplikacja,
  unikalność kanału) chcemy mieć wprost w SQL-u.

## Konsekwencje

- Dwa osobne toolchainy (Go + Node), więc CI i `make doctor` muszą pilnować wersji.
- Kontrakt między backendem a frontendem trzeba utrzymywać jawnie: `docs/api/openapi.yaml`.
- Statusy per kanał i ponowna wysyłka są naturalne, bo jednostką pracy jest jeden wiersz
  `deliveries`.
