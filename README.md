# 📩 Simple SMTP Email Ingest Server

A lightweight, developer-friendly SMTP server built in Go that accepts incoming emails, parses them into structured data (JSON-like), and allows any software to consume or process the email content easily.

Ideal for use cases like:
- Internal tools needing email intake
- Automated systems (alerts, notifications, etc.)
- Email-to-API pipelines
- Email testing environments

---

## ⚙️ Features

- 📥 Accepts SMTP email messages over TLS
- 📄 Parses:
  - Subject, sender, recipients
  - Text and HTML bodies
  - Attachments
  - MIME headers and content types
- 🧾 SPF validation (to verify sender IP)
- 🔜 DKIM and DMARC validation (coming soon)
- 🧰 Easy to extend: just plug your handler into the `receiveEmail()` function
- 🧩 Simple to integrate with any system (webhooks, DB, queues, etc.)

---

## 🚀 Getting Started

### 🔧 Prerequisites

- [Go 1.20+](https://golang.org/dl/)
- TLS certificate (self-signed is OK for local use)

---

### 2. Install and Run Server

```bash
go run .
```
