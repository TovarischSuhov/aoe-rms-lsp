package hints

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHint_APIShape pins the contract surface of the render result: the
// hints package with the exported Hint entity.
func TestHint_APIShape(t *testing.T) {
	hint := Hint{Label: "l", Params: []string{"p"}, Active: 0}

	require.IsType(t, "", hint.Label)
	require.IsType(t, []string{}, hint.Params)
	require.IsType(t, 0, hint.Active)
}

// TestHint_ConstructAndUse covers the data entity: construction is the
// behavior (the contract's canonical RMS example round-trips).
func TestHint_ConstructAndUse(t *testing.T) {
	hint := Hint{
		Label:  "percent_chance(%: percent 0..99)",
		Params: []string{"%: percent 0..99"},
		Active: 0,
	}

	assert.Equal(t, "percent_chance(%: percent 0..99)", hint.Label)
	assert.Equal(t, []string{"%: percent 0..99"}, hint.Params)
	assert.Equal(t, 0, hint.Active)
}
