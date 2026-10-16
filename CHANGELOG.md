# Changelog

## v1.1.0

### Added

- Centralized, validated environment configuration and `.env.example`.
- Decoded attachment size/count limits, total message and recipient limits,
  and MIME-part/nesting limits.
- Explicit TLS modes, TLS 1.2 minimum, SPF deadline/settings, and graceful shutdown.
- Embedding examples for accepting email in another Go application, reading
  bodies/attachments, storing raw messages, and enabling TLS.
- Regression tests for SMTP transactions, parsing, limits, SPF, configuration,
  body previews, and STARTTLS.
- Make targets for running, building, testing, vetting, formatting, and version
  reporting; dedicated certificate cleanup and safe certificate generation.
- `VERSION` metadata and `--version` reporting.

### Fixed

- Process each message during DATA rather than delaying delivery until disconnect.
- Reset envelope sender, recipients, and message state between transactions.
- Return retryable SMTP errors for application-handler failures and panics.
- Accept null reverse-paths used by delivery notifications.
- Parse standard address lists, encoded headers, quoted MIME parameters, and
  extended attachment filenames with the Go standard library.
- Apply default MIME content types and decode single-part and multipart content.
- Reject malformed/truncated content and oversized attachments instead of
  silently delivering partial messages.
- Evaluate SPF against the envelope identity, with HELO fallback for bounces,
  using `blitiri.com.ar/go/spf` (no transitive runtime dependencies).
- Preserve body reader positions when logging previews.
- Align Makefile certificate paths with runtime settings; preserve existing
  certificates and include localhost DNS/IP subject alternative names.

### Migrating from v1.0.1

- The receive callback now has signature `func(*email.Email) error`. Return nil
  only after processing or durable enqueueing succeeds. An error or panic causes
  SMTP 451 so the sending client can retry.
- `OnEmailReceived` runs once per successful DATA operation, before SMTP success,
  rather than during logout. Handlers should account for client retries.
- `Email.From` remains the visible header identity. Use `Email.EnvelopeFrom` for
  `MAIL FROM`, `RcptTo` for SMTP recipients, and `Recipients` for header recipients.
- The old custom `SPFRecord` network-collection API is replaced by
  `Email.VerifySPF` / `Email.VerifySPFContext` and the full `SPFResult` field.
  SPF remains informational by default.
- TLS-enabled startup now fails if certificate loading fails. Use
  `SMTP_TLS_MODE=off` explicitly for plaintext operation.
- The documented minimum Go version is 1.25.1. Review environment defaults and
  limits in the README before upgrading.

## v1.0.1

Previous release, preserved by the existing `v1.0.1` Git tag.
