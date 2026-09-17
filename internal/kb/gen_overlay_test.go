package kb

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overlayGuide is a guide skeleton whose create_elevation block carries one
// undocumented attribute and one positional argument: no doc block anywhere,
// so both descs start empty. The skeleton is terminated like the real guide
// export, so the lines behind it stay out of the skeleton pass.
const overlayGuide = `Syntax Skeleton

<PLAYER_SETUP>

create_elevation N
{
	flag_attr
}
________________
`

// overlayGuideDocumented adds a glossary block for the attribute behind the
// skeleton, so the guide text — not the overlay — must win.
const overlayGuideDocumented = overlayGuide + `
flag_attr
Game versions: All

Guide text.
`

// TestGenKB_OverlayFillsEmptyDesc covers the overlay application rule: it
// fills the descs extraction left empty — attributes by name, command
// arguments by index — and never overrides a text the guide already
// provided.
func TestGenKB_OverlayFillsEmptyDesc(t *testing.T) {
	t.Parallel()

	t.Run("fills undocumented attribute and argument", func(t *testing.T) {
		t.Parallel()

		refDir := genFixture(t)
		replaceGenFile(t, refDir, "zetnus-rms-guide.txt", overlayGuide)
		writeGenFile(t, refDir, "attribute-descs.json", `{
		  "attributes": {"flag_attr": "Manual text."},
		  "command_args": {"create_elevation": ["Positional arg text."]}
		}`)

		commands := genCommands(t, refDir, nil)
		require.Len(t, commands, 1)
		require.Len(t, commands[0].Attributes, 1)
		require.Len(t, commands[0].Args, 1)

		assert.Equal(t, "Manual text.", commands[0].Attributes[0].Desc)
		assert.Equal(t, "Positional arg text.", commands[0].Args[0].Desc)
	})

	t.Run("guide text wins over the overlay", func(t *testing.T) {
		t.Parallel()

		refDir := genFixture(t)
		replaceGenFile(t, refDir, "zetnus-rms-guide.txt", overlayGuideDocumented)
		writeGenFile(t, refDir, "attribute-descs.json", `{"attributes": {"flag_attr": "Overlay text."}}`)

		commands := genCommands(t, refDir, nil)
		require.Len(t, commands, 1)
		require.Len(t, commands[0].Attributes, 1)

		assert.Equal(t, "Guide text.", commands[0].Attributes[0].Desc)
	})
}

// TestGenKB_OverlayDescMined proves an overlay desc goes through the same
// mining pass as the extraction output: bounds in the overlay text fill the
// empty Range of the generated record.
func TestGenKB_OverlayDescMined(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	replaceGenFile(t, refDir, "zetnus-rms-guide.txt", overlayGuide)
	writeGenFile(t, refDir, "attribute-descs.json", `{"attributes": {"flag_attr": "number (0-9)"}}`)

	commands := genCommands(t, refDir, nil)
	require.Len(t, commands, 1)
	require.Len(t, commands[0].Attributes, 1)

	assert.Equal(t, ValueRange{Min: "0", Max: "9"}, commands[0].Attributes[0].Range)
}

// TestGenKB_OverlayArgsIndexBounds covers the positional guard of the
// command_args overlay: a list shorter than the argument run fills the
// matching prefix and skips the rest without failing the generation.
func TestGenKB_OverlayArgsIndexBounds(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	replaceGenFile(t, refDir, "zetnus-rms-guide.txt", `Syntax Skeleton

<PLAYER_SETUP>

create_elevation N N
{
	flag_attr
}
________________
`)
	writeGenFile(t, refDir, "attribute-descs.json",
		`{"command_args": {"create_elevation": ["Only the first."]}}`)

	commands := genCommands(t, refDir, nil)
	require.Len(t, commands, 1)
	require.Len(t, commands[0].Args, 2)

	assert.Equal(t, "Only the first.", commands[0].Args[0].Desc)
	assert.Empty(t, commands[0].Args[1].Desc, "no overlay entry, nothing invented")
}

// TestGenKB_OverlayInvalidJSON checks the decode error path: a broken
// overlay aborts the generation with the overlay file named.
func TestGenKB_OverlayInvalidJSON(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	writeGenFile(t, refDir, "attribute-descs.json", `{"attributes": `)

	err := GenKB(refDir, t.TempDir(), nil)

	require.ErrorContains(t, err, "attribute-descs")
}

// TestGenKB_OverlayCoverageLog covers the coverage report wiring: a missing
// overlay file is not an error, and every name still without a desc is
// reported with a WARN naming it.
func TestGenKB_OverlayCoverageLog(t *testing.T) {
	t.Parallel()

	refDir := genFixture(t)
	replaceGenFile(t, refDir, "zetnus-rms-guide.txt", overlayGuide)

	var buf bytes.Buffer
	err := GenKB(refDir, t.TempDir(), slog.New(slog.NewTextHandler(&buf, nil)))

	require.NoError(t, err, "missing overlay file is not an error")

	assert.Contains(t, buf.String(), "flag_attr", "uncovered name is reported")
	assert.Contains(t, buf.String(), "WARN")
}

// TestDescCoverage pins the coverage counters of the generated data:
// totals count instances, filled counts the non-empty descs, and the empty
// list holds the unique attribute names without a desc, sorted.
func TestDescCoverage(t *testing.T) {
	t.Parallel()
	commands := []Command{
		{
			Name: "filled_cmd",
			Args: []CommandArg{
				{Name: "a", Desc: "documented"},
				{Name: "b", Desc: ""},
			},
			Attributes: []CommandArg{
				{Name: "shared", Desc: "documented"},
				{Name: "gap", Desc: ""},
			},
		},
		{
			Name:       "empty_cmd",
			Args:       []CommandArg{{Name: "c", Desc: "documented"}},
			Attributes: []CommandArg{{Name: "zeta", Desc: ""}, {Name: "alpha", Desc: ""}, {Name: "gap", Desc: ""}},
		},
	}

	cov := descCoverage(commands)

	assert.Equal(t, 5, cov.attrsTotal)
	assert.Equal(t, 1, cov.attrsFilled)
	assert.Equal(t, 3, cov.argsTotal)
	assert.Equal(t, 2, cov.argsFilled)
	assert.Equal(t, []string{"alpha", "gap", "zeta"}, cov.emptyNames)
}

// genCommands generates the kb over the fixture and returns the decoded
// rms-commands.json.
func genCommands(t *testing.T, refDir string, log *slog.Logger) []commandJSON {
	t.Helper()

	dataDir := t.TempDir()
	require.NoError(t, GenKB(refDir, dataDir, log))

	return readGenJSON[commandJSON](t, filepath.Join(dataDir, "rms-commands.json"))
}
