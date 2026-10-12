package rms

import (
	"aoe2-lsp/internal/common"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadFixture reads a testdata fixture.
func loadFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)

	return string(raw)
}

// TestParse_Fixtures parses every fixture and checks sections and
// diagnostics.
func TestParse_Fixtures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fixture    string
		sections   []string
		wantDiags  int
		diagSubstr []string
	}{
		{
			name:      "plain sections",
			fixture:   "sections.rms",
			sections:  []string{"player_setup", "land_generation"},
			wantDiags: 0,
		},
		{
			name:      "positional attribute blocks",
			fixture:   "positional.rms",
			sections:  []string{"terrain_generation"},
			wantDiags: 0,
		},
		{
			name:      "random blocks",
			fixture:   "random.rms",
			sections:  []string{"objects_generation"},
			wantDiags: 0,
		},
		{
			name:      "conditionals",
			fixture:   "conditionals.rms",
			sections:  []string{"cliff_generation", "land_generation"},
			wantDiags: 0,
		},
		{
			name:      "DE expressions",
			fixture:   "expressions.rms",
			sections:  []string{"land_generation", "player_setup"},
			wantDiags: 0,
		},
		{
			name:      "includes and XS block",
			fixture:   "includes.rms",
			sections:  []string{"global", "land_generation", "objects_generation"},
			wantDiags: 0,
		},
		{
			name:     "broken input recovers",
			fixture:  "broken.rms",
			sections: []string{"land_generation", "objects_generation"},
			diagSubstr: []string{
				`unexpected "}"`,
				`unclosed "{" block`,
				`"endif" without a matching opening block`,
				`unexpected "@"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := loadFixture(t, tt.fixture)

			file, diags := Parse(source, tt.fixture)

			require.NotNil(t, file, "Parse must never return a nil file")
			require.Equal(t, tt.fixture, file.Name)

			names := make([]string, 0, len(file.Sections))
			for _, sec := range file.Sections {
				names = append(names, sec.Name)
			}

			assert.Equal(t, tt.sections, names)

			if tt.wantDiags > 0 {
				assert.Len(t, diags, tt.wantDiags)
			}

			for _, want := range tt.diagSubstr {
				found := false
				for _, d := range diags {
					if strings.Contains(d.Message, want) {
						found = true
						break
					}
				}

				assert.True(t, found, "expected a diagnostic containing %q, got %v", want, messages(diags))
			}

			assertSorted(t, diags)
		})
	}
}

// TestParse_PositionalSemantics checks that attributes written after a
// command (in its braces) belong to that command in all layout styles.
func TestParse_PositionalSemantics(t *testing.T) {
	t.Parallel()
	file, diags := Parse(loadFixture(t, "positional.rms"), "positional.rms")
	require.Empty(t, diags)

	terrain := file.Sections[0].Statements
	require.Len(t, terrain, 3)

	assert.Equal(t, "create_terrain", terrain[0].Name)
	assert.Equal(t, "FOREST", terrain[0].Args[0].Value)
	assert.Len(t, terrain[0].Attributes, 4)
	assert.Equal(t, "land_percent", terrain[0].Attributes[0].Name)
	assert.Equal(t, "12", terrain[0].Attributes[0].Value.Value)

	// brace on its own line
	assert.Len(t, terrain[1].Attributes, 2)

	// one-line block
	assert.Len(t, terrain[2].Attributes, 2)
}

// TestParse_RandomNesting checks start_random/percent_chance children.
func TestParse_RandomNesting(t *testing.T) {
	t.Parallel()
	file, diags := Parse(loadFixture(t, "random.rms"), "random.rms")
	require.Empty(t, diags)

	stmts := file.Sections[0].Statements
	require.Len(t, stmts, 2)

	start := stmts[0]
	assert.Equal(t, KindRandom, start.Kind)
	assert.Equal(t, "start_random", start.Name)
	require.Len(t, start.Children, 2)

	first := start.Children[0]
	assert.Equal(t, "percent_chance", first.Name)
	assert.Equal(t, "35", first.Args[0].Value)
	require.Len(t, first.Children, 1)
	assert.Equal(t, "create_object", first.Children[0].Name)

	// brace-less attributes attach to their create_* command
	require.Len(t, first.Children[0].Attributes, 1)
	assert.Equal(t, "number_of_objects", first.Children[0].Attributes[0].Name)

	// attributes inside the random block of a command still attach to it
	relic := stmts[1]
	assert.Equal(t, "create_object", relic.Name)
	assert.Len(t, relic.Attributes, 3)
}

// TestParse_Conditionals checks the if/elseif/else sibling structure.
func TestParse_Conditionals(t *testing.T) {
	t.Parallel()
	file, diags := Parse(loadFixture(t, "conditionals.rms"), "conditionals.rms")
	require.Empty(t, diags)

	cliffs := file.Sections[0].Statements
	require.Len(t, cliffs, 3)

	for i, name := range []string{"if", "elseif", "else"} {
		assert.Equal(t, KindConditional, cliffs[i].Kind)
		assert.Equal(t, name, cliffs[i].Name)
	}

	assert.Equal(t, "TINY_MAP", cliffs[0].Args[0].Value)
	assert.Equal(t, KindConst, cliffs[0].Args[0].Kind)
	require.Len(t, cliffs[0].Children, 2)
	assert.Equal(t, "min_number_of_cliffs", cliffs[0].Children[0].Name)

	// attributes inside conditional branches attach to the owning command
	land := file.Sections[1].Statements[0]
	assert.Len(t, land.Attributes, 3)
}

// TestParse_Expressions checks DE math trees, floats and percents.
func TestParse_Expressions(t *testing.T) {
	t.Parallel()
	file, diags := Parse(loadFixture(t, "expressions.rms"), "expressions.rms")
	require.Empty(t, diags)

	attrs := map[string]Expr{}
	for _, stmt := range file.Sections[0].Statements {
		for _, a := range stmt.Attributes {
			attrs[a.Name] = a.Value
		}
	}

	mul := attrs["land_percent"]
	assert.Equal(t, KindBinary, mul.Kind)
	assert.Equal(t, "*", mul.Value)
	require.Len(t, mul.Children, 2)
	assert.Equal(t, "3", mul.Children[0].Value)
	assert.Equal(t, "4", mul.Children[1].Value)

	// precedence: 1000 + (500*2)
	sum := attrs["number_of_tiles"]
	require.Equal(t, "+", sum.Value)
	require.Len(t, sum.Children, 2)
	assert.Equal(t, "1000", sum.Children[0].Value)
	assert.Equal(t, "*", sum.Children[1].Value)

	neg := attrs["bottom_border"]
	assert.Equal(t, KindUnary, neg.Kind)
	assert.Equal(t, "-", neg.Value)
	require.Len(t, neg.Children, 1)
	assert.Equal(t, "2", neg.Children[0].Value)

	mixed := attrs["other_zone_avoidance_distance"]
	assert.Equal(t, "+", mixed.Value)
	assert.Equal(t, KindUnary, mixed.Children[0].Kind)

	pct := attrs["circle_radius"]
	assert.Equal(t, KindPercent, pct.Kind)
	assert.Equal(t, "2.5%", pct.Value)

	assert.Equal(t, "7.5", attrs["left_border"].Value)
}

// TestParse_IncludesAndXs checks #include handling and the XS block.
func TestParse_IncludesAndXs(t *testing.T) {
	t.Parallel()
	file, diags := Parse(loadFixture(t, "includes.rms"), "includes.rms")
	require.Empty(t, diags)

	require.Len(t, file.Includes, 1)
	assert.Equal(t, "Team_Islands_lands.rms", file.Includes[0].Path)
	assert.Equal(t, uint32(1), file.Includes[0].Range.Start.Line)
	assert.Equal(t, uint32(9), file.Includes[0].Range.Start.Column)

	require.Len(t, file.XsBlocks, 1)
	block := file.XsBlocks[0]
	assert.Contains(t, block.Code, "xsGetMapSeed")
	assert.NotContains(t, block.Code, "OBJECTS_GENERATION")
	assert.True(t, block.Range.Start.Before(block.Range.End))

	// the global #const lives in the implicit global section
	global := file.Sections[0]
	assert.Equal(t, "global", global.Name)
	require.Len(t, global.Statements, 1)
	assert.Equal(t, "#const", global.Statements[0].Name)

	objects := file.Sections[2]
	require.Len(t, objects.Statements, 1)
	assert.Equal(t, "create_object", objects.Statements[0].Name)

	// brace-less attributes attach to their create_* command
	require.Len(t, objects.Statements[0].Attributes, 1)
	assert.Equal(t, "number_of_objects", objects.Statements[0].Attributes[0].Name,
		"attributes outside braces attach to the create_* command")
}

// TestParse_NeverNil checks the total-garbage path.
// TestParse_IncludeDrs covers the #include_drs directive and dotted
// file arguments — published maps use it in 45% of the corpus sample.
func TestParse_IncludeDrs(t *testing.T) {
	t.Parallel()

	src := "#include_drs random_map.def 54000\n#const ROCKS 40\n"

	file, diags := Parse(src, "drs.rms")

	require.Empty(t, diags)

	global := file.Sections[0]
	require.Len(t, global.Statements, 2)
	assert.Equal(t, "#include_drs", global.Statements[0].Name)
	assert.Equal(t, "#const", global.Statements[1].Name)
}

// TestParse_ConditionalSpansSections checks that if/elseif chains may
// cross section headers — the WSVG-style published maps switch whole map
// variants inside one conditional.
func TestParse_ConditionalSpansSections(t *testing.T) {
	t.Parallel()

	src := `<LAND_GENERATION>
if WSVG_ACROPOLIS
base_terrain GRASS
<OBJECTS_GENERATION>
create_object WOLF
elseif WSVG_HAMBURGER
base_terrain DIRT
endif
`

	file, diags := Parse(src, "span.rms")

	require.Empty(t, diags, "sections must not close conditional scopes")

	land := file.Sections[0]
	assert.Equal(t, "land_generation", land.Name)
	require.Len(t, land.Statements, 1)

	cond := land.Statements[0]
	assert.Equal(t, "if", cond.Name)

	// children from both sections belong to the conditional
	require.Len(t, cond.Children, 2)
	assert.Equal(t, "base_terrain", cond.Children[0].Name)
	assert.Equal(t, "create_object", cond.Children[1].Name)
}

// TestParse_UnterminatedConditionalAtEOF keeps the EOF diagnostic for a
// genuinely unterminated if.
func TestParse_UnterminatedConditionalAtEOF(t *testing.T) {
	t.Parallel()

	_, diags := Parse("if TINY_MAP\ncreate_object WOLF\n", "eof.rms")

	require.Len(t, diags, 1)
	assert.Equal(t, "unterminated block(s): if", diags[0].Message)
}

// TestParse_ImplicitAttributeBlockClosedByBrace covers the brace-less
// create_* attribute context terminated by a stray-looking "}".
func TestParse_ImplicitAttributeBlockClosedByBrace(t *testing.T) {
	t.Parallel()

	src := `<OBJECTS_GENERATION>
create_object TUNA
number_of_objects 19
set_scaling_to_map_size
}
`

	file, diags := Parse(src, "implicit.rms")

	require.Empty(t, diags)

	stmts := file.Sections[0].Statements
	require.Len(t, stmts, 1)
	assert.Equal(t, "create_object", stmts[0].Name)
	require.Len(t, stmts[0].Attributes, 2)
	assert.Equal(t, "number_of_objects", stmts[0].Attributes[0].Name)
}

// TestParse_NonAsciiWords checks that 8-bit text lexes as words instead
// of a stream of "unexpected byte" diagnostics.
func TestParse_NonAsciiWords(t *testing.T) {
	t.Parallel()

	src := "<LAND_GENERATION>\nbase_terrain café GRASS\n"

	_, diags := Parse(src, "nonascii.rms")

	for _, d := range diags {
		assert.NotContains(t, d.Message, "unexpected",
			"non-ASCII bytes are word characters, not token garbage: %s", d.Message)
	}
}

// TestParse_ImplicitAttributesAcrossBranches covers the Rooster pattern:
// a brace-less create_* context keeps collecting attributes from inside
// if/elseif/else branches — the branch picks the value at runtime.
func TestParse_ImplicitAttributesAcrossBranches(t *testing.T) {
	t.Parallel()

	src := `<OBJECTS_GENERATION>
create_object TUNA
if LR_TSUNAMI
  number_of_objects 22
else
  number_of_objects 19
endif
  set_scaling_to_map_size
}
`

	file, diags := Parse(src, "branches.rms")

	require.Empty(t, diags)

	stmts := file.Sections[0].Statements

	// the conditionals stay sibling statements; the attributes from
	// every branch attach to the command
	require.Len(t, stmts, 3)
	assert.Equal(t, "create_object", stmts[0].Name)
	assert.Equal(t, "if", stmts[1].Name)
	assert.Equal(t, "else", stmts[2].Name)

	require.Len(t, stmts[0].Attributes, 3)
	assert.Equal(t, "number_of_objects", stmts[0].Attributes[0].Name)
	assert.Equal(t, "set_scaling_to_map_size", stmts[0].Attributes[2].Name)
}

// TestParse_QuotesInsideCommentAreBlanked covers quoted text inside a
// block comment: the quotes must not leak as string arguments.
func TestParse_QuotesInsideCommentAreBlanked(t *testing.T) {
	t.Parallel()

	src := "<LAND_GENERATION>\ncreate_land  /* the \"relic isle\" :) */\n"

	file, diags := Parse(src, "quoted.rms")

	require.Empty(t, diags)

	stmts := file.Sections[0].Statements
	require.Len(t, stmts, 1)
	assert.Empty(t, stmts[0].Args, "comment bytes never become arguments")
}

func TestParse_NeverNil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
	}{
		{name: "empty", src: ""},
		{name: "only comment", src: "/* nothing */"},
		{name: "garbage", src: "\x00\x01 @@ <<<< \x02"},
		{name: "unclosed everything", src: "<LAND_GENERATION>\ncreate_land {\nif\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, _ := Parse(tt.src, "garbage")

			require.NotNil(t, file)
		})
	}
}

// TestRmsFile_StatementAt checks hover navigation, including the
// attribute-returns-owner requirement.
func TestRmsFile_StatementAt(t *testing.T) {
	t.Parallel()
	source := loadFixture(t, "positional.rms")
	file, _ := Parse(source, "positional.rms")

	// cursor on the attribute "number_of_clumps 8" (line 5, col 4)
	stmt, found := file.StatementAt(common.Pos{Line: 5, Column: 4})
	require.True(t, found)
	assert.Equal(t, "create_terrain", stmt.Name, "attribute position returns the owning command")
	assert.Len(t, stmt.Attributes, 4)

	// cursor on the command word itself
	stmt, found = file.StatementAt(common.Pos{Line: 3, Column: 5})
	require.True(t, found)
	assert.Equal(t, "create_terrain", stmt.Name)

	// cursor after the last statement returns the preceding one
	stmt, found = file.StatementAt(common.Pos{Line: 12, Column: 0})
	require.True(t, found)
	assert.Equal(t, "create_terrain", stmt.Name)
}

// TestRmsFile_SectionAt checks completion-context navigation.
func TestRmsFile_SectionAt(t *testing.T) {
	t.Parallel()
	source := loadFixture(t, "includes.rms")
	file, _ := Parse(source, "includes.rms")

	sec, found := file.SectionAt(common.Pos{Line: 13, Column: 3})
	require.True(t, found)
	assert.Equal(t, "land_generation", sec.Name)

	sec, found = file.SectionAt(common.Pos{Line: 2, Column: 1})
	require.True(t, found)
	assert.Equal(t, "global", sec.Name)

	_, found = file.SectionAt(common.Pos{Line: 999, Column: 0})
	assert.False(t, found)
}

// TestParse_DiagnosticsSorted is covered per fixture; this checks the
// explicit ordering invariant on the broken input.
func TestParse_DiagnosticsSortedExplicit(t *testing.T) {
	t.Parallel()
	_, diags := Parse(loadFixture(t, "broken.rms"), "broken.rms")

	assertSorted(t, diags)
}

// assertSorted checks that diagnostics are ordered by position.
func assertSorted(t *testing.T, diags []common.Diagnostic) {
	t.Helper()

	for i := 1; i < len(diags); i++ {
		prev, cur := diags[i-1].Range.Start, diags[i].Range.Start

		if prev.Line == cur.Line {
			require.LessOrEqual(t, prev.Column, cur.Column)

			continue
		}

		require.Less(t, prev.Line, cur.Line)
	}
}

// messages extracts diagnostic messages for failure output.
func messages(diags []common.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}

	return out
}

func TestParse_ClosingTag_NoPhantomSection(t *testing.T) {
	t.Parallel()
	src := "<PLAYER_SETUP>\n" +
		"random_placement\n" +
		"</PLAYER_SETUP>\n" +
		"<LAND_GENERATION>\n" +
		"base_terrain GRASS\n" +
		"</LAND_GENERATION>\n"

	file, diags := Parse(src, "closing.rms")

	require.Empty(t, diags)

	names := make([]string, 0, len(file.Sections))
	for _, sec := range file.Sections {
		names = append(names, sec.Name)
	}

	require.Equal(t, []string{"player_setup", "land_generation"}, names,
		"a closing tag must not reopen the section it closes")
	require.NotEmpty(t, file.Sections[0].Statements)
	require.NotEmpty(t, file.Sections[1].Statements)
}

func TestParse_ClosingTag_PostCloseGlobal(t *testing.T) {
	t.Parallel()
	src := "<CLIFF_GENERATION>\n" +
		"</CLIFF_GENERATION>\n" +
		"create_land TERRAIN_GRASS\n"

	file, _ := Parse(src, "post.rms")

	names := make([]string, 0, len(file.Sections))
	for _, sec := range file.Sections {
		names = append(names, sec.Name)
	}

	require.Equal(t, []string{"cliff_generation", "global"}, names,
		"statements after a closing tag are global")

	global := file.Sections[len(file.Sections)-1]
	require.Len(t, global.Statements, 1)
	require.Equal(t, "create_land", global.Statements[0].Name)
}

func TestParse_WordIndexRecordsAllKinds(t *testing.T) {
	t.Parallel()
	src := "<LAND_GENERATION>\n" +
		"create_terrain FOREST {\n" +
		"	land_percent 12\n" +
		"}\n" +
		"</LAND_GENERATION>\n"

	file, _ := Parse(src, "words.rms")

	counts := make(map[string]int)
	for _, w := range file.words {
		counts[w.name]++
	}

	require.Equal(t, 2, counts["land_generation"], "opening and closing headers")
	require.Equal(t, 1, counts["create_terrain"], "command name")
	require.Equal(t, 1, counts["FOREST"], "const argument value")
	require.Equal(t, 1, counts["land_percent"], "attribute name")
	require.NotContains(t, counts, "12", "numbers are not words")
}

// TestParse_IncludeRecordsPathArgumentRange checks that include directives
// record the path argument with its own range — quoted and bare forms.
func TestParse_IncludeRecordsPathArgumentRange(t *testing.T) {
	t.Parallel()
	src := "#include \"parts/econ.rms\"\n#include parts/bare.inc\n"

	file, diags := Parse(src, "main.rms")
	require.Empty(t, diags)

	require.Len(t, file.Includes, 2)

	first := file.Includes[0]
	assert.Equal(t, "parts/econ.rms", first.Path)
	assert.Equal(t, common.Pos{Line: 0, Column: 9, Offset: 9}, first.Range.Start)
	assert.Equal(t, common.Pos{Line: 0, Column: 25, Offset: 25}, first.Range.End)

	second := file.Includes[1]
	assert.Equal(t, "parts/bare.inc", second.Path)
	assert.Equal(t, common.Pos{Line: 1, Column: 9, Offset: 35}, second.Range.Start)
	assert.Equal(t, common.Pos{Line: 1, Column: 23, Offset: 49}, second.Range.End)

	assert.Empty(t, file.XsIncludes)
}

// TestParse_IncludeXSArgumentAndInlineBlock checks the dual mode of
// #includeXS with a file argument: the external path is recorded while the
// region after the directive stays an inline XsBlock.
func TestParse_IncludeXSArgumentAndInlineBlock(t *testing.T) {
	t.Parallel()
	src := "#includeXS lib/helpers.xs\nvoid sharedFn(int n) { }\n"

	file, diags := Parse(src, "main.rms")
	require.Empty(t, diags)

	require.Len(t, file.XsIncludes, 1)
	assert.Equal(t, "lib/helpers.xs", file.XsIncludes[0].Path)
	assert.Equal(t, common.Pos{Line: 0, Column: 11, Offset: 11}, file.XsIncludes[0].Range.Start)
	assert.Equal(t, common.Pos{Line: 0, Column: 25, Offset: 25}, file.XsIncludes[0].Range.End)
	assert.Empty(t, file.Includes)

	require.Len(t, file.XsBlocks, 1)
	assert.Equal(t, "void sharedFn(int n) { }", file.XsBlocks[0].Code)
}

// TestParse_IncludeWithoutPath checks the missing-argument syntax error:
// a diagnostic is reported and no Include is created.
func TestParse_IncludeWithoutPath(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#include\n", "main.rms")

	assert.Empty(t, file.Includes)
	assert.Empty(t, file.XsIncludes)
	require.Len(t, diags, 1)
	assert.Equal(t, "syntax", diags[0].Code)
}

// TestParse_UnclosedXsBlockAtEOFWithoutNewline checks that an inline XS
// block left open at end of file (no trailing newline) spans to the end
// of the last line instead of panicking on the line-index boundary.
func TestParse_UnclosedXsBlockAtEOFWithoutNewline(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#includeXS\nvoid main() { int x = 1; }", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.XsBlocks, 1)

	assert.Equal(t, "void main() { int x = 1; }", file.XsBlocks[0].Code)
	assert.Equal(t, common.Pos{Line: 1, Column: 0, Offset: 11}, file.XsBlocks[0].Range.Start)
	assert.Equal(t, common.Pos{Line: 1, Column: 26, Offset: 37}, file.XsBlocks[0].Range.End)
}

// TestParse_BareIncludeXSAtEOF checks the bare #includeXS directive as
// the very last line without a trailing newline: an empty zero-width
// block at end of file, without panicking (the block start line equals
// the line count).
func TestParse_BareIncludeXSAtEOF(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#includeXS", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.XsBlocks, 1)

	assert.Equal(t, "", file.XsBlocks[0].Code)
	assert.Equal(t, common.Pos{Line: 0, Column: 10, Offset: 10}, file.XsBlocks[0].Range.Start)
	assert.Equal(t, common.Pos{Line: 0, Column: 10, Offset: 10}, file.XsBlocks[0].Range.End)
}

// TestParse_XsBlockTerminatedByDirective checks the regular mid-file
// case: a block closed by a following directive keeps spanning to the
// start of the terminating line.
func TestParse_XsBlockTerminatedByDirective(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#includeXS\nvoid f() { }\n#include other.rms\n", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.XsBlocks, 1)

	assert.Equal(t, "void f() { }", file.XsBlocks[0].Code)
	assert.Equal(t, common.Pos{Line: 1, Column: 0, Offset: 11}, file.XsBlocks[0].Range.Start)
	assert.Equal(t, common.Pos{Line: 2, Column: 0, Offset: 24}, file.XsBlocks[0].Range.End)
}

// TestParse_XsBlockTrailingBlankLines checks that trailing blank lines
// are trimmed from an open block: the range ends on the last non-blank
// line, the code carries no trailing blanks.
func TestParse_XsBlockTrailingBlankLines(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#includeXS\nvoid f() { }\n\n\n#include other.rms\n", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.XsBlocks, 1)

	assert.Equal(t, "void f() { }", file.XsBlocks[0].Code)
	assert.Equal(t, common.Pos{Line: 2, Column: 0, Offset: 24}, file.XsBlocks[0].Range.End)
}

// TestParse_UnclosedBlockCommentInXsRegion pins #93: block-comment state
// does not cross an inline-XS region boundary. An unclosed /* inside the
// region yields one extent contained in the region, the terminating
// section header stays visible to the walk, and every section after the
// region lands in the AST — the file is not silently swallowed. Closed
// /* */ and // inside the region keep reaching Comments (the contract
// asymmetry pinned by TestRmsComments_HashInsideXsBlock), and comment
// texts slice byte-exact by offset — the formatter's read path.
func TestParse_UnclosedBlockCommentInXsRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		blocks   []xsScanBlock
		comments []common.Range
		texts    []string
		sections []xsScanSection
	}{
		{
			name: "unclosed /* no longer swallows the sections after the region",
			src: "#includeXS\n" +
				"/* xs\n" +
				"<LAND_GENERATION>\n" +
				"create_land X\n" +
				"</LAND_GENERATION>\n",
			blocks: []xsScanBlock{{
				code: "/* xs",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 2, Column: 0, Offset: 17}},
			}},
			comments: []common.Range{commentExtent(1, 0, 1, 5)},
			texts:    []string{"/* xs"},
			sections: []xsScanSection{{name: "land_generation", stmts: []string{"create_land"}}},
		},
		{
			name: "terminator swallowed by the open block still closes the region",
			src: "#includeXS\n" +
				"int a = 1;\n" +
				"/* open\n" +
				"<LAND_GENERATION>\n" +
				"create_land X\n" +
				"</LAND_GENERATION>\n" +
				"<ELEVATION_GENERATION>\n" +
				"create_elevation 3\n" +
				"</ELEVATION_GENERATION>\n",
			blocks: []xsScanBlock{{
				code: "int a = 1;\n/* open",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 3, Column: 0, Offset: 30}},
			}},
			comments: []common.Range{commentExtent(2, 0, 2, 7)},
			texts:    []string{"/* open"},
			sections: []xsScanSection{
				{name: "land_generation", stmts: []string{"create_land"}},
				{name: "elevation_generation", stmts: []string{"create_elevation"}},
			},
		},
		{
			name: "closed comments inside the region keep their extents",
			src: "#includeXS\n" +
				"/* xs pair */\n" +
				"code(); // tail\n" +
				"<LAND_GENERATION>\n" +
				"create_land X\n" +
				"</LAND_GENERATION>\n",
			blocks: []xsScanBlock{{
				code: "/* xs pair */\ncode(); // tail",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 3, Column: 0, Offset: 41}},
			}},
			comments: []common.Range{
				commentExtent(1, 0, 1, 13),
				commentExtent(2, 8, 2, 15),
			},
			texts:    []string{"/* xs pair */", "// tail"},
			sections: []xsScanSection{{name: "land_generation", stmts: []string{"create_land"}}},
		},
		{
			name: "block opened on the directive line stays on the directive line",
			src: "#includeXS /* open\n" +
				"<LAND_GENERATION>\n" +
				"create_land X\n" +
				"</LAND_GENERATION>\n",
			blocks: []xsScanBlock{{
				code: "",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 19}, {Line: 1, Column: 0, Offset: 19}},
			}},
			comments: []common.Range{commentExtent(0, 11, 0, 18)},
			texts:    []string{"/* open"},
			sections: []xsScanSection{{name: "land_generation", stmts: []string{"create_land"}}},
		},
		{
			name: "terminator with a block-comment tail still closes the region",
			src: "#includeXS\n" +
				"/* xs\n" +
				"<LAND_GENERATION> /* keep\n" +
				"create_land X\n" +
				"*/\n" +
				"</LAND_GENERATION>\n" +
				"create_terrain GRASS\n",
			blocks: []xsScanBlock{{
				code: "/* xs",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 2, Column: 0, Offset: 17}},
			}},
			comments: []common.Range{
				commentExtent(1, 0, 1, 5),
				commentExtent(2, 18, 4, 2),
			},
			// create_land X rides inside the /* keep */ comment; the
			// statement after the closed comment survives in global
			texts: []string{"/* xs", "/* keep\ncreate_land X\n*/"},
			sections: []xsScanSection{
				{name: "land_generation", stmts: []string{}},
				{name: "global", stmts: []string{"create_terrain"}},
			},
		},
		{
			name: "terminator with a line-comment tail still closes the region",
			src: "#includeXS\n" +
				"/* xs\n" +
				"<LAND_GENERATION> // note\n" +
				"create_land X\n" +
				"</LAND_GENERATION>\n",
			blocks: []xsScanBlock{{
				code: "/* xs",
				span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 2, Column: 0, Offset: 17}},
			}},
			comments: []common.Range{
				commentExtent(1, 0, 1, 5),
				commentExtent(2, 18, 2, 25),
			},
			texts:    []string{"/* xs", "// note"},
			sections: []xsScanSection{{name: "land_generation", stmts: []string{"create_land"}}},
		},
		{
			name: "includeXS directive terminating a region opens the next one",
			src: "#includeXS\n" +
				"/* xs\n" +
				"#includeXS lib.xs\n" +
				"void f() { }\n" +
				"<LAND_GENERATION>\n" +
				"create_land X\n",
			blocks: []xsScanBlock{
				{
					code: "/* xs",
					span: [2]common.Pos{{Line: 1, Column: 0, Offset: 11}, {Line: 2, Column: 0, Offset: 17}},
				},
				{
					code: "void f() { }",
					span: [2]common.Pos{{Line: 3, Column: 0, Offset: 35}, {Line: 4, Column: 0, Offset: 48}},
				},
			},
			comments: []common.Range{commentExtent(1, 0, 1, 5)},
			texts:    []string{"/* xs"},
			sections: []xsScanSection{{name: "land_generation", stmts: []string{"create_land"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, diags := Parse(tt.src, "t.rms")

			require.Empty(t, diags, tt.name)
			require.Len(t, file.XsBlocks, len(tt.blocks), tt.name)

			for j, want := range tt.blocks {
				assert.Equal(t, want.code, file.XsBlocks[j].Code, tt.name)
				assert.Equal(t, common.Range{Start: want.span[0], End: want.span[1]}, file.XsBlocks[j].Range, tt.name)
			}

			assert.Equal(t, tt.comments, commentExtents(file.Comments), tt.name)
			assert.Equal(t, tt.texts, commentTexts(tt.src, file.Comments), tt.name)
			assert.Equal(t, tt.sections, xsScanSections(file), tt.name)
		})
	}
}

// xsScanBlock is the XsBlock expectation of the region comment tests: the
// verbatim code with the block's line span.
type xsScanBlock struct {
	code string
	span [2]common.Pos
}

// xsScanSection is the section expectation of the region comment tests:
// the section name with the command names of its direct statements.
type xsScanSection struct {
	name  string
	stmts []string
}

// xsScanSections projects the parsed sections into the xsScanSection
// shape the tests compare against.
func xsScanSections(file RmsFile) []xsScanSection {
	out := make([]xsScanSection, 0, len(file.Sections))

	for _, sec := range file.Sections {
		stmts := make([]string, 0, len(sec.Statements))
		for _, st := range sec.Statements {
			stmts = append(stmts, st.Name)
		}

		out = append(out, xsScanSection{name: sec.Name, stmts: stmts})
	}

	return out
}

// TestParse_EndRandomClosesUnterminatedIf checks the implicit-close
// warning: end_random silently closing an unterminated if reports a
// warning naming both blocks.
func TestParse_EndRandomClosesUnterminatedIf(t *testing.T) {
	t.Parallel()
	src := "start_random\nif 1\npercent_chance 50\ncreate_terrain GRASS\nend_random\n"

	file, diags := Parse(src, "t.rms")

	require.Len(t, diags, 1)
	assert.Equal(t, common.SeverityWarning, diags[0].Severity)
	assert.Equal(t, "syntax", diags[0].Code)
	assert.Equal(t, `"end_random" closes an unterminated "if" block`, diags[0].Message)
	assert.Equal(t, uint32(4), diags[0].Range.Start.Line)

	// The random block itself is the legitimate close target and stays
	// nested in the tree.
	start := file.Sections[0].Statements[0]
	assert.Equal(t, "start_random", start.Name)
	require.Len(t, start.Children, 1)
}

// TestParse_EndRandomClosedNestNoWarning checks that a properly paired
// nesting (if/endif inside start_random) produces no implicit-close
// warnings.
func TestParse_EndRandomClosedNestNoWarning(t *testing.T) {
	t.Parallel()
	src := "start_random\nif 1\npercent_chance 50\ncreate_terrain GRASS\nendif\nend_random\n"

	file, diags := Parse(src, "t.rms")

	assert.Empty(t, diags)
	start := file.Sections[0].Statements[0]
	assert.Equal(t, "start_random", start.Name)
}

// TestParse_PercentChanceImplicitNoWarn checks the documented exception:
// an unterminated percent_chance branch closes implicitly without a
// warning.
func TestParse_PercentChanceImplicitNoWarn(t *testing.T) {
	t.Parallel()
	src := "start_random\npercent_chance 50\ncreate_terrain GRASS\nend_random\n"

	_, diags := Parse(src, "t.rms")

	assert.Empty(t, diags)
}

// TestParse_EndRandomWithoutMatch checks the unmatched-closer error is
// preserved: a lone end_random stays an error, not a warning.
func TestParse_EndRandomWithoutMatch(t *testing.T) {
	t.Parallel()
	_, diags := Parse("end_random\n", "t.rms")

	require.Len(t, diags, 1)
	assert.Equal(t, common.SeverityError, diags[0].Severity)
	assert.Equal(t, `"end_random" without a matching opening block`, diags[0].Message)
}

// TestReferences_ByName checks the by-name form: same occurrences as
// ReferencesAt, without needing a position in this file.
func TestReferences_ByName(t *testing.T) {
	t.Parallel()
	src := "create_elevator 7\ncreate_elevator 3\nbase_terrain GRASS\n"
	file, diags := Parse(src, "main.rms")
	require.Empty(t, diags)

	ranges := file.References("create_elevator")
	require.Len(t, ranges, 2)
	assert.Equal(t, uint32(0), ranges[0].Start.Line)
	assert.Equal(t, uint32(1), ranges[1].Start.Line)

	// Equivalence: ReferencesAt at any occurrence yields the same set.
	for _, r := range ranges {
		mid := common.Pos{Line: r.Start.Line, Column: r.Start.Column + 2, Offset: r.Start.Offset + 2}
		assert.Equal(t, ranges, file.ReferencesAt(mid))
	}

	// Unknown name: empty result.
	assert.Empty(t, file.References("no_such_command"))
}

// TestParse_IncludeXSArgEmptyRegionNoBlock checks that an argumented
// #includeXS whose inline region is empty (a terminator on the next
// line) owns no XsBlock — no phantom outline node.
func TestParse_IncludeXSArgEmptyRegionNoBlock(t *testing.T) {
	t.Parallel()
	src := "#includeXS lib/helpers.xs\n<land_generation>\ncreate_elevator 7\n"

	file, diags := Parse(src, "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.XsIncludes, 1)
	assert.Equal(t, "lib/helpers.xs", file.XsIncludes[0].Path)
	assert.Empty(t, file.XsBlocks)
}

// TestParse_StringLiteralKeepsCommentMarkers checks that comment
// blanking respects string literals: the path and range of a quoted
// include argument survive comment markers inside the quotes.
func TestParse_StringLiteralKeepsCommentMarkers(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#include \"a//b.rms\"\n", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.Includes, 1)

	assert.Equal(t, "a//b.rms", file.Includes[0].Path)
	assert.Equal(t, common.Pos{Line: 0, Column: 9, Offset: 9}, file.Includes[0].Range.Start)
	assert.Equal(t, common.Pos{Line: 0, Column: 19, Offset: 19}, file.Includes[0].Range.End)
}

// TestParse_StringLiteralKeepsBlockMarker checks a block-comment opener
// inside a quoted path neither opens a comment nor mangles the range.
func TestParse_StringLiteralKeepsBlockMarker(t *testing.T) {
	t.Parallel()
	file, diags := Parse("#include \"a/*b.rms\"\ncreate_elevator 7\n", "t.rms")

	require.Empty(t, diags)
	require.Len(t, file.Includes, 1)

	assert.Equal(t, "a/*b.rms", file.Includes[0].Path)

	// the command after the string still parses — no runaway comment
	assert.Equal(t, "create_elevator", file.Sections[0].Statements[0].Name)
}

// TestFirstWord checks the lexer-faithful first-word cut: leading blanks
// never enter the answer, the word breaks at the first non-word byte, and
// a non-word opening token answers empty.
func TestFirstWord(t *testing.T) {
	t.Parallel()

	for _, tc := range [...]struct {
		line string
		want string
	}{
		{"if TURF 1", "if"},
		{"    if TURF 1", "if"},
		{"\tif}", "if"},
		{"else{", "else"},
		{"endif,", "endif"},
		{"start_random 5", "start_random"},
		{"#const A 1", "#const"},
		{"random_map.def 54000", "random_map.def"},
		{"_private", "_private"},
		{"create_object BOAR\r", "create_object"},
		{"50 percent", ""},
		{`"str" x`, ""},
		{"{", ""},
		{"", ""},
		{"   ", ""},
	} {
		if got := FirstWord(tc.line); got != tc.want {
			t.Errorf("FirstWord(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

// exprShape is the AST expectation of one value expression in the
// operand-failure tests: Expr projected into a comparable recursive shape.
type exprShape struct {
	kind     string
	value    string
	span     common.Range
	children []exprShape
}

// argShapes projects expressions into the exprShape form; empty input
// stays nil so the table literals can spell "no children" as nil.
func argShapes(exprs []Expr) []exprShape {
	if len(exprs) == 0 {
		return nil
	}

	out := make([]exprShape, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, exprShape{
			kind:     e.Kind,
			value:    e.Value,
			span:     e.Range,
			children: argShapes(e.Children),
		})
	}

	return out
}

// TestParse_OperandFailureDiagnostics pins the six operand-failure
// emitters (issue #96): a failed operand parse is never silent — every
// consumed byte is covered by a diagnostic, and the recovery forms
// (childless unary, zero-width inner, kept arity) stay in the AST.
func TestParse_OperandFailureDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []common.Diagnostic
		args []exprShape
	}{
		{
			// №1: at EOF the span ends at the last consumed token —
			// the operator itself.
			name: "missing operand after binary op",
			src:  "create_land 1 +\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
				Severity: common.SeverityError,
				Message:  `missing operand after "+"`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindNumber,
				value: "1",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
			}},
		},
		{
			// №2: the EOF anchor degrades to the operator token's range;
			// the childless unary is the recovery form and stays in Args.
			name: "missing operand after unary op",
			src:  "create_land -\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
				Severity: common.SeverityError,
				Message:  `missing operand after unary "-"`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindUnary,
				value: "-",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
			}},
		},
		{
			// №3: the group span `(`…`)` is reported; the zero-width
			// inner Expr keeps its exact pre-fix shape — the zero Expr{}
			// (no kind, zero-width range).
			name: "parenthesis without a valid expression",
			src:  "create_land ( )\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
				Severity: common.SeverityError,
				Message:  `"(" without a valid expression`,
				Code:     "syntax",
			}},
			args: []exprShape{{}},
		},
		{
			// №3 cascade: the inner group failure recovers with ok=true,
			// so the outer `(` gets its own group report — one anchor per
			// paren token, position order after the stable sort.
			name: "nested parenthesis without a valid expression",
			src:  "create_land (( )\n",
			want: []common.Diagnostic{
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 16, Offset: 16}},
					Severity: common.SeverityError,
					Message:  `"(" without a valid expression`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 13, Offset: 13}, End: common.Pos{Line: 0, Column: 16, Offset: 16}},
					Severity: common.SeverityError,
					Message:  `"(" without a valid expression`,
					Code:     "syntax",
				},
			},
			args: []exprShape{{}},
		},
		{
			// №2 inside a group: the unary failure recovers with ok=true,
			// and the group report covers the consumed `(` the childless
			// unary's range does not represent.
			name: "unary failure inside a group",
			src:  "create_land ( - )\n",
			want: []common.Diagnostic{
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 17, Offset: 17}},
					Severity: common.SeverityError,
					Message:  `"(" without a valid expression`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 16, Offset: 16}, End: common.Pos{Line: 0, Column: 17, Offset: 17}},
					Severity: common.SeverityError,
					Message:  `missing operand after unary "-"`,
					Code:     "syntax",
				},
			},
			args: []exprShape{{
				kind:  KindUnary,
				value: "-",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
			}},
		},
		{
			// №3 + №2: the inner unary failure recovers right before the
			// group's `)`, which the group then consumes itself — the
			// group report covers it too, or the paren would be lost to
			// both the AST and the diagnostics.
			name: "group report covers the closing parenthesis",
			src:  "create_land ( - , )\n",
			want: []common.Diagnostic{
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 19, Offset: 19}},
					Severity: common.SeverityError,
					Message:  `"(" without a valid expression`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 16, Offset: 16}, End: common.Pos{Line: 0, Column: 17, Offset: 17}},
					Severity: common.SeverityError,
					Message:  `missing operand after unary "-"`,
					Code:     "syntax",
				},
			},
			args: []exprShape{{
				kind:  KindUnary,
				value: "-",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
			}},
		},
		{
			// №4: the stray comma is anchored where it lies; the recovery
			// must not skip past the following `2` (off-by-one regression)
			// and the call stays closed by its `)`.
			name: "unexpected token in call arguments",
			src:  "create_land f(1, , 2)\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 17, Offset: 17}, End: common.Pos{Line: 0, Column: 18, Offset: 18}},
				Severity: common.SeverityError,
				Message:  `unexpected "," in call arguments`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindIdent,
				value: "f",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 21, Offset: 21}},
				children: []exprShape{
					{kind: KindNumber, value: "1", span: common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}}},
					{kind: KindNumber, value: "2", span: common.Range{Start: common.Pos{Line: 0, Column: 19, Offset: 19}, End: common.Pos{Line: 0, Column: 20, Offset: 20}}},
				},
			}},
		},
		{
			// №5: an unclosed call paren is the only warning of the table,
			// anchored on `(` — the call itself is complete in the AST.
			name: "unclosed call parenthesis",
			src:  "create_land f(1, 2\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 13, Offset: 13}, End: common.Pos{Line: 0, Column: 14, Offset: 14}},
				Severity: common.SeverityWarning,
				Message:  `unclosed "(" in call to "f"`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindIdent,
				value: "f",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 18, Offset: 18}},
				children: []exprShape{
					{kind: KindNumber, value: "1", span: common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}}},
					{kind: KindNumber, value: "2", span: common.Range{Start: common.Pos{Line: 0, Column: 17, Offset: 17}, End: common.Pos{Line: 0, Column: 18, Offset: 18}}},
				},
			}},
		},
		{
			// №3, unclosed branch: a clean inner parse with no `)` — the
			// paren is not in the AST, so it gets its own warning; the
			// inner expression stays a real operand, not a recovery form.
			name: "unclosed group parenthesis",
			src:  "create_land (1\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
				Severity: common.SeverityWarning,
				Message:  `unclosed "("`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindNumber,
				value: "1",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 13, Offset: 13}, End: common.Pos{Line: 0, Column: 14, Offset: 14}},
			}},
		},
		{
			// №3, unclosed branch on the outer group: the inner group
			// closes cleanly, so only the outer `(` is uncovered — one
			// warning, no group error report.
			name: "unclosed outer group around a closed inner one",
			src:  "create_land ((1)\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
				Severity: common.SeverityWarning,
				Message:  `unclosed "("`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindNumber,
				value: "1",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
			}},
		},
		{
			// №6: the run loop continues past the bad token — the `5` is
			// parsed, arity is not swallowed by the failure.
			name: "leading comma does not swallow the rest",
			src:  "create_land , 5\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
				Severity: common.SeverityError,
				Message:  `unexpected ","`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindNumber,
				value: "5",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 15, Offset: 15}},
			}},
		},
		{
			// R9: the failed `)` lies inside the №1 span — the span covers
			// it, so no second diagnostic anchors the same token.
			name: "span covers the failed token without a second anchor",
			src:  "create_land 1 + )\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 17, Offset: 17}},
				Severity: common.SeverityError,
				Message:  `missing operand after "+"`,
				Code:     "syntax",
			}},
			args: []exprShape{{
				kind:  KindNumber,
				value: "1",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, diags := Parse(tt.src, "test.rms")

			require.NotNil(t, file)
			require.Len(t, file.Sections[0].Statements, 1)

			assert.Equal(t, tt.want, diags)
			assert.Equal(t, tt.args, argShapes(file.Sections[0].Statements[0].Args))
		})
	}
}

// TestParse_OperandFailure_NoDuplicateAnchors pins the anchor rule: two
// failures on one line report once per distinct token in position order —
// the `)` inside the missing-operand span is not anchored a second time.
func TestParse_OperandFailure_NoDuplicateAnchors(t *testing.T) {
	t.Parallel()

	_, diags := Parse("create_land , 1 + )\n", "test.rms")

	assert.Equal(t, []common.Diagnostic{
		{
			Range:    common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 13, Offset: 13}},
			Severity: common.SeverityError,
			Message:  `unexpected ","`,
			Code:     "syntax",
		},
		{
			Range:    common.Range{Start: common.Pos{Line: 0, Column: 16, Offset: 16}, End: common.Pos{Line: 0, Column: 19, Offset: 19}},
			Severity: common.SeverityError,
			Message:  `missing operand after "+"`,
			Code:     "syntax",
		},
	}, diags)
}

// TestParse_EmptyCallIsValid guards the positive case: a closed empty
// call is valid — the call-argument emitters must not fire on it.
func TestParse_EmptyCallIsValid(t *testing.T) {
	t.Parallel()

	file, diags := Parse("create_land f()\n", "test.rms")

	require.NotNil(t, file)
	assert.Empty(t, diags)

	require.Len(t, file.Sections[0].Statements, 1)

	call := file.Sections[0].Statements[0].Args[0]
	assert.Equal(t, KindIdent, call.Kind)
	assert.Equal(t, "f", call.Value)
	assert.Empty(t, call.Children)
	assert.Equal(t, common.Range{
		Start: common.Pos{Line: 0, Column: 12, Offset: 12},
		End:   common.Pos{Line: 0, Column: 15, Offset: 15},
	}, call.Range)
}

// attrShape is the AST expectation of one attribute in the
// hash-attribute tests: Attribute projected into a comparable shape
// (flag attributes keep the zero exprShape).
type attrShape struct {
	name  string
	value exprShape
	span  common.Range
}

// attrShapes projects attributes into the attrShape form; empty input
// stays nil so the table literals can spell "no attributes" as nil.
func attrShapes(attrs []Attribute) []attrShape {
	if len(attrs) == 0 {
		return nil
	}

	out := make([]attrShape, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, attrShape{
			name:  a.Name,
			value: exprShape{kind: a.Value.Kind, value: a.Value.Value, span: a.Value.Range},
			span:  a.Range,
		})
	}

	return out
}

// TestParse_HashAttributeDiagnostic pins the #-word handling inside a
// one-line attribute block (issue #95): the lexer folds `#` into a word,
// so `#weird` looks like an attribute name and a format-then-reparse
// cycle silently drops the line. The i-loop reports exactly once on the
// hash word and keeps it in the AST as the recovery form; a `#` in value
// position (multi-line attribute value, command argument) is untouched,
// and the existing unexpected path for `#weird(1)` stays as-is.
func TestParse_HashAttributeDiagnostic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		want     []common.Diagnostic
		attrs    []attrShape
		args     []exprShape
		comments []common.Range
	}{
		{
			// The reproducer: `#weird` reaches the i-loop as a word, so
			// the new hash check anchors it; the recovery form keeps the
			// attribute (value tail absorbed) so reparse is stable.
			name: "inline block hash attribute",
			src:  "create_land { #weird 1 }\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 20, Offset: 20}},
				Severity: common.SeverityError,
				Message:  `"#weird" is not an attribute name: #-lines are comments`,
				Code:     "syntax",
			}},
			attrs: []attrShape{{
				name: "#weird",
				value: exprShape{
					kind:  KindNumber,
					value: "1",
					span:  common.Range{Start: common.Pos{Line: 0, Column: 21, Offset: 21}, End: common.Pos{Line: 0, Column: 22, Offset: 22}},
				},
				span: common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 22, Offset: 22}},
			}},
		},
		{
			// A real attribute precedes the hash word: only the hash word
			// is anchored, and the valueless recovery attribute follows
			// base_size in the AST.
			name: "trailing hash flag attribute",
			src:  "create_land { base_size 2 #tag }\n",
			want: []common.Diagnostic{{
				Range:    common.Range{Start: common.Pos{Line: 0, Column: 26, Offset: 26}, End: common.Pos{Line: 0, Column: 30, Offset: 30}},
				Severity: common.SeverityError,
				Message:  `"#tag" is not an attribute name: #-lines are comments`,
				Code:     "syntax",
			}},
			attrs: []attrShape{
				{
					name: "base_size",
					value: exprShape{
						kind:  KindNumber,
						value: "2",
						span:  common.Range{Start: common.Pos{Line: 0, Column: 24, Offset: 24}, End: common.Pos{Line: 0, Column: 25, Offset: 25}},
					},
					span: common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 25, Offset: 25}},
				},
				{
					name:  "#tag",
					value: exprShape{},
					span:  common.Range{Start: common.Pos{Line: 0, Column: 26, Offset: 26}, End: common.Pos{Line: 0, Column: 30, Offset: 30}},
				},
			},
		},
		{
			// A `#` line inside a multi-line block is a comment: the run
			// loop consumes it before the lexer sees it — no diagnostics,
			// the extent lands in RmsFile.Comments.
			name:     "hash comment line inside a block",
			src:      "create_land {\n    # plain comment\n    base_size 2\n}\n",
			comments: []common.Range{{Start: common.Pos{Line: 1, Column: 4, Offset: 18}, End: common.Pos{Line: 1, Column: 19, Offset: 33}}},
		},
		{
			// Boundary: the hash check lives in inlineAttributes only — a
			// `#`-word in value position of a multi-line attribute line
			// parses as a plain ident, no new diagnostics.
			name: "hash word as attribute value in a multiline block",
			src:  "create_land {\n    base_size #tag\n}\n",
			attrs: []attrShape{{
				name: "base_size",
				value: exprShape{
					kind:  KindIdent,
					value: "#tag",
					span:  common.Range{Start: common.Pos{Line: 1, Column: 14, Offset: 28}, End: common.Pos{Line: 1, Column: 18, Offset: 32}},
				},
				span: common.Range{Start: common.Pos{Line: 1, Column: 4, Offset: 18}, End: common.Pos{Line: 1, Column: 18, Offset: 32}},
			}},
		},
		{
			// Boundary: a `#`-word as a command argument is a value too.
			name: "hash word as command argument",
			src:  "create_land #tag\n",
			args: []exprShape{{
				kind:  KindIdent,
				value: "#tag",
				span:  common.Range{Start: common.Pos{Line: 0, Column: 12, Offset: 12}, End: common.Pos{Line: 0, Column: 16, Offset: 16}},
			}},
		},
		{
			// `#weird(1)` fails startsAttribute (next token is `(`), so the
			// gate reports before the hash check can — the existing
			// unexpected path, unchanged, one report per token.
			name: "hash word before a call parenthesis keeps the unexpected path",
			src:  "create_land { #weird(1) }\n",
			want: []common.Diagnostic{
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 14, Offset: 14}, End: common.Pos{Line: 0, Column: 20, Offset: 20}},
					Severity: common.SeverityError,
					Message:  `unexpected "#weird"`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 20, Offset: 20}, End: common.Pos{Line: 0, Column: 21, Offset: 21}},
					Severity: common.SeverityError,
					Message:  `unexpected "("`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 21, Offset: 21}, End: common.Pos{Line: 0, Column: 22, Offset: 22}},
					Severity: common.SeverityError,
					Message:  `unexpected "1"`,
					Code:     "syntax",
				},
				{
					Range:    common.Range{Start: common.Pos{Line: 0, Column: 22, Offset: 22}, End: common.Pos{Line: 0, Column: 23, Offset: 23}},
					Severity: common.SeverityError,
					Message:  `unexpected ")"`,
					Code:     "syntax",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, diags := Parse(tt.src, "test.rms")

			require.NotNil(t, file)
			require.Len(t, file.Sections[0].Statements, 1)

			assert.Equal(t, tt.want, diags)

			if tt.attrs != nil {
				assert.Equal(t, tt.attrs, attrShapes(file.Sections[0].Statements[0].Attributes))
			}

			if tt.args != nil {
				assert.Equal(t, tt.args, argShapes(file.Sections[0].Statements[0].Args))
			}

			if tt.comments != nil {
				assert.Equal(t, tt.comments, file.Comments)
			}
		})
	}
}
