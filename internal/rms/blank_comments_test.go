package rms

import (
	"aoe2-lsp/internal/common"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// blankFixture normalizes src the way run() does — CRLF folded to LF, then
// split on newlines — and derives the byte offset of every line start, the
// exact view blankComments receives from the walk (parse.go run()).
func blankFixture(src string) ([]string, []int) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	starts := make([]int, len(lines))

	for i := 1; i < len(lines); i++ {
		starts[i] = starts[i-1] + len(lines[i-1]) + 1
	}

	return lines, starts
}

// blankPos builds an expected extent endpoint on line at byte column col
// with the file offset resolved the way blankComments resolves it
// (starts[line] + col).
func blankPos(line, col, offset int) common.Pos {
	return common.Pos{Line: uint32(line), Column: uint32(col), Offset: offset}
}

// blankRange builds an expected [start, end) extent in Line/Column/Offset.
func blankRange(sl, sc, so, el, ec, eo int) common.Range {
	return common.Range{Start: blankPos(sl, sc, so), End: blankPos(el, ec, eo)}
}

// pinBlanked pins the composite characterization contract of one case:
// every output line keeps its input length; bytes inside the returned
// extents are blanked to 0x20 — each byte of multi-byte UTF-8 runes and
// lone '\r' residues included, the line-separator '\n' the protected
// exception; bytes outside every extent are untouched; and comments,
// unclosed, hasUnclosed equal the pins exactly.
func pinBlanked(t *testing.T, src string, wantComments []common.Range, wantUnclosed common.Range, wantHasUnclosed bool) {
	t.Helper()

	lines, starts := blankFixture(src)
	joined := strings.Join(lines, "\n") // the normalized source
	out, comments, unclosed, hasUnclosed := blankComments(lines, starts)

	require.Len(t, out, len(lines))

	blanked := strings.Join(out, "\n")
	require.Len(t, blanked, len(joined), "blanking must preserve the file length")

	for i := range lines {
		require.Len(t, out[i], len(lines[i]), "line %d changed length", i)
	}

	covered := make([]bool, len(joined))

	for _, r := range comments {
		for p := r.Start.Offset; p < r.End.Offset; p++ {
			if joined[p] != '\n' {
				require.Equal(t, byte(0x20), blanked[p], "byte %d inside an extent is not blanked", p)
			}

			covered[p] = true
		}
	}

	for p := range joined {
		if !covered[p] {
			require.Equal(t, joined[p], blanked[p], "byte %d outside every extent was modified", p)
		}
	}

	require.Equal(t, wantComments, comments, "comment extents")
	require.Equal(t, wantUnclosed, unclosed, "unclosed extent")
	require.Equal(t, wantHasUnclosed, hasUnclosed, "hasUnclosed")
}

// TestBlankComments pins blankComments byte-for-byte on the current
// implementation: the scan blanks comment bytes through per-byte line
// rebuilds (issue #209's O(L²) hot spot), every non-'\n' byte of a
// scanned extent becoming 0x20 while lengths, untouched bytes and the
// extent bookkeeping stay exact. The pins characterize master and act as
// the equivalence guard for the []byte rewrite.
func TestBlankComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		src             string
		wantComments    []common.Range
		wantUnclosed    common.Range
		wantHasUnclosed bool
	}{
		{
			name:         "multibyte UTF-8 inside a block blanks byte by byte",
			src:          "/* привет */\nrest",
			wantComments: []common.Range{blankRange(0, 0, 0, 0, 18, 18)},
		},
		{
			name:         "CJK and emoji bytes blank byte by byte across lines",
			src:          "/* 日\n😀 */\nz",
			wantComments: []common.Range{blankRange(0, 0, 0, 1, 7, 14)},
		},
		{
			name:         "a lone CR inside a block blanks to a space",
			src:          "/* a\rb */\n",
			wantComments: []common.Range{blankRange(0, 0, 0, 0, 9, 9)},
		},
		{
			name:         "CRLF residue leaves lone CR bytes that blank inside the block",
			src:          "/* \r\r\nstill\r\nx */\nend",
			wantComments: []common.Range{blankRange(0, 0, 0, 2, 4, 15)},
		},
		{
			name:            "'*/' split across lines does not close the block",
			src:             "/*\n*\n/\nafter",
			wantComments:    []common.Range{blankRange(0, 0, 0, 3, 5, 12)},
			wantUnclosed:    blankRange(0, 0, 0, 3, 5, 12),
			wantHasUnclosed: true,
		},
		{
			name: "'/*' split across lines never opens a block",
			src:  "/\n* still plain */\nz",
		},
		{
			name:         "quotes inside a block are comment bytes, not string delimiters",
			src:          `/* say "hi" */ x`,
			wantComments: []common.Range{blankRange(0, 0, 0, 0, 14, 14)},
		},
		{
			name:         "an unterminated quote inside a block hides nothing",
			src:          `/* "*/ tail`,
			wantComments: []common.Range{blankRange(0, 0, 0, 0, 6, 6)},
		},
		{
			name:         "'//' and '/*' inside a block are plain bytes",
			src:          "/* // not\nstill /* x */ in */\nend",
			wantComments: []common.Range{blankRange(0, 0, 0, 1, 13, 23)},
		},
		{
			name:         "'/*' after '//' blanks as a line comment, no block opens",
			src:          "// lead /* open\nx",
			wantComments: []common.Range{blankRange(0, 0, 0, 0, 15, 15)},
		},
		{
			name:         "'/*' inside a string literal is not a comment",
			src:          `s := "a /* b" // tail`,
			wantComments: []common.Range{blankRange(0, 14, 14, 0, 21, 21)},
		},
		{
			name:         "string state resets at the next line",
			src:          "\"\n/* open */\n",
			wantComments: []common.Range{blankRange(1, 0, 2, 1, 10, 12)},
		},
		{
			name:            "unclosed block to EOF with a trailing newline",
			src:             "create_land X\n/* trailing\n",
			wantComments:    []common.Range{blankRange(1, 0, 14, 2, 0, 26)},
			wantUnclosed:    blankRange(1, 0, 14, 2, 0, 26),
			wantHasUnclosed: true,
		},
		{
			name:            "unclosed block to EOF without a trailing newline",
			src:             "create_land X\n/* trailing",
			wantComments:    []common.Range{blankRange(1, 0, 14, 1, 11, 25)},
			wantUnclosed:    blankRange(1, 0, 14, 1, 11, 25),
			wantHasUnclosed: true,
		},
		{
			name:         "a block opened on the #includeXS directive line clamps to that line",
			src:          "#includeXS ext.xs /* h\nvoid f() { }\n<LAND_GENERATION>\n",
			wantComments: []common.Range{blankRange(0, 18, 18, 0, 22, 22)},
		},
		{
			name:         "an unclosed block inside a region clamps at the region terminator",
			src:          "#includeXS\n/* xs\n<LAND_GENERATION>\ncreate_land X\n</LAND_GENERATION>\n",
			wantComments: []common.Range{blankRange(1, 0, 11, 1, 5, 16)},
		},
		{
			name: "a terminator masked by a comment tail still closes the region",
			// the pre-check reads the prefix before any comment marker, so
			// the clamped extent ends on the previous line and the scan
			// restarts fresh on the terminator line — its own '/* keep'
			// tail then opens a plain unclosed block outside the region
			src: "#includeXS ext.xs\n/* open\n<LAND_GENERATION> /* keep\n</LAND_GENERATION>\n",
			wantComments: []common.Range{
				blankRange(1, 0, 18, 1, 7, 25),
				blankRange(2, 18, 44, 4, 0, 71),
			},
			wantUnclosed:    blankRange(2, 18, 44, 4, 0, 71),
			wantHasUnclosed: true,
		},
		{
			name: "empty and single-byte lines pass through untouched",
			src:  "\n\nx\n\n/\n\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pinBlanked(t, tt.src, tt.wantComments, tt.wantUnclosed, tt.wantHasUnclosed)
		})
	}
}
