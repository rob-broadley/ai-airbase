BINARY         := marshal
DEV_IMAGE      := sapper
GOMOD_CACHE    := marshal-gomod-cache
GOBUILD_CACHE  := marshal-gobuild-cache
IMAGE          := revetment
VERSION        := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION       := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
CREATED        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_ARGS     := --build-arg VERSION=$(VERSION) \
                  --build-arg REVISION=$(REVISION) \
                  --build-arg CREATED=$(CREATED)

INSTALL_DIR := $(HOME)/.local/bin

# When running inside a container (/.containerenv set by Podman, /.dockerenv by Docker),
# tools are available directly. Otherwise delegate to the dev container on the host.
ifneq (,$(or $(wildcard /.containerenv),$(wildcard /.dockerenv)))
RUN :=
GORUN = sh -c 'cd $(CURDIR)/marshal && "$$@"' --
else
RUN := podman run --rm \
    -v $(CURDIR):/workspace:Z \
    -v $(GOMOD_CACHE):/root/go/pkg/mod \
    -v $(GOBUILD_CACHE):/root/.cache/go-build \
    $(DEV_IMAGE)
GORUN = $(RUN) sh -c 'cd /workspace/marshal && "$$@"' --
endif

.DEFAULT_GOAL := build

.PHONY: dev-image image build test coverage fmt fmt-check fmt-md fmt-md-check vet lint check tidy install clean cache-clean

dev-image:
	podman build $(BUILD_ARGS) -t $(DEV_IMAGE) -f dev/Containerfile .

image:
	podman build $(BUILD_ARGS) -t $(IMAGE) -f revetment/Containerfile revetment/

build:
	mkdir -p bin
	$(GORUN) go build -ldflags "-X main.version=$(VERSION)" -o ../bin/$(BINARY) ./cmd

test:
	$(GORUN) go test -race ./...

coverage:
	$(GORUN) sh -c 'go test -race -coverprofile=../coverage.txt ./... && go tool cover -html=../coverage.txt -o ../coverage.html'

fmt:
	$(GORUN) goimports -w -local github.com/rob-broadley/ai-airbase/marshal .

fmt-check:
	@$(GORUN) sh -c 'out=$$(gofmt -l .); [ -z "$$out" ] || { printf "unformatted:\n%s\n" "$$out"; exit 1; }'

fmt-md:
	$(RUN) mdformat --wrap keep .

fmt-md-check:
	$(RUN) mdformat --check --wrap keep .

vet:
	$(GORUN) go vet ./...

lint:
	$(GORUN) golangci-lint run

check: fmt-check fmt-md-check vet lint

tidy:
	$(GORUN) go mod tidy

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
