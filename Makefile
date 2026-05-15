BINARY      := legendaryos-builder
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w \
               -X main.Version=$(VERSION) \
               -X main.Commit=$(COMMIT) \
               -X main.BuildDate=$(BUILD_DATE)
GOFLAGS     := -trimpath
DIST_DIR    := dist

.PHONY: all build release install clean test fmt vet tidy help

all: build

# ── Development build ─────────────────────────────────────────────────────────
build:
	@printf "  \033[96m⬡\033[0m  Building \033[97;1m$(BINARY)\033[0m $(VERSION) ...\n"
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .
	@printf "  \033[92m✓\033[0m  $(BINARY) ready\n"

# ── Release build — linux/amd64 ───────────────────────────────────────────────
release: fmt vet
	@printf "  \033[96m⬡\033[0m  Building release binary (linux/amd64) ...\n"
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) .
	@cd $(DIST_DIR) && tar -czf $(BINARY)-linux-amd64.tar.gz $(BINARY)
	@cd $(DIST_DIR) && sha256sum $(BINARY)-linux-amd64.tar.gz > checksums.sha256
	@printf "  \033[92m✓\033[0m  $(DIST_DIR)/$(BINARY)-linux-amd64.tar.gz\n"
	@cat $(DIST_DIR)/checksums.sha256

# ── Install to /usr/local/bin ─────────────────────────────────────────────────
install: build
	@printf "  \033[96m⬡\033[0m  Installing to /usr/local/bin/$(BINARY) ...\n"
	@sudo install -m 0755 $(BINARY) /usr/local/bin/$(BINARY)
	@printf "  \033[92m✓\033[0m  Installed\n"

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test ./... -v -count=1

clean:
	@printf "  \033[96m⬡\033[0m  Cleaning ...\n"
	@rm -f $(BINARY)
	@rm -rf $(DIST_DIR)
	@printf "  \033[92m✓\033[0m  Clean\n"

help:
	@printf "\n  \033[96;1m⬡ LegendaryOS Builder — Makefile\033[0m\n\n"
	@printf "  \033[97;1mTargets:\033[0m\n"
	@printf "    \033[96mmake\033[0m             build legendaryos-builder binary\n"
	@printf "    \033[96mmake release\033[0m     build dist/legendaryos-builder-linux-amd64.tar.gz\n"
	@printf "    \033[96mmake install\033[0m     sudo install to /usr/local/bin\n"
	@printf "    \033[96mmake fmt\033[0m         go fmt\n"
	@printf "    \033[96mmake vet\033[0m         go vet\n"
	@printf "    \033[96mmake tidy\033[0m        go mod tidy\n"
	@printf "    \033[96mmake test\033[0m        run tests\n"
	@printf "    \033[96mmake clean\033[0m       remove binary and dist/\n\n"
