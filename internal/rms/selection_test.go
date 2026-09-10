package rms

import (
	"aoe2-lsp/internal/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRmsFile_EnclosingRanges pins the selection-expansion chain:
// containment only (innermost first), from the expression operand
// through the attribute line and statement to the section; positions
// outside every node answer the section alone or nothing.
func TestRmsFile_EnclosingRanges(t *testing.T) {
	t.Parallel()

	src := "<LAND_GENERATION>\n" +
		"create_terrain FOREST {\n" +
		"number_of_clumps 8\n" +
		"clump_size 2 + SCALE\n" +
		"}\n" +
		"</LAND_GENERATION>\n"
	file, diags := Parse(src, "t.rms")
	require.Empty(t, diags)

	section := common.Range{
		Start: common.Pos{Line: 0, Column: 0, Offset: 0},
		End:   common.Pos{Line: 5, Column: 0, Offset: 84},
	}
	stmt := common.Range{
		Start: common.Pos{Line: 1, Column: 0, Offset: 18},
		End:   common.Pos{Line: 4, Column: 1, Offset: 83},
	}

	tests := []struct {
		name string
		pos  common.Pos
		want []common.Range
	}{
		{
			name: "operand expands through the expression to the section",
			pos:  common.Pos{Line: 3, Column: 15, Offset: 76}, // on SCALE
			want: []common.Range{
				{Start: common.Pos{Line: 3, Column: 15, Offset: 76}, End: common.Pos{Line: 3, Column: 20, Offset: 81}},
				{Start: common.Pos{Line: 3, Column: 11, Offset: 72}, End: common.Pos{Line: 3, Column: 20, Offset: 81}},
				{Start: common.Pos{Line: 3, Column: 0, Offset: 61}, End: common.Pos{Line: 3, Column: 20, Offset: 81}},
				stmt,
				section,
			},
		},
		{
			name: "attribute name answers the attribute line",
			pos:  common.Pos{Line: 2, Column: 0, Offset: 42}, // on number_of_clumps
			want: []common.Range{
				{Start: common.Pos{Line: 2, Column: 0, Offset: 42}, End: common.Pos{Line: 2, Column: 18, Offset: 60}},
				stmt,
				section,
			},
		},
		{
			name: "section header answers the section alone",
			pos:  common.Pos{Line: 0, Column: 1, Offset: 1},
			want: []common.Range{section},
		},
		{
			name: "past the last section answers nothing",
			pos:  common.Pos{Line: 6, Column: 0, Offset: 103},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, file.EnclosingRanges(tt.pos))
		})
	}
}
