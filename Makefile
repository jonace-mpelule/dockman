SHELL := /bin/sh

BINARY := dockman
DIST := dist
VERSION ?=
GIT_VERSION := $(if $(VERSION),$(VERSION),$(shell git describe --tags --dirty --always 2>/dev/null || echo dev))

# OS/arch matrix for release builds
BUILD_MATRIX := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: build test clean guard-version guard-clean set-version publish

build:
	@mkdir -p $(DIST)
	@echo "Building $(BINARY) version $(GIT_VERSION) for: $(BUILD_MATRIX)"
	@for target in $(BUILD_MATRIX); do \
		OS=$${target%/*}; \
		ARCH=$${target#*/}; \
		EXT=""; \
		if [ "$$OS" = "windows" ]; then EXT=".exe"; fi; \
		OUT="$(DIST)/$(BINARY)-$${OS}-$${ARCH}$${EXT}"; \
		echo "  -> $$OUT"; \
		GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=0 go build -ldflags "-X main.version=$(GIT_VERSION)" -o $$OUT .; \
	done

test:
	@go test ./...

clean:
	@rm -rf $(DIST)

guard-version:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required, e.g. make set-version VERSION=v0.2.0"; exit 1; fi

guard-clean:
	@if ! git diff --quiet; then echo "Working tree is dirty. Commit or stash changes before publishing."; exit 1; fi

set-version: guard-version guard-clean
	@echo "$(VERSION)" > VERSION
	@git add VERSION
	@if git diff --cached --quiet; then echo "VERSION unchanged; aborting."; exit 1; fi
	@git commit -m "chore: release $(VERSION)"
	@echo "Tagging release $(VERSION)"
	@git tag -a $(VERSION) -m "Release $(VERSION)"

publish: guard-version guard-clean
	@echo "Publishing $(VERSION)"
	@git push origin HEAD
	@git push origin $(VERSION)
