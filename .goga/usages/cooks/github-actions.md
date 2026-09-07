# GitHub Actions

Conventions for CI workflows in this repository (`.github/workflows/`).
Audience: anyone adding or modifying workflows — the CI must stay
deterministic, minimal, and consistent with local validation commands
from `conventions.md`.

## Workflow Rules

- First-party actions only: `actions/checkout`, `actions/setup-go`,
  `golangci/golangci-lint-action`. No third-party release actions —
  the `gh` CLI (preinstalled on runners) creates releases.
- Pin tool versions explicitly: `golangci-lint` version in the action
  step must match the local dev version; Go comes from
  `go-version-file: go.mod` (never hardcode a Go version).
- Declare least privilege: `permissions: contents: read` for checks;
  release workflow adds `contents: write`.
- Set `concurrency` with `cancel-in-progress` so superseded PR runs
  do not burn minutes.
- Set `timeout-minutes` on every job.
- Cross-compile from `ubuntu-latest` only — deps are pure Go, so
  `CGO_ENABLED=0` needs no native OS runners.

## Pattern: checks workflow (ci.yml)

Triggers: push to `master` + all pull requests. Single job reusing the
module cache runs the full `conventions.md` validation set.

```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  checks:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: |
          go install golang.org/x/tools/cmd/goimports@latest
          test -z "$(goimports -l .)"        # fail if any file needs formatting
      - run: go test -race ./...
      - uses: golangci/golangci-lint-action@v8
        with:
          version: 2.13.2                     # keep in sync with local dev
      - run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

## Pattern: release build (release.yml)

Trigger: push of tag `v*` only. Matrix cross-compile of `cmd/aoe2-lsp`,
version injected via ldflags, archives plus checksums attached to a
GitHub Release. The release workflow must be on `master` before
tagging — tag events run the workflow at the tag's commit.

```yaml
on:
  push:
    tags: ['v*']
```

Matrix targets: `windows/amd64` (`.zip`), `linux/amd64`,
`darwin/amd64`, `darwin/arm64` (`.tar.gz`). Build step shape:

```bash
CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
  -ldflags "-X main.version=${GITHUB_REF_NAME}" \
  -o "aoe2-lsp-${GOOS}-${GOARCH}${EXT}" ./cmd/aoe2-lsp
```

Upload archives + a combined `SHA256SUMS` to the release created with
`gh release create "$GITHUB_REF_NAME" --generate-notes`.

## Local Verification

- Workflows cannot run locally — the first push of a new workflow is
  its first real test. Keep steps identical to local commands:
  `goimports -l .`, `go test -race ./...`, `golangci-lint run`,
  `govulncheck ./...`.
- Before pushing, confirm locally: clean `goimports -l .`, green tests
  (memory-capped sandbox), `golangci-lint run` at the pinned version.
