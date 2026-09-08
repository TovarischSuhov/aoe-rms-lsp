package rms

import (
	"aoe2-lsp/common"
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
	require.Len(t, first.Children, 2)
	assert.Equal(t, "create_object", first.Children[0].Name)
	assert.Equal(t, "number_of_objects", first.Children[1].Name)

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
	require.Len(t, objects.Statements, 2)
	assert.Equal(t, "create_object", objects.Statements[0].Name)
	assert.Equal(t, "number_of_objects", objects.Statements[1].Name,
		"attributes outside braces become plain statements")
}

// TestParse_NeverNil checks the total-garbage path.
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
