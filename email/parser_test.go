package email

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func TestParseStandardMessages(t *testing.T) {
	for _, tc := range []struct{ name, headers, body, want string }{
		{"default content type", "", "hello", "hello"},
		{"base64", "Content-Type: TEXT/PLAIN\r\nContent-Transfer-Encoding: base64\r\n", "aGVsbG8=", "hello"},
		{"quoted printable", "Content-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n", "hello=3Dworld", "hello=world"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := parseEmail(strings.NewReader("From: sender@example.com\r\n" + tc.headers + "\r\n" + tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if e.BodyText == nil {
				t.Fatal("missing text body")
			}
			body, err := io.ReadAll(e.BodyText)
			if err != nil || string(body) != tc.want {
				t.Fatalf("body = %q, err = %v", body, err)
			}
			if e.BodyText.ContentType() != "text/plain" {
				t.Fatal(e.BodyText.ContentType())
			}
		})
	}
}

func TestAddressListsAndEncodedHeaders(t *testing.T) {
	e, err := parseEmail(strings.NewReader("From: =?UTF-8?Q?Jane_Doe?= <sender@example.com>\r\nTo: \"Doe, Jane\" <jane@example.com>, Team: a@example.com, b@example.com;\r\nCc: c@example.com\r\nSubject: =?UTF-8?B?SGVsbG8=?=\r\n\r\nhello"))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Recipients) != 4 || len(e.Headers.To) != 3 || e.Headers.To[0].Name != "Doe, Jane" || e.From.Name != "Jane Doe" || e.Subject != "Hello" {
		t.Fatalf("incorrect headers: %+v", e.Headers)
	}
	if len(e.RcptTo) != 0 {
		t.Fatal("header recipients must not populate envelope recipients")
	}
}

func attachmentMessage(t *testing.T, payloads ...string) string {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, payload := range payloads {
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"text/plain"},
			"Content-Disposition":       {`attachment; filename*=utf-8''report%3Bname.txt`},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, base64.StdEncoding.EncodeToString([]byte(payload))); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return "From: sender@example.com\r\nContent-Type: multipart/mixed; boundary=\"" + mw.Boundary() + "\"\r\n\r\n" + body.String()
}

func TestAttachmentLimits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		count    int
		size     int64
		payloads []string
		reject   bool
	}{
		{"at size limit", 1, 5, []string{"hello"}, false},
		{"over size limit", 1, 4, []string{"hello"}, true},
		{"at count limit", 2, 5, []string{"hello", "world"}, false},
		{"over count limit", 1, 5, []string{"hello", "world"}, true},
		{"attachments prohibited", 0, 5, []string{"hello"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := DefaultLimits()
			limits.MaxAttachments, limits.MaxAttachmentSize = tc.count, tc.size
			e, err := parseEmailWithLimits(strings.NewReader(attachmentMessage(t, tc.payloads...)), limits)
			if tc.reject {
				if !errors.Is(err, ErrLimitExceeded) {
					t.Fatalf("want limit error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(e.Attachments) != len(tc.payloads) || e.BodyText != nil {
				t.Fatal("text attachment misclassified")
			}
			if e.Attachments[0].Filename() != "report;name.txt" {
				t.Fatal(e.Attachments[0].Filename())
			}
			for i, attachment := range e.Attachments {
				body, err := io.ReadAll(attachment)
				if err != nil || string(body) != tc.payloads[i] || attachment.Size != int64(len(body)) {
					t.Fatalf("bad attachment: %q, %v", body, err)
				}
			}
		})
	}
}

func TestNestedMultipartAndQuotedBoundary(t *testing.T) {
	raw := "From: sender@example.com\r\nContent-Type: Multipart/Mixed; BOUNDARY=\"outer;boundary\"\r\n\r\n" +
		"--outer;boundary\r\nContent-Type: multipart/alternative; boundary=inner\r\n\r\n" +
		"--inner\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n=3D41\r\n" +
		"--inner\r\nContent-Type: text/html\r\n\r\n<b>hello</b>\r\n--inner--\r\n--outer;boundary--\r\n"
	e, err := parseEmail(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(e.BodyText)
	if err != nil || string(body) != "=41" {
		t.Fatalf("part decoded more than once: %q, %v", body, err)
	}
	if e.BodyHTML == nil || len(e.Attachments) != 0 {
		t.Fatal("nested parts not flattened")
	}
	limits := DefaultLimits()
	limits.MaxMIMEParts = 2
	if _, err := parseEmailWithLimits(strings.NewReader(raw), limits); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("MIME part limit not enforced: %v", err)
	}
}

func TestMalformedMultipartAndEncodingRejected(t *testing.T) {
	for _, raw := range []string{
		"Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\ntruncated",
		"Content-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\n%%%",
	} {
		if _, err := parseEmail(strings.NewReader(raw)); err == nil {
			t.Fatal("malformed content was accepted")
		}
	}
}

func TestMessageSizeBoundary(t *testing.T) {
	raw := "From: sender@example.com\r\n\r\nhello"
	limits := DefaultLimits()
	limits.MaxMessageBytes = int64(len(raw))
	if _, err := parseEmailWithLimits(strings.NewReader(raw), limits); err != nil {
		t.Fatal(err)
	}
	limits.MaxMessageBytes--
	if _, err := parseEmailWithLimits(strings.NewReader(raw), limits); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("want size rejection, got %v", err)
	}
}
