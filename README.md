# SMTP Email Ingest Server

A small Go SMTP server that accepts messages, extracts headers, text/HTML bodies
and attachments, and passes each message to an application handler.

## Requirements and running

- Go **1.25.1 or later**.
- A certificate and private key when TLS is enabled.
- Permission to bind the configured port (port 25 commonly requires elevated
  privileges; port 2525 is convenient for local development).

Generate a local self-signed certificate:

```sh
make ssl
```

Run with optional STARTTLS on port 2525:

```sh
SMTP_ADDR=127.0.0.1:2525 go run .
```

TLS-enabled modes fail at startup if the certificate cannot be loaded. For a
plaintext local test, explicitly select `SMTP_TLS_MODE=off`.

## Configuration

All environment variables are loaded and validated in [`config.go`](config.go).
Invalid values fail startup. Sizes are **integer bytes**, and durations use Go
syntax such as `5s` or `1m`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SMTP_ADDR` | `0.0.0.0:25` | Listener host and port |
| `SMTP_DOMAIN` | `localhost` | Hostname advertised in SMTP greetings |
| `TRUSTED_DOMAINS` | empty | Comma-separated accepted recipient domains; empty accepts any domain; exact, case-insensitive matches |
| `SMTP_TLS_MODE` | `starttls` | `off`, `starttls`, or `required` |
| `TLS_CERT_FILE` | `config/localhost.crt` | PEM TLS certificate file |
| `TLS_KEY_FILE` | `config/localhost.key` | PEM private key file |
| `SMTP_READ_TIMEOUT` | `30s` | SMTP connection read timeout |
| `SMTP_WRITE_TIMEOUT` | `30s` | SMTP connection write timeout |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown deadline after SIGINT/SIGTERM |
| `MAX_MESSAGE_BYTES` | `26214400` (25 MiB) | Maximum complete wire message size, including headers and encoding overhead |
| `MAX_RECIPIENTS` | `50` | Maximum envelope recipients per transaction |
| `MAX_ATTACHMENTS` | `10` | Maximum attachments per message; **0 prohibits attachments** |
| `MAX_ATTACHMENT_SIZE` | `10485760` (10 MiB) | Maximum **decoded bytes per attachment** |
| `MAX_MIME_PARTS` | `1000` | Maximum MIME entities per message, including multipart containers and the root |
| `SPF_ENABLED` | `true` | Evaluate SPF before handing the message to the application |
| `SPF_TIMEOUT` | `5s` | Total deadline for one SPF evaluation |

`starttls` advertises STARTTLS but also accepts plaintext SMTP, as is customary
for public mail reception. `required` rejects `MAIL FROM` until STARTTLS has
completed. Both TLS-enabled modes use TLS 1.2 or later. `off` disables TLS
explicitly and does not load certificate files.

An example environment file is provided in [`.env.example`](.env.example). The
application reads the **process environment**; it does not automatically load
`.env` files or depend on a dotenv package. For a trusted local file:

```sh
cp .env.example .env
# Edit .env as needed, then export its settings:
set -a
. ./.env
set +a
go run .
```

For example, to allow at most three attachments of 2 MiB each:

```sh
MAX_ATTACHMENTS=3 MAX_ATTACHMENT_SIZE=2097152 SMTP_ADDR=127.0.0.1:2525 go run .
```

### Attachment and MIME behavior

- Attachment limits apply to both regular files and inline non-body content.
  Text parts marked as attachments or carrying filenames count as attachments.
- Attachment size is checked while reading the decoded stream. Base64 and
  quoted-printable overhead does not count toward the attachment limit, but
  does count toward `MAX_MESSAGE_BYTES`.
- Exceeding message, attachment, MIME-part, or nesting limits rejects the
  **entire message with SMTP 552**. No partial message is delivered.
- MIME nesting is limited to 30 levels. Truncated multipart content, invalid
  encodings, and malformed headers are rejected with SMTP 554.
- Missing content types default to `text/plain; charset=us-ascii`. Both
  single-part and multipart body transfer encodings are decoded. Charset
  metadata is preserved; arbitrary body charset conversion is not performed.
- Header address lists, quoted names, encoded words, quoted MIME parameters,
  and extended attachment filenames use Go's standard parsers.

## Integrating a handler

The default `service.Service.OnEmail` logs metadata, headers, and a short body
preview. Replace or extend it to persist messages, call a webhook, or enqueue
work. The preview preserves the parser-created body reader positions.

```go
func (s *Service) OnEmail(message *email.Email) error {
    // Persist/enqueue the message here before returning nil.
    return nil
}
```

Each successful DATA transaction calls the handler immediately, including when
several messages arrive on one connection. SMTP success is returned only after
the handler returns nil. Handler errors or panics produce a retryable SMTP 451.
There is no built-in durable queue: the default logging handler does not store
messages. A handler that performs side effects before failing should account
for client retries and possible duplicate submissions.

`OnEmailFailed` receives the transaction's sender, recipients, a fresh raw-message
reader, and the error. If reception was interrupted or the message exceeded its
size limit, the raw reader contains only the buffered prefix, bounded by the
message limit.

`Email` distinguishes:

- `EnvelopeFrom`: SMTP `MAIL FROM`; empty for delivery notifications/bounces.
- `From`: visible message `From:` header.
- `RcptTo`: SMTP envelope recipients, including Bcc recipients.
- `Recipients`: visible `To:` and `Cc:` header recipients.
- `HELO` and `ClientIP`: connecting sender identity.

## SPF and dependencies

SPF evaluates the envelope sender, falling back to HELO for null senders. The
full result is stored in `Email.SPFResult` (`pass`, `fail`, `softfail`, `neutral`,
`none`, `temperror`, or `permerror`); `Email.SPF` is true only for a pass. A blank
result means it was not checked. Results, including negative results, are cached
on the message. SPF is **informational** and does not itself reject messages.
DKIM and DMARC verification are not implemented.

Runtime dependencies are intentionally small:

- `github.com/emersion/go-smtp` and its existing SASL dependency.
- `blitiri.com.ar/go/spf`, which uses the Go standard library at runtime and
  handles SPF mechanisms, qualifiers, includes, redirects, macros, and lookup
  limits. Its upstream test dependencies are not application runtime imports.

Configuration, MIME parsing, TLS, limits, and project tests use the standard
library.

## Development checks

```sh
go test -race ./...
go vet ./...
```

Regression tests cover persistent SMTP connections and resets, handler failures,
null senders, MIME decoding, attachment boundaries, malformed content, SPF with
an in-memory DNS resolver, configuration validation, and required STARTTLS.
