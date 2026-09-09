// did-you-mean suggestions: the closest known name for a typo,
// appended to unknown-name diagnostics. Pure string logic, no IO.
package analysis

import (
	"fmt"
	"strings"
)

// suggestSuffix picks the closest candidate for a typo and renders the
// message suffix; empty when nothing is close enough. Distance is
// case-insensitive Levenshtein against a length-proportional threshold
// (max(1, len/4)); equal distances resolve to the lexicographically
// smaller name, so the suggestion never depends on candidate order.
func suggestSuffix(names []string, typo string) string {
	best, bestDist := "", -1

	for _, name := range names {
		dist := levenshtein(strings.ToLower(typo), strings.ToLower(name))
		if dist > max(1, len(typo)/4) {
			continue
		}

		if bestDist == -1 || dist < bestDist || (dist == bestDist && name < best) {
			best, bestDist = name, dist
		}
	}

	if bestDist == -1 {
		return ""
	}

	return fmt.Sprintf("; did you mean %q?", best)
}

// commandNames projects every kb command name — the unknown-command
// candidate pool.
func (a *Analyzer) commandNames() []string {
	commands := a.store.Commands("")

	names := make([]string, 0, len(commands))
	for _, cmd := range commands {
		names = append(names, cmd.Name)
	}

	return names
}

// xsSymbolNames projects kb function and constant names — the
// undefined-symbol candidate pool (the file's declared names join at
// the call site).
func (a *Analyzer) xsSymbolNames() []string {
	functions := a.store.Functions()
	constants := a.store.Constants("")

	names := make([]string, 0, len(functions)+len(constants))
	for _, fn := range functions {
		names = append(names, fn.Name)
	}

	for _, c := range constants {
		names = append(names, c.Name)
	}

	return names
}

// declaredNames projects the declared-name map for the candidate pool.
func declaredNames(declared map[string]bool) []string {
	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}

	return names
}

// levenshtein computes the edit distance between two lowercase strings
// with the classic two-row DP.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}

	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		curr[0] = i

		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}

			curr[j] = min(min(curr[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}

		prev, curr = curr, prev
	}

	return prev[len(br)]
}
