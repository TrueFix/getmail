package service

import (
	"html"
	"io"
	"log"

	"github.com/TrueFix/getmail/email"
)

// logEmailMetadata logs high-level metadata of the email.
func logEmailMetadata(e *email.Email) {
	log.Printf(
		"\n[RECEIVED EMAIL]\nIP: %s\nFrom: %s\nTo: %v\nSubject: %s\nText Body: %v\nHTML Body: %v\nAttachments: %d",
		e.ClientIP,
		e.From.Email,
		e.RcptTo,
		e.Subject,
		e.BodyText != nil,
		e.BodyHTML != nil,
		len(e.Attachments),
	)

	if spf, err := e.VerifySPF(); err != nil {
		log.Printf("[WARNING] SPF verification failed: %v", err)
	} else {
		log.Printf("[INFO] SPF verified: %v", spf)
	}
}

// logEmailHeaders logs detailed headers of the email.
func logEmailHeaders(e *email.Email) {
	h := e.Headers

	log.Printf("[HEADERS]")
	log.Printf("Mime-Version: %s", h.MimeVersion)
	log.Printf("Date: %s", h.Date)
	log.Printf("Subject: %s", h.Subject)
	log.Printf("From: %s <%s>", h.From.Name, h.From.Email)
	log.Printf("To: %+v", h.To)
	log.Printf("Cc: %+v", h.Cc)
	log.Printf("RcptTo: %+v", e.RcptTo)
	log.Printf("Content-Type: %s/%s; Params: %v", h.ContentType.MediaType, h.ContentType.SubType, h.ContentType.Params)
	log.Printf("Content-Transfer-Encoding: %s", h.ContentTransferEncoding)
}

// logEmailBodies prints up to 100 bytes of text and HTML bodies.
func logEmailBodies(e *email.Email) {
	log.Printf("[BODY]")

	if e.BodyText != nil {
		body, err := previewBody(e.BodyText)
		if err != nil {
			log.Printf("[ERROR] Reading text body: %v", err)
		} else {
			log.Printf("Text Body (preview): %s", string(body))
		}
	}

	if e.BodyHTML != nil {
		body, err := previewBody(e.BodyHTML)
		if err != nil {
			log.Printf("[ERROR] Reading HTML body: %v", err)
		} else {
			log.Printf("HTML Body (preview): %s", html.UnescapeString(string(body)))
		}
	}
}

// previewBody reads and returns the first 100 bytes of a body.
func previewBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(body) > 100 {
		body = body[:100]
	}
	return body, nil
}
