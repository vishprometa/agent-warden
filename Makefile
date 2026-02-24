.PHONY: all build dashboard binary dev test lint docker clean install

# Version info from git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)

BINARY  := agentwarden
GO      := CGO_ENABLED=1 go

# ─── Targets ────────────────────────────────────────────────────────────────

all: clean lint test build

build: dashboard binary

dashboard:
	cd dashboard && npm install --no-audit --no-fund && npm run build
	rm -rf internal/dashboard/dist
	cp -r dashboard/dist internal/dashboard/dist

binary:
	$(GO) build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/agentwarden

dev:
	$(GO) run -ldflags="$(LDFLAGS)" ./cmd/agentwarden start --dev

test:
	$(GO) test ./...

lint:
	golangci-lint run ./...

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t agentwarden:$(VERSION) .

clean:
	rm -rf bin/
	rm -rf internal/dashboard/dist
	rm -rf dashboard/dist
	rm -f $(BINARY)

install: build
	$(GO) install -ldflags="$(LDFLAGS)" ./cmd/agentwarden
