#!/usr/bin/env bash
# Cut a release: compute the next version, generate a changelog section from
# conventional commits since the last tag, prepend it to CHANGELOG.md, commit,
# tag and push. The pushed v* tag triggers .github/workflows/release.yml,
# which builds the binaries and creates the GitHub Release.
#
# Usage: scripts/release.sh <major|minor|patch|vX.Y.Z> [--dry-run]
#   major|minor|patch — bump the last tag (prerelease suffix is dropped first:
#     v0.1.0-rc.1 + patch → v0.1.1)
#   vX.Y.Z — explicit version (e.g. v0.1.0 to finalize an rc)
#   --dry-run — print the version and the changelog, change nothing
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)" || {
	printf 'release: not inside a git repository\n' >&2
	exit 1
}
cd "$REPO_ROOT"

die() { printf 'release: %s\n' "$*" >&2; exit 1; }

usage() {
	printf 'Usage: scripts/release.sh <major|minor|patch|vX.Y.Z> [--dry-run]\n'
	exit 1
}

spec=""
dry_run=0
for arg in "$@"; do
	case "$arg" in
	--dry-run) dry_run=1 ;;
	-*) usage ;;
	*) [ -n "$spec" ] && usage; spec="$arg" ;;
	esac
done
[ -n "$spec" ] || usage

branch="$(git branch --show-current)"
[ "$branch" = "master" ] || die "not on master (currently: ${branch:-detached})"

if ! { git diff --quiet && git diff --cached --quiet; }; then
	die "working tree is not clean"
fi

git fetch --quiet origin master
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/master)" ] ||
	die "master is not in sync with origin/master — pull/rebase first"

last_tag="$(git describe --abbrev=0 --tags --match 'v*' 2>/dev/null || true)"

# next_version <bump|explicit>
next_version() {
	local base minor patch
	if [[ "$spec" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		printf '%s\n' "${spec#v}"
		return
	fi
	base="${last_tag#v}"
	base="${base%%-*}" # drop prerelease suffix: v0.1.0-rc.1 → 0.1.0
	base="${base:-0.0.0}"
	major="${base%%.*}"
	minor="$(printf '%s' "$base" | cut -d. -f2)"
	patch="${base##*.}"
	case "$spec" in
	major) printf '%d.0.0\n' "$((major + 1))" ;;
	minor) printf '%d.%d.0\n' "$major" "$((minor + 1))" ;;
	patch) printf '%d.%d.%d\n' "$major" "$minor" "$((patch + 1))" ;;
	*) usage ;;
	esac
}

version="$(next_version)"
tag="v$version"
[ -z "$(git tag -l "$tag")" ] || die "tag $tag already exists"

# changelog — grouped conventional-commit subjects since the last tag.
changelog() {
	local range="${last_tag:+$last_tag..}HEAD"
	local subjects subject
	subjects="$(git log --format='%s' "$range" | grep -v -E '^Merge (pull request|branch|remote)')"

	local -A order=( [feat]=0 [fix]=1 [perf]=2 [refactor]=3 [test]=4 [ci]=5 [docs]=6 [chore]=7 )
	local -A seen
	local -A groups
	local max=-1 key rank
	while IFS= read -r subject; do
		key="$(printf '%s' "$subject" | cut -d: -f1)"
		case "$key" in
		feat|fix|perf|refactor|test|ci|docs|chore) ;;
		*) key="chore" ;;
		esac
		groups[$key]+="$subject"$'\n'
		rank="${order[$key]}"
		[ "$rank" -gt "$max" ] && max="$rank"
		seen[$key]="$rank"
	done <<<"$subjects"

	for rank in $(seq 0 "$max"); do
		for key in "${!seen[@]}"; do
			[ "${seen[$key]}" = "$rank" ] || continue
			printf '### %s\n\n' "$(title_of "$key")"
			# shellcheck disable=SC2154
			while IFS= read -r subject; do
				[ -n "$subject" ] && printf -- '- %s\n' "$subject"
			done <<<"${groups[$key]}"
			printf '\n'
		done
	done
}

title_of() {
	case "$1" in
	feat) echo 'Features' ;;
	fix) echo 'Fixes' ;;
	perf) echo 'Performance' ;;
	refactor) echo 'Refactoring' ;;
	test) echo 'Tests' ;;
	ci) echo 'CI' ;;
	docs) echo 'Docs' ;;
	chore) echo 'Chore' ;;
	esac
}

today="$(date +%F)"
section="$(printf '## %s (%s)\n\n%s' "$tag" "$today" "$(changelog)")"

if [ "$dry_run" = "1" ]; then
	printf 'would release %s (last tag: %s)\n\n%s' \
		"$tag" "${last_tag:-none}" "$section"
	exit 0
fi

[ -f CHANGELOG.md ] || printf '# Changelog\n\n' >CHANGELOG.md

tmp="$(mktemp)"
{
	head -n 2 CHANGELOG.md
	printf '%s\n\n' "$section"
	tail -n +3 CHANGELOG.md
} >"$tmp"

if cmp -s CHANGELOG.md "$tmp"; then
	die "generated changelog is empty or identical — nothing to release?"
fi
mv "$tmp" CHANGELOG.md

git add CHANGELOG.md
git commit -q -m "docs: changelog $tag"
git tag -a "$tag" -m "Release $tag"
git push origin master "$tag"

printf 'released %s — CI is building the GitHub Release:\n' "$tag"
printf 'https://github.com/%s/actions/workflows/release.yml\n' \
	"$(git remote get-url origin | sed -E 's#.*[:/]([^/:]+/[^/.]+)(\.git)?$#\1#')"
