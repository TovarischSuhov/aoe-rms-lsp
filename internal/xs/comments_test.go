package xs

import (
	"aoe2-lsp/internal/common"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// xsCommentsFixture carries one comment of every kind — the block one
// multi-line — interleaved with code and with a string that mimics a
// comment; it drives the ordering, the disjointness and the text pins.
const xsCommentsFixture = "/* header\n   spans lines */\n" +
	"void f() {\n" +
	"  g(\"a // b\"); // tail\n" +
	"  /* arg */ h();\n" +
	"}\n"

// commentExtent builds the expected [start, end) extent in Line/Column.
func commentExtent(startLine, startCol, endLine, endCol int) common.Range {
	return common.Range{
		Start: common.Pos{Line: uint32(startLine), Column: uint32(startCol)},
		End:   common.Pos{Line: uint32(endLine), Column: uint32(endCol)},
	}
}

// commentExtents strips the Offset coordinate so a case pins just its
// Line/Column pair. That is a readability convenience, not a claim that
// offsets are less reliable: XsParse does not normalize EOL (it scans the
// source's raw bytes), so Offset is byte-exact too — commentTexts below
// pins it, and the CRLF case exercises those raw bytes.
func commentExtents(ranges []common.Range) []common.Range {
	out := make([]common.Range, 0, len(ranges))

	for _, r := range ranges {
		r.Start.Offset = 0
		r.End.Offset = 0
		out = append(out, r)
	}

	return out
}

// commentTexts slices the source by the extents' offsets — the extraction
// the future XS printer relies on, since comment texts are not stored in
// the AST.
func commentTexts(src string, ranges []common.Range) []string {
	out := make([]string, 0, len(ranges))

	for _, r := range ranges {
		out = append(out, src[r.Start.Offset:r.End.Offset])
	}

	return out
}

// TestXsComments_Extents pins the exact extent per comment kind: line
// comments to the end of their line (the last line without a trailing
// newline included), block comments from "/*" through "*/" (multi-line and
// unterminated ones alike). Comment markers inside a string literal are not
// comments — the scanner keeps the whole literal as one string token.
func TestXsComments_Extents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []common.Range
		// check optionally pins facts the extent alone cannot express for a
		// case — the text sliced by offset, the parsing of the code that
		// follows the comment; nil means the extent is the whole assertion.
		check func(t *testing.T, file XsFile)
	}{
		{
			name: "line comment on the last line without a newline runs to the end of input",
			src:  "void f() {}\n// tail",
			want: []common.Range{commentExtent(1, 0, 1, 7)},
		},
		{
			name: "line comment in a CRLF file ends before the carriage return",
			src:  "int x; // c\r\nint y;",
			want: []common.Range{commentExtent(0, 7, 0, 11)},
			check: func(t *testing.T, file XsFile) {
				t.Helper()

				// XsParse scans raw bytes: the \r is not part of the comment
				// text, so an anchored print does not re-emit it.
				require.Equal(t, []string{"// c"}, commentTexts("int x; // c\r\nint y;", file.Comments))
				// the code after the comment still parses, on the next line.
				require.Len(t, file.Decls, 2)
				require.Equal(t, uint32(1), file.Decls[1].Range.Start.Line)
			},
		},
		{
			name: "block comment spans lines through its terminator",
			src:  "/* header\n   spans */ void f() {}\n",
			want: []common.Range{commentExtent(0, 0, 1, 11)},
		},
		{
			name: "comment markers inside a string literal are not comments",
			src:  "void f() { g(\"a // b /* c */\"); }",
			want: []common.Range{},
		},
		{
			name: "comment inside an expression keeps its own extent",
			src:  "int x = 1 /* c */ + 2;",
			want: []common.Range{commentExtent(0, 10, 0, 17)},
		},
		{
			name: "line comment inside an argument list runs to the end of the line",
			src:  "void f() { g(a, // c\n b); }",
			want: []common.Range{commentExtent(0, 16, 0, 20)},
		},
		{
			name: "unterminated block comment runs from /* to the end of input",
			src:  "int x = 1; /* dangling",
			want: []common.Range{commentExtent(0, 11, 0, 22)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := XsParse(tt.src, "comments.xs")

			require.Equal(t, tt.want, commentExtents(file.Comments), tt.name)

			if tt.check != nil {
				tt.check(t, file)
			}
		})
	}
}

// TestXsComments_NoStrings pins the string exclusion: string tokens keep
// feeding the silence of CallAt/VisibleAt, but never reach Comments — a
// string literal anchored as a comment would be printed twice.
func TestXsComments_NoStrings(t *testing.T) {
	t.Parallel()

	t.Run("a lone string literal yields no comments", func(t *testing.T) {
		t.Parallel()

		file, _ := XsParse("string s = \"a // b /* c */\";", "noString.xs")

		require.Empty(t, file.Comments)
	})

	t.Run("comment texts never carry string bytes", func(t *testing.T) {
		t.Parallel()

		src := "void f() {\n  g(\"KEEPME // hidden\"); // real\n}\n"

		file, _ := XsParse(src, "mixed.xs")

		texts := commentTexts(src, file.Comments)

		require.NotContains(t, strings.Join(texts, "\n"), "KEEPME")
		require.Contains(t, texts, "// real")
	})
}

// TestXsComments_SortedAndDisjoint pins the structural requirements of the
// contract: the extents come out ordered by position and never overlap —
// the printer anchors them by comparing against node ranges.
func TestXsComments_SortedAndDisjoint(t *testing.T) {
	t.Parallel()

	file, _ := XsParse(xsCommentsFixture, "mixed.xs")

	cs := file.Comments

	require.NotEmpty(t, cs, "the fixture must actually yield comments")

	require.True(t, slices.IsSortedFunc(cs, func(a, b common.Range) int {
		switch {
		case a.Start.Before(b.Start):
			return -1
		case b.Start.Before(a.Start):
			return 1
		default:
			return 0
		}
	}), "extents are sorted by position")

	for i := 1; i < len(cs); i++ {
		require.False(t, cs[i-1].End.After(cs[i].Start), "extents do not overlap")
	}
}

// TestXsComments_TextsByOffset pins the extraction contract: comment texts
// are not stored, and src[start.Offset:end.Offset] must yield the exact
// bytes — from "/*" through "*/" across lines, or to the end of the line.
func TestXsComments_TextsByOffset(t *testing.T) {
	t.Parallel()

	file, _ := XsParse(xsCommentsFixture, "mixed.xs")

	require.Equal(t, []string{
		"/* header\n   spans lines */",
		"// tail",
		"/* arg */",
	}, commentTexts(xsCommentsFixture, file.Comments))
}

// probeInsideCall reports whether pos falls inside some recorded call span —
// from the callee through the argument-list end (the eofPos frontier for an
// unterminated list). It is a premise check for the cases below, not an
// assertion about navigation: a probe outside every call span would answer
// found=false on its own, so the case could no longer attribute the silence
// to the comment or the string stream and would pass vacuously.
func probeInsideCall(file XsFile, pos common.Pos) bool {
	for i := range file.calls {
		if file.calls[i].contains(pos) {
			return true
		}
	}

	return false
}

// TestXsComments_CallAndVisibleSilent pins the silence regression gate: the
// very extents the scanner derives answer found=false from CallAt and
// VisibleAt, so signature help and completion stay silent inside a comment
// of every kind and inside a string literal that spans two physical lines —
// the split must feed both streams, not only strings. Each position sits
// inside a call's span (probeInsideCall checks that premise), so the silence
// can only come from the comment or the string: losing either stream makes
// the case fail.
func TestXsComments_CallAndVisibleSilent(t *testing.T) {
	t.Parallel()

	block := "void f() { g(a /* c */, b); }"
	plateau := "void f() { g(a, /* plateau\n   higher */ b); }"
	line := "void f() { g(a, // c\n b); }"
	unterminated := "void f() { g(a, /* dangling"
	// The escaped newline keeps the literal one token across two physical
	// lines: the string half of the multi-line Start fix. Anchoring the
	// token's start after its consumption underflows Column, so the extent
	// stops matching any position and this probe answers found=true.
	escapedNewline := "void f() { g(\"ab\\\ncd\"); }"

	tests := []struct {
		name string
		src  string
		pos  common.Pos
	}{
		{
			name: "inside a block comment within a call",
			src:  block,
			pos:  posAtN(block, "c */", 0),
		},
		{
			name: "inside a multi-line block comment within a call",
			src:  plateau,
			pos:  posAtN(plateau, "higher", 0),
		},
		{
			name: "inside a line comment within a call's argument list",
			src:  line,
			pos:  posAtN(line, "c\n", 0),
		},
		{
			name: "inside an unterminated block comment within a call",
			src:  unterminated,
			pos:  posAtN(unterminated, "dangling", 0),
		},
		{
			name: "inside a string literal spanning two lines via an escaped newline",
			src:  escapedNewline,
			pos:  posAtN(escapedNewline, "cd", 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := XsParse(tt.src, "comments.xs")

			require.True(t, probeInsideCall(file, tt.pos),
				"the probe must sit inside a call span, otherwise the case is vacuous: %s", tt.name)

			_, callFound := file.CallAt(tt.pos)
			_, visFound := file.VisibleAt(tt.pos)

			require.False(t, callFound, "CallAt must stay silent: %s", tt.name)
			require.False(t, visFound, "VisibleAt must stay silent: %s", tt.name)
		})
	}
}
