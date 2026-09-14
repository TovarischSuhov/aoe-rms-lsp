package include

import (
	"aoe2-lsp/internal/common"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nthRange builds the exact word range of needle's nth (0-based)
// occurrence in src, counting bytes — the expected site coordinates.
func nthRange(src string, needle string, nth int) common.Range {
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
				return common.Range{
					Start: common.Pos{
						Line:   uint32(line),
						Column: uint32(column),
						Offset: offset,
					},
					End: common.Pos{
						Line:   uint32(line),
						Column: uint32(column) + uint32(len(needle)),
						Offset: offset + len(needle),
					},
				}
			}

			nth--
		}

		column++
		offset++
	}

	panic("nthRange: needle occurrence not found")
}

// TestResolver_RenameSites_NotRenameable checks the local gate: builtins
// in .xs, the command/attribute/section vocabulary of .rms and
// string/comment positions answer found=false — the position is not
// renameable before any closure work happens.
func TestResolver_RenameSites_NotRenameable(t *testing.T) {
	t.Parallel()

	xsSrc := "void m() { trChatPrint(0, \"seed\"); }\n"
	rmsSrc := "<LAND_GENERATION>\ncreate_land\nland_percent 50\n/* the relic isle */\n</LAND_GENERATION>\n"

	tests := []struct {
		name string
		file string
		src  string
		pos  common.Pos
	}{
		{"builtin callee", "b.xs", xsSrc, nthRange(xsSrc, "trChatPrint", 0).Start},
		{"string literal", "b.xs", xsSrc, nthRange(xsSrc, "seed", 0).Start},
		{"command", "t.rms", rmsSrc, nthRange(rmsSrc, "create_land", 0).Start},
		{"attribute", "t.rms", rmsSrc, nthRange(rmsSrc, "land_percent", 0).Start},
		{"section", "t.rms", rmsSrc, nthRange(rmsSrc, "LAND_GENERATION", 0).Start},
		{"comment", "t.rms", rmsSrc, nthRange(rmsSrc, "relic", 0).Start},
	}

	for _, tt := range tests {
		uris := writeTree(t, t.TempDir(), map[string]string{tt.file: tt.src})

		sites, found := NewResolver(fakeSource{}).
			RenameSites(context.Background(), uris[tt.file], tt.pos)

		require.False(t, found, "case=%s", tt.name)
		assert.Empty(t, sites, "case=%s", tt.name)
	}
}

// TestResolver_RenameSites_ParamLocalStaysInFile pins the scope
// boundary: a param binding answers only the declaring file's sites —
// the same-name top-level of that file and any occurrence in an open
// includer stay out of the answer.
func TestResolver_RenameSites_ParamLocalStaysInFile(t *testing.T) {
	t.Parallel()

	lib := "int x = 1;\nvoid f(int x) { x = 2; }\n"
	main := "#includeXS lib.xs\nvoid main() { x = 1; }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"lib.xs":   lib,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["lib.xs"], nthRange(lib, "x", 2).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["lib.xs"], Range: nthRange(lib, "x", 1)}, // the param declaration
		{URI: uris["lib.xs"], Range: nthRange(lib, "x", 2)}, // the shadowed use
	}, sites)
}

// TestResolver_RenameSites_TopLevelDirectAndReverse checks the closure
// merge for a top-level XS binding: the sites of the declaring lib.xs
// plus the inline occurrence in the open includer main.rms, translated
// into .rms file coordinates.
func TestResolver_RenameSites_TopLevelDirectAndReverse(t *testing.T) {
	t.Parallel()

	main := "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n"
	lib := "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":       main,
		"parts/econ.rms": "base_terrain GRASS\n",
		"parts/lib.xs":   lib,
	})

	source := fakeSource{uris["main.rms"]: main, uris["parts/lib.xs"]: lib}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["parts/lib.xs"], nthRange(lib, "sharedFn", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["main.rms"], Range: nthRange(main, "sharedFn", 0)}, // the inline call, .rms coordinates
		{URI: uris["parts/lib.xs"], Range: nthRange(lib, "sharedFn", 0)},
		{URI: uris["parts/lib.xs"], Range: nthRange(lib, "sharedFn", 1)},
	}, sites)
}

// TestResolver_RenameSites_UseInForeignFileNotFound pins the designed
// boundary: rename starts only from a file that resolves the binding
// locally. An inline call of a closure function in the includer and a
// value use of a #const declared in an included file both answer
// found=false.
func TestResolver_RenameSites_UseInForeignFileNotFound(t *testing.T) {
	t.Parallel()

	main := "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n"
	lib := "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"
	nav := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":       main,
		"parts/econ.rms": "base_terrain GRASS\n",
		"parts/lib.xs":   lib,
	})

	constMain := "#include \"const.rms\"\n<LAND_GENERATION>\ncreate_land\nland_percent FOO\n</LAND_GENERATION>\n"
	consts := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":  constMain,
		"const.rms": "#const FOO 5\n",
	})

	tests := []struct {
		name string
		uri  string
		pos  common.Pos
	}{
		{"inline call of a closure function", nav["main.rms"], nthRange(main, "sharedFn", 0).Start},
		{"value use of an included #const", consts["main.rms"], nthRange(constMain, "FOO", 0).Start},
	}

	for _, tt := range tests {
		sites, found := NewResolver(fakeSource{}).
			RenameSites(context.Background(), tt.uri, tt.pos)

		require.False(t, found, "case=%s", tt.name)
		assert.Empty(t, sites, "case=%s", tt.name)
	}
}

// TestResolver_RenameSites_RmsConstAcrossFiles checks the rms half of
// the merge: renaming a #const from its declaring file covers the
// declaration plus the value use in the open includer. The declaration
// is reachable through two closure roots (the file's own and the
// includer's) yet appears once — dedup by (URI, Range) — and the answer
// is sorted (URI, position).
func TestResolver_RenameSites_RmsConstAcrossFiles(t *testing.T) {
	t.Parallel()

	constRms := "#const FOO 5\n"
	main := "#include \"const.rms\"\n<LAND_GENERATION>\ncreate_land\nland_percent FOO\n</LAND_GENERATION>\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":  main,
		"const.rms": constRms,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["const.rms"], nthRange(constRms, "FOO", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["const.rms"], Range: nthRange(constRms, "FOO", 0)}, // the declaration
		{URI: uris["main.rms"], Range: nthRange(main, "FOO", 0)},      // the includer's value use
	}, sites)
}

// TestResolver_RenameSites_ForeignShadowingExcluded checks per-file
// scope classification in the merge: an inline use without a local
// binding in the includer joins, the param-shadowed occurrences of
// another included .xs do not.
func TestResolver_RenameSites_ForeignShadowingExcluded(t *testing.T) {
	t.Parallel()

	main := "#includeXS lib.xs\n#includeXS other.xs\nvoid main() { sharedN = 1; }\n"
	lib := "int sharedN = 1;\n"
	other := "void h(int sharedN) { sharedN = 2; }\nvoid k() { sharedN = 3; }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"lib.xs":   lib,
		"other.xs": other,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["lib.xs"], nthRange(lib, "sharedN", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["lib.xs"], Range: nthRange(lib, "sharedN", 0)},
		{URI: uris["main.rms"], Range: nthRange(main, "sharedN", 0)},
		{URI: uris["other.xs"], Range: nthRange(other, "sharedN", 2)}, // the unshadowed use only
	}, sites)
}

// TestResolver_RenameSites_SameNameTopLevelMerged checks the name-based
// merge: a same-name top-level declaration in another file of the
// closure joins the answer with its own sites.
func TestResolver_RenameSites_SameNameTopLevelMerged(t *testing.T) {
	t.Parallel()

	main := "#includeXS lib.xs\n#includeXS other.xs\n"
	lib := "int sharedN = 1;\n"
	other := "int sharedN = 5;\nvoid k() { sharedN = 3; }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"lib.xs":   lib,
		"other.xs": other,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["lib.xs"], nthRange(lib, "sharedN", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["lib.xs"], Range: nthRange(lib, "sharedN", 0)},
		{URI: uris["other.xs"], Range: nthRange(other, "sharedN", 0)}, // the foreign declaration
		{URI: uris["other.xs"], Range: nthRange(other, "sharedN", 1)}, // and its use
	}, sites)
}

// TestResolver_RenameSites_ForeignInitializerOnlyDecl pins the foreign
// half of the initializer quirk guard: a same-name top-level variable
// whose only occurrence in a foreign file is the declaration token (an
// initializer, no uses) still joins the merge — its declaration is a
// rename site, not a silent drop out of the WorkspaceEdit.
func TestResolver_RenameSites_ForeignInitializerOnlyDecl(t *testing.T) {
	t.Parallel()

	lib := "int SHARED = 1;\nvoid a() { SHARED = 2; }\n"
	other := "int SHARED = 5;\n" // the initializer-only twin: no uses
	main := "#includeXS lib.xs\n#includeXS other.xs\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"lib.xs":   lib,
		"other.xs": other,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["lib.xs"], nthRange(lib, "SHARED", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["lib.xs"], Range: nthRange(lib, "SHARED", 0)},
		{URI: uris["lib.xs"], Range: nthRange(lib, "SHARED", 1)},
		{URI: uris["other.xs"], Range: nthRange(other, "SHARED", 0)},
	}, sites)
}

// TestResolver_RenameSites_InlineBlockDeclaration pins the guard for
// the queried .rms file's own inline block: a top-level variable
// declared inside the block (with an initializer, queried at its
// declaration token) still merges across the closure — the answer must
// not depend on which file of the binding the cursor sits in.
func TestResolver_RenameSites_InlineBlockDeclaration(t *testing.T) {
	t.Parallel()

	main := "#includeXS other.xs\nint TWEAK = 1;\nvoid f() { TWEAK = 2; }\n"
	other := "int TWEAK = 5;\nvoid k() { TWEAK = 3; }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"other.xs": other,
	})

	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["main.rms"], nthRange(main, "TWEAK", 0).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["main.rms"], Range: nthRange(main, "TWEAK", 0)}, // the block declaration, .rms coordinates
		{URI: uris["main.rms"], Range: nthRange(main, "TWEAK", 1)}, // the block use
		{URI: uris["other.xs"], Range: nthRange(other, "TWEAK", 0)},
		{URI: uris["other.xs"], Range: nthRange(other, "TWEAK", 1)},
	}, sites)
}

// TestResolver_RenameSites_CrlfText pins the offset spaces of the
// merge key: the RMS parser counts offsets after CRLF normalization
// while the editor state keeps the raw bytes — the extracted site text
// must still name the binding, so a CRLF document keeps every closure
// file in the answer. The expected ranges are built on the LF form:
// that is the parser's offset space (line/column are unaffected).
func TestResolver_RenameSites_CrlfText(t *testing.T) {
	t.Parallel()

	mainLF := "#const FOO 5\n#include \"part.rms\"\n" +
		"<LAND_GENERATION>\ncreate_land\nland_percent FOO\n</LAND_GENERATION>\n"
	partLF := "<LAND_GENERATION>\nbase_terrain FOO\n</LAND_GENERATION>\n"
	main := strings.ReplaceAll(mainLF, "\n", "\r\n")
	part := strings.ReplaceAll(partLF, "\n", "\r\n")
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms": main,
		"part.rms": part,
	})

	// the open document carries the raw CRLF editor state
	source := fakeSource{uris["main.rms"]: main}

	sites, found := NewResolver(source).RenameSites(
		context.Background(), uris["main.rms"], nthRange(main, "FOO", 1).Start)

	require.True(t, found)
	assert.Equal(t, []Target{
		{URI: uris["main.rms"], Range: nthRange(mainLF, "FOO", 0)},
		{URI: uris["main.rms"], Range: nthRange(mainLF, "FOO", 1)},
		{URI: uris["part.rms"], Range: nthRange(partLF, "FOO", 0)},
	}, sites)
}

// TestResolver_RenameSites_DefinitionConsistency pins the invariant
// shared with Definition: from every rename site, Definition lands on a
// site of the same answer — rename never edits a position Definition
// would not attribute to the binding.
func TestResolver_RenameSites_DefinitionConsistency(t *testing.T) {
	t.Parallel()

	main := "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n"
	lib := "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"
	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":       main,
		"parts/econ.rms": "base_terrain GRASS\n",
		"parts/lib.xs":   lib,
	})

	r := NewResolver(fakeSource{uris["main.rms"]: main, uris["parts/lib.xs"]: lib})

	sites, found := r.RenameSites(
		context.Background(), uris["parts/lib.xs"], nthRange(lib, "sharedFn", 0).Start)
	require.True(t, found)
	require.NotEmpty(t, sites)

	siteKeys := make(map[targetKey]bool, len(sites))
	for _, s := range sites {
		siteKeys[targetKey{uri: s.URI, r: s.Range}] = true
	}

	for _, s := range sites {
		def, ok := r.Definition(context.Background(), s.URI, s.Range.Start)
		require.True(t, ok, "site %s %v", s.URI, s.Range.Start)
		assert.True(t, siteKeys[targetKey{uri: def.URI, r: def.Range}],
			"the definition of site %s %v must itself be a rename site", s.URI, s.Range.Start)
	}
}
