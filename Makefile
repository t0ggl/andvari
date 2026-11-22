BIN_DIR ?= bin
TARGETOS ?= $(GOOS)
TARGETARCH ?= $(GOARCH)
LDFLAGS := -w -s

.PHONY: test sast test-backend install-frontend build build-frontend build-backend

test: sast
	$(MAKE) build-frontend test-backend

sast:
	./scripts/sast.sh

test-backend:
	go vet ./...
	go test -v -cover ./...

install-frontend:
	npm --prefix web ci

build: build-frontend
	$(MAKE) build-backend

build-frontend: install-frontend
	npm --prefix web run build

build-backend:
	CGO_ENABLED=0 GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/andvari ./cmd/andvari
