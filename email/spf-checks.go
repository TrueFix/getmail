package email

import (
	"context"
	"time"

	"blitiri.com.ar/go/spf"
)

// VerifySPF checks the SMTP envelope identity, never the visible From header.
func (e *Email) VerifySPF() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return e.VerifySPFContext(ctx)
}

func (e *Email) VerifySPFContext(ctx context.Context, options ...spf.Option) (bool, error) {
	if e.SPFResult != "" {
		return e.SPF, e.spfError
	}
	if e.ClientIP == nil || (e.EnvelopeFrom.Email == "" && e.HELO == "") {
		return false, nil
	}
	sender := e.EnvelopeFrom.Email
	if sender == "" {
		sender = "postmaster@" + e.HELO
	}
	options = append(options, spf.WithContext(ctx))
	result, reason := spf.CheckHostWithSender(e.ClientIP, e.HELO, sender, options...)
	if ctx.Err() != nil {
		result, reason = spf.TempError, ctx.Err()
	}
	e.SPFResult, e.SPF = string(result), result == spf.Pass
	// The library also returns diagnostic match reasons on successful checks.
	if result == spf.TempError || result == spf.PermError {
		e.spfError = reason
		return false, reason
	}
	return e.SPF, nil
}
