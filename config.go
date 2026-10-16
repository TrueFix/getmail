package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains all environment-controlled application settings. Byte limits
// are decimal integers; durations use Go syntax, for example "30s".
type Config struct {
	SMTPAddr          string
	SMTPDomain        string
	TrustedDomains    []string
	TLSMode           string
	TLSCertFile       string
	TLSKeyFile        string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ShutdownTimeout   time.Duration
	MaxMessageBytes   int64
	MaxRecipients     int
	MaxAttachments    int
	MaxAttachmentSize int64
	MaxMIMEParts      int
	SPFEnabled        bool
	SPFTimeout        time.Duration
}

func loadConfig() (Config, error) {
	return configFromEnv(os.LookupEnv)
}

func configFromEnv(lookup func(string) (string, bool)) (Config, error) {
	var errs []string
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	number := func(key string, fallback, minimum int64) int64 {
		value, err := strconv.ParseInt(get(key, strconv.FormatInt(fallback, 10)), 10, 64)
		if err != nil || value < minimum || value > int64(^uint(0)>>1) {
			errs = append(errs, fmt.Sprintf("%s must be an integer >= %d within platform limits", key, minimum))
		}
		return value
	}
	duration := func(key, fallback string) time.Duration {
		value, err := time.ParseDuration(get(key, fallback))
		if err != nil || value <= 0 {
			errs = append(errs, key+" must be a positive duration (e.g. 30s)")
		}
		return value
	}
	boolean := func(key, fallback string) bool {
		value, err := strconv.ParseBool(get(key, fallback))
		if err != nil {
			errs = append(errs, key+" must be true or false")
		}
		return value
	}
	c := Config{
		SMTPAddr:          get("SMTP_ADDR", "0.0.0.0:25"),
		SMTPDomain:        get("SMTP_DOMAIN", "localhost"),
		TLSMode:           strings.ToLower(get("SMTP_TLS_MODE", "starttls")),
		TLSCertFile:       get("TLS_CERT_FILE", "config/localhost.crt"),
		TLSKeyFile:        get("TLS_KEY_FILE", "config/localhost.key"),
		ReadTimeout:       duration("SMTP_READ_TIMEOUT", "30s"),
		WriteTimeout:      duration("SMTP_WRITE_TIMEOUT", "30s"),
		ShutdownTimeout:   duration("SHUTDOWN_TIMEOUT", "10s"),
		MaxMessageBytes:   number("MAX_MESSAGE_BYTES", 25*1024*1024, 1),
		MaxRecipients:     int(number("MAX_RECIPIENTS", 50, 1)),
		MaxAttachments:    int(number("MAX_ATTACHMENTS", 10, 0)),
		MaxAttachmentSize: number("MAX_ATTACHMENT_SIZE", 10*1024*1024, 1),
		MaxMIMEParts:      int(number("MAX_MIME_PARTS", 1000, 1)),
		SPFEnabled:        boolean("SPF_ENABLED", "true"),
		SPFTimeout:        duration("SPF_TIMEOUT", "5s"),
	}
	if _, _, err := net.SplitHostPort(c.SMTPAddr); err != nil {
		errs = append(errs, "SMTP_ADDR must be a host:port address")
	}
	if c.SMTPDomain == "" || strings.ContainsAny(c.SMTPDomain, " \t\r\n") {
		errs = append(errs, "SMTP_DOMAIN must be a nonempty hostname")
	}
	if c.TLSMode != "off" && c.TLSMode != "starttls" && c.TLSMode != "required" {
		errs = append(errs, "SMTP_TLS_MODE must be off, starttls, or required")
	}
	if c.TLSMode != "off" && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
		errs = append(errs, "TLS_CERT_FILE and TLS_KEY_FILE are required when TLS is enabled")
	}
	for domain := range strings.SplitSeq(get("TRUSTED_DOMAINS", ""), ",") {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			if strings.ContainsAny(domain, "@/ \t\r\n") {
				errs = append(errs, "TRUSTED_DOMAINS must contain comma-separated domain names")
			}
			c.TrustedDomains = append(c.TrustedDomains, domain)
		}
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(errs, "; "))
	}
	return c, nil
}
