package complete

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCandidate_Contract(t *testing.T) {
	t.Parallel()
	c := Candidate{
		Label:  "create_land",
		Kind:   "command",
		Detail: "land_generation",
		Sort:   "1create_land",
	}

	require.IsType(t, "", c.Label)
	require.IsType(t, "", c.Kind)
	require.IsType(t, "", c.Detail)
	require.IsType(t, "", c.Sort)

	require.Equal(t, "create_land", c.Label)
	require.Equal(t, "command", c.Kind)
	require.Equal(t, "land_generation", c.Detail)
	require.Equal(t, "1create_land", c.Sort)
}

func TestCandidate_ConstructAndUse(t *testing.T) {
	t.Parallel()
	c := Candidate{Label: "x", Kind: "local", Detail: "", Sort: "0x"}

	require.Equal(t, "x", c.Label)
	require.Empty(t, c.Detail, "empty detail is the designed no-data state")
}
