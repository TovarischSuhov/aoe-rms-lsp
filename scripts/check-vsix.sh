#!/usr/bin/env bash
# Verify that every contributes.* path of a VS Code extension exists in the
# packaged .vsix (#68.3): VS Code silently ignores broken grammar, language
# and snippet paths, so a packaging regression only surfaces as quietly
# missing features.
#
# Usage: scripts/check-vsix.sh <extension-dir> <file.vsix>
#   <extension-dir> — directory holding package.json (paths are read from
#     the manifest, never duplicated here, so the check cannot drift from
#     what it validates)
#   <file.vsix> — the packaged extension to inspect
set -euo pipefail

[ "$#" -eq 2 ] || { printf 'usage: check-vsix.sh <extension-dir> <file.vsix>\n' >&2; exit 2; }
ext_dir="$1"
vsix="$2"

[ -f "$ext_dir/package.json" ] || { printf 'check-vsix: %s/package.json not found\n' "$ext_dir" >&2; exit 2; }
[ -f "$vsix" ] || { printf 'check-vsix: %s not found\n' "$vsix" >&2; exit 2; }

contrib_paths="$(jq -r '
  [.contributes.grammars[]?.path,
   .contributes.languages[]?.configuration,
   .contributes.snippets[]?.path]
  | map(select(. != null))
  | .[]
' "$ext_dir/package.json")"

# jq exiting 0 on a package.json without contributes would make the check
# vacuously green — treat an empty path list as a broken setup.
[ -n "$contrib_paths" ] || { printf 'check-vsix: no contributes paths in %s/package.json — check the jq filter\n' "$ext_dir" >&2; exit 2; }

# vsce packs the extension payload under an "extension/" directory inside
# the .vsix (alongside [Content_Types].xml and the manifest) — compare
# against the payload-relative names.
unzip -Z1 "$vsix" | sed 's|^extension/||' >"$vsix.list"

missing=0
while IFS= read -r p; do
	[ -n "$p" ] || continue
	rel="${p#./}" # package.json says ./syntaxes/…, zip entries have no ./
	if ! grep -qxF "$rel" "$vsix.list"; then
		printf 'check-vsix: MISSING in %s: %s\n' "$(basename "$vsix")" "$p" >&2
		missing=1
	fi
done <<<"$contrib_paths"

rm -f "$vsix.list"
if [ "$missing" -ne 0 ]; then
	exit 1
fi
printf 'check-vsix: all contributes paths present in %s\n' "$(basename "$vsix")"
