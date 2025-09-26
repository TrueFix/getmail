DOMAIN       := localhost
KEY_FILE     := config/$(DOMAIN).key
CERT_FILE    := config/$(DOMAIN).crt
CONFIG_FILE  := config/openssl-san.cnf
DAYS_VALID   := 365

SHELL := /bin/bash

.PHONY: help ssl clean

help: ## Show this help menu
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@grep --no-filename -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "%-25s %s\n", $$1, $$2}'

ssl: ## Generate a self-signed SSL certificate with SAN for $(DOMAIN)
	@echo "🔐 Generating self-signed SSL certificate for $(DOMAIN)..."

	@echo "📁 Creating config directory if it doesn't exist..."
	mkdir -p config

	@echo "Generating private key..."
	openssl genrsa -out $(KEY_FILE) 2048

	@echo "⚙️ Creating OpenSSL config with SAN..."
	@echo "[req]" > $(CONFIG_FILE)
	@echo "default_bits = 2048" >> $(CONFIG_FILE)
	@echo "prompt = no" >> $(CONFIG_FILE)
	@echo "default_md = sha256" >> $(CONFIG_FILE)
	@echo "req_extensions = req_ext" >> $(CONFIG_FILE)
	@echo "distinguished_name = dn" >> $(CONFIG_FILE)
	@echo "" >> $(CONFIG_FILE)
	@echo "[dn]" >> $(CONFIG_FILE)
	@echo "CN = $(DOMAIN)" >> $(CONFIG_FILE)
	@echo "" >> $(CONFIG_FILE)
	@echo "[req_ext]" >> $(CONFIG_FILE)
	@echo "subjectAltName = @alt_names" >> $(CONFIG_FILE)
	@echo "" >> $(CONFIG_FILE)
	@echo "[alt_names]" >> $(CONFIG_FILE)
	@echo "DNS.1 = $(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.2 = www.$(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.3 = mail.$(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.4 = smtp.$(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.5 = imap.$(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.6 = pop3.$(DOMAIN)" >> $(CONFIG_FILE)
	@echo "DNS.7 = localhost" >> $(CONFIG_FILE)

	@echo "Creating self-signed certificate..."
	openssl req -x509 -days $(DAYS_VALID) \
		-key $(KEY_FILE) \
		-out $(CERT_FILE) \
		-subj "/CN=$(DOMAIN)" \
		-config $(CONFIG_FILE) \
		-extensions req_ext
	
	@echo "✅ Self-signed certificate and key generated:"
	@echo "   - Certificate: $(CERT_FILE)"
	@echo "   - Key:         $(KEY_FILE)"


clean: ## Clean up generated files
	@echo "🧹 Cleaning generated files..."
	rm -f $(KEY_FILE) $(CERT_FILE) $(CONFIG_FILE)
	@echo "Done."