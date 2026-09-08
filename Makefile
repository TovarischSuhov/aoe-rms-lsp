.PHONY: all build test lint fmt check clean release

all: build

build:
	go build ./...

# Race detector runs in CI; locally tests MUST stay under the memory cap
# (a runaway test once OOM'd the machine).
test:
	timeout 300 systemd-run --user --scope -p MemoryMax=1500M -p MemorySwapMax=0 \
		bash -c 'go test ./... -count=1'

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

# CLAUDE.md pre-completion sequence: fmt → build → memory-cap test → lint → goga lint
check: fmt build test lint
	goga lint

clean:
	rm -rf dist

# Cut a release: make release BUMP=patch|minor|major (or VERSION=vX.Y.Z).
# Generates CHANGELOG.md, commits, tags and pushes; CI builds the binaries.
release:
	./scripts/release.sh $(if $(VERSION),$(VERSION),$(BUMP))
