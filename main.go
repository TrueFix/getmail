package main

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/TrueFix/getmail/email"
	"github.com/TrueFix/getmail/service"
	"github.com/emersion/go-smtp"
)

// createTLSConfig loads TLS certificates and returns the TLS config.
func createTLSConfig() (*tls.Config, error) {
	// Check if cert files exist
	if _, err := os.Stat("config/localhost.crt"); err == nil {
		if _, err := os.Stat("config/localhost.key"); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	// Load cert and key
	cert, err := tls.LoadX509KeyPair("config/localhost.crt", "config/localhost.key")
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   "localhost",
	}, nil
}

// runSMTPServer sets up and starts the SMTP server.
func runSMTPServer() error {
	tlsConfig, err := createTLSConfig()
	if err != nil {
		return err
	}

	externalService := &service.Service{}

	backend := email.NewBackend(
		externalService.OnEmail,
		externalService.OnEmailFailed,
		[]string{}, // Trusted domains
	)

	server := smtp.NewServer(backend)
	server.Addr = "0.0.0.0:25"
	server.TLSConfig = tlsConfig
	server.WriteTimeout = 10 * time.Second
	server.ReadTimeout = 10 * time.Second
	server.MaxMessageBytes = 1024 * 1024
	server.MaxRecipients = 50

	log.Println("[INFO] SMTP server listening on", server.Addr)
	return server.ListenAndServe()
}

func main() {
	if err := runSMTPServer(); err != nil {
		log.Fatal("[FATAL]", err)
	}
}
