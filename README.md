# SMTP Email Ingest Server

**Version: v1.1.0** · [Changelog and migration notes](CHANGELOG.md)

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

Certificate generation requires OpenSSL 1.1.1 or later. Existing certificate/key
pairs are preserved; an incomplete pair produces an error. Generated private
keys have mode `0600`. Certificates include the selected hostname, `localhost`,
`127.0.0.1`, and `::1` as subject alternative names.

Run with optional STARTTLS on port 2525:

```sh
SMTP_ADDR=127.0.0.1:2525 go run .
```

The Makefile also passes its selected hostname and TLS paths to the application:

```sh
make ssl run DOMAIN=mail.example.com SMTP_ADDR=127.0.0.1:2525
```

`SMTP_DOMAIN`, `TLS_CERT_FILE`, and `TLS_KEY_FILE` can be supplied through the
environment or as make arguments. `DOMAIN` is a shorthand for `SMTP_DOMAIN`
when the latter is unset. Under make, certificate paths default to
`config/<SMTP_DOMAIN>.crt` and `config/<SMTP_DOMAIN>.key`. Direct `go run .`
continues to use the defaults in the configuration table below. Make does not
automatically load `.env` files.

Paths containing spaces are supported when quoted:

```sh
make ssl run SMTP_DOMAIN=mail.example.com \
  TLS_CERT_FILE="config/local tls/server.crt" \
  TLS_KEY_FILE="config/local tls/server.key" \
  SMTP_ADDR=127.0.0.1:2525
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

## Accepting email in your own Go application

Import `github.com/TrueFix/getmail/email` to use the parser and SMTP backend in
your own program. The executable's `config.go` belongs to `package main`; an
embedded application sets `email.Options` and `smtp.Server` fields itself.

### 1. Complete local receiver

After the `v1.1.0` Git tag is published, install the versioned module in your
application:

```sh
go mod init example.com/mail-receiver
go get github.com/TrueFix/getmail@v1.1.0
```

Save this as `main.go`. It accepts recipients at `example.test` over plaintext
SMTP on localhost, prints the text body, and inspects attachment metadata:

```go
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
```

Run `go mod tidy` and `go run .` in your application directory. The same example
is included in this repository at [`examples/receive/main.go`](examples/receive/main.go):

```sh
go run ./examples/receive
```

Each connection has its own session. Different connections can invoke your
handler concurrently, so shared application state must support concurrent use.
Passing `nil` for trusted domains accepts recipients in any domain. Use
`email.DefaultOptions()` before overriding individual settings; passing an
`Options` value replaces the defaults rather than merging boolean settings.

### 2. Read HTML and decoded attachment bytes

Replace `receiveEmail` in the example with this callback when your integration
needs HTML or the attachment contents. It uses the same `io` and `log` imports:

```go
func receiveEmail(message *email.Email) error {
	if message.BodyHTML != nil {
		body, err := io.ReadAll(message.BodyHTML)
		if err != nil {
			return err
		}
		log.Printf("HTML body: %s", body)
	}
	for _, attachment := range message.Attachments {
		data, err := io.ReadAll(attachment)
		if err != nil {
			return err
		}
		log.Printf("Received %q: %d decoded bytes", attachment.Filename(), len(data))
		// Pass data to your storage or processing code here.
	}
	return nil
}
```

Bodies and attachments are consumable readers containing transfer-decoded
bytes. Parser-created `EmailContent` values also support `Seek(0, io.SeekStart)`
if you need to read them again. Original attachment filenames are metadata; use
application-generated names when choosing storage paths.

### 3. Store the original message as an `.eml` file

Save this helper as `save.go` alongside the complete receiver. Call
`return saveRawEmail(message, "messages")` from your callback to acknowledge a
message only after writing the file and closing it successfully:

The helper is also included at [`examples/receive/save.go`](examples/receive/save.go).

```go
package main

import (
	"errors"
	"io"
	"os"

	"github.com/TrueFix/getmail/email"
)

func saveRawEmail(message *email.Email, directory string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, message.ID+"-*.eml")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, message.Raw)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	return nil
}
```

This stores the original headers, encoded bodies, and attachments as one file.
For stronger durability guarantees, use your application's durable queue or
storage transaction. The message ID identifies this reception, so a sender
retry can have a new ID.

### 4. Enable STARTTLS in an embedded server

Add `crypto/tls` to the complete example's imports. Before constructing the
backend, require an encrypted SMTP transaction:

```go
options.RequireTLS = true
```

After constructing `server`, configure its certificate before listening:

```go
certificate, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil {
	log.Fatal(err)
}
server.TLSConfig = &tls.Config{
	Certificates: []tls.Certificate{certificate},
	MinVersion:   tls.VersionTLS12,
}
```

Set `RequireTLS` to false if plaintext SMTP should also be accepted while
STARTTLS is available. Options are copied into the backend at construction;
configure them before calling `email.NewBackend`.

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
make help    # List targets
make build   # Build bin/getmail; BUILD_DIR can override bin
make test    # go test -race ./...
make vet     # go vet ./...
make fmt     # Format Go sources
make version # Print v1.1.0 without loading certificates or starting SMTP
make clean   # Delete the executable, preserving certificates
```

To replace generated certificates, explicitly remove the selected pair first:

```sh
make clean-ssl DOMAIN=mail.example.com
make ssl DOMAIN=mail.example.com DAYS_VALID=365
```

`clean-ssl` deletes only the selected key/certificate paths. Development targets
add no Go dependencies; certificate tooling uses OpenSSL and standard Unix tools.

Regression tests cover persistent SMTP connections and resets, handler failures,
null senders, MIME decoding, attachment boundaries, malformed content, SPF with
an in-memory DNS resolver, configuration validation, and required STARTTLS.

## Versioning

[`VERSION`](VERSION) is the application version source, currently **v1.1.0**.
The value is embedded into builds and can be checked with:

```sh
go run . --version
# Or after make build:
./bin/getmail --version
```

Git tags identify published Go module versions; `go.mod` declares the module
path and Go toolchain requirement, not the repository's release number. Keep
`VERSION`, the release heading in `CHANGELOG.md`, and the release Git tag in
sync. A `v1.1.0` tag must point to a commit containing these changes and be
pushed before remote consumers can install `@v1.1.0`.

See [the v1.1.0 migration notes](CHANGELOG.md#migrating-from-v101) for the
error-returning callback and other upgrade changes from v1.0.1.
