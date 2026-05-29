GO=/usr/local/go/bin/go

.PHONY: build test run

build:
	$(GO) build -o dehydrator ./cmd/server

test:
	$(GO) test ./...

run:
	$(GO) run ./cmd/server
