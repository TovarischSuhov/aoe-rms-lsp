package rms

import (
	"aoe2-lsp/internal/common"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRmsSymbols_APIShape(t *testing.T) {
	t.Parallel()
	file, _ := Parse("<LAND_GENERATION>\n</LAND_GENERATION>\n", "shape.rms")

	syms := file.Symbols()

	require.IsType(t, []common.Symbol{}, syms)
	require.Len(t, syms, 1)
	require.IsType(t, "", syms[0].Kind)
	require.IsType(t, common.Range{}, syms[0].Selection)
}

func TestRmsReferencesAt_APIShape(t *testing.T) {
	t.Parallel()
	file, _ := Parse("create_terrain FOREST\n", "shape.rms")

	ranges := file.ReferencesAt(common.Pos{Line: 0, Column: 15})

	require.IsType(t, []common.Range{}, ranges)
}

func TestRmsSymbols_SectionTreeWithNestedBlocks(t *testing.T) {
	t.Parallel()
	src := "<LAND_GENERATION>\n" +
		"create_player_lands {\n" +
		"	terrain_type DIRT\n" +
		"}\n" +
		"start_random\n" +
		"	percent_chance 50\n" +
		"	create_land TERRAIN_GRASS\n" +
		"end_random\n" +
		"</LAND_GENERATION>\n"

	file, _ := Parse(src, "tree.rms")

	syms := file.Symbols()

	require.Len(t, syms, 1)

	section := syms[0]
	require.Equal(t, "section", section.Kind)
	require.Equal(t, "land_generation", section.Name)
	require.Equal(t, uint32(1), section.Selection.Start.Column,
		"selection is the name inside the angle brackets")
	require.Len(t, section.Children, 2)

	lands := section.Children[0]
	require.Equal(t, "command", lands.Kind)
	require.Equal(t, "create_player_lands", lands.Name)
	require.Empty(t, lands.Children, "attributes are not outline nodes")

	random := section.Children[1]
	require.Equal(t, "start_random", random.Name)

	chance := random.Children[0]
	require.Equal(t, "percent_chance", chance.Name)
	require.Equal(t, "create_land", chance.Children[0].Name)
}

func TestRmsSymbols_GlobalStatementsAtRoot(t *testing.T) {
	t.Parallel()
	src := "create_land TERRAIN_GRASS\n" +
		"<LAND_GENERATION>\n" +
		"base_terrain GRASS\n" +
		"</LAND_GENERATION>\n"

	file, _ := Parse(src, "global.rms")

	syms := file.Symbols()

	require.Len(t, syms, 2)
	require.Equal(t, "command", syms[0].Kind)
	require.Equal(t, "create_land", syms[0].Name)
	require.Equal(t, "section", syms[1].Kind)

	for _, node := range syms {
		require.NotEqual(t, "global", node.Name, "no global section node")
	}
}

func TestRmsSymbols_XsBlockPlacement(t *testing.T) {
	t.Parallel()
	src := "<OBJECTS_GENERATION>\n" +
		"create_object VILLAGER\n" +
		"#includeXS\n" +
		"void f() {}\n" +
		"</OBJECTS_GENERATION>\n" +
		"#includeXS\n" +
		"void g() {}\n"

	file, _ := Parse(src, "blocks.rms")

	syms := file.Symbols()

	require.Len(t, syms, 2, "the section plus the post-section block at root")

	var section common.Symbol

	for _, node := range syms {
		if node.Kind == "section" {
			section = node
		}
	}

	require.Equal(t, "objects_generation", section.Name)
	require.True(t, section.Range.Start.Before(syms[1].Range.Start),
		"root nodes stay position-ordered")

	xsChildren := symbolsNamed(section.Children, "#includeXS")
	require.Len(t, xsChildren, 1, "the embedded block nests into its section")
	require.Equal(t, "xs", xsChildren[0].Kind)
	require.Equal(t, file.XsBlocks[0].Range, xsChildren[0].Range)
	require.Equal(t, xsChildren[0].Range, xsChildren[0].Selection)

	require.Len(t, symbolsNamed(syms, "#includeXS"), 1, "the outside block is a root node")
	require.Len(t, symbolsNamed(section.Children, "create_object"), 1,
		"command children and the xs child interleave by position")
}

func TestRmsSymbols_OutsideSectionXsBlockAtRoot(t *testing.T) {
	t.Parallel()
	src := "<OBJECTS_GENERATION>\n" +
		"</OBJECTS_GENERATION>\n" +
		"#includeXS\n" +
		"void g() {}\n"

	file, _ := Parse(src, "rootblock.rms")

	syms := file.Symbols()

	require.Len(t, syms, 2)
	require.Equal(t, "section", syms[0].Kind)
	require.Equal(t, "xs", syms[1].Kind)
	require.Equal(t, file.XsBlocks[0].Range, syms[1].Range)
}

func TestRmsReferencesAt_AllWordKinds(t *testing.T) {
	t.Parallel()
	src := "<LAND_GENERATION>\n" +
		"create_terrain FOREST\n" +
		"create_terrain FOREST\n" +
		"</LAND_GENERATION>\n"

	lines := strings.Split(src, "\n")
	file, _ := Parse(src, "words.rms")

	forest2 := strings.Index(lines[2], "FOREST")

	ranges := file.ReferencesAt(common.Pos{Line: 2, Column: uint32(forest2)})

	require.Len(t, ranges, 2, "both const occurrences")

	for i := 1; i < len(ranges); i++ {
		require.True(t, ranges[i-1].Start.Before(ranges[i].Start), "sorted")
	}

	require.Equal(t, uint32(1), ranges[0].Start.Line)
	require.Equal(t, uint32(2), ranges[1].Start.Line)

	cmds := file.ReferencesAt(common.Pos{Line: 1, Column: uint32(strings.Index(lines[1], "create_terrain"))})
	require.Len(t, cmds, 2, "command name references work too")
}

func TestRmsReferencesAt_NumberEmpty(t *testing.T) {
	t.Parallel()
	src := "land_percent 12\n"

	file, _ := Parse(src, "number.rms")

	require.Empty(t, file.ReferencesAt(common.Pos{Line: 0, Column: uint32(strings.Index(src, "12"))}))
}

// TestRmsWordAt pins the recorded-name contract of the word index
// (issue #204, AC1/AC2): WordAt answers the name recorded at parse time
// for the word token containing pos — the exact key References and
// ReferencesAt answer — never a slice of the raw source. Section names
// are recorded lowercase whatever their source spelling; const/ident
// values keep their written case; non-word positions (numbers, gaps,
// header punctuation) answer ("", false) — containment, never the
// nearest word.
func TestRmsWordAt(t *testing.T) {
	t.Parallel()
	src := "<LAND_GENERATION>\n" +
		"create_terrain FOREST plains {\n" +
		"  base_terrain GRASS\n" +
		"}\n" +
		"land_percent 12\n" +
		"</LAND_GENERATION>\n"

	file, _ := Parse(src, "wordat.rms")

	tests := []struct {
		name  string
		pos   common.Pos
		want  string
		found bool
	}{
		{
			name:  "opening section name token answers the recorded lowercase name (AC2)",
			pos:   linePos(src, 0, "LAND_GENERATION", 0),
			want:  "land_generation",
			found: true,
		},
		{
			name:  "mid-word position answers the same recorded name",
			pos:   linePos(src, 0, "LAND_GENERATION", 5),
			want:  "land_generation",
			found: true,
		},
		{
			name:  "closing section name token answers the recorded lowercase name",
			pos:   linePos(src, 5, "LAND_GENERATION", 0),
			want:  "land_generation",
			found: true,
		},
		{
			name:  "command name word",
			pos:   linePos(src, 1, "create_terrain", 0),
			want:  "create_terrain",
			found: true,
		},
		{
			name:  "const value word keeps its written case",
			pos:   linePos(src, 1, "FOREST", 0),
			want:  "FOREST",
			found: true,
		},
		{
			name:  "ident value word",
			pos:   linePos(src, 1, "plains", 0),
			want:  "plains",
			found: true,
		},
		{
			name:  "attribute name word",
			pos:   linePos(src, 2, "base_terrain", 0),
			want:  "base_terrain",
			found: true,
		},
		{
			name:  "number literal is not an indexed word",
			pos:   linePos(src, 4, "12", 0),
			found: false,
		},
		{
			name:  "inter-token gap is not an indexed word",
			pos:   linePos(src, 1, " FOREST", 0),
			found: false,
		},
		{
			name:  "angle bracket of a header is not the section word",
			pos:   linePos(src, 0, "<", 0),
			found: false,
		},
		{
			name:  "closing slash of a closing tag is not the section word",
			pos:   linePos(src, 5, "/", 0),
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := file.WordAt(tt.pos)

			require.Equal(t, tt.found, found)
			require.Equal(t, tt.want, got)

			// The recorded name is the exact References key (AC4):
			// ReferencesAt(pos) delegates to it by construction.
			if tt.found {
				require.Equal(t, file.References(got), file.ReferencesAt(tt.pos))
			}
		})
	}
}

// rawPos builds the position of needle on the given line of the raw
// source: Line/Column address the normalized view every AST Range uses,
// while Offset is derived from the raw bytes — the pair an editor
// naively reports for a CRLF file (issue #204).
func rawPos(src string, line int, needle string) common.Pos {
	rawLines := strings.Split(src, "\n")
	col := strings.Index(rawLines[line], needle)

	offset := col
	for i := 0; i < line; i++ {
		offset += len(rawLines[i]) + 1
	}

	return common.Pos{Line: uint32(line), Column: uint32(col), Offset: offset}
}

// TestRmsWordAt_CRLFParity pins the EOL invariance of WordAt (issue
// #204, AC3): a CRLF source answers with the same recorded names at the
// same Line/Column positions as its LF twin, even when the query carries
// the raw-bytes Offset — containment compares Line/Column only
// (internal/common/range.go), so an offset-based lookup must never leak
// into the answer.
func TestRmsWordAt_CRLFParity(t *testing.T) {
	t.Parallel()
	lfSrc := "<LAND_GENERATION>\n" +
		"create_terrain FOREST\n" +
		"</LAND_GENERATION>\n"
	crlfSrc := strings.ReplaceAll(lfSrc, "\n", "\r\n")

	lf, _ := Parse(lfSrc, "lf.rms")
	crlf, _ := Parse(crlfSrc, "crlf.rms")

	probes := []struct {
		name   string
		line   int
		needle string
		want   string
		found  bool
	}{
		{"opening section name", 0, "LAND_GENERATION", "land_generation", true},
		{"command name", 1, "create_terrain", "create_terrain", true},
		{"const value", 1, "FOREST", "FOREST", true},
		{"closing section name", 2, "LAND_GENERATION", "land_generation", true},
		{"inter-token gap", 1, " ", "", false},
	}

	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			lfPos := rawPos(lfSrc, p.line, p.needle)
			crlfPos := rawPos(crlfSrc, p.line, p.needle)

			require.Equal(t, lfPos.Line, crlfPos.Line, "fixture sanity: same line")
			require.Equal(t, lfPos.Column, crlfPos.Column, "fixture sanity: same column")
			if p.line > 0 {
				require.Greater(t, crlfPos.Offset, lfPos.Offset,
					"fixture sanity: the raw CRLF offset diverges from the normalized offset")
			}

			lfName, lfFound := lf.WordAt(lfPos)
			crlfName, crlfFound := crlf.WordAt(crlfPos)

			require.Equal(t, p.found, lfFound, "LF twin baseline")
			require.Equal(t, p.found, crlfFound, "CRLF answers found on the same Line/Column")
			require.Equal(t, lfName, crlfName, "CRLF answers the same recorded name as the LF twin")
			require.Equal(t, p.want, crlfName)
		})
	}
}

// symbolsNamed filters outline nodes by name.
func symbolsNamed(nodes []common.Symbol, name string) []common.Symbol {
	out := make([]common.Symbol, 0)

	for _, n := range nodes {
		if n.Name == name {
			out = append(out, n)
		}
	}

	return out
}

func TestRmsSymbols_FixturesInvariantSweep(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	var check func(n common.Symbol)

	check = func(n common.Symbol) {
		require.True(t, rangeWithin(n.Selection, n.Range),
			"Selection must stay inside Range: %s", n.Name)
		require.NotEqual(t, "global", n.Name, "no global section node")

		for _, child := range n.Children {
			require.True(t, rangeWithin(child.Range, n.Range),
				"children must stay inside the parent Range: %s", n.Name)
			check(child)
		}
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".rms") {
			continue
		}

		raw, err := os.ReadFile("testdata/" + entry.Name())
		require.NoError(t, err)

		file, _ := Parse(string(raw), entry.Name())

		for _, root := range file.Symbols() {
			check(root)
		}
	}
}

// rangeWithin reports whether inner lies inside outer.
func rangeWithin(inner common.Range, outer common.Range) bool {
	return !inner.Start.Before(outer.Start) && !outer.End.Before(inner.End)
}

// TestRmsArgAt_APIShape pins the argument-context contract surface: the
// ArgAt method on RmsFile returning the exported ArgSite with the Kind*
// vocabulary.
func TestRmsArgAt_APIShape(t *testing.T) {
	t.Parallel()
	src := "create_elevator PLAYER_1 5"
	file, _ := Parse(src, "shape.rms")

	site, found := file.ArgAt(common.Pos{Line: 0, Column: uint32(strings.Index(src, "PLAYER_1"))})

	require.True(t, found)
	require.IsType(t, ArgSite{}, site)
	require.IsType(t, Statement{}, site.Stmt)
	require.Equal(t, KindArg, site.Kind)
	require.Equal(t, 0, site.Index)
	require.Equal(t, "", site.Name)
	require.Equal(t, "create_elevator", site.Stmt.Name)
	require.Equal(t, "arg", KindArg)
	require.Equal(t, "attr", KindAttr)
	require.Equal(t, "none", KindNone)
}

// linePos builds the position of needle's first occurrence on the given
// line of a fixture (offset shifts within the needle).
func linePos(src string, line int, needle string, offset int) common.Pos {
	lines := strings.Split(src, "\n")

	return common.Pos{Line: uint32(line), Column: uint32(strings.Index(lines[line], needle) + offset)}
}

// TestArgAt_OnPositionalArg covers baseline positional-argument
// discrimination.
func TestArgAt_OnPositionalArg(t *testing.T) {
	t.Parallel()
	src := "create_elevator PLAYER_1 5"
	file, _ := Parse(src, "t.rms")

	site, found := file.ArgAt(linePos(src, 0, "PLAYER_1", 0))

	require.True(t, found)
	require.Equal(t, KindArg, site.Kind)
	require.Equal(t, 0, site.Index)
	require.Equal(t, "create_elevator", site.Stmt.Name)
}

// TestArgAt_OnAttributeValueAndName covers attribute discrimination on
// the value and on the name (name-match identity; flag attributes work
// through the name branch).
func TestArgAt_OnAttributeValueAndName(t *testing.T) {
	t.Parallel()
	src := "create_elevator A {\n  number_of_objects 5\n}\n"
	file, _ := Parse(src, "t.rms")

	site, found := file.ArgAt(linePos(src, 1, "5", 0))
	require.True(t, found)
	require.Equal(t, KindAttr, site.Kind)
	require.Equal(t, "number_of_objects", site.Name)
	require.Equal(t, "create_elevator", site.Stmt.Name)

	site, found = file.ArgAt(linePos(src, 1, "number_of_objects", 0))
	require.True(t, found)
	require.Equal(t, KindAttr, site.Kind)
	require.Equal(t, "number_of_objects", site.Name)
}

// TestArgAt_TrailingSameLine covers the band-owner fix: a cursor past the
// last argument of a command — the primary RMS hint moment — is owned by
// its own statement, not an earlier one.
func TestArgAt_TrailingSameLine(t *testing.T) {
	t.Parallel()
	src := "create_elevation 7 "
	file, _ := Parse(src, "t.rms")

	site, found := file.ArgAt(common.Pos{Line: 0, Column: uint32(len(src))})

	require.True(t, found)
	require.Equal(t, KindNone, site.Kind)
	require.Equal(t, "create_elevation", site.Stmt.Name)
}

// TestArgAt_BraceAndGapPositions covers step 6: braces and inter-token
// gaps never guess a label (SC6).
func TestArgAt_BraceAndGapPositions(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3 {\n  spacing 5\n}\n"
	file, _ := Parse(src, "t.rms")

	for name, pos := range map[string]common.Pos{
		"open brace":  linePos(src, 0, "{", 0),
		"close brace": linePos(src, 2, "}", 0),
		"gap":         linePos(src, 0, " 3", 0),
	} {
		site, found := file.ArgAt(pos)

		require.True(t, found, name)
		require.Equal(t, KindNone, site.Kind, name)
		require.Equal(t, "create_elevation", site.Stmt.Name, name)
	}
}

// TestArgAt_NonStatementLinesAfterCommand pins the categorical exclusion
// in the only configuration that exposes it — a preceding command
// exists: header, include-path and #-comment cursors answer silence,
// not the previous command with kind=none.
func TestArgAt_NonStatementLinesAfterCommand(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3\n" +
		"<LAND_GENERATION>\n" +
		"#include \"other.rms\"\n" +
		"# a plain comment\n"
	file, _ := Parse(src, "t.rms")

	site, found := file.ArgAt(linePos(src, 0, "3", 0))
	require.True(t, found)
	require.Equal(t, KindArg, site.Kind)
	require.Equal(t, 0, site.Index)

	_, found = file.ArgAt(linePos(src, 1, "LAND", 0))
	require.False(t, found, "section header")

	_, found = file.ArgAt(linePos(src, 2, "other.rms", 1))
	require.False(t, found, "quoted include path")

	_, found = file.ArgAt(linePos(src, 3, "plain", 0))
	require.False(t, found, "#-comment line")
}

// TestArgAt_OnDirectiveAndSectionHeader covers the exclusion list in a
// leading configuration: #const statements are filtered by the
// #-prefix owner rule.
func TestArgAt_OnDirectiveAndSectionHeader(t *testing.T) {
	t.Parallel()
	src := "#include other.rms\n" +
		"<LAND_GENERATION>\n" +
		"#const TERRAIN 7\n" +
		"create_elevation 3\n"
	file, _ := Parse(src, "t.rms")

	_, found := file.ArgAt(linePos(src, 0, "other.rms", 0))
	require.False(t, found, "include path")

	_, found = file.ArgAt(linePos(src, 1, "LAND", 0))
	require.False(t, found, "section header")

	_, found = file.ArgAt(linePos(src, 2, "7", 0))
	require.False(t, found, "#const statement")
}

// TestArgAt_InComment covers comment exclusion through the recorded
// blankComments extents.
func TestArgAt_InComment(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3 /* hill */"
	file, _ := Parse(src, "t.rms")

	_, found := file.ArgAt(linePos(src, 0, "hill", 0))

	require.False(t, found)
}

// TestArgAt_InString covers string exclusion on statement lines: editing
// a quoted attribute value renders no hint.
func TestArgAt_InString(t *testing.T) {
	t.Parallel()
	src := "create_object GOLF_BALL {\n  object_name \"grassland\"\n}\n"
	file, _ := Parse(src, "t.rms")

	_, found := file.ArgAt(linePos(src, 1, "land", 0))

	require.False(t, found)
}

// TestArgAt_NestedBlockInnermost covers «владеет самый внутренний
// statement» for nested random blocks.
func TestArgAt_NestedBlockInnermost(t *testing.T) {
	t.Parallel()
	src := "start_random\n" +
		"  percent_chance 50\n" +
		"    create_elevation 3\n" +
		"  end_random\n" +
		"end_random\n"
	file, _ := Parse(src, "t.rms")

	site, found := file.ArgAt(linePos(src, 2, "3", 0))

	require.True(t, found)
	require.Equal(t, "create_elevation", site.Stmt.Name)
	require.Equal(t, KindArg, site.Kind)
	require.Equal(t, 0, site.Index)
}

// TestArgAt_Deterministic covers the determinism requirement.
func TestArgAt_Deterministic(t *testing.T) {
	t.Parallel()
	src := "create_elevation 3 {\n  spacing 5\n}\n"
	file, _ := Parse(src, "t.rms")

	pos := linePos(src, 1, "5", 0)

	first, found1 := file.ArgAt(pos)
	second, found2 := file.ArgAt(pos)

	require.True(t, found1)
	require.True(t, found2)
	require.Equal(t, first, second)
}
