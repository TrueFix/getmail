SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# DOMAIN remains a convenient shorthand; application environment settings win.
DOMAIN ?= localhost
SMTP_DOMAIN ?= $(DOMAIN)
TLS_KEY_FILE ?= config/$(SMTP_DOMAIN).key
TLS_CERT_FILE ?= config/$(SMTP_DOMAIN).crt
DAYS_VALID ?= 365
BUILD_DIR ?= bin

export SMTP_DOMAIN TLS_KEY_FILE TLS_CERT_FILE DAYS_VALID BUILD_DIR

.PHONY: help ssl clean-ssl run build test vet fmt version clean

help: ## Show available commands
	@printf '%s\n' 'Usage: make [TARGET] [VARIABLE=value ...]' ''
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' "$(firstword $(MAKEFILE_LIST))"
	@printf '\n%s\n' 'Example: make ssl run DOMAIN=mail.example.com SMTP_ADDR=127.0.0.1:2525'

ssl: ## Generate a self-signed certificate; preserve an existing pair
	@if [[ -e "$$TLS_KEY_FILE" || -L "$$TLS_KEY_FILE" || -e "$$TLS_CERT_FILE" || -L "$$TLS_CERT_FILE" ]]; then \
		if [[ -f "$$TLS_KEY_FILE" && -f "$$TLS_CERT_FILE" ]]; then \
			printf '%s\n' 'Keeping existing TLS key and certificate.'; \
			exit 0; \
		fi; \
		printf '%s\n' 'TLS output already exists without a complete file pair; use different paths or explicitly remove it.' >&2; \
		exit 1; \
	fi; \
	if [[ "$$TLS_KEY_FILE" == "$$TLS_CERT_FILE" ]]; then \
		printf '%s\n' 'TLS_KEY_FILE and TLS_CERT_FILE must be different paths.' >&2; exit 1; \
	fi; \
	command -v openssl >/dev/null; \
	umask 077; \
	mkdir -p -- "$$(dirname -- "$$TLS_KEY_FILE")" "$$(dirname -- "$$TLS_CERT_FILE")"; \
	tmp="$$(mktemp -d)"; \
	trap 'rm -rf -- "$$tmp"' EXIT; \
	san="DNS:$$SMTP_DOMAIN,IP:127.0.0.1,IP:::1"; \
	if [[ "$$SMTP_DOMAIN" != localhost ]]; then san="$$san,DNS:localhost"; fi; \
	openssl req -x509 -newkey rsa:2048 -nodes -sha256 \
		-days "$$DAYS_VALID" -subj "/CN=$$SMTP_DOMAIN" \
		-addext "subjectAltName=$$san" \
		-addext 'basicConstraints=critical,CA:FALSE' \
		-addext 'keyUsage=critical,digitalSignature,keyEncipherment' \
		-addext 'extendedKeyUsage=serverAuth' \
		-keyout "$$tmp/key.pem" -out "$$tmp/cert.pem"; \
	install -m 600 -- "$$tmp/key.pem" "$$TLS_KEY_FILE"; \
	install -m 644 -- "$$tmp/cert.pem" "$$TLS_CERT_FILE"; \
	printf 'Certificate: %s\nPrivate key: %s\n' "$$TLS_CERT_FILE" "$$TLS_KEY_FILE"

clean-ssl: ## Explicitly delete the selected TLS key and certificate
	rm -f -- "$$TLS_KEY_FILE" "$$TLS_CERT_FILE"

run: ## Run the server using the selected hostname and certificate paths
	go run .

build: ## Build bin/getmail (override BUILD_DIR to change the directory)
	mkdir -p -- "$$BUILD_DIR"
	go build -o "$$BUILD_DIR/getmail" .

test: ## Run tests with the race detector
	go test -race ./...

vet: ## Run Go static analysis
	go vet ./...

fmt: ## Format Go source files
	gofmt -w *.go email service examples

version: ## Print the application version without starting the server
	@go run . --version

clean: ## Delete the built executable; preserve TLS files
	rm -f -- "$$BUILD_DIR/getmail"
