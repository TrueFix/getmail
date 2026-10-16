package email

import (
	"errors"
	"io"
	"net"
	stdsmtp "net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
)

func TestSMTPMultipleTransactionsAndReset(t *testing.T) {
	received := make(chan *Email, 3)
	backend := NewBackend(func(e *Email) error { received <- e; return nil }, nil, []string{"example.com"}, Options{Limits: DefaultLimits()})
	server := smtp.NewServer(backend)
	server.Domain = "localhost"
	server.ReadTimeout, server.WriteTimeout = 5*time.Second, 5*time.Second
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	client, err := stdsmtp.NewClient(conn, "localhost")
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Mail("abandoned@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt("abandoned@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := client.Reset(); err != nil {
		t.Fatal(err)
	}
	for _, recipient := range []string{"first@example.com", "second@example.com"} {
		// Null envelope senders must be accepted for delivery notifications.
		if err := client.Mail(""); err != nil {
			t.Fatal(err)
		}
		if err := client.Rcpt("outside@other.test"); err == nil {
			t.Fatal("untrusted recipient accepted")
		}
		if err := client.Rcpt(recipient); err != nil {
			t.Fatal(err)
		}
		writer, err := client.Data()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, "From: visible@other.test\r\nTo: visible@example.com\r\nSubject: "+recipient+"\r\n\r\nhello"); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case e := <-received:
			if len(e.RcptTo) != 1 || e.RcptTo[0].Email != recipient {
				t.Fatalf("wrong recipients: %+v", e.RcptTo)
			}
			if e.EnvelopeFrom.Email != "" || e.From.Email != "visible@other.test" {
				t.Fatal("envelope/header sender mixed")
			}
			if len(e.Recipients) != 1 || e.Recipients[0].Email != "visible@example.com" {
				t.Fatal("header recipients lost")
			}
			if e.ClientIP == nil || e.HELO == "" {
				t.Fatal("connection identity missing")
			}
		case <-time.After(time.Second):
			t.Fatal("message not delivered before DATA acknowledgment")
		}
	}
	if err := client.Quit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
		t.Fatal("duplicate delivery on logout")
	default:
	}
}

func TestDataFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*Email) error
		raw     string
		code    int
	}{
		{"handler error", func(*Email) error { return errors.New("storage unavailable") }, "From: a@example.com\r\n\r\nhello", 451},
		{"handler panic", func(*Email) error { panic("oops") }, "From: a@example.com\r\n\r\nhello", 451},
		{"missing handler", nil, "From: a@example.com\r\n\r\nhello", 451},
		{"malformed", func(*Email) error { t.Fatal("handler called for malformed message"); return nil }, "not a header\r\n\r\nhello", 554},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := 0
			s := &Session{OnEmailReceived: tc.handler, OnEmailFailed: func(_ EmailUser, _ []EmailUser, raw io.Reader, _ error) {
				failures++
				body, err := io.ReadAll(raw)
				if err != nil || string(body) != tc.raw {
					t.Fatalf("failure callback lost raw message: %q %v", body, err)
				}
			}}
			err := s.Data(strings.NewReader(tc.raw))
			var smtpErr *smtp.SMTPError
			if !errors.As(err, &smtpErr) || smtpErr.Code != tc.code || failures != 1 {
				t.Fatalf("err = %v, failure calls = %d", err, failures)
			}
		})
	}
}

func TestAttachmentRejectionNeverDelivers(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxAttachmentSize = 4
	s := &Session{Options: Options{Limits: limits}, OnEmailReceived: func(*Email) error { t.Fatal("oversized attachment delivered"); return nil }}
	err := s.Data(strings.NewReader(attachmentMessage(t, "hello")))
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 552 {
		t.Fatalf("want 552, got %v", err)
	}
}

func TestRequireTLS(t *testing.T) {
	s := &Session{Options: Options{RequireTLS: true}}
	var smtpErr *smtp.SMTPError
	if err := s.Mail("a@example.com", nil); !errors.As(err, &smtpErr) || smtpErr.Code != 530 {
		t.Fatalf("plaintext MAIL accepted: %v", err)
	}
}

type failedDataReader struct{}

func (failedDataReader) Read([]byte) (int, error) { return 0, smtp.ErrDataTooLarge }

func TestSMTPReaderSizeError(t *testing.T) {
	s := &Session{OnEmailReceived: func(*Email) error { t.Fatal("oversized message delivered"); return nil }}
	var smtpErr *smtp.SMTPError
	if err := s.Data(failedDataReader{}); !errors.As(err, &smtpErr) || smtpErr.Code != 552 {
		t.Fatalf("SMTP size error changed to temporary failure: %v", err)
	}
}
