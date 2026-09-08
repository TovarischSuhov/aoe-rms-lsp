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
	# XS libraries — the pool's only real-world .xs sources
	"mardaravicius/aoe2de_xslibs"
	"GoKuModder/AoE2_AI_Modder"
	"qferre/aoe2-xs-comm"
	"SpiRaL-network/xs-lecuyer-rng"
	"patgarz/aoe2de"
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
# rejected entries (fetch failures, unusable archives) until the
# requested count of text scripts lands. Zip random maps (aoe2map.net
# "ZR@", PK magic) are unpacked: every inner .rms/.xs becomes its own
# sample entry under its own index; the archive itself never lands.
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

	if [[ $(head -c 2 "$out") == "PK" ]]; then
		unpacked=$(mktemp -d)

		if ! python3 - "$out" "$unpacked" <<'PY'
import os
import re
import sys
import zipfile

zip_path, dest = sys.argv[1], sys.argv[2]


def safe(name):
    return re.sub(r"[^A-Za-z0-9_.@-]", "_", name)


try:
    with zipfile.ZipFile(zip_path) as zf:
        infos = [info for info in zf.infolist() if not info.is_dir()]
        if len(infos) > 64 or sum(i.file_size for i in infos) > 10 * 1024 * 1024:
            sys.exit(3)  # junk/zip-bomb guard
        extracted = 0
        for info in infos:
            # basename only: paths inside the archive are never honored
            base = safe(os.path.basename(info.filename))
            if not base.lower().endswith((".rms", ".xs")):
                continue
            with open(os.path.join(dest, base), "wb") as out:
                out.write(zf.read(info))
            extracted += 1
        sys.exit(0 if extracted else 2)
except zipfile.BadZipFile:
    sys.exit(1)
PY
		then
			echo "WARN: zip payload unusable: $display" >&2
			rm -rf "$unpacked" "$out"

			continue
		fi

		rm -f "$out"

		for inner in "$unpacked"/*; do
			[[ -f "$inner" ]] || continue
			(( accepted >= count )) && break

			i=$((i + 1))
			mv "$inner" "$dest/$(printf '%03d' "$i")__$(basename "$inner")"
			accepted=$((accepted + 1))
		done

		rm -rf "$unpacked"

		continue
	fi

	accepted=$((accepted + 1))
done <<<"$shuffled"

echo "corpus: $accepted text scripts in $dest (requested $count)"
