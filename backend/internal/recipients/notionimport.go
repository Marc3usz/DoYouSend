package recipients

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Marc3usz/DoYouSend/backend/internal/integrations/notion"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// NotionConfig is the Notion database the panel import reads (ADR-0017).
// The values come from the settings panel (ADR-0018), read on every request.
type NotionConfig struct {
	Token        string
	DatabaseID   string // ID or URL
	DataSourceID string // wins over DatabaseID when set
	PropertyMap  string // "Notion property=column,...", see notion.ParseMapping
}

// TableFunc turns an import table (header row first) into its per-row
// report; Importer.CheckTable and Importer.ImportTable are TableFuncs.
type TableFunc func(ctx context.Context, table [][]string) (ImportReport, error)

// NotionImport serves POST /api/recipients/import/notion/check and
// POST /api/recipients/import/notion. Rows go through the same TableFunc
// pipeline as an uploaded file; the answer adds the Notion page of every row.
// iam.Middleware lets only administrators through (iam/policy.go).
type NotionImport struct {
	Config  func() NotionConfig
	Process TableFunc
	// Imported, when set, runs after a successful call, e.g. to write the
	// audit log. Only the storing endpoint sets it.
	Imported func(ctx context.Context, rep ImportReport, pages int)
	// ClientOptions are passed to notion.NewClient (tests point it at a fake).
	ClientOptions []notion.Option
	Logger        *slog.Logger
}

func (h NotionImport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cfg := h.Config()
	if strings.TrimSpace(cfg.Token) == "" ||
		(strings.TrimSpace(cfg.DatabaseID) == "" && strings.TrimSpace(cfg.DataSourceID) == "") {
		writeFileError(w, http.StatusConflict, "notion_not_configured",
			"set the Notion token and database in the settings panel")
		return
	}
	mapping, err := notion.ParseMapping(cfg.PropertyMap)
	if err != nil {
		writeFileError(w, http.StatusUnprocessableEntity, "notion_invalid_config", err.Error())
		return
	}
	client, err := notion.NewClient(cfg.Token, h.ClientOptions...)
	if err != nil {
		writeFileError(w, http.StatusConflict, "notion_not_configured", err.Error())
		return
	}

	pages, err := client.Pages(r.Context(), cfg.DatabaseID, cfg.DataSourceID, MaxImportRows)
	if err != nil {
		writeNotionError(w, logger, err)
		return
	}
	if len(pages) == 0 {
		writeFileError(w, http.StatusUnprocessableEntity, "empty_file", "the Notion database has no rows")
		return
	}

	report, err := h.Process(r.Context(), notion.ToTable(pages, mapping))
	if err != nil {
		writeNotionTableError(w, logger, err)
		return
	}
	if h.Imported != nil {
		h.Imported(r.Context(), report, len(pages))
	}
	httpx.JSON(w, http.StatusOK, toNotionReportJSON(report, pages))
}

// writeNotionError maps a failure to read Notion to its status and code.
// Notion error messages never carry the token.
func writeNotionError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var apiErr *notion.APIError
	switch {
	case errors.Is(err, notion.ErrInvalidID), errors.Is(err, notion.ErrNoSource):
		writeFileError(w, http.StatusUnprocessableEntity, "notion_invalid_config", err.Error())
	case errors.Is(err, notion.ErrMultipleDataSources):
		writeFileError(w, http.StatusUnprocessableEntity, "notion_ambiguous_source", err.Error())
	case errors.Is(err, notion.ErrNoDataSource):
		writeFileError(w, http.StatusUnprocessableEntity, "notion_no_source", err.Error())
	case errors.Is(err, notion.ErrTooManyRows):
		writeFileError(w, http.StatusUnprocessableEntity, "too_many_rows", err.Error())
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized:
		writeFileError(w, http.StatusBadGateway, "notion_unauthorized", err.Error())
	case errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusForbidden):
		writeFileError(w, http.StatusBadGateway, "notion_not_found", err.Error())
	default:
		logger.Warn("recipients notion import: read notion", "err", err)
		writeFileError(w, http.StatusBadGateway, "notion_unavailable", err.Error())
	}
}

// writeNotionTableError maps a failure of the import pipeline.
func writeNotionTableError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, ErrMissingColumns):
		writeFileError(w, http.StatusUnprocessableEntity, "missing_columns", err.Error())
	case errors.Is(err, ErrEmptyFile), errors.Is(err, ErrInvalidHeader):
		writeFileError(w, http.StatusUnprocessableEntity, "empty_file", err.Error())
	case errors.Is(err, ErrTooManyRows):
		writeFileError(w, http.StatusUnprocessableEntity, "too_many_rows", err.Error())
	default:
		// Nothing was stored (ImportTable is all-or-nothing). The error
		// carries no contact data.
		logger.Error("recipients notion import: storage", "err", err)
		writeFileError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

type (
	notionReportJSON struct {
		reportJSON
		PageCount int              `json:"pageCount"`
		Pages     []notionPageJSON `json:"pages"`
	}
	notionPageJSON struct {
		Row    int    `json:"row"`
		PageID string `json:"pageId"`
		URL    string `json:"url"`
	}
)

// toNotionReportJSON adds the page of every row: table row i+2 is pages[i]
// (row 1 is the header, see notion.ToTable).
func toNotionReportJSON(rep ImportReport, pages []notion.Page) notionReportJSON {
	out := notionReportJSON{reportJSON: toReportJSON(rep), PageCount: len(pages), Pages: make([]notionPageJSON, len(pages))}
	for i, p := range pages {
		out.Pages[i] = notionPageJSON{Row: i + 2, PageID: p.ID, URL: p.Link()}
	}
	return out
}
