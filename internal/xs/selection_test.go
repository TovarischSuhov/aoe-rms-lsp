package xs

import (
	"aoe2-lsp/internal/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestXsFile_EnclosingRanges pins the selection-expansion chain:
// containment only (innermost first), from the expression through the
// statement and nesting blocks to the declaration; positions outside
// every node answer nothing.
func TestXsFile_EnclosingRanges(t *testing.T) {
	t.Parallel()

	src := "void f() {\n" +
		"int a = 1 + b;\n" +
		"if (a > 0) {\n" +
		"g(a);\n" +
		"}\n" +
		"}\n"
	file, diags := XsParse(src, "t.xs")
	require.Empty(t, diags)

	decl := common.Range{
		Start: common.Pos{Line: 0, Column: 0, Offset: 0},
		End:   common.Pos{Line: 3, Column: 5, Offset: 44},
	}
	ifStmt := common.Range{
		Start: common.Pos{Line: 2, Column: 0, Offset: 26},
		End:   common.Pos{Line: 3, Column: 5, Offset: 44},
	}

	tests := []struct {
		name string
		pos  common.Pos
		want []common.Range
	}{
		{
			name: "operand expands through the declaration statement",
			pos:  common.Pos{Line: 1, Column: 12, Offset: 23}, // on b
			want: []common.Range{
				{Start: common.Pos{Line: 1, Column: 12, Offset: 23}, End: common.Pos{Line: 1, Column: 13, Offset: 24}},
				{Start: common.Pos{Line: 1, Column: 8, Offset: 19}, End: common.Pos{Line: 1, Column: 13, Offset: 24}},
				{Start: common.Pos{Line: 1, Column: 4, Offset: 15}, End: common.Pos{Line: 1, Column: 13, Offset: 24}},
				{Start: common.Pos{Line: 1, Column: 0, Offset: 11}, End: common.Pos{Line: 1, Column: 14, Offset: 25}},
				decl,
			},
		},
		{
			name: "argument expands through the call, block and if",
			pos:  common.Pos{Line: 3, Column: 2, Offset: 41}, // on a of g(a)
			want: []common.Range{
				{Start: common.Pos{Line: 3, Column: 2, Offset: 41}, End: common.Pos{Line: 3, Column: 3, Offset: 42}},
				{Start: common.Pos{Line: 3, Column: 0, Offset: 39}, End: common.Pos{Line: 3, Column: 3, Offset: 42}},
				{Start: common.Pos{Line: 3, Column: 0, Offset: 39}, End: common.Pos{Line: 3, Column: 5, Offset: 44}},
				{Start: common.Pos{Line: 2, Column: 11, Offset: 37}, End: common.Pos{Line: 3, Column: 5, Offset: 44}},
				ifStmt,
				decl,
			},
		},
		{
			name: "declaration keyword answers the declaration alone",
			pos:  common.Pos{Line: 0, Column: 1, Offset: 1}, // on void
			want: []common.Range{decl},
		},
		{
			name: "if keyword skips the condition expression",
			pos:  common.Pos{Line: 2, Column: 1, Offset: 27},
			want: []common.Range{ifStmt, decl},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, file.EnclosingRanges(tt.pos))
		})
	}
}
