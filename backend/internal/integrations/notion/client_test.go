package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testToken = "ntn_test_token_not_real"

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(testToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c
}

func checkHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
		t.Errorf("Authorization = %q", got)
	}
	if got := r.Header.Get("Notion-Version"); got != APIVersion {
		t.Errorf("Notion-Version = %q, want %q", got, APIVersion)
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	if _, err := NewClient("  "); !errors.Is(err, ErrNoToken) {
		t.Fatalf("NewClient(blank) error = %v, want ErrNoToken", err)
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "1a2b3c4d5e6f40718293a4b5c6d7e8f9", want: "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9"},
		{in: "1A2B3C4D-5E6F-4071-8293-A4B5C6D7E8F9", want: "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9"},
		{in: "https://www.notion.so/szkola/Odbiorcy-1a2b3c4d5e6f40718293a4b5c6d7e8f9?v=ffffffffffffffffffffffffffffffff", want: "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9"},
		{in: "not-an-id", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseID(tt.in)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidID) {
					t.Fatalf("ParseID() error = %v, want ErrInvalidID", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseID() = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestResolveDataSource(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantID  string
		wantErr error
	}{
		{name: "one", status: 200, body: `{"data_sources":[{"id":"ds-1","name":"Odbiorcy"}]}`, wantID: "ds-1"},
		{name: "none", status: 200, body: `{"data_sources":[]}`, wantErr: ErrNoDataSource},
		{name: "several", status: 200, body: `{"data_sources":[{"id":"ds-1","name":"A"},{"id":"ds-2","name":"B"}]}`, wantErr: ErrMultipleDataSources},
		{name: "not shared", status: 404, body: `{"object":"error","status":404,"code":"object_not_found","message":"Could not find database"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				checkHeaders(t, r)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/databases/db-1" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			ds, err := c.ResolveDataSource(context.Background(), "db-1")
			switch {
			case tt.status != 200:
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Code != "object_not_found" {
					t.Fatalf("error = %v, want APIError object_not_found", err)
				}
				if strings.Contains(err.Error(), testToken) {
					t.Errorf("error leaks the token: %v", err)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			default:
				if err != nil || ds.ID != tt.wantID {
					t.Fatalf("ResolveDataSource() = %+v, %v, want %s", ds, err, tt.wantID)
				}
			}
		})
	}
}

// pagesHandler serves total pages in pages of size per page, following the
// cursor, and fails the first call with 429 when throttleFirst is set.
func pagesHandler(t *testing.T, total, size int, throttleFirst bool, calls *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		checkHeaders(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/data_sources/ds-1/query" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if throttleFirst && n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"object":"error","status":429,"code":"rate_limited","message":"slow down"}`)
			return
		}
		var req struct {
			PageSize    int    `json:"page_size"`
			StartCursor string `json:"start_cursor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req.PageSize != pageSize {
			t.Errorf("page_size = %d, want %d", req.PageSize, pageSize)
		}
		start := 0
		if req.StartCursor != "" {
			_, _ = fmt.Sscanf(req.StartCursor, "c%d", &start)
		}
		end := min(start+size, total)
		var results []string
		for i := start; i < end; i++ {
			results = append(results, fmt.Sprintf(`{"object":"page","id":"p%d","properties":{}}`, i))
		}
		next := "null"
		if end < total {
			next = fmt.Sprintf(`"c%d"`, end)
		}
		fmt.Fprintf(w, `{"object":"list","results":[%s],"has_more":%t,"next_cursor":%s}`, strings.Join(results, ","), end < total, next)
	}
}

func TestQueryAll(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		maxRows   int
		throttle  bool
		wantPages int
		wantCalls int32
		wantErr   error
	}{
		{name: "single page", total: 3, maxRows: 10, wantPages: 3, wantCalls: 1},
		{name: "follows cursor", total: 7, maxRows: 10, wantPages: 7, wantCalls: 3},
		{name: "empty", total: 0, maxRows: 10, wantPages: 0, wantCalls: 1},
		{name: "retries 429", total: 2, maxRows: 10, throttle: true, wantPages: 2, wantCalls: 2},
		{name: "too many rows", total: 11, maxRows: 10, wantErr: ErrTooManyRows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := newTestClient(t, pagesHandler(t, tt.total, 3, tt.throttle, &calls))
			pages, err := c.QueryAll(context.Background(), "ds-1", tt.maxRows)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("QueryAll() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryAll() error = %v", err)
			}
			if len(pages) != tt.wantPages {
				t.Errorf("got %d pages, want %d", len(pages), tt.wantPages)
			}
			for i, p := range pages {
				if p.ID != fmt.Sprintf("p%d", i) {
					t.Errorf("pages[%d].ID = %s, order not kept", i, p.ID)
				}
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestQueryAllGivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := c.QueryAll(context.Background(), "ds-1", 10)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("error = %v, want APIError 503", err)
	}
	if got := calls.Load(); got != maxRetries+1 {
		t.Errorf("calls = %d, want %d", got, maxRetries+1)
	}
}

func TestQueryAllStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if _, err := c.QueryAll(ctx, "ds-1", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRetryAfter(t *testing.T) {
	tests := map[string]string{"": "1s", "2": "2s", "-1": "1s", "abc": "1s", "3600": "30s"}
	for in, want := range tests {
		if got := retryAfter(in).String(); got != want {
			t.Errorf("retryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}
