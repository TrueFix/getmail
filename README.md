# 📬 getmail

**v1.1.0** — Small Go SMTP server that accepts mail, parses MIME, and hands each message to your code.

[Changelog & migration from v1.0.1 →](CHANGELOG.md)

---

## ✨ What you get

- SMTP ingestion with optional **STARTTLS**
- Parsed **text/HTML**, **attachments**, headers, envelope vs visible addresses
- **SPF** check on envelope sender (informational; does not auto-reject)
- Limits for message size, recipients, attachments, and MIME parts
- Embed the `email` package in your own app — no need to run this binary

---

## 🚀 Quick start

**Requirements:** Go 1.25.1+, OpenSSL if you generate dev certs.

```sh
make ssl                              # self-signed cert (keeps existing pair)
SMTP_ADDR=127.0.0.1:2525 go run .     # local listener
```

Plaintext only (no cert):

```sh
SMTP_TLS_MODE=off SMTP_ADDR=127.0.0.1:2525 go run .
```

Port **25** often needs root; **2525** is fine for dev.

```sh
make help    # all make targets
make test    # race-enabled tests
```

---

## ⚙️ Configuration

Set via environment (validated in [`config.go`](config.go)). Copy [`.env.example`](.env.example) and export it — the app does **not** load `.env` by itself.

| Variable | Default | Notes |
| --- | --- | --- |
| `SMTP_ADDR` | `0.0.0.0:25` | Listen address |
| `SMTP_DOMAIN` | `localhost` | SMTP banner hostname |
| `TRUSTED_DOMAINS` | *(empty = any)* | Comma-separated recipient domains |
| `SMTP_TLS_MODE` | `starttls` | `off` · `starttls` · `required` |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | `config/localhost.*` | Required when TLS is on |
| `MAX_MESSAGE_BYTES` | 25 MiB | Full wire size |
| `MAX_ATTACHMENTS` | `10` | `0` = no attachments |
| `MAX_ATTACHMENT_SIZE` | 10 MiB | **Decoded** bytes per file |
| `MAX_RECIPIENTS` | `50` | Per transaction |
| `MAX_MIME_PARTS` | `1000` | |
| `SPF_ENABLED` | `true` | |
| `SPF_TIMEOUT` | `5s` | |

Over-limit messages → SMTP **552**. Handler error → **451** (client may retry).

`make ssl run DOMAIN=mail.example.com SMTP_ADDR=127.0.0.1:2525` wires cert paths for that domain automatically.

---

## 🧩 Use in your Go app

Install:

```sh
go get github.com/TrueFix/getmail@v1.1.0
```

Runnable example in this repo:

```sh
go run ./examples/receive
```

Source: [`examples/receive/main.go`](examples/receive/main.go) — wire `email.NewBackend(yourCallback, onFailed, trustedDomains, options)` and `smtp.NewServer`.

**Handler contract**

- Return `nil` from your callback → SMTP accepts the message.
- Return an error → **451**; design for idempotency / retries.
- `Email.EnvelopeFrom` = SMTP sender · `Email.From` = header · `Email.RcptTo` = envelope (incl. Bcc).

Optional helper to save raw `.eml`: [`examples/receive/save.go`](examples/receive/save.go).

---

## 🔒 TLS with Make

```sh
make ssl DOMAIN=mail.example.com
make run DOMAIN=mail.example.com SMTP_ADDR=127.0.0.1:2525
```

Regenerate certs: `make clean-ssl DOMAIN=…` then `make ssl` again.

---

## 📦 Version

```sh
go run . --version
# or: make version
```

Release tags match [Go module versions](https://github.com/TrueFix/getmail/tags).

---

## 📄 License

[LICENSE](LICENSE)
