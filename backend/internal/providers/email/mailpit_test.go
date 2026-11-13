package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/providers"
)

func TestNewMailpit_NilLogger(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{}, nil)
	if m.logger == nil {
		t.Fatal("expected default logger when nil is provided")
	}
	if m.Channel() != providers.ChannelEmail {
		t.Errorf("Channel() = %v, want %v", m.Channel(), providers.ChannelEmail)
	}
}

func TestMailpit_EmptyTo(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{Host: "localhost", Port: "1025"}, slog.Default())

	_, err := m.Send(context.Background(), providers.Message{
		RecipientID: "r-010",
		To:          "",
		Subject:     "Test",
		Body:        "test body",
	})

	if err == nil {
		t.Fatal("expected error for empty To")
	}
	if !providers.IsPermanent(err) {
		t.Errorf("empty To should be permanent error, got: %v", err)
	}
}

func TestMailpit_HeaderInjection(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{Host: "localhost", Port: "1025"}, slog.Default())

	tests := []struct {
		name    string
		msg     providers.Message
		wantErr string
	}{
		{
			name: "CRLF injection in Subject",
			msg: providers.Message{
				RecipientID: "r-inj-1",
				To:          "parent@example.test",
				Subject:     "Zebranie\r\nBcc: evil@example.test",
				Body:        "test",
			},
			wantErr: "subject contains newline",
		},
		{
			name: "LF injection in Subject",
			msg: providers.Message{
				RecipientID: "r-inj-2",
				To:          "parent@example.test",
				Subject:     "Zebranie\nBcc: evil@example.test",
				Body:        "test",
			},
			wantErr: "subject contains newline",
		},
		{
			name: "CRLF injection in To",
			msg: providers.Message{
				RecipientID: "r-inj-3",
				To:          "parent@example.test\r\nBcc: evil@example.test",
				Subject:     "Zebranie",
				Body:        "test",
			},
			wantErr: "recipient address contains newline",
		},
		{
			name: "LF injection in To",
			msg: providers.Message{
				RecipientID: "r-inj-4",
				To:          "parent@example.test\nBcc: evil@example.test",
				Subject:     "Zebranie",
				Body:        "test",
			},
			wantErr: "recipient address contains newline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := m.Send(context.Background(), tt.msg)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !providers.IsPermanent(err) {
				t.Errorf("header injection attempt should be permanent error, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestMailpit_DryRun(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{
		Host:   "unreachable.invalid",
		Port:   "9999",
		DryRun: true,
	}, nil)

	res, err := m.Send(context.Background(), providers.Message{
		RecipientID: "r-dry",
		To:          "parent@example.test",
		Subject:     "Zebranie",
		Body:        "Treść",
	})

	if err != nil {
		t.Fatalf("unexpected error in DryRun mode: %v", err)
	}
	if res.ProviderMessageID == "" {
		t.Fatal("expected non-empty ProviderMessageID")
	}
}

func TestMailpit_CancelledContext(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{Host: "localhost", Port: "1025"}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Send(ctx, providers.Message{
		RecipientID: "r-011",
		To:          "parent@example.test",
		Subject:     "Test",
		Body:        "test body",
	})

	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !providers.IsTransient(err) {
		t.Errorf("cancelled context should be transient, got: %v", err)
	}
}

func TestBuildRFC822(t *testing.T) {
	t.Parallel()

	msgID := "<test-123@example.test>"
	got := buildRFC822(
		"Szkola <no-reply@example.test>",
		"",
		"parent@example.test",
		"Zebranie z rodzicami — klasa 3A",
		"Zapraszamy na zebranie",
		msgID,
	)

	tests := []struct {
		name     string
		contains string
	}{
		{"has From header", "From: Szkola <no-reply@example.test>"},
		{"has To header", "To: parent@example.test"},
		{"has Q-encoded Subject", "Subject: =?utf-8?q?"},
		{"has Message-ID", "Message-ID: " + msgID},
		{"has Date header", "Date: "},
		{"has Content-Type", "Content-Type: text/plain; charset=utf-8"},
		{"has body", "Zapraszamy na zebranie"},
	}
	if strings.Contains(got, "Reply-To:") {
		t.Errorf("empty replyTo still wrote a Reply-To header:\n%s", got)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(got, tt.contains) {
				t.Errorf("RFC822 message missing %q:\n%s", tt.contains, got)
			}
		})
	}
}

func TestClassifySMTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		isPermanent bool
		isTransient bool
	}{
		{
			name:        "textproto 550 permanent error",
			err:         &textproto.Error{Code: 550, Msg: "User unknown"},
			isPermanent: true,
			isTransient: false,
		},
		{
			name:        "textproto 554 transaction failed",
			err:         &textproto.Error{Code: 554, Msg: "Transaction failed"},
			isPermanent: true,
			isTransient: false,
		},
		{
			name:        "textproto 421 service unavailable",
			err:         &textproto.Error{Code: 421, Msg: "Service unavailable"},
			isPermanent: false,
			isTransient: true,
		},
		{
			name:        "textproto 451 greylisted",
			err:         &textproto.Error{Code: 451, Msg: "Local error, please try again"},
			isPermanent: false,
			isTransient: true,
		},
		{
			name:        "net.OpError transient",
			err:         &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			isPermanent: false,
			isTransient: true,
		},
		{
			name:        "fallback string 5xx error",
			err:         errors.New("500 Syntax error"),
			isPermanent: true,
			isTransient: false,
		},
		{
			name:        "fallback string 4xx error",
			err:         errors.New("420 Timeout"),
			isPermanent: false,
			isTransient: true,
		},
		{
			name:        "generic error is transient",
			err:         errors.New("unexpected EOF"),
			isPermanent: false,
			isTransient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := classifySMTPError("r-test", tt.err)
			if got := providers.IsPermanent(classified); got != tt.isPermanent {
				t.Errorf("IsPermanent() = %v, want %v", got, tt.isPermanent)
			}
			if got := providers.IsTransient(classified); got != tt.isTransient {
				t.Errorf("IsTransient() = %v, want %v", got, tt.isTransient)
			}
		})
	}
}

// TestMailpit_Send_LocalListenerSuccess tests the happy path of Send
// against a mock local TCP listener implementing the SMTP greeting and handshake.
func TestMailpit_Send_LocalListenerSuccess(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}
	defer func() { _ = l.Close() }()

	// Run minimal mock SMTP server in background.
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		reply := func(msg string) { _, _ = fmt.Fprint(conn, msg) }

		r := bufio.NewReader(conn)
		// 1. Initial greeting
		reply("220 127.0.0.1 ESMTP Service Ready\r\n")

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			upper := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				reply("250-127.0.0.1\r\n250 HELP\r\n")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				reply("250 2.1.0 Ok\r\n")
			case strings.HasPrefix(upper, "RCPT TO:"):
				reply("250 2.1.5 Ok\r\n")
			case upper == "DATA":
				reply("354 End data with <CR><LF>.<CR><LF>\r\n")
				for {
					dataLine, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if dataLine == ".\r\n" || dataLine == ".\n" {
						break
					}
				}
				reply("250 2.0.0 Ok: queued\r\n")
			case upper == "QUIT":
				reply("221 2.0.0 Bye\r\n")
				return
			default:
				reply("500 Unrecognized command\r\n")
			}
		}
	}()

	host, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	m := NewMailpit(MailpitConfig{
		Host: host,
		Port: port,
		From: "no-reply@example.test",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := m.Send(ctx, providers.Message{
		RecipientID: "r-success",
		To:          "parent@example.test",
		Subject:     "Ważna informacja",
		Body:        "Zebranie szkolne odbędzie się jutro.",
	})

	if err != nil {
		t.Fatalf("unexpected error during SMTP send: %v", err)
	}
	if res.ProviderMessageID == "" {
		t.Fatal("expected non-empty ProviderMessageID")
	}
	if !strings.HasPrefix(res.ProviderMessageID, "<") || !strings.HasSuffix(res.ProviderMessageID, ">") {
		t.Errorf("ProviderMessageID %q is not a valid Message-ID header format", res.ProviderMessageID)
	}
}

func TestMailpit_GetProfile(t *testing.T) {
	t.Parallel()

	m := NewMailpit(MailpitConfig{
		Host: "localhost",
		Port: "1025",
	}, nil)

	profile, err := m.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("unexpected GetProfile error: %v", err)
	}

	if profile.Provider != "mailpit" {
		t.Errorf("Provider = %q, want 'mailpit'", profile.Provider)
	}
	if profile.RemainCredits != nil {
		t.Errorf("RemainCredits = %v, want nil", profile.RemainCredits)
	}
}

func TestSenderAddress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from, wantEnvelope, wantHeader string
	}{
		{"no-reply@example.test", "no-reply@example.test", "<no-reply@example.test>"},
		{"DoYouSend <no-reply@example.test>", "no-reply@example.test", `"DoYouSend" <no-reply@example.test>`},
		{"Szkoła Demo <sekretariat@example.test>", "sekretariat@example.test", "=?utf-8?q?Szko=C5=82a_Demo?= <sekretariat@example.test>"},
		{"not an address", "not an address", "not an address"},
	}
	for _, tt := range tests {
		envelope, header := senderAddress(tt.from)
		if envelope != tt.wantEnvelope || header != tt.wantHeader {
			t.Errorf("senderAddress(%q) = %q, %q; want %q, %q", tt.from, envelope, header, tt.wantEnvelope, tt.wantHeader)
		}
	}
}

// smtpSeen is what fakeSMTP saw of one message.
type smtpSeen struct{ mailFrom, fromHeader, replyToHeader string }

// fakeSMTP accepts one SMTP session on a local port and reports the envelope
// sender and the From and Reply-To headers.
func fakeSMTP(t *testing.T) (host, port string, got <-chan smtpSeen) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	ch := make(chan smtpSeen, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		reply := func(msg string) { _, _ = fmt.Fprint(conn, msg) }
		r := bufio.NewReader(conn)
		reply("220 127.0.0.1 ESMTP\r\n")
		var s smtpSeen
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimSpace(line)
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				reply("250-127.0.0.1\r\n250 HELP\r\n")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				s.mailFrom = trimmed
				reply("250 Ok\r\n")
			case strings.HasPrefix(upper, "RCPT TO:"):
				reply("250 Ok\r\n")
			case upper == "DATA":
				reply("354 Go\r\n")
				for {
					dataLine, err := r.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(dataLine, "From: "):
						s.fromHeader = strings.TrimSpace(dataLine)
					case strings.HasPrefix(dataLine, "Reply-To: "):
						s.replyToHeader = strings.TrimSpace(dataLine)
					}
					if dataLine == ".\r\n" {
						break
					}
				}
				reply("250 Ok: queued\r\n")
			case upper == "QUIT":
				reply("221 Bye\r\n")
				ch <- s
				return
			default:
				reply("500 Unrecognized\r\n")
			}
		}
	}()
	host, port, _ = net.SplitHostPort(l.Addr().String())
	return host, port, ch
}

// TestMailpit_SendWithDisplayName is a regression test: MAIL FROM used to get
// the whole "Name <address>" value, which Mailpit rejects with 501.
func TestMailpit_SendWithDisplayName(t *testing.T) {
	t.Parallel()

	host, port, got := fakeSMTP(t)
	m := NewMailpit(MailpitConfig{Host: host, Port: port, From: "Szkoła Demo <sekretariat@example.test>"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Send(ctx, providers.Message{RecipientID: "r-1", To: "parent@example.test", Subject: "Test", Body: "Treść"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	s := <-got
	if s.mailFrom != "MAIL FROM:<sekretariat@example.test>" && !strings.HasPrefix(s.mailFrom, "MAIL FROM:<sekretariat@example.test> ") {
		t.Errorf("MAIL FROM = %q, want the bare address", s.mailFrom)
	}
	if want := "From: =?utf-8?q?Szko=C5=82a_Demo?= <sekretariat@example.test>"; s.fromHeader != want {
		t.Errorf("From header = %q, want %q", s.fromHeader, want)
	}
	if s.replyToHeader != "" {
		t.Errorf("Reply-To = %q, want none", s.replyToHeader)
	}
}

// TestMailpit_SendFromChosenSender: a sender chosen for the batch (ADR-0021)
// replaces the configured one in the envelope and the header.
func TestMailpit_SendFromChosenSender(t *testing.T) {
	t.Parallel()

	host, port, got := fakeSMTP(t)
	m := NewMailpit(MailpitConfig{Host: host, Port: port, From: "Szkoła <no-reply@example.test>"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msg := providers.Message{
		RecipientID: "r-1", To: "parent@example.test", Subject: "Test", Body: "Treść",
		From:    providers.Address{Email: "anna.nowak@example.test", Name: "Anna Nowak"},
		ReplyTo: providers.Address{Email: "sekretariat@example.test"},
	}
	if _, err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	s := <-got
	if !strings.HasPrefix(s.mailFrom, "MAIL FROM:<anna.nowak@example.test>") {
		t.Errorf("MAIL FROM = %q, want the chosen sender", s.mailFrom)
	}
	if want := `From: "Anna Nowak" <anna.nowak@example.test>`; s.fromHeader != want {
		t.Errorf("From header = %q, want %q", s.fromHeader, want)
	}
	if want := "Reply-To: <sekretariat@example.test>"; s.replyToHeader != want {
		t.Errorf("Reply-To header = %q, want %q", s.replyToHeader, want)
	}
}

// TestMailpit_NoAnswerAfterDataIsNotRetried: once the whole message reached the
// server, a missing answer may mean it is already queued, so the error must not
// be transient (the dispatcher would send it again).
func TestMailpit_NoAnswerAfterDataIsNotRetried(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 ok\r\n")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch up := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			case up == "DATA":
				_, _ = fmt.Fprint(conn, "354 go\r\n")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
				}
				time.Sleep(2 * time.Second) // never answers the end of data in time
				return
			}
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	m := NewMailpit(MailpitConfig{Host: host, Port: port, From: "no-reply@example.test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err = m.Send(ctx, providers.Message{RecipientID: "r-1", To: "parent@example.test", Subject: "S", Body: "B"})
	if err == nil || providers.IsTransient(err) || providers.IsPermanent(err) {
		t.Errorf("err = %v, want an unclassified error", err)
	}
}
