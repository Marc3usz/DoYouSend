package sms

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

type mockConsumer struct {
	reports []providers.DeliveryReport
	err     error
}

func (m *mockConsumer) ConsumeDeliveryReports(_ context.Context, reports []providers.DeliveryReport) error {
	if m.err != nil {
		return m.err
	}
	m.reports = append(m.reports, reports...)
	return nil
}

func TestHandleSMSAPIDLR_SuccessGET(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/providers/sms/dlr?MsgId=msg-001&status=404&donedate=1631525653", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != SMSAPIDLRAckResponse {
		t.Fatalf("body = %q, want %q", rr.Body.String(), SMSAPIDLRAckResponse)
	}

	if len(consumer.reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(consumer.reports))
	}
	r := consumer.reports[0]
	if r.ProviderMessageID != "msg-001" {
		t.Errorf("ProviderMessageID = %q, want 'msg-001'", r.ProviderMessageID)
	}
	if r.Status != providers.StatusDelivered {
		t.Errorf("Status = %q, want %q", r.Status, providers.StatusDelivered)
	}
	if r.Channel != providers.ChannelSMS {
		t.Errorf("Channel = %q, want %q", r.Channel, providers.ChannelSMS)
	}
}

func TestHandleSMSAPIDLR_SuccessBatchGET(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/providers/sms/dlr?MsgId=id1,id2&status=404,405", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != SMSAPIDLRAckResponse {
		t.Fatalf("body = %q, want %q", rr.Body.String(), SMSAPIDLRAckResponse)
	}

	if len(consumer.reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(consumer.reports))
	}
	if consumer.reports[0].Status != providers.StatusDelivered {
		t.Errorf("report[0].Status = %q, want delivered", consumer.reports[0].Status)
	}
	if consumer.reports[1].Status != providers.StatusFailed {
		t.Errorf("report[1].Status = %q, want failed", consumer.reports[1].Status)
	}
}

func TestHandleSMSAPIDLR_SuccessPOST(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	form := url.Values{}
	form.Set("MsgId", "post-msg-1")
	form.Set("status", "404")

	req := httptest.NewRequest(http.MethodPost, "/providers/sms/dlr", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != SMSAPIDLRAckResponse {
		t.Fatalf("body = %q, want %q", rr.Body.String(), SMSAPIDLRAckResponse)
	}

	if len(consumer.reports) != 1 || consumer.reports[0].ProviderMessageID != "post-msg-1" {
		t.Fatalf("unexpected reports: %+v", consumer.reports)
	}
}

func TestHandleSMSAPIDLR_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	for _, method := range []string{http.MethodDelete, http.MethodPut, http.MethodPatch} {
		req := httptest.NewRequest(method, "/providers/sms/dlr", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("[%s] got status %d, want %d", method, rr.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHandleSMSAPIDLR_MalformedParameters(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	cases := []struct {
		name string
		url  string
	}{
		{"missing MsgId", "/providers/sms/dlr?status=404"},
		{"missing status", "/providers/sms/dlr?MsgId=msg-1"},
		{"unrecognized status", "/providers/sms/dlr?MsgId=msg-1&status=9999"},
		{"mismatched batch counts", "/providers/sms/dlr?MsgId=msg-1,msg-2&status=404"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("got status %d, want %d", rr.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleSMSAPIDLR_ConsumerError(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{err: errors.New("db connection failure")}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/providers/sms/dlr?MsgId=msg-001&status=404", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleSMSAPIDLR_NilConsumer(t *testing.T) {
	t.Parallel()

	handler := HandleSMSAPIDLR(nil, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/providers/sms/dlr?MsgId=msg-001&status=404", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleSMSAPIDLR_BodyTooLarge(t *testing.T) {
	t.Parallel()

	consumer := &mockConsumer{}
	handler := HandleSMSAPIDLR(consumer, slog.Default())

	// 2 MiB body
	largeBody := strings.Repeat("a=1&", (2<<20)/4)
	req := httptest.NewRequest(http.MethodPost, "/providers/sms/dlr", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleSMSAPIDLR_LogsDoNotLeakData(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	consumer := &mockConsumer{err: errors.New("storage down")}
	handler := HandleSMSAPIDLR(consumer, logger)

	req := httptest.NewRequest(http.MethodGet, "/providers/sms/dlr?MsgId=secret-msg-id&status=404", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logOutput := buf.String()
	if strings.Contains(logOutput, "+48") || strings.Contains(logOutput, "@") {
		t.Errorf("sensitive contact info found in logs: %s", logOutput)
	}
}
