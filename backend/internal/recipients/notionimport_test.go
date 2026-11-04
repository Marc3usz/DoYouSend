package recipients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/integrations/notion"
)

const testDatabaseID = "1429989f-e8ac-4eff-bc8f-57f56486db54"

// notionPage renders one Notion page with rich-text properties.
func notionPage(id string, props map[string]string) string {
	parts := make([]string, 0, len(props))
	for name, v := range props {
		parts = append(parts, fmt.Sprintf(`%q:{"type":"rich_text","rich_text":[{"plain_text":%q}]}`, name, v))
	}
	return fmt.Sprintf(`{"object":"page","id":%q,"url":"https://www.notion.so/%s","properties":{%s}}`,
		id, strings.ReplaceAll(id, "-", ""), strings.Join(parts, ","))
}

// fakeNotion answers the two calls the import makes. dbStatus/dbBody override
// the database response.
func fakeNotion(t *testing.T, dbStatus int, dbBody string, pages ...string) []notion.Option {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/databases/"):
			w.WriteHeader(dbStatus)
			_, _ = io.WriteString(w, dbBody)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/data_sources/ds-1/query":
			_, _ = fmt.Fprintf(w, `{"results":[%s],"has_more":false,"next_cursor":null}`, strings.Join(pages, ","))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	return []notion.Option{notion.WithBaseURL(srv.URL), notion.WithHTTPClient(srv.Client())}
}

const oneSource = `{"data_sources":[{"id":"ds-1","name":"Leady"}]}`

func serveNotion(t *testing.T, h NotionImport) *httptest.ResponseRecorder {
	t.Helper()
	if h.Logger == nil {
		h.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/recipients/import/notion/check", nil))
	return rec
}

func configured() NotionConfig {
	return NotionConfig{Token: "ntn_test", DatabaseID: "https://www.notion.so/Leady-1429989fe8ac4effbc8f57f56486db54"}
}

func TestNotionImportReport(t *testing.T) {
	opts := fakeNotion(t, http.StatusOK, oneSource,
		notionPage("p-1", map[string]string{"Name": "Anna", "Surname": "Testowa", "Mail": "anna.testowa@example.test", "Typ": "rodzic"}),
		notionPage("p-2", map[string]string{"Name": "Piotr", "Surname": "", "Mail": "piotr.testowy@example.test", "Typ": "uczeń"}),
		notionPage("p-3", map[string]string{"Name": "Ola", "Surname": "Testowa", "Mail": "ANNA.testowa@example.test", "Typ": "rodzic"}),
	)
	cfg := configured()
	cfg.PropertyMap = "Name=imię,Surname=nazwisko,Mail=e-mail,Typ=typ"
	// No "telefon" column in Notion: map an absent property onto it.
	cfg.PropertyMap += ",Telefon=telefon"

	rec := serveNotion(t, NotionImport{
		Config:        func() NotionConfig { return cfg },
		Process:       NewImporter(&fakeStore{}).CheckTable,
		ClientOptions: opts,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		Valid      []struct{ Row int }
		Invalid    []struct{ Row int }
		Duplicates []struct {
			Row            int
			DuplicateOfRow int
		}
		PageCount int
		Pages     []struct {
			Row    int
			PageID string
			URL    string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Valid) != 1 || got.Valid[0].Row != 2 ||
		len(got.Invalid) != 1 || got.Invalid[0].Row != 3 ||
		len(got.Duplicates) != 1 || got.Duplicates[0].DuplicateOfRow != 2 {
		t.Errorf("report = %+v", got)
	}
	if got.PageCount != 3 || len(got.Pages) != 3 || got.Pages[1].Row != 3 || got.Pages[1].PageID != "p-2" ||
		got.Pages[1].URL != "https://www.notion.so/p2" {
		t.Errorf("pages = %d %+v", got.PageCount, got.Pages)
	}
}

func TestNotionImportErrors(t *testing.T) {
	page := notionPage("p-1", map[string]string{"Imię": "Anna"})
	tests := []struct {
		name       string
		cfg        NotionConfig
		dbStatus   int
		dbBody     string
		pages      []string
		process    TableFunc
		wantStatus int
		wantCode   string
	}{
		{name: "no token", cfg: NotionConfig{DatabaseID: testDatabaseID}, wantStatus: 409, wantCode: "notion_not_configured"},
		{name: "no database", cfg: NotionConfig{Token: "ntn_test"}, wantStatus: 409, wantCode: "notion_not_configured"},
		{name: "bad id", cfg: NotionConfig{Token: "ntn_test", DatabaseID: "leady"}, wantStatus: 422, wantCode: "notion_invalid_config"},
		{name: "bad mapping", cfg: NotionConfig{Token: "ntn_test", DatabaseID: testDatabaseID, PropertyMap: "Name"}, wantStatus: 422, wantCode: "notion_invalid_config"},
		{name: "several sources", dbBody: `{"data_sources":[{"id":"ds-1","name":"A"},{"id":"ds-2","name":"B"}]}`, wantStatus: 422, wantCode: "notion_ambiguous_source"},
		{name: "no source", dbBody: `{"data_sources":[]}`, wantStatus: 422, wantCode: "notion_no_source"},
		{name: "bad token", dbStatus: 401, dbBody: `{"object":"error","status":401,"code":"unauthorized","message":"API token is invalid."}`, wantStatus: 502, wantCode: "notion_unauthorized"},
		{name: "not shared", dbStatus: 404, dbBody: `{"object":"error","status":404,"code":"object_not_found","message":"Could not find database"}`, wantStatus: 502, wantCode: "notion_not_found"},
		{name: "empty database", wantStatus: 422, wantCode: "empty_file"},
		{name: "missing columns", pages: []string{page}, wantStatus: 422, wantCode: "missing_columns"},
		{
			name: "storage", pages: []string{page},
			process:    func(context.Context, [][]string) (ImportReport, error) { return ImportReport{}, ErrStorage },
			wantStatus: 500, wantCode: "internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg == (NotionConfig{}) {
				tt.cfg = configured()
			}
			if tt.dbStatus == 0 {
				tt.dbStatus = http.StatusOK
			}
			if tt.dbBody == "" {
				tt.dbBody = oneSource
			}
			if tt.process == nil {
				tt.process = NewImporter(&fakeStore{}).CheckTable
			}
			rec := serveNotion(t, NotionImport{
				Config:        func() NotionConfig { return tt.cfg },
				Process:       tt.process,
				ClientOptions: fakeNotion(t, tt.dbStatus, tt.dbBody, tt.pages...),
			})
			var body struct{ Code, Message string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != tt.wantStatus || body.Code != tt.wantCode {
				t.Errorf("got %d %q, want %d %q (%s)", rec.Code, body.Code, tt.wantStatus, tt.wantCode, body.Message)
			}
			if strings.Contains(rec.Body.String(), "ntn_test") {
				t.Error("the response leaks the token")
			}
		})
	}
}

func TestNotionImportCallsImportedOnlyOnSuccess(t *testing.T) {
	page := notionPage("p-1", map[string]string{"Imię": "Anna", "Nazwisko": "Testowa", "E-mail": "anna.testowa@example.test", "Telefon": "", "Typ": "rodzic"})
	var calls, gotPages int
	h := NotionImport{
		Config:        configured,
		Process:       NewImporter(&fakeStore{}).CheckTable,
		ClientOptions: fakeNotion(t, http.StatusOK, oneSource, page),
		Imported:      func(_ context.Context, _ ImportReport, pages int) { calls++; gotPages = pages },
	}
	if rec := serveNotion(t, h); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	h.Process = func(context.Context, [][]string) (ImportReport, error) { return ImportReport{}, errors.New("boom") }
	if rec := serveNotion(t, h); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if calls != 1 || gotPages != 1 {
		t.Errorf("Imported called %d times with %d pages, want once with 1", calls, gotPages)
	}
}
