BINARY := bridge
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.built=$(BUILT)

.PHONY: build test verify clean

build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/bridge

test:
	go test ./...

verify:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...
	go build ./...

clean:
	rm -f $(BINARY)
