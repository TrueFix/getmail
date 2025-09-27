package email

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type Headers map[string]string

func (h Headers) GetValue(key string) string {
	value := h[key]
	return value
}

func (h Headers) GetFirst(key string) string {
	value := h.GetValue(key)
	values := strings.Split(value, ";")
	if len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

func (h Headers) GetParam(headerKey, paramKey string) string {
	value, ok := h[headerKey]
	if !ok {
		return ""
	}

	parts := strings.SplitSeq(value, ";")

	for part := range parts {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			// LogInfo("GetParam", fmt.Sprintf("Skipping malformed parameter in header: %q", part))
			continue
		}
		key := strings.TrimSpace(pair[0])
		val := strings.Trim(strings.TrimSpace(pair[1]), `"`)
		if key == paramKey {
			return val
		}
	}

	return ""
}

func (h Headers) GetString(key string) string {
	return h.GetValue(key)
}

// EmailContent represents a part of an email, such as the body or an attachment.
// it is also an io.Reader to read the content.
type EmailContent struct {
	R       io.Reader          `json:"R,omitempty"`       // R is the reader for the part's content.
	Headers EmailContentHeader `json:"Headers,omitempty"` // Headers are the headers associated with the part.
	Size    int64              `json:"Size,omitempty"`    // Size is the size of the content in bytes.
}

func (rp *EmailContent) Filename() string {
	disposition := rp.Headers.Extra.GetParam("Content-Disposition", "filename")
	if disposition != "" {
		return disposition
	}
	return ""
}

func (rp *EmailContent) ContentType() string {
	contentType := rp.Headers.ContentType.MediaType
	subType := rp.Headers.ContentType.SubType

	return fmt.Sprintf("%s/%s", contentType, subType)
}

func (rp *EmailContent) Read(p []byte) (n int, err error) {
	if rp.R == nil {
		return 0, io.EOF
	}
	return rp.R.Read(p)
}

type EmailUser struct {
	Name  string `json:"Name,omitempty"`  // Name is the display name of the user.
	Email string `json:"Email,omitempty"` // Email is the email address of the user.
}

func (eu EmailUser) HasDomain(domains []string) (string, bool) {
	parts := strings.SplitN(eu.Email, "@", 2)
	if len(parts) != 2 {
		return "", false
	}
	domain := parts[1]
	for _, d := range domains {
		if strings.EqualFold(d, domain) {
			return domain, true
		}
	}
	return domain, false
}

// 2 Types for email headers and email structs

type HeaderContentType struct {
	MediaType string  `json:"Media-Type,omitempty"` // MediaType is the main type of the content, e.g., "text"
	SubType   string  `json:"Sub-Type,omitempty"`   // SubType is the subtype of the content, e.g., "plain"
	Params    Headers `json:"Params,omitempty"`     // Params are additional parameters, e.g., charset, boundary
}

type EmailContentHeader struct {
	MimeVersion             string            `json:"MIME-Version,omitempty"`              // MIME-Version
	ContentType             HeaderContentType `json:"Content-Type,omitempty"`              // Content-Type
	ContentTransferEncoding string            `json:"Content-Transfer-Encoding,omitempty"` // Content-Transfer-Encoding
	Extra                   Headers           `json:"Extra,omitempty"`                     // Extra headers
}

type MimeHeaders struct {
	MimeVersion string `json:"MIME-Version,omitempty"` // MIME-Version
	Date        string `json:"Date,omitempty"`         // Date
	Subject     string `json:"Subject,omitempty"`      // Subject

	From EmailUser   `json:"From,omitempty"` // From
	To   []EmailUser `json:"To,omitempty"`   // To
	Cc   []EmailUser `json:"Cc,omitempty"`   // Cc

	ContentType             HeaderContentType `json:"Content-Type,omitempty"`              // Content-Type
	ContentTransferEncoding string            `json:"Content-Transfer-Encoding,omitempty"` // Content-Transfer-Encoding

	Extra Headers `json:"Extra,omitempty"` // Extra headers
}

type Email struct {
	// Unique ID for the email
	ID string `json:"ID,omitempty"`

	// Timestamp when the email was received
	ReceivedAt time.Time `json:"ReceivedAt,omitempty"`

	// Client ip address
	ClientIP net.IP

	// From is the email address of the sender.
	From EmailUser

	// Recipients is a list of email addresses of the recipients like To, Cc and Bcc.
	RcptTo []EmailUser

	// To is a list of email addresses of the recipients like To, Cc
	Recipients []EmailUser

	// Subject is the subject of the email.
	Subject string

	// Headers is a map of additional headers to include in the email.
	Headers *MimeHeaders

	// Raw is the raw email data. filename: email.eml
	Raw io.Reader

	// Body is the body of the email.
	Body io.Reader // Raw body data, can be plain text or HTML.

	BodyText *EmailContent
	BodyHTML *EmailContent

	// Attachments is a list of file paths to attach to the email.
	Attachments []*EmailContent

	// Verification checks
	SPF   bool // SPF check result
	DKIM  bool // DKIM check result
	DMARC bool // DMARC check result
}

func NewEmail() *Email {
	uuid, _ := NewUUIDv7()
	return &Email{
		ID:         uuid.String(), // UUIDv7
		ReceivedAt: time.Now(),    // Optional auto-initialized field
	}
}

func (e *Email) VerifySPF() (bool, error) {
	// check if we already have the result
	if e.SPF {
		return e.SPF, nil
	}

	if e.ClientIP == nil || e.From.Email == "" {
		return false, nil // Cannot verify SPF without client IP or sender email
	}
	domain := strings.SplitN(e.From.Email, "@", 2)[1]

	spfRecord := NewSPFRecord(domain)
	return spfRecord.CheckSPF(domain, e.ClientIP)
}
