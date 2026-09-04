package rms

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
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
	file, diags := Parse(loadFixture(t, "includes.rms"), "includes.rms")
	require.Empty(t, diags)

	assert.Equal(t, []string{"Team_Islands_lands.rms"}, file.Includes)

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
