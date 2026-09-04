package kb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStore_Success loads the embedded knowledge base and checks the
// headline counters.
func TestNewStore_Success(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	assert.Len(t, store.Functions(), 204)

	all := store.Constants("")
	require.NotEmpty(t, all)

	sections := make(map[string]bool)
	for _, c := range all {
		sections[c.Section] = true
	}

	assert.Len(t, sections, 27)
	assert.NotEmpty(t, store.Commands(""))
}

// TestStore_Function covers exact-name function lookups.
func TestStore_Function(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	fn, found := store.Function("xsGetMapSeed")
	require.True(t, found)
	assert.Equal(t, "xsGetMapSeed", fn.Name)
	assert.NotEmpty(t, fn.ReturnType)

	_, found = store.Function("xsGetMapSeed ")
	assert.False(t, found)

	_, found = store.Function("XSGETMAPSEED")
	assert.False(t, found, "lookups must be case-sensitive")
}

// TestStore_Functions_Order checks file-order preservation for completion.
func TestStore_Functions_Order(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	fns := store.Functions()
	require.NotEmpty(t, fns)
	assert.Equal(t, "xsGetGoal", fns[0].Name, "sorted data-file order")
}

// TestStore_Constant covers exact-name and section-scoped constant lookups.
func TestStore_Constant(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	c, found := store.Constant("cDarkAge")
	require.True(t, found)
	assert.Equal(t, "age", c.Section)
	assert.Equal(t, "0", c.Value)

	_, found = store.Constant("c_dark_age")
	assert.False(t, found)

	ages := store.Constants("age")
	assert.NotEmpty(t, ages)

	for _, c := range ages {
		assert.Equal(t, "age", c.Section)
	}

	assert.Empty(t, store.Constants("no such section"))
}

// TestStore_Command covers exact-name and section-scoped command lookups.
func TestStore_Command(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	cmd, found := store.Command("water_definition")
	require.True(t, found)
	assert.Equal(t, "player_setup", cmd.Section)
	assert.Equal(t, "153015", cmd.SinceUpdate)
	assert.NotEmpty(t, cmd.Desc)
	assert.NotEmpty(t, cmd.GameVersions)

	_, found = store.Command("create_elevator")
	assert.False(t, found, "unknown command must not be found")

	lands := store.Commands("land_generation")
	require.NotEmpty(t, lands)

	names := make(map[string]bool)
	for _, c := range lands {
		assert.Equal(t, "land_generation", c.Section)
		names[c.Name] = true
	}

	assert.True(t, names["create_land"])
}

// TestStore_Attribute covers attribute lookups per command.
func TestStore_Attribute(t *testing.T) {
	store, err := NewStore()
	require.NoError(t, err)

	attr, found := store.Attribute("create_land", "terrain_type")
	require.True(t, found)
	assert.Equal(t, "terrain_type", attr.Name)
	assert.Equal(t, "const", attr.Kind)
	assert.False(t, attr.Required, "attributes are optional")

	_, found = store.Attribute("create_land", "no_such_attribute")
	assert.False(t, found)

	_, found = store.Attribute("no_such_command", "terrain_type")
	assert.False(t, found)
}

// TestIndexFunctions_InvalidPayload covers schema validation failures.
func TestIndexFunctions_InvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "duplicate name",
			payload: `[{"name":"f","params":[]},{"name":"f","params":[]}]`,
		},
		{
			name:    "empty name",
			payload: `[{"name":"","params":[]}]`,
		},
		{
			name:    "unknown key",
			payload: `[{"name":"f","params":[],"surprise":1}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			err := s.indexFunctions([]byte(tt.payload))

			assert.Error(t, err)
		})
	}
}

// TestIndexConstants_InvalidPayload covers schema validation failures.
func TestIndexConstants_InvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "duplicate name",
			payload: `[{"name":"c","value":"1"},{"name":"c","value":"2"}]`,
		},
		{
			name:    "empty name",
			payload: `[{"name":"","value":"1"}]`,
		},
		{
			name:    "unknown key",
			payload: `[{"name":"c","value":"1","oops":true}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			err := s.indexConstants([]byte(tt.payload))

			assert.Error(t, err)
		})
	}
}

// TestIndexCommands_InvalidPayload covers schema validation failures.
func TestIndexCommands_InvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "duplicate name",
			payload: `[{"name":"cmd"},{"name":"cmd"}]`,
		},
		{
			name:    "empty name",
			payload: `[{"name":""}]`,
		},
		{
			name:    "unknown key",
			payload: `[{"name":"cmd","wat":0}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()

			err := s.indexCommands([]byte(tt.payload))

			assert.Error(t, err)
		})
	}
}
