package email

import (
	"io"

	"github.com/emersion/go-smtp"
)

// Backend implements the SMTP backend.
type Backend struct {
	TrustedDomains  []string
	OnEmailReceived func(email *Email)
	OnEmailFailed   func(from EmailUser, to []EmailUser, raw io.Reader, err error)
}

// NewSession initializes a new SMTP session.
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		State:           c,
		OnEmailReceived: bkd.OnEmailReceived,
		OnEmailFailed:   bkd.OnEmailFailed,
	}, nil
}

func NewBackend(
	OnEmailReceived func(email *Email),
	OnEmailFailed func(from EmailUser, to []EmailUser, raw io.Reader, err error),
	TrustedDomains []string,
) *Backend {
	return &Backend{
		OnEmailReceived: OnEmailReceived,
		OnEmailFailed:   OnEmailFailed,
		TrustedDomains:  TrustedDomains,
	}
}
