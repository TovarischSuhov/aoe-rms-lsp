package kb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractRmsCommands_GlossaryFillsAttrDescs covers the glossary merge in
// the reference-docs pass: attribute doc blocks fill Desc (and the bounds of
// the first documented bullet) of the command attributes, while the
// structured skeleton kind survives the merge. On the real guide nothing
// stays empty — every attribute instance and every positional argument
// carries a description.
func TestExtractRmsCommands_GlossaryFillsAttrDescs(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	var attrs, attrGaps, argGaps int

	for _, cmd := range commands {
		for _, attr := range cmd.Attributes {
			attrs++

			if attr.Desc == "" {
				attrGaps++
			}
		}

		for _, arg := range cmd.Args {
			if arg.Desc == "" {
				argGaps++
			}
		}
	}

	assert.Equal(t, 142, attrs, "attribute instances extracted from the guide")
	assert.Equal(t, 0, attrGaps, "attributes left without a desc after the merge")
	assert.Equal(t, 0, argGaps, "positional args left without a documented desc")

	land, found := byName["create_land"]
	require.True(t, found)

	tests := []struct {
		attr   string
		desc   string
		prefix bool
		kind   string
		rng    ValueRange
	}{
		{
			attr: "land_percent",
			desc: "Percentage of the total map that the land should grow to cover.",
			kind: "percent",
			rng:  ValueRange{Min: "0", Max: "100"},
		},
		{
			attr: "land_position",
			desc: "Specify the exact origin point for a land, as a percentage of total map dimensions.",
			kind: "percent",
			rng:  ValueRange{Min: "0", Max: "99"},
		},
		{
			// Bounds come from mining the merged prose, not from a bullet.
			attr:   "clumping_factor",
			desc:   "The extent to which land growth prefers to clump together near existing tiles.",
			prefix: true,
			kind:   "number",
			rng:    ValueRange{Min: "11", Max: "40"},
		},
		{
			// Flag doc block: prose only, nothing to mine.
			attr: "set_circular_base",
			desc: "The square land origin becomes a circle of the size that would be exactly inscribed by the square.",
			kind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.attr, func(t *testing.T) {
			got := attrOf(t, land, tt.attr)

			if tt.prefix {
				assert.True(t, strings.HasPrefix(got.Desc, tt.desc), "desc = %q", got.Desc)
			} else {
				assert.Equal(t, tt.desc, got.Desc)
			}

			assert.Equal(t, tt.kind, got.Kind, "structured kind is not overwritten")
			assert.Equal(t, tt.rng, got.Range)
		})
	}
}

// TestExtractRmsCommands_VariantsBySection covers the variant selection
// rule: when several glossary records document one attribute name, the
// record whose Example block names the section of the command wins; with no
// section match the first record of the name does.
func TestExtractRmsCommands_VariantsBySection(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	tests := []struct {
		cmd    string
		attr   string
		desc   string
		prefix bool
	}{
		{
			cmd:  "create_land",
			attr: "number_of_tiles",
			desc: "Fixed number of tiles that the land should grow by.",
		},
		{
			cmd:    "create_elevation",
			attr:   "number_of_tiles",
			desc:   "Total base tile count",
			prefix: true,
		},
		{
			// The elevation record's Example block names two sections; the
			// land record names one — the elevation command must still take
			// the elevation variant.
			cmd:  "create_elevation",
			attr: "base_layer",
			desc: "Use this attribute in addition to base_terrain if (and only if) you specified a layer for the map base terrain at the beginning of <LAND_GENERATION>.",
		},
		{
			cmd:  "create_terrain",
			attr: "base_layer",
			desc: "Specify a layered terrain on which you want to place your new terrain.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.cmd+"."+tt.attr, func(t *testing.T) {
			cmd, found := byName[tt.cmd]
			require.True(t, found)

			got := attrOf(t, cmd, tt.attr).Desc

			if tt.prefix {
				assert.True(t, strings.HasPrefix(got, tt.desc), "desc = %q", got)

				return
			}

			assert.Equal(t, tt.desc, got)
		})
	}
}

// TestExtractRmsCommands_CommandDescUnchanged pins the command semantics of
// the doc pass: the glossary pass must not move the last-wins command
// records, so the command descriptions of the committed data stay put.
func TestExtractRmsCommands_CommandDescUnchanged(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)
	require.Len(t, commands, 53)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	assert.Equal(t,
		"Specify the base terrain on which you want to place your new terrain.",
		byName["base_terrain"].Desc)
	assert.Equal(t,
		"Specify a layered terrain on which you want to place your new terrain.",
		byName["base_layer"].Desc)
}

// TestExtractRmsCommands_RndDoc covers the rnd(min,max) functional form: the
// name is matched with the parenthesized suffix cut, so its doc block fills
// the command desc, the game versions and both positional args.
func TestExtractRmsCommands_RndDoc(t *testing.T) {
	t.Parallel()
	commands, err := ExtractRmsCommands(zetnusGuide, nil)
	require.NoError(t, err)

	byName := make(map[string]Command, len(commands))
	for _, cmd := range commands {
		byName[cmd.Name] = cmd
	}

	rnd, found := byName["rnd"]
	require.True(t, found)

	assert.Equal(t, "Randomize a numeric argument between min and max (inclusive).", rnd.Desc)
	assert.Equal(t, "UP/DE", rnd.GameVersions)

	require.Len(t, rnd.Args, 2)

	tests := []struct {
		idx  int
		name string
		desc string
	}{
		{idx: 0, name: "min", desc: "number"},
		{idx: 1, name: "max", desc: "number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := rnd.Args[tt.idx]

			assert.Equal(t, tt.name, arg.Name)
			assert.Equal(t, tt.desc, arg.Desc)
		})
	}
}

// TestLooksLikeSignature_PercentPlaceholder covers the placeholder gate of
// the doc pass: two-letter %<Capital> tokens are argument placeholders, and
// the non-placeholders of the guide prose stay rejected.
func TestLooksLikeSignature_PercentPlaceholder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tokens []string
		want   bool
	}{
		{name: "percent placeholder X", tokens: []string{"%X"}, want: true},
		{name: "percent placeholder pair", tokens: []string{"%X", "%Y"}, want: true},
		{name: "numeric literal", tokens: []string{"20"}, want: false},
		{name: "all-caps constant", tokens: []string{"GOLD"}, want: false},
		{
			// Metadata fragments must not read as placeholders, or prose
			// lines with a "(default: …)" tail pass the gate.
			name:   "metadata fragment",
			tokens: []string{"(default:"},
			want:   false,
		},
		{name: "single capital placeholder", tokens: []string{"N"}, want: true},
		{name: "bare percent", tokens: []string{"%"}, want: true},
		{name: "macro token", tokens: []string{"TYPE"}, want: true},
		{name: "block brace", tokens: []string{"{"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeSignature(tt.tokens))
		})
	}
}

// attrOf returns the first attribute of cmd with the given name; it fails
// the test when the command has no such attribute.
func attrOf(t *testing.T, cmd Command, name string) CommandArg {
	t.Helper()

	for _, attr := range cmd.Attributes {
		if attr.Name == name {
			return attr
		}
	}

	t.Fatalf("command %s has no attribute %s", cmd.Name, name)

	return CommandArg{}
}
