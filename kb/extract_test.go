package kb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zetnusGuide is the committed guide export relative to the package dir.
const zetnusGuide = "../docs/ref/zetnus-rms-guide.txt"

// TestExtractRmsCommands_RealGuide extracts from the committed Zetnus
// export and checks structure, metadata and changelog enrichment.
func TestExtractRmsCommands_RealGuide(t *testing.T) {
	commands, err := ExtractRmsCommands(zetnusGuide)
	require.NoError(t, err)
	require.Greater(t, len(commands), 50)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		assert.NotEmpty(t, cmd.Name)
		assert.Equal(t, strings.ToLower(cmd.Section), cmd.Section, "sections are normalized to lowercase")
		byName[cmd.Name] = cmd
	}

	land, found := byName["create_land"]
	require.True(t, found)
	assert.Equal(t, "land_generation", land.Section)
	assert.Len(t, land.Attributes, 24)
	assert.NotEmpty(t, land.Desc)

	object, found := byName["create_object"]
	require.True(t, found)
	assert.Len(t, object.Attributes, 46)
	require.NotEmpty(t, object.Args)
	assert.Equal(t, "ObjectType", object.Args[0].Name)

	water, found := byName["water_definition"]
	require.True(t, found)
	assert.Equal(t, "153015", water.SinceUpdate)

	ai, found := byName["ai_info_map_type"]
	require.True(t, found)
	require.Len(t, ai.Args, 4)
	assert.Equal(t, "MapType", ai.Args[0].Name)
	assert.False(t, ai.Args[0].Required, "documented default value")
	assert.NotEmpty(t, ai.Desc)

	cliff, found := byName["min_number_of_cliffs"]
	require.True(t, found)
	assert.NotEmpty(t, cliff.Desc, "min/max pairs share one doc block")
	assert.Equal(t, "All", cliff.GameVersions)

	_, found = byName["#includeXS"]
	assert.True(t, found, "directives are extracted as global commands")
}

// TestExtractRmsCommands_MissingFile checks the read error path.
func TestExtractRmsCommands_MissingFile(t *testing.T) {
	_, err := ExtractRmsCommands("no/such/file.txt")

	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "zetnus"))
}
