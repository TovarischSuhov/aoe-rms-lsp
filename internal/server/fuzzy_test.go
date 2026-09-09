package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFuzzyMatch_Scoring pins the scoring model: +1 per matched byte,
// +2 per consecutive match, +3 per word start (position 0 or after '_'),
// -1 per skipped byte between matches; case-insensitive; an empty query
// matches everything with score 0.
func TestFuzzyMatch_Scoring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		sym     string
		score   int
		matched bool
	}{
		{
			name:    "empty query matches everything",
			query:   "",
			sym:     "anything",
			score:   0,
			matched: true,
		},
		{
			// skipped '<' before the first match costs nothing; then
			// l(+1) a(+1+2 streak) n(+1+2) d(+1+2)
			name:    "prefix with streaks",
			query:   "land",
			sym:     "<land_generation>",
			score:   10,
			matched: true,
		},
		{
			// s(+1+3 word-start) h(+1+2) f(+1) n(+1+2) minus five
			// skipped bytes between matches ('a','r','e','d','F')
			name:    "scattered subsequence",
			query:   "shfn",
			sym:     "sharedFn",
			score:   7,
			matched: true,
		},
		{
			// s(+1+3) h/a/r/e/d(+1+2 each): a contiguous prefix
			name:    "case-insensitive",
			query:   "SHARED",
			sym:     "sharedFn",
			score:   19,
			matched: true,
		},
		{
			name:    "not a subsequence",
			query:   "zzz",
			sym:     "sharedFn",
			score:   0,
			matched: false,
		},
		{
			name:    "query longer than name",
			query:   "sharedfnx",
			sym:     "sharedFn",
			score:   0,
			matched: false,
		},
		{
			name:    "empty name with query",
			query:   "x",
			sym:     "",
			score:   0,
			matched: false,
		},
		{
			// underscore boundary: g matches at a word start (+3)
			name:    "underscore is a word boundary",
			query:   "g",
			sym:     "land_generation",
			score:   4,
			matched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			score, matched := fuzzyMatch(tt.query, tt.sym)
			require.Equal(t, tt.matched, matched, "matched")
			require.Equal(t, tt.score, score, "score")
		})
	}
}

// TestFuzzyMatch_PrefersContiguous pins the ordering signal: a tight
// prefix alignment scores strictly higher than the same letters
// scattered through a longer name.
func TestFuzzyMatch_PrefersContiguous(t *testing.T) {
	t.Parallel()

	tightScore, ok := fuzzyMatch("land", "land_generation")
	require.True(t, ok)

	scatteredScore, ok := fuzzyMatch("land", "l..a..n..d_extra_padding_name")
	require.True(t, ok)

	require.Greater(t, tightScore, scatteredScore, "contiguous match must outrank scattered")
}

// TestFuzzyMatch_Deterministic pins determinism: the same input answers
// identically across calls.
func TestFuzzyMatch_Deterministic(t *testing.T) {
	t.Parallel()

	firstScore, firstOK := fuzzyMatch("ShFn", "sharedFn")
	secondScore, secondOK := fuzzyMatch("ShFn", "sharedFn")

	require.Equal(t, firstScore, secondScore)
	require.Equal(t, firstOK, secondOK)
}
