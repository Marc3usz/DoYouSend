// Command import-notion imports recipients from a Notion database (ADR-0017).
// Rows go through the same validation and duplicate checks as a CSV/XLSX
// upload. By default it only checks and prints a report; -apply stores the
// valid rows.
//
//	NOTION_TOKEN=ntn_... NOTION_DATABASE_ID=<id or URL> go run ./cmd/import-notion
//	NOTION_TOKEN=ntn_... NOTION_DATABASE_ID=<id or URL> go run ./cmd/import-notion -apply
//
// Environment:
//
//	NOTION_TOKEN           internal integration secret (required)
//	NOTION_DATABASE_ID     database ID or URL; its only data source is read
//	NOTION_DATA_SOURCE_ID  data source ID, needed when the database has several (wins)
//	NOTION_PROPERTY_MAP    optional "Notion property=column,..." (see notion.ParseMapping)
//	DATABASE_URL           the DoYouSend database
//
// The report names rows, Notion page IDs and fields, never names, e-mails or
// phone numbers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/integrations/notion"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/config"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "import-notion:", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	if _, _, err := config.LoadDevEnv(); err != nil {
		return fmt.Errorf("load env file: %w", err)
	}
	apply := flag.Bool("apply", false, "store the valid rows (default: only check and report)")
	timeout := flag.Duration("timeout", 5*time.Minute, "give up after this long")
	flag.Parse()

	client, err := notion.NewClient(os.Getenv("NOTION_TOKEN"))
	if err != nil {
		return fmt.Errorf("%w (set NOTION_TOKEN)", err)
	}
	mapping, err := notion.ParseMapping(os.Getenv("NOTION_PROPERTY_MAP"))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pages, err := client.Pages(ctx, os.Getenv("NOTION_DATABASE_ID"), os.Getenv("NOTION_DATA_SOURCE_ID"), recipients.MaxImportRows)
	if errors.Is(err, notion.ErrNoSource) {
		return errors.New("set NOTION_DATABASE_ID (or NOTION_DATA_SOURCE_ID)")
	}
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		_, _ = fmt.Fprintln(out, "Źródło danych w Notion jest puste - nie ma czego importować.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	importer := recipients.NewImporter(recipients.NewPGStore(pool))

	table := notion.ToTable(pages, mapping)
	var report recipients.ImportReport
	if *apply {
		report, err = importer.ImportTable(ctx, table)
	} else {
		report, err = importer.CheckTable(ctx, table)
	}
	if err != nil {
		return err
	}
	printReport(out, report, pages, *apply)
	return nil
}

// printReport prints counts and, for every rejected row, its row number, the
// Notion page ID and the field problems. Table row r is pages[r-2] (row 1 is
// the header), see notion.ToTable.
func printReport(out io.Writer, rep recipients.ImportReport, pages []notion.Page, applied bool) {
	pageID := func(row int) string {
		if i := row - 2; i >= 0 && i < len(pages) {
			return pages[i].ID
		}
		return "?"
	}
	verb := "do zaimportowania"
	if applied {
		verb = "zaimportowane"
	}
	_, _ = fmt.Fprintf(out, "Stron w Notion: %d\nPoprawne (%s): %d\nBłędne: %d\nDuplikaty: %d\n",
		len(pages), verb, len(rep.Valid), len(rep.Invalid), len(rep.Duplicates))

	for _, r := range rep.Invalid {
		problems := make([]string, len(r.Errors))
		for i, e := range r.Errors {
			problems[i] = e.Field + ": " + e.Message
		}
		_, _ = fmt.Fprintf(out, "  błąd      wiersz %d (strona %s): %s\n", r.Row, pageID(r.Row), strings.Join(problems, "; "))
	}
	for _, d := range rep.Duplicates {
		of := fmt.Sprintf("wiersz %d", d.DuplicateOf)
		if d.DuplicateOf == 0 {
			of = "odbiorca " + d.ExistingID
		}
		_, _ = fmt.Fprintf(out, "  duplikat  wiersz %d (strona %s): pole %s już ma %s\n", d.Row, pageID(d.Row), d.Field, of)
	}
	if !applied && len(rep.Valid) > 0 {
		_, _ = fmt.Fprintln(out, "Nic nie zapisano. Uruchom z -apply, aby zaimportować poprawne wiersze.")
	}
}
