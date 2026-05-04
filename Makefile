BINARY         := marshal
DEV_IMAGE      := sapper
GOMOD_CACHE    := marshal-gomod-cache
GOBUILD_CACHE  := marshal-gobuild-cache

GO := podman run --rm \
    -v $(CURDIR):/workspace:Z \
    -v $(GOMOD_CACHE):/root/go/pkg/mod \
    -v $(GOBUILD_CACHE):/root/.cache/go-build \
    -w /workspace/marshal \
    $(DEV_IMAGE)
IMAGE         := revetment
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION      := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
CREATED       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_ARGS    := --build-arg VERSION=$(VERSION) \
                 --build-arg REVISION=$(REVISION) \
                 --build-arg CREATED=$(CREATED)

INSTALL_DIR := $(HOME)/.local/bin

.DEFAULT_GOAL := build

.PHONY: dev-image image build test coverage fmt fmt-check fmt-md fmt-md-check vet lint check tidy install clean cache-clean

dev-image:
	podman build $(BUILD_ARGS) -t $(DEV_IMAGE) -f dev/Containerfile .

image:
	podman build $(BUILD_ARGS) -t $(IMAGE) -f revetment/Containerfile revetment/

build:
	mkdir -p bin
	$(GO) go build -ldflags "-X main.version=$(VERSION)" -o ../bin/$(BINARY) ./cmd

test:
	$(GO) go test -race ./...

coverage:
	$(GO) sh -c 'go test -race -coverprofile=../coverage.txt ./... && go tool cover -html=../coverage.txt -o ../coverage.html'

fmt:
	$(GO) goimports -w -local github.com/rob-broadley/ai-airbase/marshal .

fmt-check:
	@$(GO) sh -c 'out=$$(gofmt -l .); [ -z "$$out" ] || { printf "unformatted:\n%s\n" "$$out"; exit 1; }'

fmt-md:
	$(GO) mdformat --wrap keep /workspace

fmt-md-check:
	$(GO) mdformat --check --wrap keep /workspace

vet:
	$(GO) go vet ./...

lint:
	$(GO) golangci-lint run

check: fmt-check fmt-md-check vet lint

tidy:
	$(GO) go mod tidy

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"
	@echo "$(PATH)" | grep -q "$(INSTALL_DIR)" || echo "WARNING: $(INSTALL_DIR) is not on your PATH"

clean:
	# Note: Go module and build caches are in named Podman volumes — run 'make cache-clean' to remove them.
	rm -f bin/$(BINARY) coverage.txt coverage.html

cache-clean:
	podman volume rm -f $(GOMOD_CACHE) $(GOBUILD_CACHE)
