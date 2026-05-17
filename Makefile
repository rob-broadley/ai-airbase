BINARY         := marshal
DEV_IMAGE      := sapper
GOMOD_CACHE    := marshal-gomod-cache
GOBUILD_CACHE  := marshal-gobuild-cache
IMAGE          := revetment
VERSION        := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION       := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
COPILOT_VERSION   ?= $(shell sed -n '1p' .copilot-version)
COPILOT_INTEGRITY ?= $(shell sed -n '2p' .copilot-version)
BUILD_ARGS        := --build-arg VERSION=$(VERSION) \
                     --build-arg REVISION=$(REVISION) \
                     --build-arg COPILOT_VERSION=$(COPILOT_VERSION) \
                     --build-arg COPILOT_INTEGRITY=$(COPILOT_INTEGRITY)

INSTALL_DIR := $(HOME)/.local/bin

# Inside a container the `container` env var is set by Podman and systemd
# container runtimes. Run tools directly; otherwise delegate to the dev image.
ifneq (,$(container))
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

.PHONY: dev-image image build test coverage fmt fmt-check fmt-md fmt-md-check vet lint check tidy install clean cache-clean update-copilot-version

dev-image:
	podman build $(BUILD_ARGS) -t $(DEV_IMAGE) -f dev/Containerfile .

update-copilot-version:
	@version="$$(curl -sf https://registry.npmjs.org/@github/copilot/latest | jq -r .version)" && \
	integrity="$$(curl -sf https://registry.npmjs.org/@github/copilot-linux-x64/$$version | jq -r .dist.integrity)" && \
	printf "%s\n%s\n" "$$version" "$$integrity" > .copilot-version && \
	echo "COPILOT_VERSION updated to $$version" && \
	echo "COPILOT_INTEGRITY updated to $$integrity"

image:
	podman build $(BUILD_ARGS) -t $(IMAGE) -f revetment/Containerfile .

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
