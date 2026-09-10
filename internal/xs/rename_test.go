package xs

import (
	"aoe2-lsp/internal/common"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// posAtN builds the position of the nth (0-based) occurrence of needle
// in src, counting byte columns per line (multi-line fixtures).
func posAtN(src string, needle string, nth int) common.Pos {
	line, col := 0, 0

	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 0

			continue
		}

		if strings.HasPrefix(src[i:], needle) {
			if nth == 0 {
				return common.Pos{Line: uint32(line), Column: uint32(col)}
			}

			nth--
		}

		col++
	}

	panic("posAtN: needle occurrence not found")
}

// renameSrc covers the shadowing chain: a top-level x, a param x in f
// (x@2, x@5), an inner-block local x (x@3 decl, x@4) and a top-level
// use in g (x@6).
const renameSrc = `int x = 1;

void f(int x) {
	x = 2;
	{
		int x = 3;
		x = 4;
	}
	x = 5;
}

void g() {
	x = 6;
}
`

// TestRenameSites_ShadowingScopes checks that sites follow the binding,
// not the name: the local use sees only the local, the param use only
// the param occurrences outside the inner block, the top-level use only
// the top-level pair.
func TestRenameSites_ShadowingScopes(t *testing.T) {
	t.Parallel()

	file, _ := XsParse(renameSrc, "rename.xs")

	sites, found := file.RenameSites(posAtN(renameSrc, "x", 4))
	require.True(t, found)
	require.Len(t, sites, 2)
	assert.Equal(t, []bool{true, false}, declFlags(sites))
	assert.Equal(t, KindLocal, sites[0].Kind)

	def, ok := file.Definition(posAtN(renameSrc, "x", 4))
	require.True(t, ok)
	assert.Equal(t, sites[0].Range, def)

	sites, found = file.RenameSites(posAtN(renameSrc, "x", 2))
	require.True(t, found)
	require.Len(t, sites, 3)
	assert.Equal(t, []bool{true, false, false}, declFlags(sites))
	assert.Equal(t, KindParam, sites[0].Kind)

	sites, found = file.RenameSites(posAtN(renameSrc, "x", 6))
	require.True(t, found)
	require.Len(t, sites, 2)
	assert.Equal(t, []bool{true, false}, declFlags(sites))
	assert.Equal(t, DeclVariable, sites[0].Kind)
}

// TestRenameSites_DefinitionConsistency walks every occurrence of x and
// checks the contract invariant: Definition(pos) names the declaration
// site that RenameSites(pos) reports with Decl=true.
func TestRenameSites_DefinitionConsistency(t *testing.T) {
	t.Parallel()

	file, _ := XsParse(renameSrc, "rename.xs")

	for nth := range 7 {
		pos := posAtN(renameSrc, "x", nth)

		def, ok := file.Definition(pos)
		require.True(t, ok, "nth=%d", nth)

		sites, found := file.RenameSites(pos)
		require.True(t, found, "nth=%d", nth)

		var decl common.Range

		for _, s := range sites {
			if s.Decl {
				decl = s.Range
			}
		}

		assert.Equal(t, def, decl, "nth=%d", nth)
		assert.NotZero(t, decl, "nth=%d: no Decl site", nth)
	}
}

// TestRenameSites_NotRenameable: a builtin callee has no declaring
// binding; a position off identifiers resolves to no occurrence.
func TestRenameSites_NotRenameable(t *testing.T) {
	t.Parallel()

	src := "void h() { trChatPrint(0, \"x\"); }"
	file, _ := XsParse(src, "builtin.xs")

	_, found := file.RenameSites(posOf(src, "trChatPrint", 0))
	assert.False(t, found)

	_, found = file.RenameSites(posOf(src, "0", 0))
	assert.False(t, found)
}

// declFlags extracts the Decl column of the sites.
func declFlags(sites []RenameSite) []bool {
	out := make([]bool, len(sites))
	for i := range sites {
		out[i] = sites[i].Decl
	}

	return out
}
