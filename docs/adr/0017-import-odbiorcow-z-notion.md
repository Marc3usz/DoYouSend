# ADR-0017: Import odbiorców z bazy Notion

- **Status:** przyjęta (zatwierdzona przez MichalK252 w #86; decyzja 4 zmieniona w PR-ze z panelem importu)
- **Data:** 2026-11-03
- **Uczestnicy:** Marc3usz (domena A: odbiorcy — właściciel), MichalK252 (do wiadomości)

## Kontekst

Szkoła (lub osoba prowadząca rekrutację) trzyma listy kontaktów — „leady”, przyszłych rodziców
i uczniów — w bazie Notion. Dziś jedyną drogą do tabeli `recipients` jest eksport do CSV/XLSX
i wgranie pliku w panelu. Chcemy, żeby import prosto z Notion był łatwy do dołożenia, bez
budowania od razu pełnego UI.

Ograniczenia:

- Wiersze z Notion muszą przejść **tę samą** walidację, normalizację telefonu i wykrywanie
  duplikatów co plik (reguła 6, raport błędnych/niepełnych/powtórzonych danych z `description.md`).
- Bez nowych zależności bez ADR-u; bez prawdziwych danych w repozytorium.
- Notion od wersji API `2025-09-03` rozdziela bazę (*database*) i jej źródła danych
  (*data sources*): zapytania o wiersze idą do `POST /v1/data_sources/{id}/query`, a
  `GET /v1/databases/{id}` zwraca listę `data_sources`. Stara ścieżka
  `POST /v1/databases/{id}/query` przestaje działać, gdy ktoś doda bazie drugie źródło danych.
- Limit ruchu Notion: ok. 3 zapytania/s na integrację, `429` z `Retry-After`.

## Decyzja

1. **Klient na bibliotece standardowej** w `backend/internal/integrations/notion`
   (`net/http` + `encoding/json`), z nagłówkiem `Notion-Version: 2025-09-03`, tokenem integracji
   wewnętrznej (tylko odczyt), stronicowaniem po 100 i ponawianiem `429`/`5xx` według
   `Retry-After` (maks. 30 s, 4 próby). Pakiet niczego nie zapisuje w Notion.
2. **Reużycie potoku importu:** Notion → tabela `[][]string` (nagłówek + wiersze) →
   `recipients.Importer.CheckTable` / `ImportTable`, które korzystają z tych samych
   `parseRows`/`classify`/zapisu w jednej transakcji co plik. Nazwy kolumn jak w szablonie CSV
   działają bez konfiguracji; inne mapuje `NOTION_PROPERTY_MAP`.
3. **Najpierw komenda CLI** `cmd/import-notion` (`make import-notion`): domyślnie tylko raport,
   `-apply` zapisuje. Raport zawiera numery wierszy i ID stron, bez danych osobowych.
4. Endpoint HTTP i ekran w panelu administratora osobnym PR-em ze zmianą
   `docs/api/openapi.yaml` (kontrakt w #93): `POST /api/recipients/import/notion/check`
   i `/import/notion`, tylko administrator, przycisk w `/odbiorcy/import`. **Zmiana
   (2026-11-03):** token, baza, źródło danych i mapowanie kolumn ustawia się w panelu
   (`/admin/ustawienia`, grupa „Import odbiorców z Notion”), a token jest szyfrowany jak klucze
   operatorów — zgodnie z ADR-0018, który zastąpił „konfigurację tylko przez `.env`” z ADR-0015.
   Zmienne `NOTION_*` w `.env` działają dalej jako wartości zapasowe i dla komendy CLI.

## Rozważane alternatywy

- **Oficjalne SDK** — Notion nie ma oficjalnego SDK dla Go; nieoficjalne (`jomei/notionapi`)
  to nowa zależność, która nie nadąża za zmianą na *data sources*. Potrzebujemy dwóch wywołań.
- **Stara ścieżka `databases/{id}/query`** — prostsza, ale psuje się po dodaniu drugiego źródła
  danych i jest wycofywana.
- **Osobna walidacja dla Notion** — dublowałaby reguły importu pliku i rozjeżdżała się z nimi.
- **Synchronizacja ciągła (webhooki Notion)** — wymaga publicznego endpointu i rozstrzygania
  konfliktów edycji; na razie wystarczy import na żądanie.

## Konsekwencje

- Import z Notion i z pliku mają identyczne reguły i ten sam format raportu.
- Baza z ponad `MaxImportRows` (5000) stronami jest odrzucana w całości, a nie importowana częściowo.
- Import tylko dodaje nowych odbiorców; istniejący kontakt to duplikat (jak przy pliku) —
  aktualizacja istniejących rekordów z Notion to osobna decyzja.
- Zapis z panelu trafia do dziennika audytu jako `recipients_imported` (źródło, liczba stron,
  zapisanych, błędnych i duplikatów — bez danych osobowych). Samo sprawdzenie nie jest logowane.
- Raport z panelu ma przy każdym wierszu link do strony w Notion, żeby poprawić dane u źródła.
