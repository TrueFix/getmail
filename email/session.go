package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/emersion/go-smtp"
)

type Options struct {
	Limits     Limits
	RequireTLS bool
	SPFEnabled bool
	SPFTimeout time.Duration
}

func DefaultOptions() Options {
	return Options{Limits: DefaultLimits(), SPFEnabled: true, SPFTimeout: 5 * time.Second}
}

// Session holds state for a connection; Reset clears each SMTP transaction.
type Session struct {
	State           *smtp.Conn
	TrustedDomains  []string
	Options         Options
	From            EmailUser
	RcptTo          []EmailUser
	Email           *Email
	OnEmailReceived func(email *Email) error
	OnEmailFailed   func(from EmailUser, to []EmailUser, raw io.Reader, err error)
}

func smtpError(code int, enhanced smtp.EnhancedCode, message string) error {
	return &smtp.SMTPError{Code: code, EnhancedCode: enhanced, Message: message}
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if s.Options.RequireTLS {
		if s.State == nil {
			return smtpError(530, smtp.EnhancedCode{5, 7, 0}, "STARTTLS required")
		}
		if _, ok := s.State.TLSConnectionState(); !ok {
			return smtpError(530, smtp.EnhancedCode{5, 7, 0}, "STARTTLS required")
		}
	}
	var sender EmailUser
	if from != "" {
		var err error
		sender, err = parseEmailUser(from)
		if err != nil {
			return smtpError(501, smtp.EnhancedCode{5, 1, 7}, "Invalid envelope sender")
		}
	}
	s.Reset()
	s.From = sender
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	recipient, err := parseEmailUser(to)
	if err != nil {
		return smtpError(501, smtp.EnhancedCode{5, 1, 3}, "Invalid recipient")
	}
	if len(s.TrustedDomains) > 0 {
		if _, ok := recipient.HasDomain(s.TrustedDomains); !ok {
			return smtpError(550, smtp.EnhancedCode{5, 7, 1}, "Recipient domain is not accepted")
		}
	}
	s.RcptTo = append(s.RcptTo, recipient)
	return nil
}

func (s *Session) Data(r io.Reader) (err error) {
	var raw []byte
	defer func() {
		if recovered := recover(); recovered != nil {
			LogError("SMTP:Data", fmt.Errorf("processing panic: %v", recovered))
			err = smtpError(451, smtp.EnhancedCode{4, 3, 0}, "Temporary message processing failure")
		}
		if err != nil {
			s.notifyFailure(raw, err)
		}
	}()
	options := s.Options
	if options.Limits == (Limits{}) {
		options.Limits = DefaultLimits()
	}
	raw, err = readBounded(r, options.Limits.MaxMessageBytes)
	if err != nil {
		if errors.Is(err, ErrLimitExceeded) || errors.Is(err, smtp.ErrDataTooLarge) {
			return smtpError(552, smtp.EnhancedCode{5, 3, 4}, "Message exceeds size limit")
		}
		return smtpError(451, smtp.EnhancedCode{4, 3, 0}, "Unable to read message")
	}
	e, parseErr := parseRawEmail(raw, options.Limits)
	if parseErr != nil {
		LogWarning("SMTP:Data", parseErr.Error())
		if errors.Is(parseErr, ErrLimitExceeded) {
			return smtpError(552, smtp.EnhancedCode{5, 3, 4}, parseErr.Error())
		}
		return smtpError(554, smtp.EnhancedCode{5, 6, 0}, "Malformed or unsupported message")
	}
	if s.State != nil {
		e.HELO = s.State.Hostname()
		if conn := s.State.Conn(); conn != nil {
			host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
			e.ClientIP = net.ParseIP(host)
		}
	}
	e.EnvelopeFrom = s.From
	e.RcptTo = append([]EmailUser(nil), s.RcptTo...)
	if options.SPFEnabled {
		timeout := options.SPFTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		_, spfErr := e.VerifySPFContext(ctx)
		cancel()
		if spfErr != nil {
			LogWarning("SMTP:SPF", spfErr.Error())
		}
	}
	s.Email = e
	if s.OnEmailReceived == nil {
		return smtpError(451, smtp.EnhancedCode{4, 3, 0}, "No message handler configured")
	}
	if handlerErr := s.OnEmailReceived(e); handlerErr != nil {
		LogError("SMTP:Data", handlerErr)
		return smtpError(451, smtp.EnhancedCode{4, 3, 0}, "Temporary message processing failure")
	}
	return nil
}

func (s *Session) notifyFailure(raw []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			LogError("SMTP:OnEmailFailed", fmt.Errorf("failure handler panic: %v", recovered))
		}
	}()
	if s.OnEmailFailed != nil {
		s.OnEmailFailed(s.From, append([]EmailUser(nil), s.RcptTo...), bytes.NewReader(raw), err)
	}
}

func (s *Session) Reset() {
	s.From = EmailUser{}
	s.RcptTo = nil
	s.Email = nil
}

func (s *Session) Logout() error {
	s.Reset()
	return nil
}
