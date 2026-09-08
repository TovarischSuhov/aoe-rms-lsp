#!/usr/bin/env bash
# Download the pinned pool of published RMS/XS scripts and sample the
# corpus deterministically.
#
# Usage:
#   scripts/corpus-fetch.sh [dest-dir]          fetch + sample (default dest: .corpus)
#   CORPUS_COUNT=N scripts/corpus-fetch.sh ...  sample size (default 100)
#   scripts/corpus-fetch.sh --update            re-pin the pool to current
#                                                repo heads (needs network,
#                                                rewrites corpus-sources.txt)
#
# The pool is pinned by commit SHA in scripts/corpus-sources.txt, so the
# same file always yields the same corpus. Downloaded scripts are NOT
# committed (upstream licenses vary); only the URL list is.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
sources="$here/corpus-sources.txt"
dest="${1:-.corpus}"
count="${CORPUS_COUNT:-100}"

repos=(
	"HSZemi/rms"
	"Naramsim/AoE2-random-map-scripts"
	"dundass/rms-playground"
	"Gatsuca/AOE2_RMS"
	"asuky/aoe2_rms"
	"rohanport/aoe2rms"
	"Maboey/aoe2_rms"
	"matsaraiva/AOE2-RMS"
)

if [[ "${1:-}" == "--update" ]]; then
	python3 - "$sources" "${repos[@]}" <<'PY'
import json
import sys
import urllib.parse
import urllib.request

out_path, repos = sys.argv[1], sys.argv[2:]

lines = []
for repo in repos:
    api = f"https://api.github.com/repos/{repo}/git/trees/HEAD?recursive=1"
    with urllib.request.urlopen(api) as resp:
        tree = json.load(resp).get("tree", [])

    sha = None
    with urllib.request.urlopen(f"https://api.github.com/repos/{repo}/commits/HEAD") as resp:
        sha = json.load(resp)["sha"]

    for entry in tree:
        path = entry["path"]
        if entry["type"] == "blob" and path.lower().endswith((".rms", ".xs")):
            quoted = urllib.parse.quote(path)
            lines.append(f"{repo}/{sha}/{quoted}\t{repo}/{path}")

with open(out_path, "w", encoding="utf-8") as out:
    out.write("# pool: raw-url (repo/sha/path) <TAB> display-name; pinned by corpus-fetch.sh --update\n")
    out.writelines(line + "\n" for line in lines)

print(f"{out_path}: {len(lines)} scripts pinned")
PY
	exit 0
fi

if [[ ! -f "$sources" ]]; then
	echo "no $sources — run scripts/corpus-fetch.sh --update first" >&2
	exit 2
fi

mkdir -p "$dest"

# Deterministic sample: fixed random source, stable sort by URL first so
# pool reordering cannot shift the selection. The walk continues past
# rejected entries (fetch failures, binary payloads such as aoe2map.net
# "ZR@" zip random maps) until the requested count of text scripts lands.
shuffled=$(grep -v '^#' "$sources" | sort -u | shuf --random-source=<(yes aoe2-corpus-v1))

i=0
accepted=0

while IFS=$'\t' read -r url display; do
	[[ -z "$url" ]] && continue
	(( accepted >= count )) && break

	i=$((i + 1))

	# Flatten to a collision-free name: sample index + sanitized tail.
	safe=$(printf '%s' "$(basename "$display")" | tr -c '[:alnum:]_.@-' '_')
	out="$dest/$(printf '%03d' "$i")__$safe"

	if ! curl -sfL --retry 2 -o "$out" "https://raw.githubusercontent.com/$url"; then
		echo "WARN: fetch failed: $display" >&2
		rm -f "$out"

		continue
	fi

	# Zip random maps (PK magic) are archives, not scripts — skip them.
	if [[ $(head -c 2 "$out") == "PK" ]]; then
		echo "WARN: zip payload skipped: $display" >&2
		rm -f "$out"

		continue
	fi

	accepted=$((accepted + 1))
done <<<"$shuffled"

echo "corpus: $accepted text scripts in $dest (requested $count)"
