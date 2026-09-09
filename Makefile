.PHONY: all build test lint fmt check corpus clean release

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

# Corpus gate: run the LSP binary over a deterministic sample of real
# maps (see scripts/corpus-fetch.sh). The fetch is skipped when the
# sample is already on disk — CI caches .corpus by the sources hash.
corpus: build
	@if [ "$$(find .corpus -type f 2>/dev/null | wc -l)" -ge "$${CORPUS_COUNT:-100}" ]; then \
		echo "corpus: sample on disk ($$(find .corpus -type f | wc -l) files), skipping fetch"; \
	else \
		scripts/corpus-fetch.sh .corpus; \
	fi
	go run ./cmd/corpus -bin aoe2-lsp -dir .corpus

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
