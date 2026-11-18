package kb

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genFixture writes a minimal but valid ref-source tree (see the kbdata
// annotation for the layout) into dir and returns its path.
func genFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	writeGenFile(t, dir, "ugc-guide/xs/functions/functions.json", `{
  "vector": [
    {
      "name": "xsVectorSet",
      "return_type": "vector",
      "params": [{"name": "x", "type": "float", "required": true, "desc": "x coord"}],
      "desc": "Sets a vector"
    }
  ]
}`)

	writeGenFile(t, dir, "ugc-guide/xs/constants/constants.json", `{
  "terrain": [{"name": "TERRAIN_GRASS", "value": 0, "desc": "grass"}]
}`)

	writeGenFile(t, dir, "zetnus-rms-guide.txt", `Syntax Skeleton

<PLAYER_SETUP>

create_elevation N
`)

	writeGenFile(t, dir, changelogFile, `## Update 7
### RMS: добавлено
- new `+"`create_elevation`"+`

## Update 5
### XS: добавлено
- new `+"`vector xsVectorSet(float, float, float)`"+` and `+"`TERRAIN_GRASS`"+`
`)

	return dir
}

// writeGenFile creates rel under root with the given content.
func writeGenFile(t *testing.T, root string, rel string, content string) {
	t.Helper()

	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// readGenJSON reads and decodes one generated file into T.
func readGenJSON[T any](t *testing.T, path string) []T {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var out []T
	require.NoError(t, json.Unmarshal(raw, &out))

	return out
}

// TestGenKB_WritesThreeFiles pins the facade and the wiring: the three
// data files appear with the shapes from the kbdata annotation, enriched
// from the changelog.
func TestGenKB_WritesThreeFiles(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	dataDir := t.TempDir()

	require.NoError(t, GenKB(refDir, dataDir, nil))

	functions := readGenJSON[functionJSON](t, filepath.Join(dataDir, "xs-functions.json"))
	require.Len(t, functions, 1)
	require.Equal(t, "xsVectorSet", functions[0].Name)
	require.Equal(t, "vector", functions[0].Category)
	require.Equal(t, "5", functions[0].SinceUpdate)
	require.Len(t, functions[0].Params, 1)

	constants := readGenJSON[constantJSON](t, filepath.Join(dataDir, "xs-constants.json"))
	require.Len(t, constants, 1)
	require.Equal(t, "TERRAIN_GRASS", constants[0].Name)
	require.Equal(t, "0", constants[0].Value)
	require.Equal(t, "5", constants[0].SinceUpdate)

	commands := readGenJSON[commandJSON](t, filepath.Join(dataDir, "rms-commands.json"))
	require.Len(t, commands, 1)
	require.Equal(t, "create_elevation", commands[0].Name)
	require.Equal(t, "player_setup", commands[0].Section)
	require.Equal(t, "7", commands[0].SinceUpdate)
	require.Len(t, commands[0].Args, 1)
	require.Equal(t, "number", commands[0].Args[0].Kind)
}

// TestGenKB_Deterministic reruns the generation over the same sources and
// requires byte-identical output files.
func TestGenKB_Deterministic(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	first := t.TempDir()
	second := t.TempDir()

	require.NoError(t, GenKB(refDir, first, nil))
	require.NoError(t, GenKB(refDir, second, nil))

	for _, name := range [...]string{"xs-functions.json", "xs-constants.json", "rms-commands.json"} {
		a, err := os.ReadFile(filepath.Join(first, name))
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(second, name))
		require.NoError(t, err)

		require.Equal(t, string(a), string(b), name)
	}
}

// TestGenKB_MissingSource checks the read error path carries the file
// context.
func TestGenKB_MissingSource(t *testing.T) {
	t.Parallel()

	refDir := t.TempDir()
	writeGenFile(t, refDir, changelogFile, "## Update 1\n")

	err := GenKB(refDir, t.TempDir(), nil)
	require.ErrorContains(t, err, "functions")
}

// TestGenKB_DuplicateNames checks the uniqueness guard: the same command
// twice in the skeleton is a build error, not a silent overwrite.
func TestGenKB_DuplicateNames(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	replaceGenFile(t, refDir, "zetnus-rms-guide.txt", `Syntax Skeleton

<PLAYER_SETUP>

create_elevation N
create_elevation N
`)

	err := GenKB(refDir, t.TempDir(), nil)
	require.ErrorContains(t, err, "duplicate")
}

// TestGenKB_MissingGuideKeepsCommands covers the optional-guide path: the
// Zetnus export is not vendored, so a ref dir without it still regenerates
// the XS files (with a WARN), while rms-commands.json is never created and
// never overwritten.
func TestGenKB_MissingGuideKeepsCommands(t *testing.T) {
	t.Parallel()

	t.Run("keeps the committed commands file untouched", func(t *testing.T) {
		t.Parallel()

		refDir := genFixture(t)
		require.NoError(t, os.Remove(filepath.Join(refDir, guideFile)))

		dataDir := t.TempDir()
		kept := filepath.Join(dataDir, "rms-commands.json")
		require.NoError(t, os.WriteFile(kept, []byte(`[{"name":"kept"}]`), 0o644))

		var buf bytes.Buffer
		require.NoError(t, GenKB(refDir, dataDir, slog.New(slog.NewTextHandler(&buf, nil))))

		raw, err := os.ReadFile(kept)
		require.NoError(t, err)

		assert.Equal(t, `[{"name":"kept"}]`, string(raw))
		assert.Contains(t, buf.String(), "guide", "the skipped source is reported")
	})

	t.Run("writes the XS files only", func(t *testing.T) {
		t.Parallel()

		refDir := genFixture(t)
		require.NoError(t, os.Remove(filepath.Join(refDir, guideFile)))

		dataDir := t.TempDir()
		require.NoError(t, GenKB(refDir, dataDir, nil))

		require.FileExists(t, filepath.Join(dataDir, "xs-functions.json"))
		require.FileExists(t, filepath.Join(dataDir, "xs-constants.json"))

		_, err := os.Stat(filepath.Join(dataDir, "rms-commands.json"))
		assert.True(t, errors.Is(err, os.ErrNotExist), "no commands file without the guide")
	})
}

// replaceGenFile overwrites rel under root.
func replaceGenFile(t *testing.T, root string, rel string, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644))
}

// TestGenKB_RenamesAndStubs pins the changelog-as-source rules: a rename
// pair stubs the old name (renamed_to, changelog category, since from
// the rename update) when even the guide lacks it, the new name keeps
// its guide record and gains the rename update as since_update, and a
// non-signature span (AI-DSL syntax) never becomes a stub.
func TestGenKB_RenamesAndStubs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeGenFile(t, dir, "ugc-guide/xs/functions/functions.json", `{
  "ui": [
    {"name": "xsUnsyncGetLocalPlayerId", "return_type": "int", "params": [], "desc": "New name"}
  ]
}`)

	writeGenFile(t, dir, "ugc-guide/xs/constants/constants.json", `{
  "terrain": [{"name": "TERRAIN_GRASS", "value": 0, "desc": "grass"}]
}`)

	writeGenFile(t, dir, "zetnus-rms-guide.txt", "Syntax Skeleton\n\n<PLAYER_SETUP>\n\ncreate_elevation N\n")

	writeGenFile(t, dir, changelogFile, `## Update 9
### XS: переименовано
- `+"`xsGetLocalPlayerId`"+` → `+"`xsUnsyncGetLocalPlayerId`"+`
- Both names read the same value.

### XS: добавлено
- `+"`int ord(string character)`"+`
- `+"`use (include \"script.xs\")`"+` — AI DSL, not a function
`)

	out := t.TempDir()
	require.NoError(t, GenKB(dir, out, nil))

	raw, err := os.ReadFile(filepath.Join(out, "xs-functions.json"))
	require.NoError(t, err)

	var fns []functionJSON
	require.NoError(t, json.Unmarshal(raw, &fns))

	byName := make(map[string]functionJSON, len(fns))
	for _, fn := range fns {
		byName[fn.Name] = fn
	}

	old := byName["xsGetLocalPlayerId"]
	assert.Equal(t, "xsUnsyncGetLocalPlayerId", old.RenamedTo, "the rename pair marks the old name")
	assert.Equal(t, stubCategory, old.Category)
	assert.Equal(t, "9", old.SinceUpdate)
	assert.Empty(t, old.Desc, "a rename stub carries no desc")

	newName := byName["xsUnsyncGetLocalPlayerId"]
	assert.Equal(t, "", newName.RenamedTo, "the guide record of the new name is not a stub")
	assert.Equal(t, "New name", newName.Desc, "guide-wins: the guide record survives untouched")
	assert.Equal(t, "9", newName.SinceUpdate, "the rename update dates the new name")

	ord := byName["ord"]
	assert.Equal(t, stubCategory, ord.Category)
	assert.Equal(t, "int", ord.ReturnType)
	require.Len(t, ord.Params, 1)
	assert.Equal(t, "character", ord.Params[0].Name)
	assert.Equal(t, "string", ord.Params[0].Type)
	assert.True(t, ord.Params[0].Required)
	assert.Equal(t, "9", ord.SinceUpdate)

	assert.NotContains(t, byName, "use", "AI-DSL syntax is not a function")
	assert.NotContains(t, byName, "include")
}

// TestGenKB_RenameMarksGuideRecord pins the guide-wins merge on the old
// name: when the guide still carries the renamed function, the record
// keeps its desc and gains renamed_to and the rename update as since.
func TestGenKB_RenameMarksGuideRecord(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeGenFile(t, dir, "ugc-guide/xs/functions/functions.json", `{
  "ui": [
    {"name": "xsGetOldThing", "return_type": "int", "params": [], "desc": "Old name"}
  ]
}`)

	writeGenFile(t, dir, "ugc-guide/xs/constants/constants.json", `{
  "terrain": [{"name": "TERRAIN_GRASS", "value": 0, "desc": "grass"}]
}`)

	writeGenFile(t, dir, "zetnus-rms-guide.txt", "Syntax Skeleton\n\n<PLAYER_SETUP>\n\ncreate_elevation N\n")

	writeGenFile(t, dir, changelogFile, `## Update 9
### XS: переименовано
- `+"`xsGetOldThing`"+` → `+"`xsGetNewThing`"+`
`)

	out := t.TempDir()
	require.NoError(t, GenKB(dir, out, nil))

	raw, err := os.ReadFile(filepath.Join(out, "xs-functions.json"))
	require.NoError(t, err)

	var fns []functionJSON
	require.NoError(t, json.Unmarshal(raw, &fns))

	byName := make(map[string]functionJSON, len(fns))
	for _, fn := range fns {
		byName[fn.Name] = fn
	}

	old := byName["xsGetOldThing"]
	assert.Equal(t, "Old name", old.Desc, "guide-wins: the guide desc survives")
	assert.Equal(t, "xsGetNewThing", old.RenamedTo)
	assert.Equal(t, "9", old.SinceUpdate)

	assert.NotContains(t, byName, "changelog stub for the old name")

	assert.Equal(t, "9", byName["xsGetNewThing"].SinceUpdate, "the new name is stubbed from the rename pair")
	assert.Equal(t, stubCategory, byName["xsGetNewThing"].Category)
}
