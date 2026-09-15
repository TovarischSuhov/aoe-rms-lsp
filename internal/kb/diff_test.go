package kb

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// diffFixtureOld and diffFixtureNew are two data directories with known
// differences: xsNew added, xsOld removed, xsKept param prose changed,
// TERRAIN_GRASS value changed, create_elevation arg kind changed and
// create_thing gained an arg (length mismatch).
const diffFixtureOld = `[
  {"name": "xsKept", "category": "vector", "return_type": "vector",
   "params": [{"name": "x", "type": "float", "required": true, "desc": "x coord"}],
   "desc": "Sets a vector", "since_update": "5"},
  {"name": "xsOld", "category": "math", "return_type": "int", "params": [],
   "desc": "old", "since_update": ""}
]`

const diffFixtureNew = `[
  {"name": "xsKept", "category": "vector", "return_type": "vector",
   "params": [{"name": "x", "type": "float", "required": true, "desc": "x coordinate"}],
   "desc": "Sets a vector", "since_update": "5"},
  {"name": "xsNew", "category": "math", "return_type": "int", "params": [],
   "desc": "new", "since_update": "9"}
]`

// writeDiffFixture writes the three data files of one variant into dir.
func writeDiffFixture(t *testing.T, dir string, functions string, constantValue string, elevationKind string, args string) {
	t.Helper()

	writeGenFile(t, dir, "xs-functions.json", functions)
	writeGenFile(t, dir, "xs-constants.json",
		`[{"name": "TERRAIN_GRASS", "section": "terrain", "value": `+constantValue+`,
		  "desc": "grass", "since_update": "5"}]`)
	writeGenFile(t, dir, "rms-commands.json", `[
  {"name": "create_elevation", "section": "elevation_generation",
   "args": [{"name": "N", "kind": "`+elevationKind+`", "range": {"min": "1", "max": "7"},
             "required": true, "desc": ""}],
   "attributes": [], "desc": "", "game_versions": "", "since_update": "7"},
  {"name": "create_thing", "section": "land_generation",
   "args": `+args+`, "attributes": [], "desc": "", "game_versions": "", "since_update": ""}
]`)
}

// diffFixtureDirs builds the old and new data directories of the fixture.
func diffFixtureDirs(t *testing.T) (string, string) {
	t.Helper()

	oldDir := t.TempDir()
	newDir := t.TempDir()

	writeDiffFixture(t, oldDir, diffFixtureOld, `"0"`, "number",
		`[{"name": "terrain", "kind": "terrain", "range": {}, "required": true, "desc": ""}]`)
	writeDiffFixture(t, newDir, diffFixtureNew, `"1"`, "percent",
		`[{"name": "terrain", "kind": "terrain", "range": {}, "required": true, "desc": ""},
		  {"name": "spacing", "kind": "number", "range": {}, "required": false, "desc": ""}]`)

	return oldDir, newDir
}

// TestDiffKB_Report pins the aggregated report over the fixture pair.
func TestDiffKB_Report(t *testing.T) {
	t.Parallel()

	oldDir, newDir := diffFixtureDirs(t)

	report, err := DiffKB(oldDir, newDir)

	require.Equal(t, DiffReport{
		Functions: EntityDiff{
			Added:   []string{"xsNew"},
			Removed: []string{"xsOld"},
			Changed: []ChangedEntity{{Name: "xsKept", Fields: []string{"params[x].desc"}}},
		},
		Constants: EntityDiff{
			Changed: []ChangedEntity{{Name: "TERRAIN_GRASS", Fields: []string{"value"}}},
		},
		Commands: EntityDiff{
			Changed: []ChangedEntity{
				{Name: "create_elevation", Fields: []string{"args[N].kind"}},
				{Name: "create_thing", Fields: []string{"args"}},
			},
		},
	}, report)
	require.NoError(t, err)
}

// TestDiffKB_IdenticalDirs checks that a directory diffed against itself is
// empty and renders the no-changes report.
func TestDiffKB_IdenticalDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeDiffFixture(t, dir, diffFixtureOld, `"0"`, "number", `[]`)

	report, err := DiffKB(dir, dir)

	require.NoError(t, err)
	require.True(t, report.Empty())
	require.Equal(t, "# KB diff\n\nNo changes.\n", report.Render())
}

// TestDiffKB_RealDataIdentical guards the refresh invariant: the embedded
// data directory diffed against itself never reports changes.
func TestDiffKB_RealDataIdentical(t *testing.T) {
	t.Parallel()

	report, err := DiffKB("data", "data")

	require.NoError(t, err)
	require.True(t, report.Empty())
}

// TestDiffKB_DuplicateNames checks the uniqueness guard carries into the
// diff loader.
func TestDiffKB_DuplicateNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGenFile(t, dir, "xs-functions.json", `[
  {"name": "dup", "category": "", "return_type": "", "params": [], "desc": "", "since_update": ""},
  {"name": "dup", "category": "", "return_type": "", "params": [], "desc": "", "since_update": ""}
]`)
	writeGenFile(t, dir, "xs-constants.json", `[]`)
	writeGenFile(t, dir, "rms-commands.json", `[]`)

	_, err := DiffKB(dir, dir)

	require.ErrorContains(t, err, "duplicate")
}

// TestDiffKB_RenderGolden pins the deterministic markdown render.
func TestDiffKB_RenderGolden(t *testing.T) {
	t.Parallel()

	oldDir, newDir := diffFixtureDirs(t)

	report, err := DiffKB(oldDir, newDir)
	require.NoError(t, err)

	golden := filepath.Join("testdata", "diff_report.md")

	if *updateGolden {
		require.NoError(t, os.WriteFile(golden, []byte(report.Render()), 0o644))
		return
	}

	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, string(want), report.Render())
}
