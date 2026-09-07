package rms

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
)

func TestRmsSymbols_APIShape(t *testing.T) {
	file, _ := Parse("<LAND_GENERATION>\n</LAND_GENERATION>\n", "shape.rms")

	syms := file.Symbols()

	require.IsType(t, []common.Symbol{}, syms)
	require.Len(t, syms, 1)
	require.IsType(t, "", syms[0].Kind)
	require.IsType(t, common.Range{}, syms[0].Selection)
}

func TestRmsReferencesAt_APIShape(t *testing.T) {
	file, _ := Parse("create_terrain FOREST\n", "shape.rms")

	ranges := file.ReferencesAt(common.Pos{Line: 0, Column: 15})

	require.IsType(t, []common.Range{}, ranges)
}

func TestRmsSymbols_SectionTreeWithNestedBlocks(t *testing.T) {
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
	src := "land_percent 12\n"

	file, _ := Parse(src, "number.rms")

	require.Empty(t, file.ReferencesAt(common.Pos{Line: 0, Column: uint32(strings.Index(src, "12"))}))
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
