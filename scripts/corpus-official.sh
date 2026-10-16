#!/usr/bin/env bash
# Stage the installed game's own RMS/XS/inc scripts as a local corpus.
#
# Usage:
#   scripts/corpus-official.sh [dest-dir]     default dest: .corpus-official
#   AOE2DE_DIR=/path/to/AoE2DE scripts/corpus-official.sh
#
# AoE2DE ships its random maps and script includes inside the licensed game
# (© Microsoft / World's Edge) — they are not redistributable. This script
# therefore only copies them into a gitignored directory on the machine
# that has the game installed; nothing staged here is ever committed.
# The directory mirrors the install layout (relative paths preserved, so
# `includes/…` targets stay resolvable); refresh it with
# `rm -rf <dest> && scripts/corpus-official.sh <dest>`.
set -euo pipefail

dest="${1:-.corpus-official}"

# find_aoe2de prints the game install root, honoring AOE2DE_DIR first and
# otherwise scanning the standard Steam library locations.
find_aoe2de() {
	if [[ -n "${AOE2DE_DIR:-}" ]]; then
		printf '%s\n' "$AOE2DE_DIR"
		return 0
	fi

	local root vdf lib
	for root in \
		"$HOME/.steam/steam" \
		"$HOME/.local/share/Steam" \
		"$HOME/.steam/root" \
		"$HOME/.steam/debian-installation"; do
		if [[ -d "$root/steamapps/common/AoE2DE" ]]; then
			printf '%s\n' "$root/steamapps/common/AoE2DE"
			return 0
		fi

		vdf="$root/steamapps/libraryfolders.vdf"
		[[ -f "$vdf" ]] || continue

		while IFS= read -r lib; do
			if [[ -d "$lib/steamapps/common/AoE2DE" ]]; then
				printf '%s\n' "$lib/steamapps/common/AoE2DE"
				return 0
			fi
		done < <(grep -oP '"path"\s+"\K[^"]+' "$vdf" 2>/dev/null || true)
	done

	return 1
}

# stage copies file $1 from source root $2 into dest $3, preserving the
# path relative to the source root.
stage() {
	local f="$1" src="$2" dst="$3" rel
	rel="${f#"$src"/}"
	mkdir -p "$dst/$(dirname "$rel")"
	cp -f "$f" "$dst/$rel"
}

if ! game="$(find_aoe2de)"; then
	echo "corpus-official: AoE2DE install not found; set AOE2DE_DIR to the game dir" >&2
	exit 2
fi

maps="$game/resources/_common/drs/gamedata_x2"
xs="$game/resources/_common/xs"

if [[ ! -d "$maps" && ! -d "$xs" ]]; then
	echo "corpus-official: no RMS/XS under $game — unexpected install layout" >&2
	exit 2
fi

mkdir -p "$dest"

count_maps=0
count_inc=0
if [[ -d "$maps" ]]; then
	while IFS= read -r f; do
		stage "$f" "$maps" "$dest"
		count_maps=$((count_maps + 1))
	done < <(find "$maps" -type f -iname '*.rms' | sort)

	while IFS= read -r f; do
		stage "$f" "$maps" "$dest"
		count_inc=$((count_inc + 1))
	done < <(find "$maps" -type f -iname '*.inc' | sort)
fi

count_xs=0
if [[ -d "$xs" ]]; then
	while IFS= read -r f; do
		stage "$f" "$xs" "$dest"
		count_xs=$((count_xs + 1))
	done < <(find "$xs" -type f -iname '*.xs' | sort)
fi

echo "corpus-official: staged $count_maps maps + $count_inc includes + $count_xs XS scripts"
echo "  from $game"
echo "  into $dest/ (gitignored — official content is NOT redistributable, never commit)"
