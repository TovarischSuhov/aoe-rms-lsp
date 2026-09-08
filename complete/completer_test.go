package complete

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
)

// at returns the position offset bytes after the first occurrence of
// needle in src, as a line/column pair (Pos compares by line/column).
func at(src string, needle string, offset int) common.Pos {
	off := strings.Index(src, needle) + offset

	line, col := 0, 0

	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}

	return common.Pos{Line: uint32(line), Column: uint32(col)}
}

// newTestCompleter builds a completer over the real embedded store.
func newTestCompleter(t *testing.T) *Completer {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewCompleter(store)
}

func TestCompleter_Contract(t *testing.T) {
	c := newTestCompleter(t)

	require.IsType(t, &Completer{}, c)

	file, _ := rms.Parse("", "t.rms")

	require.IsType(t, []Candidate{}, c.RmsAt(file, common.Pos{}))
}

func TestRmsAt_CommandNamePosition(t *testing.T) {
	src := "<land_generation>\ncreate_land\n</land_generation>\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "create_land", 3))

	byLabel := make(map[string]Candidate)

	for _, c := range cands {
		byLabel[c.Label] = c
		require.NotEqual(t, KindConstant, c.Kind, "no constants at a name position")
	}

	land := byLabel["create_land"]
	require.Equal(t, KindCommand, land.Kind)
	require.Equal(t, "land_generation", land.Detail)
	require.Equal(t, "1create_land", land.Sort)

	attr := byLabel["terrain_type"]
	require.Equal(t, KindAttribute, attr.Kind)
	require.Equal(t, "0terrain_type", attr.Sort)

	require.Equal(t, "", byLabel["set_circular_base"].Detail, "flag attribute has no detail")

	_, other := byLabel["create_elevation"]
	require.False(t, other, "commands of other sections are out of scope")
}

func TestRmsAt_ArgValuePosition_Constants(t *testing.T) {
	src := "create_elevation 3"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "3", 0))

	require.NotEmpty(t, cands)

	for _, c := range cands {
		require.Equal(t, KindConstant, c.Kind, "value positions complete constants only")
	}

	byLabel := make(map[string]Candidate)

	for _, c := range cands {
		byLabel[c.Label] = c
	}

	blue := byLabel["cColorBlue"]
	require.Equal(t, "\"<BLUE>\"", blue.Detail)
	require.Equal(t, "2cColorBlue", blue.Sort)
}

func TestRmsAt_AttrNameVsValue(t *testing.T) {
	src := "create_elevation 3 {\n  spacing 5\n}\n"
	file, _ := rms.Parse(src, "t.rms")
	c := newTestCompleter(t)

	onName := c.RmsAt(file, at(src, "spacing", 2))
	onValue := c.RmsAt(file, at(src, "5", 0))

	require.NotEmpty(t, onName)
	require.NotEmpty(t, onValue)

	byLabel := make(map[string]Candidate)

	for _, cand := range onName {
		require.Equal(t, KindAttribute, cand.Kind, "attribute name completes owner attributes")
		byLabel[cand.Label] = cand
	}

	require.Equal(t, "0spacing", byLabel["spacing"].Sort)
	require.Equal(t, "number", byLabel["spacing"].Detail)

	for _, cand := range onValue {
		require.Equal(t, KindConstant, cand.Kind, "attribute value completes constants")
	}
}

func TestRmsAt_GlobalSection_AllCommands(t *testing.T) {
	src := "percent_chance 25\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "percent_chance", 2))

	labels := make(map[string]bool)

	for _, cand := range cands {
		if cand.Kind == KindCommand {
			labels[cand.Label] = true
		}
	}

	require.True(t, labels["percent_chance"], "the global-area command itself")
	require.True(t, labels["create_land"], "synthetic global widens to every section")
	require.True(t, labels["create_elevation"])
}

func TestRmsAt_NoContext_Empty(t *testing.T) {
	src := "#include \"a.rms\"\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "include", 2))

	require.Empty(t, cands, "directives outside sections complete nothing")
}

func TestRmsAt_UnknownOwner_NoAttributes(t *testing.T) {
	src := "create_landz 5\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "create_landz", 3))

	hasCommand, hasAttr := false, false

	for _, cand := range cands {
		hasCommand = hasCommand || cand.Kind == KindCommand
		hasAttr = hasAttr || cand.Kind == KindAttribute
	}

	require.True(t, hasCommand, "section commands still complete")
	require.False(t, hasAttr, "unknown commands own no attributes")
}

func TestRmsAt_AttributeWithoutValue_DefaultsToName(t *testing.T) {
	src := "create_elevation 3 {\n  set_scale_by_size\n}\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "set_scale_by_size", 3))

	require.NotEmpty(t, cands)

	for _, cand := range cands {
		require.Equal(t, KindAttribute, cand.Kind, "no value yet — name position defaults")
	}
}
