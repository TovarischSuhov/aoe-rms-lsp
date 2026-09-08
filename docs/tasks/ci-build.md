# CI: Cross-Platform Build & Release (win/mac/linux)

Status: Done — PR #1 (task/ci-checks), PR #2 (task/ci-release)

## Current State

Repository lives on GitHub (`TovarischSuhov/aoe-rms-lsp`, branch `master`) with **no CI at all** — no `.github/` directory, no workflows. All validation (`goimports`, `go test`, `golangci-lint`) runs manually on the author's Linux machine.

Builds are manual too: a single locally built `aoe2-lsp` binary (linux/amd64, gitignored at repo root). No Windows or macOS binaries exist; users of the LSP server on those platforms must build from source themselves.

Facts affecting the design:

- `go.mod` declares `go 1.26.6`; all deps are pure Go (`go.lsp.dev/*`, `testify`) → `CGO_ENABLED=0` cross-compilation from a single Linux runner is trivial
- No integration build tags in tests → `go test ./...` covers everything
- `cmd/aoe2-lsp/main.go` has no version flag → release binaries cannot report their version
- `cmd/kbgen` is an internal KB codegen tool, not a distribution artifact
- `conventions.md` requires "All validation commands MUST pass in CI" (goimports, tests, race, lint, govulncheck)

## Description

Set up GitHub Actions CI with two workflows:

1. **`ci.yml` — checks** (push to `master` + all PRs): `goimports` check (empty `goimports -l .` output), `go test -race ./...`, `golangci-lint` (pinned version), `govulncheck ./...`
2. **`release.yml` — build & release** (push of tag `v*` only): matrix cross-compile of `cmd/aoe2-lsp` from `ubuntu-latest` with `CGO_ENABLED=0`, version injected via `ldflags -X main.version=<tag>`; targets `windows/amd64` (.zip), `linux/amd64`, `darwin/amd64`, `darwin/arm64` (.tar.gz); archives + `SHA256SUMS` uploaded to a GitHub Release created via `gh release create`

Process rules recorded in project `CLAUDE.md`: each task in its own branch, PR prepared for review; the agent commits and pushes; the user does final review and merge only.

## Scope

**In scope:**
- `.github/workflows/ci.yml` — checks workflow (fmt + test `-race` + lint + vulncheck)
- `.github/workflows/release.yml` — tag-triggered matrix build + GitHub Release
- `cmd/aoe2-lsp/main.go` — `var version = "dev"` + `--version` flag printing the version and exiting
- `.goga/usages/cooks/github-actions.md` — usage file for CI conventions
- `CLAUDE.md` — process rules section (branch-per-task, PR workflow, commit/push delegation)

**Out of scope:**
- `kbgen` in release artifacts (internal tool)
- Package registries (brew/scoop/winget), binary signing/notarization
- Native windows/macos runners (cross-compile from ubuntu instead)
- `linux/arm64` target (matrix extension is trivial if ever needed)
- VS Code extension publishing

## Acceptance Criteria

- PR #1 (subtask A): its own `ci.yml` runs green on its own PR — fmt, `go test -race`, lint, vulncheck jobs all pass; `CLAUDE.md` contains the process rules; usage file created
- PR #2 (subtask B): after merge, pushing a tag `v*` (e.g. `v0.0.1-rc.1`) triggers `release.yml`; the GitHub Release contains 4 archives + `SHA256SUMS`
- Released binaries run on their platforms and `--version` prints the tag (`dev` for local builds)
- `go test ./...` green locally (memory-capped sandbox per `CLAUDE.md`); workflows appear without config errors in the Actions tab

## Stack

- **Frameworks:** GitHub Actions (`ci.yml`, `release.yml`)
- **Libraries:** none (tooling only: `goimports`, `golangci-lint`, `govulncheck`, `gh` CLI — all preinstalled or one-step installs on runners)
- **Infrastructure:** GitHub Actions runners (`ubuntu-latest` only), GitHub Releases

## External Dependencies

| Component      | Usage file                            | Status  |
|----------------|---------------------------------------|---------|
| GitHub Actions | `.goga/usages/cooks/github-actions.md` | created |

## Risks and Constraints

- Workflows are only truly verifiable on GitHub after push; local checks limited to YAML validity (`actionlint` if available). The first real tag may surface follow-up fixes
- `golangci-lint` version on CI must be pinned explicitly — the repo has no `.golangci.yml`, so default config drift between local and CI runs is possible
- Release workflow must be merged to `master` **before** tagging — tag events run the workflow as it exists at the tag's commit
- Public repo → Actions minutes free; if the repo is private, quota applies (2000 min/month free tier)
- `go test -race` roughly doubles CI memory/CPU vs plain test run — acceptable at current codebase size, re-evaluate if it grows

## Scope Estimate

Two subtasks, each in its own branch and PR (user decision, 2026-09-07):

- **A — CI checks** (branch `task/ci-checks`): `ci.yml` + process rules in `CLAUDE.md` + usage file `github-actions.md`. Independently valuable: every later PR gets automated validation
- **B — Release build** (branch `task/ci-release`, after A): `release.yml` + version injection in `cmd/aoe2-lsp/main.go`

## Existing Architecture

No cells affected — pure infrastructure layer (`.github/`, `cmd/`, `.goga/usages/cooks/`, `CLAUDE.md`). Only code touchpoint is `cmd/aoe2-lsp/main.go` (version var), which lives outside the cell tree. No CODEMANIFEST changes; `goga lint` after doc changes.

## Notes

Grooming decisions (2026-09-07):

- Trigger scheme: checks on PR/push; build + release **only** on tag `v*` (user chose "Тег v* → Release")
- Full check set per `conventions.md` incl. `-race` and `govulncheck` (user chose "Полный набор")
- Targets: `windows/amd64`, `linux/amd64`, `darwin/amd64`, `darwin/arm64` (Apple Silicon)
- Single `ubuntu-latest` runner, `CGO_ENABLED=0` cross-compile — no native OS runners
- Archives: `.zip` for Windows, `.tar.gz` for linux/darwin, plus `SHA256SUMS`
- `gh release create` instead of third-party release actions (official/first-party actions only)
- Version injection: `ldflags "-X main.version=<tag>"`; `--version` flag for humans, `dev` default for local builds
- Process: branch per task, agent commits/pushes, user reviews and merges PRs
