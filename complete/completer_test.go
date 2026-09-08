package complete

import (
	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	t.Parallel()
	c := newTestCompleter(t)

	require.IsType(t, &Completer{}, c)

	file, _ := rms.Parse("", "t.rms")

	require.IsType(t, []Candidate{}, c.RmsAt(file, common.Pos{}))
}

func TestRmsAt_CommandNamePosition(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	src := "#include \"a.rms\"\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "include", 2))

	require.Empty(t, cands, "directives outside sections complete nothing")
}

func TestRmsAt_UnknownOwner_NoAttributes(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	src := "create_elevation 3 {\n  set_scale_by_size\n}\n"
	file, _ := rms.Parse(src, "t.rms")

	cands := newTestCompleter(t).RmsAt(file, at(src, "set_scale_by_size", 3))

	require.NotEmpty(t, cands)

	for _, cand := range cands {
		require.Equal(t, KindAttribute, cand.Kind, "no value yet — name position defaults")
	}
}

func TestCompleter_XsAt_Contract(t *testing.T) {
	t.Parallel()
	c := newTestCompleter(t)

	file, _ := xs.XsParse("void f() {}", "t.xs")

	require.IsType(t, []Candidate{}, c.XsAt(file, common.Pos{Line: 0, Column: 6}, nil))
}

func TestXsAt_SourceShadowsKb(t *testing.T) {
	t.Parallel()
	src := "int xsGetGoal() { return 1; }"
	file, _ := xs.XsParse(src, "t.xs")

	cands := newTestCompleter(t).XsAt(file, at(src, "return", 0), nil)

	var got []Candidate

	for _, cand := range cands {
		if cand.Label == "xsGetGoal" {
			got = append(got, cand)
		}
	}

	require.Len(t, got, 1, "source wins over the same-name kb entry")

	require.Equal(t, KindFunction, got[0].Kind)
	require.Equal(t, "int xsGetGoal()", got[0].Detail, "detail from the source declaration")
	require.Equal(t, "0xsGetGoal", got[0].Sort)
}

func TestXsAt_ExternalDeclsMerged(t *testing.T) {
	t.Parallel()
	src := "void main() { }"
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{
		{
			Kind: xs.DeclFunction, Name: "helper", Type: "void",
			Params: []xs.Param{{Name: "b", Type: "bool"}},
		},
		{Kind: xs.DeclInclude, Name: "z.xs"},
	}

	cands := newTestCompleter(t).XsAt(file, at(src, "main", 2), external)

	byLabel := make(map[string]Candidate)

	for _, cand := range cands {
		byLabel[cand.Label] = cand
	}

	require.Equal(t, KindFunction, byLabel["helper"].Kind)
	require.Equal(t, "void helper(bool)", byLabel["helper"].Detail)
	require.Equal(t, "0helper", byLabel["helper"].Sort)

	_, included := byLabel["z.xs"]
	require.False(t, included, "include declarations are not addressable")

	require.Equal(t, "int xsGetGoal(int)", byLabel["xsGetGoal"].Detail)
	require.Equal(t, "1xsGetGoal", byLabel["xsGetGoal"].Sort)

	require.Equal(t, "2cColorBlue", byLabel["cColorBlue"].Sort)
}

func TestXsAt_VisibleAtFalse_Empty(t *testing.T) {
	t.Parallel()
	src := "void f() { string s = \"ab\"; }"
	file, _ := xs.XsParse(src, "t.xs")

	cands := newTestCompleter(t).XsAt(file, at(src, "ab", 0), nil)

	require.Empty(t, cands, "strings answer silence")
}

func TestXsAt_SameNameDifferentKinds_BothKept(t *testing.T) {
	t.Parallel()
	src := "int foo = 1;"
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{{Kind: xs.DeclFunction, Name: "foo", Type: "void"}}

	cands := newTestCompleter(t).XsAt(file, at(src, "foo", 1), external)

	var kinds []string

	for _, cand := range cands {
		if cand.Label == "foo" {
			kinds = append(kinds, cand.Kind)
			require.Equal(t, "0foo", cand.Sort)
		}
	}

	require.ElementsMatch(t, []string{KindVariable, KindFunction}, kinds)
}

func TestXsAt_ExternalNoReturnType_DetailWithoutRet(t *testing.T) {
	t.Parallel()
	src := "void main() { }"
	file, _ := xs.XsParse(src, "t.xs")

	external := []xs.Decl{{
		Kind: xs.DeclFunction, Name: "foo",
		Params: []xs.Param{{Name: "a", Type: "int"}},
	}}

	cands := newTestCompleter(t).XsAt(file, at(src, "main", 2), external)

	byLabel := make(map[string]Candidate)

	for _, cand := range cands {
		byLabel[cand.Label] = cand
	}

	require.Equal(t, "foo(int)", byLabel["foo"].Detail, "no invented return type")
}

func TestXsAt_RuleExcludedExternIsFunction(t *testing.T) {
	t.Parallel()
	src := "rule r { condition 1 }\nextern int ex();"
	file, _ := xs.XsParse(src, "t.xs")

	cands := newTestCompleter(t).XsAt(file, at(src, "extern", 2), nil)

	byLabel := make(map[string]Candidate)

	for _, cand := range cands {
		byLabel[cand.Label] = cand
	}

	_, ruled := byLabel["r"]
	require.False(t, ruled, "rule names are not addressable")

	require.Equal(t, KindFunction, byLabel["ex"].Kind, "extern maps to function")
	require.Equal(t, "int ex()", byLabel["ex"].Detail)
}

func TestXsAt_ParamAndLocalEmptyDetail(t *testing.T) {
	t.Parallel()
	src := "void f(int p) {\n  int loc = 1;\n  loc = p;\n}\n"
	file, _ := xs.XsParse(src, "t.xs")

	cands := newTestCompleter(t).XsAt(file, at(src, "loc = p", 1), nil)

	byLabel := make(map[string]Candidate)

	for _, cand := range cands {
		byLabel[cand.Label] = cand
	}

	require.Equal(t, KindParam, byLabel["p"].Kind)
	require.Empty(t, byLabel["p"].Detail, "no type data on symbols — nothing invented")
	require.Equal(t, KindLocal, byLabel["loc"].Kind)
	require.Empty(t, byLabel["loc"].Detail)
}

func TestXsAt_ForInitCandidate(t *testing.T) {
	t.Parallel()
	src := "void main() {\n" +
		"  for (int i = 0; i < 10; i++) {\n" +
		"    i = i + 1;\n" +
		"  }\n" +
		"}"
	file, _ := xs.XsParse(src, "for.xs")

	// cursor on the i assignment inside the loop body
	cands := newTestCompleter(t).XsAt(file, at(src, "i = i", 4), nil)

	var got []Candidate

	for _, cand := range cands {
		if cand.Label == "i" {
			got = append(got, cand)
		}
	}

	require.Len(t, got, 1, "the loop variable comes from the source pool")

	require.Equal(t, KindLocal, got[0].Kind)
	require.Equal(t, "0i", got[0].Sort, "source group sorts first")
}
