# ADR-0006: Import odbiorców z XLSX przez bibliotekę excelize

- **Status:** propozycja — do akceptacji w review PR-a z importem odbiorców; przed merge
  zmienić na „przyjęta”
- **Data:** 2026-09-29
- **Uczestnicy:** Marc3usz (DEV A) proponuje; zależność w `go.mod` wymaga zgody zespołu

## Kontekst

`description.md` wymaga importu większej liczby osób z pliku. Szkoły trzymają listy uczniów
i rodziców w Excelu, więc sam CSV zmusza administratora do eksportu, przy którym łatwo o
złe kodowanie (Windows-1250) albo separator. `backend/internal/recipients` ma już import CSV
(biblioteka standardowa) i potrzebuje czytać także XLSX.

Do tej pory `backend/go.mod` nie miał żadnych zależności. Zgodnie z `CLAUDE.md` nowa
zależność wymaga ADR.

## Decyzja

Pliki XLSX czytamy biblioteką `github.com/xuri/excelize/v2` w wersji **v2.10.1**. To
ostatnia wersja działająca na Go 1.24 z ADR-0001; v2.11 wymaga Go 1.25. Biblioteki używa
wyłącznie `internal/recipients/xlsximport.go`. Czytamy tylko pierwszy arkusz, surowe wartości
komórek, z limitem rozmiaru po rozpakowaniu (64 MB).

## Rozważane alternatywy

- **Własny czytnik na `archive/zip` + `encoding/xml`** — bez zależności, ok. 200 linii.
  Odrzucone: musielibyśmy sami utrzymywać obsługę shared strings, inline strings, pustych
  komórek, przesunięć wierszy i dziwnych zapisów z różnych wersji Excela i LibreOffice.
  Dojrzała biblioteka robi to lepiej niż kod pisany pod projekt.
- **`github.com/tealeg/xlsx`** — mniej używana od excelize; nie widzimy przewagi, która
  uzasadniałaby wybór mniejszej społeczności.
- **Tylko CSV** — najprościej, ale przerzuca na administratora konwersję i ryzyko złego
  kodowania, czyli właśnie te błędy, które import ma wskazywać.

## Konsekwencje

- `go.mod` zyskuje excelize i 8 zależności pośrednich (m.in. `golang.org/x/crypto`,
  `x/net`, `x/text`, `richardlehane/mscfb`). Aktualizacje bezpieczeństwa tych pakietów
  trzeba śledzić, tak jak każdej innej zależności.
- Podbicie Go do 1.25 (np. w nowym ADR-0001) pozwoli wejść na excelize v2.11+.
- excelize trzyma cały skoroszyt w pamięci, dlatego import ma limit 5 MB na plik i 64 MB
  po rozpakowaniu (ochrona przed zip bombą, czyli małym archiwum, które po rozpakowaniu
  zajmuje ogromnie dużo miejsca). Limity są w `internal/recipients/importfile.go` i
  `xlsximport.go`.
- Stare `.xls` i skoroszyty chronione hasłem nie są obsługiwane; import zwraca czytelny
  błąd z prośbą o zapis jako `.xlsx` bez hasła.
- Nikt poza `internal/recipients` nie importuje excelize. Gdyby kolejna domena potrzebowała
  Excela (np. eksport historii), wracamy do tego ADR-u.
