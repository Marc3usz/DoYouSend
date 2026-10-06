package groups

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*http.ServeMux, *MemStore) {
	t.Helper()
	store := NewMemStore()
	dir := newFakeDirectory(fixtureRecipients()...)
	mux := http.NewServeMux()
	NewHandler(NewService(store, dir), NewResolver(store, dir), slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return mux, store
}

// do sends a request and decodes the JSON answer into out (when not nil).
func do(t *testing.T, mux *http.ServeMux, method, path, body string, out any) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: response is not JSON: %q", method, path, rec.Body.String())
		}
	}
	return rec.Code
}

type errorJSON struct {
	Code   string `json:"code"`
	Fields []struct {
		Field string `json:"field"`
	} `json:"fields"`
	IDs []string `json:"ids"`
}

func TestHandlerErrors(t *testing.T) {
	mux, store := newTestServer(t)
	rada := mustCreate(t, store, "Rada rodziców", idJan)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
		wantField  string
		wantIDs    []string
	}{
		{"malformed id", http.MethodGet, "/api/groups/xyz", "", 400, "invalid_request", "id", nil},
		{"unknown group", http.MethodGet, "/api/groups/" + idMissingGroup, "", 404, "not_found", "", nil},
		{"members of unknown group", http.MethodGet, "/api/groups/" + idMissingGroup + "/members", "", 404, "not_found", "", nil},
		{"not JSON", http.MethodPost, "/api/groups", "{", 400, "invalid_request", "", nil},
		{"empty name", http.MethodPost, "/api/groups", `{"name":"  "}`, 400, "invalid_input", "name", nil},
		{"built-in name", http.MethodPost, "/api/groups", `{"name":"wszyscy rodzice"}`, 409, "name_taken", "name", nil},
		{"taken name", http.MethodPost, "/api/groups", `{"name":"rada  RODZICÓW"}`, 409, "name_taken", "name", nil},
		{"rename built-in", http.MethodPut, "/api/groups/" + AllParentsID, `{"name":"Rodzice"}`, 409, "system_group", "", nil},
		{"delete built-in", http.MethodDelete, "/api/groups/" + AllStudentsID, "", 409, "system_group", "", nil},
		{"add to built-in", http.MethodPost, "/api/groups/" + AllParentsID + "/members", `{"recipientIds":["` + idJan + `"]}`, 409, "system_group", "", nil},
		{"add nobody", http.MethodPost, "/api/groups/" + rada.ID + "/members", `{"recipientIds":[]}`, 400, "invalid_input", "recipientIds", nil},
		{"add malformed id", http.MethodPost, "/api/groups/" + rada.ID + "/members", `{"recipientIds":["nope"]}`, 400, "invalid_input", "recipientIds[0]", nil},
		{"add unknown recipient", http.MethodPost, "/api/groups/" + rada.ID + "/members", `{"recipientIds":["` + idMaria + `","` + idNobody + `"]}`, 422, "unknown_recipient", "", []string{idNobody}},
		{"resolve nothing", http.MethodPost, "/api/groups/resolve", `{"excludedRecipientIds":["` + idJan + `"]}`, 400, "empty_selection", "", nil},
		{"resolve malformed id", http.MethodPost, "/api/groups/resolve", `{"recipientIds":["nope"]}`, 400, "invalid_input", "recipientIds[0]", nil},
		{"resolve not JSON", http.MethodPost, "/api/groups/resolve", `[]`, 400, "invalid_request", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got errorJSON
			status := do(t, mux, tt.method, tt.path, tt.body, &got)
			if status != tt.wantStatus || got.Code != tt.wantCode {
				t.Fatalf("got %d %q, want %d %q", status, got.Code, tt.wantStatus, tt.wantCode)
			}
			if tt.wantField != "" && (len(got.Fields) == 0 || got.Fields[0].Field != tt.wantField) {
				t.Errorf("fields = %+v, want %q first", got.Fields, tt.wantField)
			}
			if tt.wantIDs != nil && !reflect.DeepEqual(got.IDs, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", got.IDs, tt.wantIDs)
			}
		})
	}

	// The refused unknown-recipient add was all-or-nothing.
	if members, _ := store.Members(t.Context(), []string{rada.ID}); len(members[rada.ID]) != 1 {
		t.Errorf("members after refused add = %v, want only Jan", members[rada.ID])
	}
}

type groupResp struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	MemberCount int     `json:"memberCount"`
	CreatedAt   *string `json:"createdAt"`
}

func TestHandlerGroupLifecycle(t *testing.T) {
	mux, _ := newTestServer(t)

	var created groupResp
	if status := do(t, mux, http.MethodPost, "/api/groups", `{"name":" Chór ","description":"Próby we wtorki"}`, &created); status != http.StatusCreated {
		t.Fatalf("create status = %d", status)
	}
	if created.Name != "Chór" || created.Kind != "custom" || created.MemberCount != 0 || created.CreatedAt == nil {
		t.Fatalf("created = %+v", created)
	}

	var change struct{ Changed, Unchanged int }
	path := "/api/groups/" + strings.ToUpper(created.ID) + "/members"
	if status := do(t, mux, http.MethodPost, path, `{"recipientIds":["`+idJan+`","`+idLena+`","`+idJan+`"]}`, &change); status != 200 || change.Changed != 2 || change.Unchanged != 0 {
		t.Fatalf("add = %d %+v, want 2 changed (repeat ignored)", status, change)
	}
	if do(t, mux, http.MethodPost, path, `{"recipientIds":["`+idJan+`","`+idOskar+`"]}`, &change); change.Changed != 1 || change.Unchanged != 1 {
		t.Errorf("second add = %+v, want 1 changed, 1 unchanged", change)
	}

	var renamed groupResp
	if status := do(t, mux, http.MethodPut, "/api/groups/"+created.ID, `{"name":"Chór szkolny"}`, &renamed); status != 200 || renamed.Name != "Chór szkolny" || renamed.MemberCount != 3 {
		t.Errorf("update = %d %+v, want renamed with 3 members", status, renamed)
	}

	var members []struct {
		ID     string `json:"id"`
		Issues []struct {
			Channel string `json:"channel"`
		} `json:"issues"`
		GroupIDs *[]string `json:"groupIds"`
	}
	do(t, mux, http.MethodGet, path, "", &members)
	if len(members) != 3 || members[0].ID != idOskar || members[1].ID != idJan || members[2].ID != idLena {
		t.Fatalf("members = %+v, want Adamski, Kowalski, Nowak", members)
	}
	if len(members[2].Issues) != 1 || members[2].Issues[0].Channel != "email" || members[0].GroupIDs != nil {
		t.Errorf("members = %+v, want Lena missing e-mail and no groupIds", members)
	}

	if do(t, mux, http.MethodPost, path+"/remove", `{"recipientIds":["`+idJan+`","`+idMaria+`"]}`, &change); change.Changed != 1 || change.Unchanged != 1 {
		t.Errorf("remove = %+v, want 1 changed, 1 unchanged", change)
	}

	if status := do(t, mux, http.MethodDelete, "/api/groups/"+created.ID, "", nil); status != http.StatusNoContent {
		t.Errorf("delete status = %d", status)
	}
	if status := do(t, mux, http.MethodGet, "/api/groups/"+created.ID, "", nil); status != http.StatusNotFound {
		t.Errorf("get after delete status = %d", status)
	}
}

func TestHandlerList(t *testing.T) {
	mux, store := newTestServer(t)
	mustCreate(t, store, "Chór", idOskar)

	var list []groupResp
	if status := do(t, mux, http.MethodGet, "/api/groups", "", &list); status != 200 {
		t.Fatalf("status = %d", status)
	}
	type row struct {
		Name       string
		Kind       string
		Members    int
		HasCreated bool
	}
	var got []row
	for _, g := range list {
		got = append(got, row{g.Name, g.Kind, g.MemberCount, g.CreatedAt != nil})
	}
	want := []row{
		{"Wszyscy rodzice", "system", 3, false},
		{"Wszyscy uczniowie", "system", 2, false},
		{"Chór", "custom", 1, true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("list = %+v\nwant   %+v", got, want)
	}

	var parents groupResp
	do(t, mux, http.MethodGet, "/api/groups/"+AllParentsID, "", &parents)
	if parents.MemberCount != 3 || parents.CreatedAt != nil {
		t.Errorf("built-in group = %+v, want 3 members, null createdAt", parents)
	}
}

func TestHandlerResolve(t *testing.T) {
	mux, store := newTestServer(t)
	rada := mustCreate(t, store, "Rada rodziców", idJan, idMaria)

	body := `{
		"groupIds": ["` + AllParentsID + `", "` + rada.ID + `", "` + idMissingGroup + `"],
		"recipientIds": ["` + idJan + `", "` + idLena + `", "` + idNobody + `"],
		"excludedRecipientIds": ["` + idZofia + `"]
	}`
	var got struct {
		Recipients []struct {
			Recipient struct {
				ID string `json:"id"`
			} `json:"recipient"`
			ViaGroupIDs []string `json:"viaGroupIds"`
			Direct      bool     `json:"direct"`
			Channels    []string `json:"channels"`
		} `json:"recipients"`
		UnknownGroupIDs     []string `json:"unknownGroupIds"`
		UnknownRecipientIDs []string `json:"unknownRecipientIds"`
		ExcludedIDs         []string `json:"excludedIds"`
		MergedDuplicates    int      `json:"mergedDuplicates"`
		Summary             struct {
			Total, Complete, Partial, Unreachable int
		} `json:"summary"`
	}
	if status := do(t, mux, http.MethodPost, "/api/groups/resolve", body, &got); status != 200 {
		t.Fatalf("status = %d", status)
	}

	type row struct {
		ID       string
		Via      []string
		Direct   bool
		Channels []string
	}
	var rows []row
	for _, r := range got.Recipients {
		rows = append(rows, row{r.Recipient.ID, r.ViaGroupIDs, r.Direct, r.Channels})
	}
	want := []row{
		{idMaria, []string{AllParentsID, rada.ID}, false, []string{"email"}},
		{idJan, []string{AllParentsID, rada.ID}, true, []string{"email", "sms"}},
		{idLena, []string{}, true, []string{"sms"}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("recipients = %+v\nwant         %+v", rows, want)
	}
	if !reflect.DeepEqual(got.UnknownGroupIDs, []string{idMissingGroup}) ||
		!reflect.DeepEqual(got.UnknownRecipientIDs, []string{idNobody}) ||
		!reflect.DeepEqual(got.ExcludedIDs, []string{idZofia}) {
		t.Errorf("unknown/excluded = %v %v %v", got.UnknownGroupIDs, got.UnknownRecipientIDs, got.ExcludedIDs)
	}
	if got.MergedDuplicates != 3 {
		t.Errorf("mergedDuplicates = %d, want 3 (Jan twice, Maria once)", got.MergedDuplicates)
	}
	if got.Summary.Total != 3 || got.Summary.Complete != 1 || got.Summary.Partial != 2 || got.Summary.Unreachable != 0 {
		t.Errorf("summary = %+v, want 3 total: 1 complete, 2 partial", got.Summary)
	}
}

func TestHandlerResolveEmptyListsAreArrays(t *testing.T) {
	mux, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/groups/resolve", strings.NewReader(`{"recipientIds":["`+idOskar+`"]}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	for _, field := range []string{`"unknownGroupIds":[]`, `"unknownRecipientIds":[]`, `"excludedIds":[]`, `"viaGroupIds":[]`, `"channels":[]`, `"unreachable":1`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("response lacks %s: %s", field, rec.Body.String())
		}
	}
}
