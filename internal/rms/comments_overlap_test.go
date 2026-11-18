package rms

import (
	"aoe2-lsp/internal/common"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRmsComments_UnterminatedBlockOnHashLine pins the truncation rule: a
// /* */ opened on a plain #-line and running past it — to its own
// terminator on a later line, or to the end of the file — takes over from
// its start: the #-extent yields the tail, the bytes stay covered exactly
// once and the extents never overlap.
func TestRmsComments_UnterminatedBlockOnHashLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []common.Range
	}{
		{
			name: "block comment closed on a later line",
			src:  "#foo /* start\nstill in block */\n",
			want: []common.Range{
				commentExtent(0, 0, 0, 5),
				commentExtent(0, 5, 1, 17),
			},
		},
		{
			name: "block comment open to the end of the file",
			src:  "#foo /* start\nnever ends",
			want: []common.Range{
				commentExtent(0, 0, 0, 5),
				commentExtent(0, 5, 1, 10),
			},
		},
		{
			// #135: EOF clips the block exactly at the #-extent's end —
			// the same truncation as the two rows above, not a swallow
			name: "block comment clipped by eof at the line's end",
			src:  "#foo /* start",
			want: []common.Range{
				commentExtent(0, 0, 0, 5),
				commentExtent(0, 5, 0, 13),
			},
		},
		{
			// guard: a closed block ending with the line stays swallowed
			name: "closed block ending with the line",
			src:  "#foo /* done */",
			want: []common.Range{
				commentExtent(0, 0, 0, 15),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := Parse(tt.src, "overlap.rms")

			require.Equal(t, tt.want, commentExtents(file.Comments), tt.name)
		})
	}
}

// TestRmsComments_OverlapTextsByOffset pins the extraction side of the
// truncation: the #-head and the block comment slice into disjoint,
// complete byte runs, so the printer anchors every comment byte once.
func TestRmsComments_OverlapTextsByOffset(t *testing.T) {
	t.Parallel()

	src := "#foo /* start\nstill in block */\n"
	file, _ := Parse(src, "overlap.rms")

	require.Equal(t, []string{
		"#foo ",
		"/* start\nstill in block */",
	}, commentTexts(src, file.Comments))
}
