package kb

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zetnusGuide is the committed guide export relative to the package dir.
const zetnusGuide = "../../docs/ref/zetnus-rms-guide.txt"

// TestExtractRmsCommands_RealGuide extracts from the committed Zetnus
// export and checks structure, metadata and changelog enrichment.
func TestExtractRmsCommands_RealGuide(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
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
	t.Parallel()
	_, err := ExtractRmsCommands("no/such/file.txt", nil)

	require.ErrorContains(t, err, "zetnus")
}

// TestExtractRmsCommands_APIShape pins the mining contract of the
// extraction pipeline: create_elevation's MaxHeight arg carries the
// mined range from the real guide.
func TestExtractRmsCommands_APIShape(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	elev, found := byName["create_elevation"]
	require.True(t, found)
	require.NotEmpty(t, elev.Args)

	assert.Equal(t, "MaxHeight", elev.Args[0].Name)
	assert.Equal(t, ValueRange{Min: "1", Max: "16"}, elev.Args[0].Range)
}

// TestExtractRmsCommands_MinesRange covers the mining pass over the real
// guide: skeleton kind wins, mined range fills the empty field.
func TestExtractRmsCommands_MinesRange(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	elev, found := byName["create_elevation"]
	require.True(t, found)
	assert.Equal(t, ValueRange{Min: "1", Max: "16"}, elev.Args[0].Range)
	assert.Equal(t, "number", elev.Args[0].Kind)

	// Structured percent kind + mined bounds compose in one arg.
	cliff, found := byName["cliff_curliness"]
	require.True(t, found)
	require.NotEmpty(t, cliff.Args)

	assert.Equal(t, "percent", cliff.Args[0].Kind)
	assert.Equal(t, ValueRange{Min: "0", Max: "100"}, cliff.Args[0].Range)
}

// TestStore_MiningIdempotentWithExtraction proves the data-pipeline
// idempotency rule: extraction output marshaled to the wire shape
// (range included) re-decodes through indexCommands with no drift —
// regeneration and load-time mining agree.
func TestStore_MiningIdempotentWithExtraction(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	wire := make([]commandWire, 0, len(commands))
	for _, cmd := range commands {
		wire = append(wire, commandWire{
			Name:         cmd.Name,
			Section:      cmd.Section,
			Args:         argsToWire(cmd.Args),
			Attributes:   argsToWire(cmd.Attributes),
			Desc:         cmd.Desc,
			GameVersions: cmd.GameVersions,
			SinceUpdate:  cmd.SinceUpdate,
		})
	}

	raw, err := json.Marshal(wire)
	require.NoError(t, err)

	s := newStore()
	require.NoError(t, s.indexCommands(raw))

	for _, cmd := range commands {
		loaded, found := s.Command(cmd.Name)
		require.True(t, found, cmd.Name)

		require.Len(t, loaded.Args, len(cmd.Args), cmd.Name)
		for i := range cmd.Args {
			assert.Equal(t, cmd.Args[i].Kind, loaded.Args[i].Kind, "%s args[%d]", cmd.Name, i)
			assert.Equal(t, cmd.Args[i].Range, loaded.Args[i].Range, "%s args[%d]", cmd.Name, i)
		}

		require.Len(t, loaded.Attributes, len(cmd.Attributes), cmd.Name)
		for i := range cmd.Attributes {
			assert.Equal(t, cmd.Attributes[i].Kind, loaded.Attributes[i].Kind, "%s attrs[%d]", cmd.Name, i)
			assert.Equal(t, cmd.Attributes[i].Range, loaded.Attributes[i].Range, "%s attrs[%d]", cmd.Name, i)
		}
	}
}

// argsToWire converts command argument specifications to the wire shape.
func argsToWire(args []CommandArg) []argWire {
	out := make([]argWire, 0, len(args))
	for _, a := range args {
		out = append(out, argWire(a))
	}

	return out
}
