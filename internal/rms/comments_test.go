package rms

import (
	"aoe2-lsp/internal/common"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// commentsMixedFixture carries one comment of every kind — the block one
// multi-line — separated by statements; it drives the ordering and the
// text-extraction pins.
const commentsMixedFixture = "/* header\n   spans lines */\n" +
	"create_elevation 3 // hill top\n" +
	"\n" +
	"# land block\n" +
	"create_land TERRAIN_GRASS\n"

// commentExtent builds the expected [start, end) extent in Line/Column.
func commentExtent(startLine, startCol, endLine, endCol int) common.Range {
	return common.Range{
		Start: common.Pos{Line: uint32(startLine), Column: uint32(startCol)},
		End:   common.Pos{Line: uint32(endLine), Column: uint32(endCol)},
	}
}

// commentExtents strips the Offset coordinate: extents are pinned in
// Line/Column, which survive the parser's EOL normalization.
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
// the formatter relies on, since comment texts are not stored in the AST.
func commentTexts(src string, ranges []common.Range) []string {
	out := make([]string, 0, len(ranges))

	for _, r := range ranges {
		out = append(out, src[r.Start.Offset:r.End.Offset])
	}

	return out
}

// TestRmsComments_Extents pins the exact extents per comment kind: block
// comments from "/*" through "*/" (multi-line included), line comments to
// the end of their line, plain #-lines from the "#" to the end of the
// line. The formatter reads comment texts through them, so every byte of
// the comment must be inside the extent.
func TestRmsComments_Extents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []common.Range
	}{
		{
			name: "block comment runs from /* through */",
			src:  "create_elevation 3 /* hill */",
			want: []common.Range{commentExtent(0, 19, 0, 29)},
		},
		{
			name: "block comment spans lines up to its terminator",
			src:  "/* plateau\n   higher */ create_land\n",
			want: []common.Range{commentExtent(0, 0, 1, 12)},
		},
		{
			name: "line comment runs to the end of its line",
			src:  "create_elevation 4 // ridge",
			want: []common.Range{commentExtent(0, 19, 0, 27)},
		},
		{
			name: "hash line runs from the # to the end of the line",
			src:  "# gentle slope\ncreate_land TERRAIN_GRASS\n",
			want: []common.Range{commentExtent(0, 0, 0, 14)},
		},
		{
			name: "indented hash line starts at the #",
			src:  "  # gentle slope\n",
			want: []common.Range{commentExtent(0, 2, 0, 16)},
		},
		{
			name: "hash line absorbs the comments inside it",
			// one extent per #-line: the scanned /* */ and // sub-extents
			// are swallowed, never reported alongside
			src:  "#foo /* x */ // y",
			want: []common.Range{commentExtent(0, 0, 0, 17)},
		},
		{
			name: "block comment and hash line on one line stay apart",
			// the line is a plain #-comment (the blanked head trims to
			// "#foo"), but its extent starts at the # — the block comment
			// keeps its own extent, and the two do not overlap
			src: "/* x */ #foo",
			want: []common.Range{
				commentExtent(0, 0, 0, 7),
				commentExtent(0, 8, 0, 12),
			},
		},
		{
			name: "unclosed block comment outside a region spans to the end of the file",
			// pre-existing EOF behavior: one extent to the end of the last
			// line — the region fix must not change files without regions
			src:  "create_land X\n/* trailing",
			want: []common.Range{commentExtent(1, 0, 1, 11)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := Parse(tt.src, "comments.rms")

			require.Equal(t, tt.want, commentExtents(file.Comments), tt.name)
		})
	}
}

// TestRmsComments_DirectivesStayOut pins the directive vocabulary: a
// #const/#define/#include/#includeXS/#include_drs line is a directive of
// the grammar, not a comment — none of them lands in Comments.
func TestRmsComments_DirectivesStayOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "const",
			src:  "#const TERRAIN 7\ncreate_land TERRAIN_GRASS\n",
		},
		{
			name: "define",
			src:  "#define FOREST 1\n",
		},
		{
			name: "include",
			src:  "#include \"other.rms\"\n<LAND_GENERATION>\n</LAND_GENERATION>\n",
		},
		{
			name: "includeXS with an argument",
			src:  "#includeXS ext.xs\n<LAND_GENERATION>\n</LAND_GENERATION>\n",
		},
		{
			name: "include_drs",
			src:  "#include_drs 1 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := Parse(tt.src, "comments.rms")

			require.Empty(t, file.Comments, tt.name)
		})
	}
}

// TestRmsComments_HashInsideXsBlock pins the accepted asymmetry of the
// inline-XS region: the walk skips XS lines before the #-branch, so a
// plain #-line inside the block never lands in Comments — its bytes
// travel verbatim in XsBlock.Code. Scanned /* */ and // extents inside
// the region do reach Comments; only the #-line is out.
func TestRmsComments_HashInsideXsBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			name: "argumented includeXS region",
			src: "#includeXS ext.xs\n" +
				"# xs hash\n" +
				"<LAND_GENERATION>\n" +
				"</LAND_GENERATION>\n",
		},
		{
			name: "bare includeXS region",
			src: "<OBJECTS_GENERATION>\n" +
				"create_object VILLAGER\n" +
				"#includeXS\n" +
				"# xs hash\n" +
				"</OBJECTS_GENERATION>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := Parse(tt.src, "comments.rms")

			require.NotContains(t, commentTexts(tt.src, file.Comments), "# xs hash", tt.name)
		})
	}
}

// TestRmsComments_SortedAndDisjoint pins the structural requirements of
// the contract: the extents come out ordered by position and never
// overlap — the formatter anchors them by comparing against node ranges.
func TestRmsComments_SortedAndDisjoint(t *testing.T) {
	t.Parallel()

	file, _ := Parse(commentsMixedFixture, "mixed.rms")

	cs := file.Comments

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

// TestRmsComments_TextsByOffset pins the extraction contract: comment
// texts are not stored, and src[start.Offset:end.Offset] must yield the
// exact bytes — from "/*" through "*/" across lines, or to the end of the
// line.
func TestRmsComments_TextsByOffset(t *testing.T) {
	t.Parallel()

	file, _ := Parse(commentsMixedFixture, "mixed.rms")

	require.Equal(t, []string{
		"/* header\n   spans lines */",
		"// hill top",
		"# land block",
	}, commentTexts(commentsMixedFixture, file.Comments))
}

// TestRmsComments_ArgAtSilent pins the ArgAt regression gate: the very
// extents Comments reports answer found=false, so signature help stays
// silent inside a comment of every kind.
func TestRmsComments_ArgAtSilent(t *testing.T) {
	t.Parallel()

	block := "create_elevation 3 /* hill */"
	plateau := "/* plateau\n   higher */ create_land\n"
	line := "create_elevation 4 // ridge\n"
	hash := "create_elevation 5\n# land block\n"

	tests := []struct {
		name string
		src  string
		pos  common.Pos
	}{
		{
			name: "inside a block comment",
			src:  block,
			pos:  linePos(block, 0, "hill", 0),
		},
		{
			name: "inside a multi-line block comment",
			src:  plateau,
			pos:  linePos(plateau, 1, "higher", 0),
		},
		{
			name: "inside a line comment",
			src:  line,
			pos:  linePos(line, 0, "ridge", 0),
		},
		{
			name: "inside a hash comment",
			src:  hash,
			pos:  linePos(hash, 1, "land", 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := Parse(tt.src, "comments.rms")

			_, found := file.ArgAt(tt.pos)

			require.False(t, found, tt.name)
		})
	}
}
