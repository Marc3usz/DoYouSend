package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/groups"
	"github.com/Marc3usz/DoYouSend/backend/internal/recipients"
)

func TestHandlePreview(t *testing.T) {
	tests := []struct {
		name       string
		request    string
		resolver   *fakeResolver
		wantStatus int
		wantJSON   string
	}{
		{
			name:       "estimate for a group",
			request:    `{"subject":"Zebranie","body":"Witaj {{imie}}","selection":{"groupIds":["` + idGroup + `"]}}`,
			resolver:   &fakeResolver{res: groups.Resolution{Recipients: []groups.Resolved{anna, lukasz}}},
			wantStatus: http.StatusOK,
			wantJSON: `{"template":{"encoding":"GSM7","units":18,"parts":1},"placeholders":["imie"],
				"unknownPlaceholders":[],"recipientCount":2,"emailCount":2,"smsCount":1,"partialCount":1,
				"unreachableCount":0,"ucs2Count":0,"minPartsPerRecipient":1,"maxPartsPerRecipient":1,
				"totalSmsParts":1,"costMilli":80,"renderFailedIds":[]}`,
		},
		{
			name:       "no selection still measures the body",
			request:    `{"subject":"","body":"Zażółć"}`,
			resolver:   &fakeResolver{},
			wantStatus: http.StatusOK,
			wantJSON: `{"template":{"encoding":"UCS2","units":6,"parts":1},"placeholders":[],
				"unknownPlaceholders":[],"recipientCount":0,"emailCount":0,"smsCount":0,"partialCount":0,
				"unreachableCount":0,"ucs2Count":0,"minPartsPerRecipient":0,"maxPartsPerRecipient":0,
				"totalSmsParts":0,"costMilli":0,"renderFailedIds":[]}`,
		},
		{
			name:       "malformed json",
			request:    `{"body":`,
			resolver:   &fakeResolver{},
			wantStatus: http.StatusBadRequest,
			wantJSON:   `{"code":"invalid_request","message":"expected a JSON message draft"}`,
		},
		{
			name:    "invalid selection lists the fields",
			request: `{"subject":"","body":"x","selection":{"groupIds":["zly"]}}`,
			resolver: &fakeResolver{err: &groups.ValidationError{Fields: []recipients.FieldError{
				{Field: "groupIds[0]", Message: "not a valid ID"},
			}}},
			wantStatus: http.StatusBadRequest,
			wantJSON: `{"code":"invalid_input","message":"invalid recipient selection",
				"fields":[{"field":"groupIds[0]","message":"not a valid ID"}]}`,
		},
		{
			name:       "storage failure is internal",
			request:    `{"subject":"","body":"x","selection":{"groupIds":["` + idGroup + `"]}}`,
			resolver:   &fakeResolver{err: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
			wantJSON:   `{"code":"internal","message":"preview failed"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := HandlePreview(NewPreviewer(tt.resolver, 80), slog.New(slog.NewTextHandler(io.Discard, nil)))
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodPost, "/api/messages/preview", strings.NewReader(tt.request)))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			assertJSON(t, rec.Body.Bytes(), tt.wantJSON)
		})
	}
}

func TestHandlePreviewRejectsOversizedRequest(t *testing.T) {
	big := `{"body":"` + strings.Repeat("a", maxPreviewBody) + `"}`
	rec := httptest.NewRecorder()
	h := HandlePreview(NewPreviewer(&fakeResolver{}, 0), slog.New(slog.NewTextHandler(io.Discard, nil)))
	h(rec, httptest.NewRequest(http.MethodPost, "/api/messages/preview", strings.NewReader(big)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandlePreviewNeverLogsTheBody(t *testing.T) {
	var logs bytes.Buffer
	h := HandlePreview(NewPreviewer(&fakeResolver{err: errors.New("db down")}, 0), slog.New(slog.NewTextHandler(&logs, nil)))
	body := `{"body":"Tajna tresc","selection":{"groupIds":["` + idGroup + `"]}}`
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/messages/preview", strings.NewReader(body)))
	if strings.Contains(logs.String(), "Tajna") {
		t.Errorf("log contains the message body: %s", logs.String())
	}
}

func TestHandlePreviewDoesNotLogCancelledRequests(t *testing.T) {
	var logs bytes.Buffer
	h := HandlePreview(NewPreviewer(&fakeResolver{err: context.Canceled}, 0), slog.New(slog.NewTextHandler(&logs, nil)))
	body := `{"body":"x","selection":{"groupIds":["` + idGroup + `"]}}`
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/messages/preview", strings.NewReader(body)))
	if logs.Len() > 0 {
		t.Errorf("cancelled request was logged: %s", logs.String())
	}
}

func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if !bytes.Equal(gb, wb) {
		t.Errorf("body =\n%s\nwant\n%s", gb, wb)
	}
}
