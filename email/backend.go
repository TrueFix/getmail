package email

import (
	"io"

	"github.com/emersion/go-smtp"
)

// Backend implements the SMTP backend.
type Backend struct {
	TrustedDomains  []string
	Options         Options
	OnEmailReceived func(email *Email) error
	OnEmailFailed   func(from EmailUser, to []EmailUser, raw io.Reader, err error)
}

// NewSession initializes a new SMTP session.
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		State:           c,
		OnEmailReceived: bkd.OnEmailReceived,
		OnEmailFailed:   bkd.OnEmailFailed,
		TrustedDomains:  bkd.TrustedDomains,
		Options:         bkd.Options,
	}, nil
}

func NewBackend(
	OnEmailReceived func(email *Email) error,
	OnEmailFailed func(from EmailUser, to []EmailUser, raw io.Reader, err error),
	TrustedDomains []string,
	options ...Options,
) *Backend {
	opts := DefaultOptions()
	if len(options) > 0 {
		opts = options[0]
	}
	return &Backend{
		Options:         opts,
		OnEmailReceived: OnEmailReceived,
		OnEmailFailed:   OnEmailFailed,
		TrustedDomains:  TrustedDomains,
	}
}
