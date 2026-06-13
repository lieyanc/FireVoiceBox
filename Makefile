# FireVoiceBox — single-binary voice inbox.
# `make build` produces ./bin/firevoicebox with the React UI embedded.

BINARY := bin/firevoicebox
PKG := ./cmd/firevoicebox
WEB := web
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
VERSION_PKG := github.com/lieyan666/firevoicebox/internal/version
LD_VERSION_FLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)

.PHONY: all build web backend run dev test clean tidy release

all: build

## build: build the frontend then the single Go binary (UI embedded)
build: web backend

## web: install deps (if needed) and build the React app into internal/web/dist
web:
	cd $(WEB) && pnpm install --frozen-lockfile || pnpm install
	cd $(WEB) && pnpm build

## backend: compile the Go binary (assumes internal/web/dist is present)
backend:
	go build -ldflags="$(LD_VERSION_FLAGS)" -o $(BINARY) $(PKG)

## run: build everything and run it
run: build
	./$(BINARY)

## dev: run backend (:8080) and Vite dev server (:5173 with /api proxy) together.
## Run `make backend && ./bin/firevoicebox` in one shell and `make web-dev` in another,
## or use this target which backgrounds the API.
dev:
	go run $(PKG) & \
	cd $(WEB) && pnpm dev

## web-dev: start only the Vite dev server (proxies /api to :8080)
web-dev:
	cd $(WEB) && pnpm dev

## test: run Go tests
test:
	go test ./...

## tidy: tidy go modules
tidy:
	go mod tidy

## release: cross-compile a static linux/amd64 binary for server deployment
release: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w $(LD_VERSION_FLAGS)" -o $(BINARY)-linux-amd64 $(PKG)

## clean: remove build artifacts
clean:
	rm -rf bin
	rm -rf internal/web/dist/assets
