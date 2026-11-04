// Package notion reads rows of a Notion database so they can be imported as
// recipients through the same validation and duplicate checks as a CSV/XLSX
// file (ADR-0017). It only reads: nothing is ever written back to Notion.
//
// It talks to the Notion REST API with the standard library, using API version
// 2025-09-03, in which a database holds one or more data sources and rows are
// queried per data source.
//
// Owner: DEV A (Marc3usz), as an input to the recipients domain.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// APIVersion is the Notion-Version header sent with every request.
const APIVersion = "2025-09-03"

// DefaultBaseURL is the Notion API host.
const DefaultBaseURL = "https://api.notion.com"

const (
	pageSize        = 100 // the maximum Notion accepts
	maxResponseSize = 8 << 20
	maxRetries      = 4
	maxRetryWait    = 30 * time.Second
	defaultWait     = time.Second
)

// Errors of the package. Match them with errors.Is.
var (
	ErrNoToken   = errors.New("notion: integration token is empty")
	ErrInvalidID = errors.New("notion: not a Notion ID or URL")
	// ErrNoDataSource means the database has no data source the integration can read.
	ErrNoDataSource = errors.New("notion: database has no data source")
	// ErrMultipleDataSources means the database has several data sources and
	// the caller has to pick one by its ID.
	ErrMultipleDataSources = errors.New("notion: database has several data sources, choose one by its ID")
	// ErrTooManyRows means the data source holds more rows than the caller accepts.
	ErrTooManyRows = errors.New("notion: data source has more rows than allowed")
)

// APIError is an error response of the Notion API, e.g. object_not_found when
// the database was not shared with the integration.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("notion api: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Client calls the Notion API with an internal integration token. It is safe
// for concurrent use.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another host; tests use an httptest.Server.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient replaces the default client (30-second timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient returns a client authenticated with an internal integration token
// ("ntn_..."). The token is only ever sent in the Authorization header.
func NewClient(token string, opts ...Option) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNoToken
	}
	c := &Client{token: token, baseURL: DefaultBaseURL, http: &http.Client{Timeout: 30 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

var hexID = regexp.MustCompile(`(?i)([0-9a-f]{8})-?([0-9a-f]{4})-?([0-9a-f]{4})-?([0-9a-f]{4})-?([0-9a-f]{12})`)

// ParseID accepts a Notion ID with or without dashes, or a page/database URL
// as copied from the browser, and returns the dashed ID. In a URL the ID is
// the last 32 hex characters before "?" (the "?v=" view ID is ignored).
func ParseID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	all := hexID.FindAllStringSubmatch(s, -1)
	if len(all) == 0 {
		return "", ErrInvalidID
	}
	m := all[len(all)-1]
	return strings.ToLower(strings.Join(m[1:], "-")), nil
}

// DataSource is one data source of a database.
type DataSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResolveDataSource returns the only data source of a database. A database
// with several data sources fails with ErrMultipleDataSources, listing them,
// so the caller can pass the right data source ID instead.
func (c *Client) ResolveDataSource(ctx context.Context, databaseID string) (DataSource, error) {
	var db struct {
		DataSources []DataSource `json:"data_sources"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/databases/"+url.PathEscape(databaseID), nil, &db); err != nil {
		return DataSource{}, fmt.Errorf("get database %s: %w", databaseID, err)
	}
	switch len(db.DataSources) {
	case 0:
		return DataSource{}, fmt.Errorf("database %s: %w", databaseID, ErrNoDataSource)
	case 1:
		return db.DataSources[0], nil
	default:
		names := make([]string, len(db.DataSources))
		for i, ds := range db.DataSources {
			names[i] = fmt.Sprintf("%q (%s)", ds.Name, ds.ID)
		}
		return DataSource{}, fmt.Errorf("database %s: %w: %s", databaseID, ErrMultipleDataSources, strings.Join(names, ", "))
	}
}

// ErrNoSource means neither a database nor a data source was given.
var ErrNoSource = errors.New("notion: no database or data source ID")

// Pages reads every page of the configured source: the data source
// dataSourceID when set (it wins), otherwise the only data source of
// databaseID. Both accept an ID or a URL copied from Notion. It fails with
// ErrInvalidID, ErrNoSource, ErrNoDataSource, ErrMultipleDataSources,
// ErrTooManyRows or an *APIError.
func (c *Client) Pages(ctx context.Context, databaseID, dataSourceID string, maxRows int) ([]Page, error) {
	var dsID string
	switch {
	case strings.TrimSpace(dataSourceID) != "":
		id, err := ParseID(dataSourceID)
		if err != nil {
			return nil, fmt.Errorf("data source: %w", err)
		}
		dsID = id
	case strings.TrimSpace(databaseID) != "":
		dbID, err := ParseID(databaseID)
		if err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		ds, err := c.ResolveDataSource(ctx, dbID)
		if err != nil {
			return nil, err
		}
		dsID = ds.ID
	default:
		return nil, ErrNoSource
	}
	return c.QueryAll(ctx, dsID, maxRows)
}

// QueryAll returns every non-archived page of a data source, oldest first, so
// row numbers stay stable between a check and an import. It fails with
// ErrTooManyRows rather than return a partial result when there are more
// than maxRows pages: importing part of a roster silently is worse than not
// importing it.
func (c *Client) QueryAll(ctx context.Context, dataSourceID string, maxRows int) ([]Page, error) {
	type sort struct {
		Timestamp string `json:"timestamp"`
		Direction string `json:"direction"`
	}
	body := struct {
		PageSize    int    `json:"page_size"`
		StartCursor string `json:"start_cursor,omitempty"`
		Sorts       []sort `json:"sorts"`
	}{PageSize: pageSize, Sorts: []sort{{Timestamp: "created_time", Direction: "ascending"}}}

	var pages []Page
	for {
		var resp struct {
			Results    []Page  `json:"results"`
			HasMore    bool    `json:"has_more"`
			NextCursor *string `json:"next_cursor"`
		}
		path := "/v1/data_sources/" + url.PathEscape(dataSourceID) + "/query"
		if err := c.do(ctx, http.MethodPost, path, body, &resp); err != nil {
			return nil, fmt.Errorf("query data source %s: %w", dataSourceID, err)
		}
		for _, p := range resp.Results {
			if p.Object != "" && p.Object != "page" {
				continue // a wiki data source can also return nested data sources
			}
			pages = append(pages, p)
		}
		if len(pages) > maxRows {
			return nil, fmt.Errorf("query data source %s: %w (%d)", dataSourceID, ErrTooManyRows, maxRows)
		}
		if !resp.HasMore || resp.NextCursor == nil || *resp.NextCursor == "" {
			return pages, nil
		}
		body.StartCursor = *resp.NextCursor
	}
}

// do sends one request, retrying on 429 and 5xx with the wait Notion asks for
// (Retry-After), and decodes a 2xx JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	for attempt := 0; ; attempt++ {
		status, header, body, err := c.send(ctx, method, path, payload)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
			return nil
		}
		apiErr := parseAPIError(status, body)
		retryable := status == http.StatusTooManyRequests || status >= 500
		if !retryable || attempt >= maxRetries {
			return apiErr
		}
		if err := wait(ctx, retryAfter(header.Get("Retry-After"))); err != nil {
			return fmt.Errorf("%w (waiting to retry: %w)", apiErr, err)
		}
	}
}

func (c *Client) send(ctx context.Context, method, path string, payload []byte) (int, http.Header, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", APIVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, resp.Header, data, nil
}

func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		e.Code, e.Message = parsed.Code, parsed.Message
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}

// retryAfter reads the Retry-After seconds, capped so a bad header cannot
// stall an import for long.
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return defaultWait
	}
	return min(time.Duration(secs)*time.Second, maxRetryWait)
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
