package email

import (
	"fmt"
	"io"
	"net"

	"github.com/emersion/go-smtp"
)

// A Session is returned after successful login.
type Session struct {
	State *smtp.Conn

	TrustedDomains []string

	From   EmailUser
	RcptTo []EmailUser

	Email *Email // Current email being processed

	OnEmailReceived func(email *Email)
	OnEmailFailed   func(from EmailUser, to []EmailUser, raw io.Reader, err error)
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	eu, err := parseEmailUser(from)
	if err != nil {
		return fmt.Errorf("Mail: failed to parse sender '%s': %w", from, err)
	}

	s.From = eu
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	eu, err := parseEmailUser(to)
	if err != nil {
		return fmt.Errorf("Rcpt: failed to parse recipient '%s': %w", to, err)
	}

	// If TrustedDomains is set, only allow recipients in those domains
	if len(s.TrustedDomains) > 0 {
		if domain, ok := eu.HasDomain(s.TrustedDomains); !ok {
			return fmt.Errorf("Mail: sender domain '%s' is not a valid domain", domain)
		}
	}

	s.RcptTo = append(s.RcptTo, eu)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	email, err := parseEmail(r)
	if err != nil {
		LogWarning("SMTP:Data", fmt.Sprintf("error parsing email: %v", err))
		if s.OnEmailFailed != nil {
			s.OnEmailFailed(s.From, s.RcptTo, r, err)
		}
		return fmt.Errorf("Data: failed to parse email: %w", err)
	}
	var clientIP net.IP
	if s.State != nil && s.State.Conn() != nil {
		clientAddr := s.State.Conn().RemoteAddr().String()
		host, _, _ := net.SplitHostPort(clientAddr)
		clientIP = net.ParseIP(host)
	}
	email.ClientIP = clientIP
	email.RcptTo = s.RcptTo

	s.Email = email

	return nil
}

func (s *Session) Reset() {}

func (s *Session) Logout() error {
	// recover from panic if OnEmailReceived panics
	defer func() {
		if r := recover(); r != nil {
			LogError("SMTP:Logout", fmt.Errorf("panic in OnEmailReceived: %v", r))
			// Optionally, you could also call OnEmailFailed here
			if s.OnEmailFailed != nil && s.Email != nil {
				s.OnEmailFailed(s.From, s.RcptTo, s.Email.Raw, fmt.Errorf("panic in OnEmailReceived: %v", r))
			}
		}
	}()

	if s.OnEmailReceived != nil && s.Email != nil {
		s.OnEmailReceived(s.Email)
	}
	return nil
}
