.PHONY: all build test lint fmt check corpus corpus-official clean release

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
# maps (see scripts/corpus-fetch.sh). The fetch runs when the sample is
# missing or predates the current pin (the recorded .sources-hash); CI
# caches .corpus by the sources hash.
corpus: build
	@if scripts/corpus-fetch.sh --check .corpus; then \
		echo "corpus: sample on disk matches the pin ($$(find .corpus -type f | wc -l) files), skipping fetch"; \
	else \
		rm -rf .corpus && scripts/corpus-fetch.sh .corpus; \
	fi
	go run ./cmd/corpus -bin aoe2-lsp -dir .corpus

# Local-only corpus built from the installed game's own scripts. The
# content is Microsoft-copyrighted: it stays in gitignored .corpus-official
# and is never committed (see scripts/corpus-official.sh).
corpus-official:
	go build -o aoe2-lsp ./cmd/aoe2-lsp
	scripts/corpus-official.sh .corpus-official
	go run ./cmd/corpus -bin ./aoe2-lsp -dir .corpus-official

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
