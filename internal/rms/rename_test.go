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

// TestRenameRefs_EquivalentToRenameSites pins the equivalence contract:
// for a name this file declares, RenameRefs(name) answers exactly what
// RenameSites answers from any site of that name — the cross-file merge
// may substitute the by-name call for the positional one.
func TestRenameRefs_EquivalentToRenameSites(t *testing.T) {
	t.Parallel()

	file, _ := Parse(renameSrc, "rename.rms")

	tests := []struct {
		name string
		nth  int
	}{
		{"FOO", 1},
		{"BAR", 0},
		{"players", 1},
	}

	for _, tt := range tests {
		fromSites, found := file.RenameSites(posAtN(renameSrc, tt.name, tt.nth))
		require.True(t, found, "name=%s", tt.name)
		assert.Equal(t, fromSites, file.RenameRefs(tt.name), "name=%s", tt.name)
	}
}

// TestRenameRefs_Table pins the by-name answer: declaration plus every
// value-position occurrence for declared names; the command position of
// a name twin is not a site; names absent from the file — unknown
// identifiers and the command vocabulary — answer empty.
func TestRenameRefs_Table(t *testing.T) {
	t.Parallel()

	file, _ := Parse(renameSrc, "rename.rms")

	tests := []struct {
		name string
		want []common.Range
	}{
		{"FOO", []common.Range{
			rangeAtN(renameSrc, "FOO", 0),
			rangeAtN(renameSrc, "FOO", 1),
			rangeAtN(renameSrc, "FOO", 2),
		}},
		{"BAR", []common.Range{
			rangeAtN(renameSrc, "BAR", 0),
			rangeAtN(renameSrc, "BAR", 1),
		}},
		{"players", []common.Range{ // the command position (occurrence 2) never enters
			rangeAtN(renameSrc, "players", 0),
			rangeAtN(renameSrc, "players", 1),
		}},
		{"NOPE", nil},
		{"create_land", nil},
	}

	for _, tt := range tests {
		if tt.want == nil {
			assert.Empty(t, file.RenameRefs(tt.name), "name=%s", tt.name)

			continue
		}

		assert.Equal(t, tt.want, file.RenameRefs(tt.name), "name=%s", tt.name)
	}
}

// TestRenameRefs_UndeclaredValueUse pins the merge half of the by-name
// contract: value occurrences of a name this file does NOT declare are
// still returned (RenameSites answers found=false there) — a foreign
// file of the closure has no declaration to anchor on, so the merge
// keys on the name alone.
func TestRenameRefs_UndeclaredValueUse(t *testing.T) {
	t.Parallel()

	src := "<LAND_GENERATION>\ncreate_land\nland_percent UNDECL\nbase_terrain UNDECL\n</LAND_GENERATION>\n"
	file, _ := Parse(src, "undecl.rms")

	assert.Equal(t, []common.Range{
		rangeAtN(src, "UNDECL", 0),
		rangeAtN(src, "UNDECL", 1),
	}, file.RenameRefs("UNDECL"))
}
