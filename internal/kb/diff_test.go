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

// renamedToFixtureDirs builds the old and new data directories of the
// renamed_to fixture: one record of each entity kind (issue #215),
// identical except that the two copies carry different renamed_to
// markers.
func renamedToFixtureDirs(t *testing.T, oldRenamed string, newRenamed string) (string, string) {
	t.Helper()

	write := func(dir string, renamed string) {
		writeGenFile(t, dir, "xs-functions.json", `[
		  {"name": "xsFoo", "category": "vector", "return_type": "int", "params": [],
		   "desc": "Sets a vector", "since_update": "5", "renamed_to": "`+renamed+`"}
		]`)
		writeGenFile(t, dir, "xs-constants.json", `[
		  {"name": "TERRAIN_GRASS", "section": "terrain", "value": "0",
		   "desc": "grass", "since_update": "5", "renamed_to": "`+renamed+`"}
		]`)
		writeGenFile(t, dir, "rms-commands.json", `[
		  {"name": "create_elevation", "section": "elevation_generation",
		   "args": [{"name": "N", "kind": "number", "range": {"min": "1", "max": "7"},
		             "required": true, "desc": ""}],
		   "attributes": [], "desc": "", "game_versions": "", "since_update": "7",
		   "renamed_to": "`+renamed+`"}
		]`)
	}

	oldDir := t.TempDir()
	newDir := t.TempDir()
	write(oldDir, oldRenamed)
	write(newDir, newRenamed)

	return oldDir, newDir
}

// TestDiffKB_RenamedTo pins the renamed_to comparison of all three entity
// kinds: every marker transition — absent→set, set→absent, redirected —
// is a change with the exact field path "renamed_to", reported alone in
// Fields, making the report non-empty and rendered (issue #215).
func TestDiffKB_RenamedTo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		oldRenamed string
		newRenamed string
	}{
		{name: "absent_to_set", oldRenamed: "", newRenamed: "xsBar"},
		{name: "set_to_absent", oldRenamed: "xsBar", newRenamed: ""},
		{name: "redirected", oldRenamed: "xsA", newRenamed: "xsB"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldDir, newDir := renamedToFixtureDirs(t, tt.oldRenamed, tt.newRenamed)

			report, err := DiffKB(oldDir, newDir)

			require.NoError(t, err)
			require.Equal(t, DiffReport{
				Functions: EntityDiff{
					Changed: []ChangedEntity{{Name: "xsFoo", Fields: []string{"renamed_to"}}},
				},
				Constants: EntityDiff{
					Changed: []ChangedEntity{{Name: "TERRAIN_GRASS", Fields: []string{"renamed_to"}}},
				},
				Commands: EntityDiff{
					Changed: []ChangedEntity{{Name: "create_elevation", Fields: []string{"renamed_to"}}},
				},
			}, report)
			require.False(t, report.Empty())

			rendered := report.Render()
			require.Contains(t, rendered, "- ~ xsFoo: renamed_to")
			require.Contains(t, rendered, "- ~ TERRAIN_GRASS: renamed_to")
			require.Contains(t, rendered, "- ~ create_elevation: renamed_to")
			require.NotContains(t, rendered, "No changes.")
		})
	}
}

// TestDiffKB_RenamedToUnchanged pins the negative side of the comparison:
// equal markers — both empty or both the same non-empty value — are never
// reported, in any of the three entity kinds (issue #215).
func TestDiffKB_RenamedToUnchanged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		renamed string
	}{
		{name: "both_empty", renamed: ""},
		{name: "both_same_non_empty", renamed: "xsBar"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldDir, newDir := renamedToFixtureDirs(t, tt.renamed, tt.renamed)

			report, err := DiffKB(oldDir, newDir)

			require.NoError(t, err)
			require.Equal(t, DiffReport{}, report)
			require.True(t, report.Empty())
			require.Equal(t, "# KB diff\n\nNo changes.\n", report.Render())
		})
	}
}

// TestDiffKB_RenamedToWithOtherFields pins the fixed field order of a
// combined change: renamed_to lands last in Fields, after desc and
// since_update, and the render joins the paths with ", " (issue #215).
func TestDiffKB_RenamedToWithOtherFields(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, dir string, renamed string, desc string, since string) {
		t.Helper()

		writeGenFile(t, dir, "xs-functions.json", `[
		  {"name": "xsFoo", "category": "vector", "return_type": "int", "params": [],
		   "desc": "`+desc+`", "since_update": "`+since+`", "renamed_to": "`+renamed+`"}
		]`)
		writeGenFile(t, dir, "xs-constants.json", `[]`)
		writeGenFile(t, dir, "rms-commands.json", `[]`)
	}

	cases := []struct {
		name       string
		oldDesc    string
		newDesc    string
		oldSince   string
		newSince   string
		fields     []string
		renderLine string
	}{
		{name: "desc_and_renamed_to",
			oldDesc: "Sets a vector", newDesc: "Keeps a vector",
			oldSince: "5", newSince: "5",
			fields:     []string{"desc", "renamed_to"},
			renderLine: "- ~ xsFoo: desc, renamed_to"},
		{name: "desc_since_update_and_renamed_to",
			oldDesc: "Sets a vector", newDesc: "Keeps a vector",
			oldSince: "5", newSince: "9",
			fields:     []string{"desc", "since_update", "renamed_to"},
			renderLine: "- ~ xsFoo: desc, since_update, renamed_to"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldDir := t.TempDir()
			newDir := t.TempDir()
			write(t, oldDir, "xsA", tt.oldDesc, tt.oldSince)
			write(t, newDir, "xsB", tt.newDesc, tt.newSince)

			report, err := DiffKB(oldDir, newDir)

			require.NoError(t, err)
			require.Equal(t, EntityDiff{
				Changed: []ChangedEntity{{Name: "xsFoo", Fields: tt.fields}},
			}, report.Functions)
			require.False(t, report.Empty())

			rendered := report.Render()
			require.Contains(t, rendered, tt.renderLine)
			require.NotContains(t, rendered, "No changes.")
		})
	}
}
