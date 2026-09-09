package server

import "strings"

// fuzzyMatch reports whether query is a subsequence of name
// (case-insensitive) and scores the alignment: +1 per matched byte,
// +2 per consecutive match, +3 per word start (position 0 or right
// after '_'), and -1 for every byte skipped between matches — tighter
// alignments outrank scattered ones. An empty query matches any name
// with score 0. The match is greedy left-to-right; the byte-wise scan
// after lowercasing keeps the function deterministic for any input.
func fuzzyMatch(query string, name string) (score int, matched bool) {
	if query == "" {
		return 0, true
	}

	q := strings.ToLower(query)
	n := strings.ToLower(name)

	prev := -2

	qi := 0

	for ni := 0; ni < len(n) && qi < len(q); ni++ {
		if n[ni] != q[qi] {
			if qi > 0 {
				score--
			}

			continue
		}

		score++

		if ni == prev+1 {
			score += 2
		}

		if ni == 0 || n[ni-1] == '_' {
			score += 3
		}

		prev = ni
		qi++
	}

	if qi < len(q) {
		return 0, false
	}

	return score, true
}
