package kb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

// replaceGenFile overwrites rel under root.
func replaceGenFile(t *testing.T, root string, rel string, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644))
}
