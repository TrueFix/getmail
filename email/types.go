package email

import (
	"fmt"
	"io"
	"mime"
	"net"
	"net/textproto"
	"strings"
	"time"
)

type Headers map[string]string

func (h Headers) GetValue(key string) string {
	if value, ok := h[key]; ok {
		return value
	}
	value := h[textproto.CanonicalMIMEHeaderKey(key)]
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
	_, params, err := mime.ParseMediaType(h.GetValue(headerKey))
	if err != nil {
		return ""
	}
	return params[strings.ToLower(paramKey)]
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
	return rp.Headers.ContentType.Params["name"]
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

func (rp *EmailContent) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := rp.R.(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	return 0, fmt.Errorf("email content is not seekable")
}

type EmailUser struct {
	Name  string `json:"Name,omitempty"`  // Name is the display name of the user.
	Email string `json:"Email,omitempty"` // Email is the email address of the user.
}

func (eu EmailUser) HasDomain(domains []string) (string, bool) {
	at := strings.LastIndexByte(eu.Email, '@')
	if at < 0 {
		return "", false
	}
	domain := eu.Email[at+1:]
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

	// EnvelopeFrom is the SMTP MAIL FROM identity (empty for a bounce).
	EnvelopeFrom EmailUser
	HELO         string

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
	SPF       bool   // SPF check result
	SPFResult string // Full SPF result; empty when not checked.
	spfError  error
	DKIM      bool // DKIM check result
	DMARC     bool // DMARC check result
}

func NewEmail() *Email {
	uuid, _ := NewUUIDv7()
	return &Email{
		ID:         uuid.String(), // UUIDv7
		ReceivedAt: time.Now(),    // Optional auto-initialized field
	}
}
