BIN_DIR ?= bin
VERSION ?= dev
export GOOS ?= $(TARGETOS)
export GOARCH ?= $(TARGETARCH)
LDFLAGS := -w -s -X github.com/t0ggl/andvari/internal/config.Version=$(VERSION)

.PHONY: test sast test-backend install-frontend install-backend build build-frontend build-backend

test: sast
	$(MAKE) install-frontend build-frontend install-backend test-backend

sast:
	./scripts/sast.sh

test-backend:
	go vet ./...
	go test -v -cover ./...

install-frontend:
	npm --prefix web ci

install-backend:
	go mod download

build: install-frontend install-backend
	$(MAKE) build-frontend
	$(MAKE) build-backend

build-frontend:
	npm --prefix web run build

build-backend:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/andvari ./cmd/andvari
