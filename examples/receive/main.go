// A local SMTP receiver that embeds getmail in another Go application.
package main

import (
	"io"
	"log"
	"time"

	"github.com/TrueFix/getmail/email"
	"github.com/emersion/go-smtp"
)

func receiveEmail(message *email.Email) error {
	log.Printf("ID=%s envelope=%s from=%s recipients=%v subject=%q",
		message.ID, message.EnvelopeFrom.Email, message.From.Email,
		message.RcptTo, message.Subject)
	if message.BodyText != nil {
		body, err := io.ReadAll(message.BodyText)
		if err != nil {
			return err
		}
		log.Printf("Text: %s", body)
	}
	for _, attachment := range message.Attachments {
		log.Printf("Attachment: name=%q type=%s decoded-bytes=%d",
			attachment.Filename(), attachment.ContentType(), attachment.Size)
	}
	// Persist or enqueue work before returning nil to acknowledge the message.
	return nil
}

func receiveFailed(from email.EmailUser, to []email.EmailUser, raw io.Reader, err error) {
	log.Printf("Message rejected: from=%s recipients=%v error=%v", from.Email, to, err)
}

func main() {
	options := email.DefaultOptions()
	options.SPFEnabled = false // No DNS checks for this local development example.
	options.Limits.MaxAttachments = 3
	options.Limits.MaxAttachmentSize = 2 * 1024 * 1024

	backend := email.NewBackend(receiveEmail, receiveFailed, []string{"example.test"}, options)
	server := smtp.NewServer(backend)
	server.Addr = "127.0.0.1:2525"
	server.Domain = "localhost"
	server.MaxMessageBytes = options.Limits.MaxMessageBytes
	server.MaxRecipients = 50
	server.ReadTimeout = 30 * time.Second
	server.WriteTimeout = 30 * time.Second

	log.Printf("Example SMTP receiver listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
