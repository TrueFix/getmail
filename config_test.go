package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestConfigDefaultsAndOverrides(t *testing.T) {
	c, err := configFromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxAttachments != 10 || c.MaxAttachmentSize != 10485760 || c.TLSMode != "starttls" || !c.SPFEnabled {
		t.Fatalf("bad defaults: %+v", c)
	}
	c, err = configFromEnv(env(map[string]string{
		"SMTP_ADDR": "127.0.0.1:2525", "SMTP_DOMAIN": "mail.example.com", "SMTP_TLS_MODE": "off",
		"MAX_ATTACHMENTS": "0", "MAX_ATTACHMENT_SIZE": "1024", "MAX_MESSAGE_BYTES": "2048",
		"TRUSTED_DOMAINS": "Example.com, other.test ", "SPF_ENABLED": "false", "SMTP_READ_TIMEOUT": "1m",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxAttachments != 0 || c.MaxAttachmentSize != 1024 || c.SPFEnabled || len(c.TrustedDomains) != 2 || c.TrustedDomains[0] != "example.com" {
		t.Fatalf("overrides not applied: %+v", c)
	}
	server, err := newSMTPServer(c)
	if err != nil {
		t.Fatal(err)
	}
	if server.TLSConfig != nil || server.MaxMessageBytes != 2048 || server.Addr != "127.0.0.1:2525" || server.Domain != "mail.example.com" {
		t.Fatal("server configuration not applied")
	}
}

func TestInvalidConfig(t *testing.T) {
	for key, value := range map[string]string{
		"SMTP_ADDR": "bad-address", "SMTP_DOMAIN": "bad\r\nhost", "SMTP_TLS_MODE": "typo",
		"MAX_ATTACHMENTS": "-1", "MAX_ATTACHMENT_SIZE": "0", "MAX_MESSAGE_BYTES": "no",
		"MAX_RECIPIENTS": "0", "MAX_MIME_PARTS": "0", "SMTP_READ_TIMEOUT": "-1s",
		"SPF_TIMEOUT": "invalid", "SPF_ENABLED": "maybe", "TLS_CERT_FILE": "", "TRUSTED_DOMAINS": "a@b.test",
	} {
		t.Run(key, func(t *testing.T) {
			if _, err := configFromEnv(env(map[string]string{key: value})); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("invalid %s accepted: %v", key, err)
			}
		})
	}
}

func TestMissingTLSCertificateFailsStartup(t *testing.T) {
	c, err := configFromEnv(env(map[string]string{"TLS_CERT_FILE": t.TempDir() + "/missing.crt"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newSMTPServer(c); err == nil {
		t.Fatal("missing TLS certificate silently downgraded to plaintext")
	}
	c.TLSMode = "off"
	if _, err := createTLSConfig(c); err != nil {
		t.Fatal("explicit TLS off should not load certificates")
	}
}

func TestRequiredSTARTTLS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := configFromEnv(env(map[string]string{"SMTP_TLS_MODE": "required", "TLS_CERT_FILE": certFile, "TLS_KEY_FILE": keyFile, "SPF_ENABLED": "false"}))
	if err != nil {
		t.Fatal(err)
	}
	server, err := newSMTPServer(c)
	if err != nil {
		t.Fatal(err)
	}
	if server.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("TLS minimum must be 1.2")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Close(); <-done })
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	client, err := smtp.NewClient(conn, "localhost")
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Mail("a@example.com"); err == nil {
		t.Fatal("plaintext MAIL accepted")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	if err := client.StartTLS(&tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}); err != nil {
		t.Fatal(err)
	}
	if err := client.Mail("a@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := client.Quit(); err != nil {
		t.Fatal(err)
	}
}
