package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TrueFix/getmail/email"
	"github.com/TrueFix/getmail/service"
	"github.com/emersion/go-smtp"
)

func createTLSConfig(c Config) (*tls.Config, error) {
	if c.TLSMode == "off" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

func newSMTPServer(c Config) (*smtp.Server, error) {
	tlsConfig, err := createTLSConfig(c)
	if err != nil {
		return nil, err
	}
	externalService := &service.Service{}
	backend := email.NewBackend(externalService.OnEmail, externalService.OnEmailFailed, c.TrustedDomains, email.Options{
		Limits: email.Limits{
			MaxMessageBytes: c.MaxMessageBytes, MaxAttachments: c.MaxAttachments,
			MaxAttachmentSize: c.MaxAttachmentSize, MaxMIMEParts: c.MaxMIMEParts,
		},
		RequireTLS: c.TLSMode == "required", SPFEnabled: c.SPFEnabled, SPFTimeout: c.SPFTimeout,
	})
	server := smtp.NewServer(backend)
	server.Addr = c.SMTPAddr
	server.Domain = c.SMTPDomain
	server.TLSConfig = tlsConfig
	server.WriteTimeout = c.WriteTimeout
	server.ReadTimeout = c.ReadTimeout
	server.MaxMessageBytes = c.MaxMessageBytes
	server.MaxRecipients = c.MaxRecipients
	return server, nil
}

func runSMTPServer(ctx context.Context, c Config) error {
	server, err := newSMTPServer(c)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("[INFO] SMTP listening on %s (TLS mode: %s)", c.SMTPAddr, c.TLSMode)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}

func main() {
	c, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runSMTPServer(ctx, c); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
		log.Fatal("[FATAL] ", err)
	}
}
