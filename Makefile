.PHONY: build run test test-race tidy fmt vet psql migrate migrate-test clean

# On the VPS use the full path — the `go` in PATH is 1.18.
GO ?= go

# Load DATABASE_URL etc from .env before running any make target:
#   set -a && source .env && set +a && make run

build:
	$(GO) build -o bin/sms-server ./cmd/server
	$(GO) build -o bin/smsctl ./cmd/smsctl

run: build
	./bin/sms-server

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test -race ./... -count=1

migrate:
	$(GO) run ./cmd/smsctl migrate up

migrate-test:
	DATABASE_URL="$${DATABASE_URL_TEST}" $(GO) run ./cmd/smsctl migrate up

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

psql:
	psql "$${DATABASE_URL}"

clean:
	rm -rf bin/
