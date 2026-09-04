package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPos_Ordering(t *testing.T) {
	tests := []struct {
		name       string
		a          Pos
		b          Pos
		wantBefore bool
		wantAfter  bool
	}{
		{
			name:       "equal positions",
			a:          Pos{Line: 3, Column: 7, Offset: 42},
			b:          Pos{Line: 3, Column: 7, Offset: 42},
			wantBefore: false,
			wantAfter:  false,
		},
		{
			name:       "equal coordinates ignore offset",
			a:          Pos{Line: 3, Column: 7, Offset: 10},
			b:          Pos{Line: 3, Column: 7, Offset: 99},
			wantBefore: false,
			wantAfter:  false,
		},
		{
			name:       "same line smaller column",
			a:          Pos{Line: 3, Column: 0, Offset: 42},
			b:          Pos{Line: 3, Column: 13, Offset: 55},
			wantBefore: true,
			wantAfter:  false,
		},
		{
			name:       "same line larger column",
			a:          Pos{Line: 3, Column: 13, Offset: 55},
			b:          Pos{Line: 3, Column: 0, Offset: 42},
			wantBefore: false,
			wantAfter:  true,
		},
		{
			name:       "smaller line regardless of column",
			a:          Pos{Line: 2, Column: 100, Offset: 900},
			b:          Pos{Line: 3, Column: 0, Offset: 42},
			wantBefore: true,
			wantAfter:  false,
		},
		{
			name:       "larger line regardless of column",
			a:          Pos{Line: 4, Column: 0, Offset: 100},
			b:          Pos{Line: 3, Column: 100, Offset: 99},
			wantBefore: false,
			wantAfter:  true,
		},
		{
			name:       "zero positions",
			a:          Pos{},
			b:          Pos{},
			wantBefore: false,
			wantAfter:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantBefore, tt.a.Before(tt.b))
			assert.Equal(t, tt.wantAfter, tt.a.After(tt.b))
		})
	}
}
