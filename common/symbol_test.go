package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymbol_FieldsAndShape(t *testing.T) {
	tests := []struct {
		name string
		sym  Symbol
	}{
		{
			name: "leaf node",
			sym: Symbol{
				Kind:      "function",
				Name:      "regenerateMap",
				Range:     Range{Start: Pos{Line: 1, Column: 0}, End: Pos{Line: 5, Column: 1}},
				Selection: Range{Start: Pos{Line: 1, Column: 5}, End: Pos{Line: 1, Column: 17}},
				Children:  nil,
			},
		},
		{
			name: "node with children",
			sym: Symbol{
				Kind:      "section",
				Name:      "land_generation",
				Range:     Range{Start: Pos{Line: 0, Column: 0}, End: Pos{Line: 9, Column: 1}},
				Selection: Range{Start: Pos{Line: 0, Column: 1}, End: Pos{Line: 0, Column: 17}},
				Children: []Symbol{
					{Kind: "command", Name: "create_land"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.sym, tt.sym)

			require.IsType(t, "", tt.sym.Kind)
			require.IsType(t, "", tt.sym.Name)
			require.IsType(t, Range{}, tt.sym.Range)
			require.IsType(t, Range{}, tt.sym.Selection)
			require.IsType(t, []Symbol{}, tt.sym.Children)
		})
	}
}
