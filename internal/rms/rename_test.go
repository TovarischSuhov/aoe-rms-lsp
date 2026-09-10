package rms

import (
	"aoe2-lsp/internal/common"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// posAtN builds the full position (line, column, offset) of the nth
// (0-based) occurrence of needle in src, counting bytes.
func posAtN(src string, needle string, nth int) common.Pos {
	line, column, offset := 0, 0, 0

	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			line++
			column = 0
			offset++

			continue
		}

		if strings.HasPrefix(src[i:], needle) {
			if nth == 0 {
				return common.Pos{
					Line:   uint32(line),
					Column: uint32(column),
					Offset: offset,
				}
			}

			nth--
		}

		column++
		offset++
	}

	panic("posAtN: needle occurrence not found")
}

// rangeAtN builds the exact word range of needle's nth occurrence.
func rangeAtN(src string, needle string, nth int) common.Range {
	start := posAtN(src, needle, nth)

	return common.Range{
		Start: start,
		End: common.Pos{
			Line:   start.Line,
			Column: start.Column + uint32(len(needle)),
			Offset: start.Offset + len(needle),
		},
	}
}

// renameSrc covers the positional discrimination: FOO and BAR are
// user-declared (#const/#define), players collides with the command
// vocabulary. FOO occurs as: 0 — #const declaration, 1 — attribute
// value, 2 — second attribute value; players as: 0 — #const
// declaration, 1 — value use, 2 — command position.
const renameSrc = `#const FOO 5
#define BAR 7
#const players 3

<LAND_GENERATION>
create_land
	land_percent FOO
	base_terrain BAR
	terrain_state players FOO
</LAND_GENERATION>

<PLAYER_SETUP>
players 2
</PLAYER_SETUP>
`

// TestRenameSites_DeclarationAndUses checks the happy path: a value
// use of FOO returns the declaration plus every value-position
// occurrence, sorted, with no duplicates.
func TestRenameSites_DeclarationAndUses(t *testing.T) {
	t.Parallel()

	file, _ := Parse(renameSrc, "rename.rms")

	ranges, found := file.RenameSites(posAtN(renameSrc, "FOO", 1))

	expected := []common.Range{
		rangeAtN(renameSrc, "FOO", 0),
		rangeAtN(renameSrc, "FOO", 1),
		rangeAtN(renameSrc, "FOO", 2),
	}

	require.True(t, found)
	assert.Equal(t, expected, ranges)
}

// TestRenameSites_NameCollidesWithCommand checks the contract's
// collision rule: #const players 3 does not make the command word
// players a rename site — the command position answers found=false,
// the value use answers declaration plus use.
func TestRenameSites_NameCollidesWithCommand(t *testing.T) {
	t.Parallel()

	file, _ := Parse(renameSrc, "rename.rms")

	_, found := file.RenameSites(posAtN(renameSrc, "players", 1))
	require.True(t, found)

	// The command position is not a site of the constant.
	_, found = file.RenameSites(posAtN(renameSrc, "players", 2))
	assert.False(t, found)

	// The same query from the declaration position agrees.
	fromDecl, _ := file.RenameSites(posAtN(renameSrc, "players", 0))
	assert.Equal(t, []common.Range{
		rangeAtN(renameSrc, "players", 0),
		rangeAtN(renameSrc, "players", 1),
	}, fromDecl)
}

// TestRenameSites_NotRenameable covers the found=false vocabulary:
// commands, attribute names, section header words and builtin kb
// constants have no user declaration to rename.
func TestRenameSites_NotRenameable(t *testing.T) {
	t.Parallel()

	src := "<LAND_GENERATION>\ncreate_land\nland_percent 50\n</LAND_GENERATION>\ncreate_object cColorBlue\n"
	file, _ := Parse(src, "norename.rms")

	for _, needle := range []string{"LAND_GENERATION", "create_land", "land_percent", "cColorBlue"} {
		_, found := file.RenameSites(posAtN(src, needle, 0))
		assert.False(t, found, "needle=%s", needle)
	}
}

// TestRenameSites_RegressionReferencesUntouched pins the references
// contract: same-name words of every kind still answer — the rename
// discrimination does not leak into ReferencesAt.
func TestRenameSites_RegressionReferencesUntouched(t *testing.T) {
	t.Parallel()

	file, _ := Parse(renameSrc, "rename.rms")

	ranges := file.ReferencesAt(posAtN(renameSrc, "players", 1))
	assert.Len(t, ranges, 3) // declaration + value use + command word
}
