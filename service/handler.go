package service

import (
	"io"
	"log"

	"github.com/TrueFix/getmail/email"
)

type Service struct{}

func (m *Service) OnEmail(email *email.Email) error {
	logEmailMetadata(email)
	logEmailHeaders(email)
	logEmailBodies(email)
	return nil
}

func (m *Service) OnEmailFailed(from email.EmailUser, to []email.EmailUser, raw io.Reader, err error) {
	log.Printf("Service: Failed to process email from %s to %d recipients: %v", from.Email, len(to), err)
}
