SHELL := /bin/sh

APP := picoclip
BIND ?= 0.0.0.0
PORT ?= 8080
BASE_URL ?= http://127.0.0.1:$(PORT)
TMP_DIR := tmp
DEV_BIN := $(TMP_DIR)/$(APP)
AIR := ./bin/air
TEMPL := ./bin/templ

.PHONY: help tools templ-generate build build-dev run dev test test-go test-e2e test-e2e-headed vet fmt lint check clean kill-8080

help:
	@printf '%s\n' \
	  'PicoClip development commands:' \
	  '' \
	  '  make tools          Install local dev tools: templ and air' \
	  '  make templ-generate Run templ generate (safe when no .templ files exist)' \
	  '  make run            Run app with go run' \
	  '  make dev            Run app with air live reload' \
	  '  make build          Build ./picoclip' \
	  '  make test-go        Run Go tests' \
	  '  make test-e2e       Run Playwright E2E tests' \
	  '  make check          Run full validation' \
	  '  make kill-8080      Kill process bound to port 8080'

tools:
	go install github.com/a-h/templ/cmd/templ@v0.3.1020
	go install github.com/air-verse/air@latest
	printf '%s\n' '#!/bin/sh' 'exec "$$(go env GOPATH)/bin/templ" "$$@"' > bin/templ
	printf '%s\n' '#!/bin/sh' 'exec "$$(go env GOPATH)/bin/air" "$$@"' > bin/air
	chmod +x bin/templ bin/air

templ-generate:
	$(TEMPL) generate

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

lint: vet

build:
	go build -o $(APP) cmd/picoclip/main.go

build-dev:
	mkdir -p $(TMP_DIR)
	go build -o $(DEV_BIN) cmd/picoclip/main.go

run:
	BIND=$(BIND) PORT=$(PORT) go run cmd/picoclip/main.go

dev:
	BIND=$(BIND) PORT=$(PORT) $(AIR) -c .air.toml

test-go:
	go test ./...

test-e2e:
	BASE_URL=$(BASE_URL) ./scripts/run-e2e.sh

test-e2e-headed:
	BASE_URL=$(BASE_URL) ./scripts/run-e2e.sh --headed

test: test-go

check: templ-generate fmt test-go vet build test-e2e

kill-8080:
	-fuser -k 8080/tcp

clean:
	rm -rf $(TMP_DIR) $(APP) e2e/test-results e2e/playwright-report
