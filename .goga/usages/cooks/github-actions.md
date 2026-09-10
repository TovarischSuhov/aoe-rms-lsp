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
          version: v2.13.2                    # v-prefix required by the action; keep in sync with local dev
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

A single job building the targets sequentially is an accepted variant for
small modules — it produces the same archives and avoids artifact passing
between matrix jobs (this repo's release.yml uses it; add `-trimpath` for
reproducible builds).

The release job also builds the VS Code extension package (see
`vscode-extension.md`): node 22 → `npm install` → `npm run check` →
`npm run package`, and the resulting `.vsix` lands in `dist/` together
with the platform archives — one `SHA256SUMS` and one
`gh release create` cover everything.

The tag itself is put by `scripts/release.sh` (`make release`): it computes
the next version, prepends the conventional-commit changelog to
`CHANGELOG.md`, bumps `version` in `editors/vscode/package.json` to the
same tag, commits and pushes `master` + tag together, so the workflow
always runs at a commit that carries its own changelog entry and the
extension version matching the release.

Upload archives + a combined `SHA256SUMS` to the release created with
`gh release create "$GITHUB_REF_NAME" --generate-notes`.

## Pattern: post-publish asset verification

A published release is not the same as a downloadable one — assets can
keep answering 404 for a while after `gh release create` returns (observed
in issue #68: the atom feed listed the release while the asset endpoint
was still missing). The atom feed is never the source of truth.

After creating the release, the workflow HEADs every asset URL
anonymously (no auth headers) and retries until it gets HTTP 200 within
a bounded budget; running out of retries fails the job. Keep the total
retry budget inside the job's `timeout-minutes` so the check cannot hang
a release forever, and do not treat transient non-200s as success.

## Pattern: packaged-artifact contributes check

`vsce package` does not validate that paths referenced from the
extension's `contributes` (`grammars[].path`, `languages[].configuration`)
exist inside the archive, and VS Code silently ignores broken paths —
the extension installs fine with dead references (issue #68 §3).

The CI `vscode` job therefore unpacks the built `.vsix` after packaging
and verifies every contributed path against the actual archive listing.
Read the paths from `package.json` at check time — never duplicate them
as a hardcoded list in the step, or the check drifts away from the thing
it verifies. A missing file fails the job.

## Local Verification

- Workflows cannot run locally — the first push of a new workflow is
  its first real test. Keep steps identical to local commands:
  `goimports -l .`, `go test -race ./...`, `golangci-lint run`,
  `govulncheck ./...`.
- Before pushing, confirm locally: clean `goimports -l .`, green tests
  (memory-capped sandbox), `golangci-lint run` at the pinned version.
