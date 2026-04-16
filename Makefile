.DEFAULT_GOAL := help

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
BUILD_TAGS ?= json1 fts5
SERVER_BIN ?= mahresources
CLI_BIN ?= mr

DEFAULT_BUILD_TAGS := json1 fts5

.PHONY: help bootstrap bootstrap-e2e build build-css build-js build-server cli \
	install install-server install-cli docs docs-cli docs-lint docs-site docs-serve \
	docs-fresh test test-go test-ui test-e2e test-cli-e2e test-cli-doctest \
	test-a11y test-e2e-all lint ci openapi openapi-validate dev clean

help:
	@printf '%s\n' \
		'Usage: make <target> [VAR=value]' \
		'' \
		'Common targets:' \
		'  help               Show this help message' \
		'  bootstrap          Install root, docs-site, and e2e npm dependencies' \
		'  bootstrap-e2e      Run bootstrap and install Playwright Chromium for e2e' \
		'  build              Build the app (CSS + JS + Go server binary)' \
		'  build-css          Build Tailwind CSS' \
		'  build-js           Build the Vite bundle' \
		'  build-server       Build the Go server binary with BUILD_TAGS' \
		'  cli                Build the mr CLI binary' \
		'  install            Install both binaries into BINDIR' \
		'  install-server     Install the server binary into BINDIR' \
		'  install-cli        Install the CLI binary into BINDIR' \
		'  docs               Regenerate CLI docs and build the docs site' \
		'  docs-cli           Regenerate docs-site/docs/cli/ from mr help text' \
		'  docs-lint          Validate embedded mr help text' \
		'  docs-site          Build the Docusaurus docs site' \
		'  docs-serve         Build and serve the Docusaurus docs site locally' \
		'  docs-fresh         Regenerate CLI docs and fail if committed output is stale' \
		'  test               Run the fast local checks (Go tests + frontend unit tests)' \
		'  test-go            Run Go tests with BUILD_TAGS' \
		'  test-ui            Run frontend unit tests' \
		'  test-e2e           Run browser e2e tests against an ephemeral server' \
		'  test-cli-e2e       Run CLI e2e tests against an ephemeral server' \
		'  test-cli-doctest   Run CLI doctests against an ephemeral server' \
		'  test-a11y          Run accessibility e2e tests against an ephemeral server' \
		'  test-e2e-all       Run browser and CLI e2e suites in parallel' \
		'  lint               Run staticcheck (requires staticcheck in PATH)' \
		'  ci                 Run the main local CI checks' \
		'  openapi            Generate openapi.yaml from code' \
		'  openapi-validate   Validate openapi.yaml' \
		'  dev                Run the existing watch workflow (requires CompileDaemon)' \
		'  clean              Remove local binaries and transient test/docs artifacts' \
		'' \
		'Variables (override like: make install BINDIR=/tmp/bin):' \
		'  PREFIX=$(PREFIX)' \
		'  BINDIR=$(BINDIR)' \
		'  DESTDIR=$(DESTDIR)' \
		'  BUILD_TAGS=$(BUILD_TAGS)' \
		'  SERVER_BIN=$(SERVER_BIN)' \
		'  CLI_BIN=$(CLI_BIN)'

bootstrap:
	npm ci
	cd docs-site && npm ci
	cd e2e && npm ci

bootstrap-e2e: bootstrap
	cd e2e && npx playwright install chromium

build:
	@set -e; \
	if [ "$(SERVER_BIN)" = "mahresources" ] && [ "$(strip $(BUILD_TAGS))" = "$(DEFAULT_BUILD_TAGS)" ]; then \
		npm run build; \
	else \
		$(MAKE) build-css; \
		$(MAKE) build-js; \
		$(MAKE) build-server; \
	fi

build-css:
	npm run build-css

build-js:
	npm run build-js

build-server:
	@mkdir -p "$(dir $(SERVER_BIN))"
	go build --tags '$(BUILD_TAGS)' -o "$(SERVER_BIN)" .

cli:
	@set -e; \
	if [ "$(CLI_BIN)" = "mr" ]; then \
		npm run build-cli; \
	else \
		mkdir -p "$(dir $(CLI_BIN))"; \
		go build -o "$(CLI_BIN)" ./cmd/mr/; \
	fi

install:
	@set -e; \
	$(MAKE) install-server; \
	$(MAKE) install-cli

install-server: build-server
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 "$(SERVER_BIN)" "$(DESTDIR)$(BINDIR)/$(notdir $(SERVER_BIN))"

install-cli: cli
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 "$(CLI_BIN)" "$(DESTDIR)$(BINDIR)/$(notdir $(CLI_BIN))"

docs:
	@set -e; \
	$(MAKE) docs-cli; \
	$(MAKE) docs-site

docs-cli: cli
	@set -e; \
	if [ "$(CLI_BIN)" = "mr" ]; then \
		npm run docs-gen; \
	else \
		cli_path="$(CLI_BIN)"; \
		case "$$cli_path" in \
			*/*) ;; \
			*) cli_path="./$$cli_path" ;; \
		esac; \
		"$$cli_path" docs dump --format markdown --output docs-site/docs/cli/; \
	fi

docs-lint: cli
	@set -e; \
	if [ "$(CLI_BIN)" = "mr" ]; then \
		npm run docs-lint; \
	else \
		cli_path="$(CLI_BIN)"; \
		case "$$cli_path" in \
			*/*) ;; \
			*) cli_path="./$$cli_path" ;; \
		esac; \
		"$$cli_path" docs lint; \
	fi

docs-site:
	cd docs-site && npm run build

docs-serve:
	@set -e; \
	$(MAKE) docs-site; \
	cd docs-site && npm run serve

docs-fresh:
	@set -e; \
	$(MAKE) docs-cli; \
	if ! git diff --quiet -- docs-site/docs/cli/; then \
		echo "docs-site/docs/cli/ is out of sync with CLI help text."; \
		echo "Run 'make docs-cli' locally and commit the result."; \
		git diff --stat -- docs-site/docs/cli/; \
		exit 1; \
	fi

test:
	@set -e; \
	$(MAKE) test-go; \
	$(MAKE) test-ui

test-go:
	go test --tags '$(BUILD_TAGS)' ./...

test-ui:
	npm run test:unit

test-e2e:
	cd e2e && npm run test:with-server

test-cli-e2e:
	cd e2e && npm run test:with-server:cli

test-cli-doctest:
	cd e2e && npm run test:with-server:cli-doctest

test-a11y:
	cd e2e && npm run test:with-server:a11y

test-e2e-all:
	cd e2e && npm run test:with-server:all

lint:
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck is required for 'make lint'."; \
		echo "Install it with: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi

ci:
	@set -e; \
	$(MAKE) test-go; \
	$(MAKE) lint; \
	$(MAKE) docs-lint; \
	$(MAKE) docs-fresh; \
	$(MAKE) test-cli-doctest

openapi:
	go run ./cmd/openapi-gen

openapi-validate:
	go run ./cmd/openapi-gen/validate.go openapi.yaml

dev:
	@if command -v CompileDaemon >/dev/null 2>&1; then \
		npm run watch; \
	else \
		echo "CompileDaemon is required for 'make dev'."; \
		echo "Install it first and re-run the target."; \
		exit 1; \
	fi

clean:
	rm -f -- "$(SERVER_BIN)" "$(CLI_BIN)"
	rm -rf -- docs-site/build docs-site/.docusaurus test-results \
		e2e/playwright-report e2e/test-results e2e/.playwright
