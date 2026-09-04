package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRange_Contains(t *testing.T) {
	tests := []struct {
		name string
		r    Range
		p    Pos
		want bool
	}{
		{
			name: "inside single line",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 3, Column: 13, Offset: 55}},
			p:    Pos{Line: 3, Column: 7, Offset: 49},
			want: true,
		},
		{
			name: "at inclusive start",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 3, Column: 13, Offset: 55}},
			p:    Pos{Line: 3, Column: 0, Offset: 42},
			want: true,
		},
		{
			name: "at exclusive end",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 3, Column: 13, Offset: 55}},
			p:    Pos{Line: 3, Column: 13, Offset: 55},
			want: false,
		},
		{
			name: "before start on same line",
			r:    Range{Start: Pos{Line: 3, Column: 5, Offset: 47}, End: Pos{Line: 3, Column: 13, Offset: 55}},
			p:    Pos{Line: 3, Column: 2, Offset: 44},
			want: false,
		},
		{
			name: "after end on same line",
			r:    Range{Start: Pos{Line: 3, Column: 5, Offset: 47}, End: Pos{Line: 3, Column: 13, Offset: 55}},
			p:    Pos{Line: 3, Column: 20, Offset: 62},
			want: false,
		},
		{
			name: "on earlier line",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 5, Column: 0, Offset: 120}},
			p:    Pos{Line: 2, Column: 50, Offset: 30},
			want: false,
		},
		{
			name: "on later line",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 5, Column: 0, Offset: 120}},
			p:    Pos{Line: 6, Column: 0, Offset: 200},
			want: false,
		},
		{
			name: "inside multiline range",
			r:    Range{Start: Pos{Line: 3, Column: 0, Offset: 42}, End: Pos{Line: 5, Column: 0, Offset: 120}},
			p:    Pos{Line: 4, Column: 9, Offset: 80},
			want: true,
		},
		{
			name: "empty range contains nothing",
			r:    Range{Start: Pos{Line: 3, Column: 5, Offset: 47}, End: Pos{Line: 3, Column: 5, Offset: 47}},
			p:    Pos{Line: 3, Column: 5, Offset: 47},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.Contains(tt.p))
		})
	}
}
