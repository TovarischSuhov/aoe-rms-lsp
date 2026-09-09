package include

import (
	"aoe2-lsp/internal/common"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/uri"
)

// writeTree writes name→content files under dir and returns their URIs.
func writeTree(t *testing.T, dir string, tree map[string]string) map[string]string {
	t.Helper()

	uris := make(map[string]string, len(tree))
	for name, content := range tree {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		uris[name] = uri.File(path).String()
	}

	return uris
}

// TestResolver_APIShape checks the contract surface: NewResolver builds a
// *Resolver over a Source and Closure answers with a Closure value.
func TestResolver_APIShape(t *testing.T) {
	t.Parallel()
	r := NewResolver(fakeSource{})
	require.NotNil(t, r)

	c := r.Closure(context.Background(), "file:///none.rms")
	assert.Equal(t, "file:///none.rms", c.Root)
}

// TestResolver_ClosureDFSAndMissing checks the core closure: DFS order,
// resolved directives and one MissingInclude per missing directive.
func TestResolver_ClosureDFSAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// missing.rms is deliberately NOT written
	uris := writeTree(t, dir, map[string]string{
		"main.rms": "#include \"a.rms\"\n#includeXS lib.xs\n#include missing.rms\n",
		"a.rms":    "#include \"b.rms\"\n",
		"b.rms":    "",
		"lib.xs":   "void sharedFn(int n) { }\n",
	})

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["main.rms"])

	require.Len(t, c.Rms, 3) // main, a, b — DFS order
	assert.Equal(t, uris["main.rms"], c.Rms[0].URI)
	assert.Equal(t, uris["a.rms"], c.Rms[1].URI)
	assert.Equal(t, uris["b.rms"], c.Rms[2].URI)

	require.Len(t, c.Xs, 1)
	assert.Equal(t, uris["lib.xs"], c.Xs[0].URI)
	assert.Equal(t, "sharedFn", c.Xs[0].File.Decls[0].Name)

	require.Len(t, c.Resolved, 3)
	assert.Equal(t, uris["a.rms"], c.Resolved[0].Target)
	assert.Equal(t, uris["b.rms"], c.Resolved[1].Target) // a's subtree resolves first (DFS)
	assert.Equal(t, uris["lib.xs"], c.Resolved[2].Target)

	require.Len(t, c.Missing, 1)
	assert.Equal(t, uris["main.rms"], c.Missing[0].Owner)
	assert.Equal(t, "missing.rms", c.Missing[0].Path)
	assert.Equal(t, uint32(2), c.Missing[0].Range.Start.Line)
	assert.True(t, c.Missing[0].Range.Start.Before(c.Missing[0].Range.End))
}

// TestResolver_EditorStateWinsOverDisk checks that an open document's
// text overrides its disk copy.
func TestResolver_EditorStateWinsOverDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		"a.rms":   "#include \"old.rms\"\n",
		"old.rms": "",
		"new.rms": "",
	})

	source := fakeSource{uris["a.rms"]: "#include \"new.rms\"\n"}
	r := NewResolver(source)
	c := r.Closure(context.Background(), uris["a.rms"])

	require.Len(t, c.Resolved, 1)
	targetPath := uri.URI(c.Resolved[0].Target).FsPath()
	assert.Contains(t, targetPath, "new.rms")
}

// TestResolver_RootUnavailable checks degradation: an unopened and
// unreadable root yields an empty closure, no panic.
func TestResolver_RootUnavailable(t *testing.T) {
	t.Parallel()
	r := NewResolver(fakeSource{})

	c := r.Closure(context.Background(), "file:///no/such/file.rms")
	assert.Equal(t, "file:///no/such/file.rms", c.Root)
	assert.Empty(t, c.Rms)
	assert.Empty(t, c.Xs)
	assert.Empty(t, c.Resolved)
	assert.Empty(t, c.Missing)
}

// TestResolver_CycleTerminates checks that A→B→A cycles visit each file
// once and the call returns.
func TestResolver_CycleTerminates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		"a.rms": "#include \"b.rms\"\n",
		"b.rms": "#include \"a.rms\"\n",
	})

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["a.rms"])

	assert.Len(t, c.Rms, 2)
	assert.Len(t, c.Resolved, 2)
	assert.Empty(t, c.Missing)
}

// TestResolver_DepthAndCountLimits checks that a deep chain stops
// expanding at maxDepth while the boundary entry stays in the closure.
func TestResolver_DepthAndCountLimits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tree := map[string]string{"f70.rms": ""}
	for i := range 70 {
		tree[fmt.Sprintf("f%d.rms", i)] = fmt.Sprintf("#include \"f%d.rms\"\n", i+1)
	}
	tree["f70.rms"] = ""

	uris := writeTree(t, dir, tree)

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["f0.rms"])

	// f0 (depth 0) .. f64 (depth 64) enter; f65+ are not expanded.
	assert.Len(t, c.Rms, maxDepth+1)
	assert.Empty(t, c.Missing)
}

// navTree builds the shared navigation fixture: main includes econ.rms
// and lib.xs; the inline block calls sharedFn.
func navTree(t *testing.T) map[string]string {
	t.Helper()

	return writeTree(t, t.TempDir(), map[string]string{
		"main.rms":       "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n",
		"parts/econ.rms": "base_terrain GRASS\n",
		"parts/lib.xs":   "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n",
	})
}

// TestResolver_DefinitionIncludeDirective checks scenario 1: the cursor on
// an include path jumps to the target file start (0:0, zero length).
func TestResolver_DefinitionIncludeDirective(t *testing.T) {
	t.Parallel()
	uris := navTree(t)

	r := NewResolver(fakeSource{})
	target, found := r.Definition(context.Background(), uris["main.rms"], common.Pos{Line: 0, Column: 12})

	require.True(t, found)
	assert.Equal(t, uris["parts/econ.rms"], target.URI)
	assert.Equal(t, common.Range{}, target.Range)
}

// TestResolver_DefinitionExternalDecl checks scenario 2: an inline XS name
// resolves to the declaring file's name range.
func TestResolver_DefinitionExternalDecl(t *testing.T) {
	t.Parallel()
	uris := navTree(t)

	// sharedFn sits on line 2 (the inline block), column 14..22
	prefix := len("#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\n")
	pos := common.Pos{Line: 2, Column: 18, Offset: prefix + 18}

	r := NewResolver(fakeSource{})
	target, found := r.Definition(context.Background(), uris["main.rms"], pos)

	require.True(t, found)
	assert.Equal(t, uris["parts/lib.xs"], target.URI)
	assert.Equal(t, common.Pos{Line: 0, Column: 5, Offset: 5}, target.Range.Start)
	assert.Equal(t, common.Pos{Line: 0, Column: 13, Offset: 13}, target.Range.End)
}

// TestResolver_DefinitionBuiltinNotFound checks the negative: builtins do
// not resolve.
func TestResolver_DefinitionBuiltinNotFound(t *testing.T) {
	t.Parallel()
	uris := writeTree(t, t.TempDir(), map[string]string{
		"b.xs": "void m() { xsGetMapSeed(); }\n",
	})

	r := NewResolver(fakeSource{})
	_, found := r.Definition(context.Background(), uris["b.xs"], common.Pos{Line: 0, Column: 15, Offset: 15})

	assert.False(t, found)
}

// TestResolver_ReferencesReverseOverOpenDocs checks scenario 3: references
// from the included lib.xs find occurrences in the including main.rms.
func TestResolver_ReferencesReverseOverOpenDocs(t *testing.T) {
	t.Parallel()
	uris := navTree(t)

	source := fakeSource{uris["main.rms"]: readFile(t, uris["main.rms"]), uris["parts/lib.xs"]: readFile(t, uris["parts/lib.xs"])}
	r := NewResolver(source)

	// cursor on the sharedFn declaration in lib.xs (line 0, column 5..14)
	pos := common.Pos{Line: 0, Column: 8, Offset: 8}

	refs := r.References(context.Background(), uris["parts/lib.xs"], pos)
	require.Len(t, refs, 3) // decl + call in lib.xs, inline call in main.rms

	byURI := map[string]int{}
	for _, ref := range refs {
		byURI[ref.URI]++
	}
	assert.Equal(t, 2, byURI[uris["parts/lib.xs"]])
	assert.Equal(t, 1, byURI[uris["main.rms"]])

	// sorted by (URI, position)
	assert.True(t, refs[0].URI <= refs[1].URI)
	assert.True(t, refs[1].URI <= refs[2].URI)
}

// TestResolver_ReferencesDedupAndSort checks that duplicate collection
// through multiple roots collapses and the result is sorted.
func TestResolver_ReferencesDedupAndSort(t *testing.T) {
	t.Parallel()
	uris := navTree(t)

	source := fakeSource{uris["main.rms"]: readFile(t, uris["main.rms"]), uris["parts/lib.xs"]: readFile(t, uris["parts/lib.xs"])}
	r := NewResolver(source)

	pos := common.Pos{Line: 0, Column: 8, Offset: 8}
	refs := r.References(context.Background(), uris["parts/lib.xs"], pos)

	keys := map[targetKey]int{}
	for _, ref := range refs {
		keys[targetKey{uri: ref.URI, r: ref.Range}]++
	}
	for _, n := range keys {
		assert.Equal(t, 1, n, "no duplicates by (URI, Range)")
	}
}

// readFile loads a file:// URI's content for fixtures.
func readFile(t *testing.T, uriArg string) string {
	t.Helper()

	raw, err := os.ReadFile(uri.URI(uriArg).FsPath())
	require.NoError(t, err)

	return string(raw)
}

// TestResolver_EscapeBeyondRootMissing checks the workspace bound: a
// #include escaping the root document's directory becomes MissingInclude
// even when the target exists on disk.
func TestResolver_EscapeBeyondRootMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inner := filepath.Join(dir, "maps")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outside.rms"),
		[]byte("create_elevator 7\n"), 0o644))

	uris := writeTree(t, dir, map[string]string{
		"maps/main.rms": "#include ../outside.rms\n",
	})

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["maps/main.rms"])

	assert.Empty(t, c.Resolved)
	require.Len(t, c.Missing, 1)
	assert.Equal(t, "../outside.rms", c.Missing[0].Path)
}

// TestResolver_DirectoryTargetMissing checks a directory target: not a
// regular file → MissingInclude, not a silently dropped Resolved entry.
func TestResolver_DirectoryTargetMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))

	uris := writeTree(t, dir, map[string]string{"main.rms": "#include sub\n"})

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["main.rms"])

	assert.Empty(t, c.Resolved)
	require.Len(t, c.Missing, 1)
}

// TestResolver_TwoSpellingsOwnTargetURI checks the cached disk copy does
// not leak the first requester's URI spelling into targets: each query
// gets its own spelling back.
func TestResolver_TwoSpellingsOwnTargetURI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"sub/main.rms": "#includeXS\nint q = 1;\nvoid f() { q = 2; }\n",
	})

	plain := uri.File(filepath.Join(dir, "sub", "main.rms")).String()
	dotted := "file://" + filepath.ToSlash(dir) + "/sub/../sub/main.rms"
	require.NotEqual(t, plain, dotted)
	require.Equal(t, canonicalPath(plain), canonicalPath(dotted), "same cache key")

	r := NewResolver(fakeSource{})

	// the q occurrence inside the inline XS block (line 2, column 11)
	pos := common.Pos{Line: 2, Column: 11}

	t1, ok := r.Definition(context.Background(), plain, pos)
	require.True(t, ok)
	assert.Equal(t, plain, t1.URI)

	t2, ok := r.Definition(context.Background(), dotted, pos)
	require.True(t, ok)
	assert.Equal(t, dotted, t2.URI)
}

// TestResolver_SetRootsFallback checks the resolution order: the
// owner's directory first, then the configured roots in priority
// order; SetRoots replaces the whole set.
func TestResolver_SetRootsFallback(t *testing.T) {
	t.Parallel()

	mapDir := t.TempDir()
	first := t.TempDir()
	second := t.TempDir()

	uris := writeTree(t, mapDir, map[string]string{
		"main.rms": "#include \"shared.rms\"\n",
	})
	writeTree(t, first, map[string]string{"shared.rms": "base_terrain GRASS\n"})
	writeTree(t, second, map[string]string{"shared.rms": "base_terrain DIRT\n"})

	r := NewResolver(fakeSource{})

	c := r.Closure(context.Background(), uris["main.rms"])
	require.Len(t, c.Missing, 1, "without roots the include is missing")

	r.SetRoots([]string{first, second})

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Empty(t, c.Missing)
	require.Len(t, c.Rms, 2)
	assert.Contains(t, c.Resolved[0].Target, first, "the first root wins")

	// replacement: only the second root stays
	r.SetRoots([]string{second})

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Empty(t, c.Missing)
	assert.Contains(t, c.Resolved[0].Target, second, "SetRoots replaces the set")

	// clearing returns to the missing state
	r.SetRoots(nil)

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Len(t, c.Missing, 1)
}

// TestResolver_SetRootsOwnerDirWins checks that a file next to the
// includer beats the same-named file in a configured root.
func TestResolver_SetRootsOwnerDirWins(t *testing.T) {
	t.Parallel()

	mapDir := t.TempDir()
	root := t.TempDir()

	uris := writeTree(t, mapDir, map[string]string{
		"main.rms":   "#include \"shared.rms\"\n",
		"shared.rms": "base_terrain GRASS\n",
	})
	writeTree(t, root, map[string]string{"shared.rms": "base_terrain DIRT\n"})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	c := r.Closure(context.Background(), uris["main.rms"])

	require.Empty(t, c.Missing)
	assert.Contains(t, c.Resolved[0].Target, mapDir, "the owner directory resolves first")
}

// TestResolver_SetRootsEscapeBlocked checks that a root-relative
// ../-path escaping every root stays missing.
func TestResolver_SetRootsEscapeBlocked(t *testing.T) {
	t.Parallel()

	outside := t.TempDir()
	root := t.TempDir()

	uris := writeTree(t, root, map[string]string{
		"map/main.rms": "#include \"../escape.rms\"\n",
	})
	writeTree(t, outside, map[string]string{"escape.rms": "base_terrain GRASS\n"})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	c := r.Closure(context.Background(), uris["map/main.rms"])

	require.Len(t, c.Missing, 1, "escapes outside the roots stay missing")
}

// TestResolver_Drop pins the force-reload semantics: a same-size
// rewrite with the mtime wound back fools the stat fingerprint, Drop
// defeats it; unknown paths stay a no-op.
func TestResolver_Drop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	libPath := filepath.Join(dir, "lib.xs")
	require.NoError(t, os.WriteFile(libPath, []byte("void aaaa() {}\n"), 0o644))

	st, err := os.Stat(libPath)
	require.NoError(t, err)

	uris := writeTree(t, dir, map[string]string{
		"main.rms": "#includeXS lib.xs\n",
	})

	r := NewResolver(fakeSource{})

	c := r.Closure(context.Background(), uris["main.rms"])
	require.Len(t, c.Xs, 1)
	require.Equal(t, "aaaa", c.Xs[0].File.Decls[0].Name)

	// same size, different content, mtime wound back — the cache wins
	require.NoError(t, os.WriteFile(libPath, []byte("void bbbb() {}\n"), 0o644))
	require.NoError(t, os.Chtimes(libPath, st.ModTime(), st.ModTime()))

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Equal(t, "aaaa", c.Xs[0].File.Decls[0].Name, "an unchanged fingerprint keeps the cache")

	r.Drop([]string{libPath, filepath.Join(dir, "unknown.xs")})

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Equal(t, "bbbb", c.Xs[0].File.Decls[0].Name, "Drop forces the disk re-read")
}
