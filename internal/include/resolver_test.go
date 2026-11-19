package include

import (
	"aoe2-lsp/internal/common"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestResolver_IncludeDrsClosure pins the #include_drs integration: the
// directive materializes in file.Includes, so the closure resolves the
// .inc next to the map and reports exactly one MissingInclude per absent
// target. The trailing legacy DRS id is not part of the path — a leaked
// id would both miss the resolve and widen the Range.
func TestResolver_IncludeDrsClosure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		"main.rms":             "#include_drs includes/scaling.inc 54201\n#include_drs missing.inc 54202\n",
		"includes/scaling.inc": "base_terrain GRASS\n",
	})

	r := NewResolver(fakeSource{})
	c := r.Closure(context.Background(), uris["main.rms"])

	// The .inc next to the map joins the RMS closure in DFS order.
	require.Len(t, c.Rms, 2)
	assert.Equal(t, uris["main.rms"], c.Rms[0].URI)
	assert.Equal(t, uris["includes/scaling.inc"], c.Rms[1].URI)

	// One resolved directive: the owner's path resolves from the map dir.
	require.Len(t, c.Resolved, 1)
	assert.Equal(t, uris["main.rms"], c.Resolved[0].Owner)
	assert.Equal(t, uris["includes/scaling.inc"], c.Resolved[0].Target)
	assert.Equal(t, "includes/scaling.inc", c.Resolved[0].Inc.Path)

	// The absent target yields exactly one MissingInclude. Path is the
	// bare path (no trailing id) and Range slices that same token out of
	// the owner's line.
	require.Len(t, c.Missing, 1)
	miss := c.Missing[0]
	require.Equal(t, "missing.inc", miss.Path)
	assert.Equal(t, uris["main.rms"], miss.Owner)

	line := "#include_drs missing.inc 54202"
	require.Equal(t, uint32(1), miss.Range.Start.Line)
	require.LessOrEqual(t, miss.Range.End.Column, uint32(len(line)))
	assert.Equal(t, "missing.inc", line[miss.Range.Start.Column:miss.Range.End.Column])
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

// TestResolver_DefinitionIncludeThroughRoot pins the issue #210 main
// case: the map lives outside the configured include root and the
// directive resolves only through it. The click must jump exactly when
// the closure diagnostics report the directive Resolved — through the
// same resolution the diagnostics used — carrying the closure's own
// Target URI and the file-start zero range.
func TestResolver_DefinitionIncludeThroughRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mapDir := t.TempDir()

	writeTree(t, root, map[string]string{
		"lib/xs/foo.xs": "void foo() { }\n",
	})

	const mainSrc = "#include lib/xs/foo.xs\n"
	mapURIs := writeTree(t, mapDir, map[string]string{
		"main.rms": mainSrc,
	})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	// parity precondition: diagnostics already accept the directive, so
	// a silent found=false here is the defect, never a fixture slip
	c := r.Closure(context.Background(), mapURIs["main.rms"])
	require.Empty(t, c.Missing)
	require.Len(t, c.Resolved, 1)

	target, found := r.Definition(context.Background(), mapURIs["main.rms"], linePos(mainSrc, 0, "lib/xs", 0))

	require.True(t, found, "a directive the closure reports Resolved must jump")
	assert.Equal(t, c.Resolved[0].Target, target.URI, "the jump lands on the closure's own Resolved target")
	assert.Equal(t, common.Range{}, target.Range)
}

// TestResolver_DefinitionRootRelativeCoveredByRoot pins scenario B of
// the issue #210 fix: a ../-target that exists next to the owner but is
// only *accepted* through a configured root's bound jumps exactly like
// the closure's Resolved entry. The owner directory stats this file
// fine, so the pre-fix owner-dir-only withinRoot check silently ate the
// click while diagnostics reported the directive Resolved.
func TestResolver_DefinitionRootRelativeCoveredByRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	const econSrc = "#include ../shared/x.xs\n"
	uris := writeTree(t, root, map[string]string{
		"map/parts/econ.rms": econSrc,
		"map/shared/x.xs":    "void xsPart() { }\n",
	})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	// parity precondition: the owner-dir candidate exists but only the
	// root's bound accepts it, so diagnostics report Resolved and the
	// click must answer the same way
	c := r.Closure(context.Background(), uris["map/parts/econ.rms"])
	require.Empty(t, c.Missing)
	require.Len(t, c.Resolved, 1)

	target, found := r.Definition(context.Background(), uris["map/parts/econ.rms"], linePos(econSrc, 0, "../shared", 0))

	require.True(t, found, "a root-covered ../-target the closure reports Resolved must jump")
	assert.Equal(t, c.Resolved[0].Target, target.URI, "the jump lands on the closure's own Resolved target")
	assert.Equal(t, uris["map/shared/x.xs"], target.URI, "... which is the root's map/shared/x.xs copy")
	assert.Equal(t, common.Range{}, target.Range)
}

// TestResolver_DefinitionOwnerDirBeatsRoot pins scenario C: when the
// target exists both next to the includer and in a configured root, the
// click lands on the owner-directory copy — resolveTarget's first
// accepted candidate (owner dir, then roots in SetRoots order), not a
// re-implemented or reordered priority (R3.1).
func TestResolver_DefinitionOwnerDirBeatsRoot(t *testing.T) {
	t.Parallel()

	mapDir := t.TempDir()
	root := t.TempDir()

	const mainSrc = "#include \"shared.rms\"\n"
	uris := writeTree(t, mapDir, map[string]string{
		"main.rms":   mainSrc,
		"shared.rms": "base_terrain GRASS\n",
	})
	writeTree(t, root, map[string]string{"shared.rms": "base_terrain DIRT\n"})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	// parity precondition: the closure itself picked the owner-directory
	// copy, so the click must not disagree with the diagnostics
	c := r.Closure(context.Background(), uris["main.rms"])
	require.Empty(t, c.Missing)
	require.Len(t, c.Resolved, 1)
	require.Equal(t, uris["shared.rms"], c.Resolved[0].Target, "the owner-directory copy resolves first")

	target, found := r.Definition(context.Background(), uris["main.rms"], linePos(mainSrc, 0, "shared.rms", 0))

	require.True(t, found, "the owner-directory copy resolves, so the click must jump")
	assert.Equal(t, uris["shared.rms"], target.URI, "the owner-directory copy beats the root copy")
	assert.Equal(t, common.Range{}, target.Range)
}

// TestResolver_DefinitionMissingParity pins scenario D: a target written
// nowhere stays found=false on the click while the same resolver's
// closure reports exactly one MissingInclude for that directive — a
// click answers exactly when diagnostics resolve (R2.1, R2.2), never
// "always jump".
func TestResolver_DefinitionMissingParity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	const mainSrc = "#include missing.rms\n"
	uris := writeTree(t, dir, map[string]string{
		"main.rms": mainSrc,
	})

	r := NewResolver(fakeSource{})

	target, found := r.Definition(context.Background(), uris["main.rms"], linePos(mainSrc, 0, "missing.rms", 0))
	assert.False(t, found, "a target that exists nowhere must not jump")
	assert.Equal(t, Target{}, target, "a miss returns the zero target")

	c := r.Closure(context.Background(), uris["main.rms"])
	require.Len(t, c.Missing, 1, "the same resolver misses exactly this directive")
	assert.Equal(t, "missing.rms", c.Missing[0].Path)
	assert.Equal(t, uris["main.rms"], c.Missing[0].Owner)
}

// TestResolver_DefinitionRootEscapeBlocked pins scenario E (R4.1, the
// definition-side mirror of TestResolver_SetRootsEscapeBlocked): the
// root-relative candidate exists on disk but escapes the owner directory
// and every configured root, so the click stays found=false. escape.rms
// sits exactly at the root-relative candidate path, making the bound —
// not a missing file — the thing that rejects it.
func TestResolver_DefinitionRootEscapeBlocked(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "root")

	const mainSrc = "#include \"../escape.rms\"\n"
	uris := writeTree(t, root, map[string]string{
		"map/main.rms": mainSrc,
	})
	writeTree(t, base, map[string]string{"escape.rms": "base_terrain GRASS\n"})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	// parity precondition: the diagnostics side misses the same
	// directive — asserted first, so found=false below is parity, not a
	// fixture slip (mirror of SetRootsEscapeBlocked)
	c := r.Closure(context.Background(), uris["map/main.rms"])
	require.Len(t, c.Missing, 1, "a target escaping every root stays missing")

	target, found := r.Definition(context.Background(), uris["map/main.rms"], linePos(mainSrc, 0, "../escape", 0))

	assert.False(t, found, "a target escaping every root must not jump")
	assert.Equal(t, Target{}, target)
}

// TestResolver_DefinitionIncludeXSThroughRoot pins the merged-list
// contract (R1.3): #includeXS rides the same Includes + XsIncludes
// directive list through resolveTarget, so a root-resolved XS library
// jumps exactly like a plain #include — dropping XsIncludes from the
// definition hit-test would strand clicks on paths the diagnostics call
// Resolved.
func TestResolver_DefinitionIncludeXSThroughRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mapDir := t.TempDir()

	rootURIs := writeTree(t, root, map[string]string{
		"lib/xs/foo.xs": "void foo() { }\n",
	})

	const mainSrc = "#includeXS lib/xs/foo.xs\n"
	mapURIs := writeTree(t, mapDir, map[string]string{
		"main.rms": mainSrc,
	})

	r := NewResolver(fakeSource{})
	r.SetRoots([]string{root})

	// parity precondition: diagnostics resolve the XS directive through
	// the root, so the click must answer the same way
	c := r.Closure(context.Background(), mapURIs["main.rms"])
	require.Empty(t, c.Missing)
	require.Len(t, c.Resolved, 1)

	target, found := r.Definition(context.Background(), mapURIs["main.rms"], linePos(mainSrc, 0, "lib/xs", 0))

	require.True(t, found, "an #includeXS the closure reports Resolved must jump like a plain #include")
	assert.Equal(t, c.Resolved[0].Target, target.URI, "the jump lands on the closure's own Resolved target")
	assert.Equal(t, rootURIs["lib/xs/foo.xs"], target.URI, "... which is the root's lib/xs/foo.xs copy")
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

// TestWithinRoot checks the workspace bound on the unit level: targets
// inside the root pass, parent-directory escapes fail on the native
// path separator (on Windows filepath.Rel yields `..\`-prefixed
// relatives), absolute paths elsewhere fail.
func TestWithinRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	cases := []struct {
		name    string
		rootDir string
		target  string
		want    bool
	}{
		{"empty root disables the bound", "", filepath.Join(root, "inc.rms"), true},
		{"target inside root", root, filepath.Join(root, "inc.rms"), true},
		{"target is the root itself", root, root, true},
		{"target in a subdirectory", root, filepath.Join(root, "sub", "inc.rms"), true},
		{"direct parent escape", root, filepath.Join(root, "..", "outside.rms"), false},
		{"nested parent escape", root, filepath.Join(root, "maps", "..", "..", "outside.rms"), false},
		{"absolute path outside root", root, filepath.Join(t.TempDir(), "other.rms"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, withinRoot(tc.rootDir, tc.target))
		})
	}
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
	// a dot-segment spelling of the same document; building it by hand
	// (file:// + path) puts the drive letter in the host position on
	// Windows, so it is spliced into the canonical spelling instead
	dotted := strings.Replace(plain, "/sub/main.rms", "/sub/../sub/main.rms", 1)
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
	// Target is a URI string; the raw path never appears in it on
	// Windows (drive percent-encoding, forward slashes)
	assert.Contains(t, c.Resolved[0].Target, uri.File(first).String(), "the first root wins")

	// replacement: only the second root stays
	r.SetRoots([]string{second})

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Empty(t, c.Missing)
	assert.Contains(t, c.Resolved[0].Target, uri.File(second).String(), "SetRoots replaces the set")

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
	assert.Contains(t, c.Resolved[0].Target, uri.File(mapDir).String(), "the owner directory resolves first")
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

	// cache keys are FsPath-normalized (the Windows drive letter comes
	// back lowercase), so Drop paths go through the same URI conversion
	r.Drop([]string{uri.File(libPath).FsPath(), uri.File(filepath.Join(dir, "unknown.xs")).FsPath()})

	c = r.Closure(context.Background(), uris["main.rms"])
	require.Equal(t, "bbbb", c.Xs[0].File.Decls[0].Name, "Drop forces the disk re-read")
}

// eolTree rebuilds a fixture tree with every "\n" of its .rms files
// replaced by eol. The RMS parser normalizes CRLF before counting
// offsets (rms.Parse), so a CRLF twin of an LF fixture is the same
// document to the word index and navigation must answer identically
// (issue #204). The .xs files keep their bytes: XS is parsed on the raw
// text, its offsets address the file's own bytes, and #204 varies the
// EOL of the RMS map only. writeTree writes the literal bytes, so the
// twins differ on disk exactly as spelled.
func eolTree(tree map[string]string, eol string) map[string]string {
	out := make(map[string]string, len(tree))

	for name, content := range tree {
		out[name] = content
		if strings.HasSuffix(name, ".rms") {
			out[name] = strings.ReplaceAll(content, "\n", eol)
		}
	}

	return out
}

// linePos locates the cursor at needle within line of src, advanced by
// col. Navigation containment compares Line/Column only
// (internal/common/range.go), so the same positions address an LF
// fixture and its CRLF twin; Offset stays zero — no query may smuggle a
// raw-text offset into a position.
func linePos(src string, line int, needle string, col int) common.Pos {
	lines := strings.Split(src, "\n")

	return common.Pos{Line: uint32(line), Column: uint32(strings.Index(lines[line], needle) + col)}
}

// rangeAt is the half-open span of a length-long token at (line, col),
// carrying the offset the word index records: RMS offsets address the
// normalized text, so the LF fixture's line starts give the exact
// values — and the CRLF twin's answer carries the same offsets (the
// normalized text is identical). Queries keep Offset zero; only
// expected answer ranges need it.
func rangeAt(src string, line int, col int, length int) common.Range {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	offset := col
	for i := range line {
		offset += len(lines[i]) + 1
	}

	return common.Range{
		Start: common.Pos{Line: uint32(line), Column: uint32(col), Offset: offset},
		End:   common.Pos{Line: uint32(line), Column: uint32(col + length), Offset: offset + length},
	}
}

// parityTree is the RMS fixture of the EOL-parity pins: main includes
// econ.rms, spells its section header lowercase (the case aspect is
// pinned separately) and carries a command word, a const value and a
// numeric literal; econ.rms repeats the command word, so a query on
// main answers across the closure.
func parityTree() map[string]string {
	return map[string]string{
		"main.rms":       "#include \"parts/econ.rms\"\n<terrain_generation>\nbase_terrain GRASS\nland_percent 12\n",
		"parts/econ.rms": "base_terrain GRASS\nbase_terrain DIRT\n",
	}
}

// parityProbes are the cursor probes on parityTree's main.rms: the
// on-word cases must answer, the not-on-word cases (numeric literal,
// inter-token gap) must stay empty (issue #204, AC5).
var parityProbes = []struct {
	name   string
	line   int
	needle string
	onWord bool
}{
	{"command word", 2, "base_terrain", true},
	{"const value word", 2, "GRASS", true},
	{"number literal is not an indexed word", 3, "12", false},
	{"inter-token gap is not an indexed word", 2, " GRASS", false},
}

// TestResolver_ReferencesCRLFEditorState pins issue #204 scenario (a):
// a CRLF map open in the editor (fakeSource) answers References exactly
// like its LF twin (AC1). RMS word offsets address the normalized text,
// so a name sliced from the raw editor bytes is garbage and silently
// empties the answer.
func TestResolver_ReferencesCRLFEditorState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	lfTexts := eolTree(parityTree(), "\n")
	uris := writeTree(t, dir, lfTexts)

	open := func(texts map[string]string) fakeSource {
		return fakeSource{uris["main.rms"]: texts["main.rms"], uris["parts/econ.rms"]: texts["parts/econ.rms"]}
	}

	// the LF twin's answers, computed before the CRLF rewrite: the
	// baseline every CRLF case must reproduce. On-word baselines are
	// guarded (non-empty, containing the queried occurrence), so parity
	// cannot pass vacuously on an empty-vs-empty accident.
	rLF := NewResolver(open(lfTexts))

	baseline := make(map[string][]Target, len(parityProbes))
	for _, tc := range parityProbes {
		pos := linePos(lfTexts["main.rms"], tc.line, tc.needle, 0)
		baseline[tc.name] = rLF.References(context.Background(), uris["main.rms"], pos)

		if !tc.onWord {
			require.Empty(t, baseline[tc.name], "LF baseline: a not-on-word position stays empty")

			continue
		}

		require.NotEmpty(t, baseline[tc.name], "LF baseline: the cursor sits on an indexed word")
		require.True(t,
			slices.ContainsFunc(baseline[tc.name], func(tg Target) bool { return tg.Range.Contains(pos) }),
			"LF baseline: the answer contains the queried occurrence")
	}

	// the CRLF twin overwrites the same paths: identical URIs make the
	// two answer sets directly comparable
	crlfTexts := eolTree(parityTree(), "\r\n")
	writeTree(t, dir, crlfTexts)

	for _, tc := range parityProbes {
		t.Run("CRLF/"+tc.name, func(t *testing.T) {
			t.Parallel()

			pos := linePos(crlfTexts["main.rms"], tc.line, tc.needle, 0)
			refs := NewResolver(open(crlfTexts)).References(context.Background(), uris["main.rms"], pos)

			if !tc.onWord {
				assert.Empty(t, refs, "a non-word position must answer empty (AC5)")

				return
			}

			assert.Equal(t, baseline[tc.name], refs,
				"the CRLF answer must equal the LF twin's set (AC1)")
		})
	}
}

// TestResolver_ReferencesCRLFOnDisk pins issue #204 scenario (b): the
// CRLF map is not open anywhere — every load goes through loadDisk's
// raw os.ReadFile bytes — and References must still equal the LF twin's
// set (AC1).
func TestResolver_ReferencesCRLFOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	lfTexts := eolTree(parityTree(), "\n")
	uris := writeTree(t, dir, lfTexts)

	// the LF twin answers from disk first; the CRLF twin then
	// overwrites the same paths, so both runs query identical URIs
	rLF := NewResolver(fakeSource{})

	baseline := make(map[string][]Target, len(parityProbes))
	for _, tc := range parityProbes {
		pos := linePos(lfTexts["main.rms"], tc.line, tc.needle, 0)
		baseline[tc.name] = rLF.References(context.Background(), uris["main.rms"], pos)

		if !tc.onWord {
			require.Empty(t, baseline[tc.name], "LF baseline: a not-on-word position stays empty")

			continue
		}

		require.NotEmpty(t, baseline[tc.name], "LF baseline: the cursor sits on an indexed word")
		require.True(t,
			slices.ContainsFunc(baseline[tc.name], func(tg Target) bool { return tg.Range.Contains(pos) }),
			"LF baseline: the answer contains the queried occurrence")
	}

	writeTree(t, dir, eolTree(parityTree(), "\r\n"))

	for _, tc := range parityProbes {
		t.Run("CRLF/"+tc.name, func(t *testing.T) {
			t.Parallel()

			// a fresh resolver: the stat-keyed cache never crosses
			// instances, so the CRLF bytes are re-read from disk
			src := eolTree(parityTree(), "\r\n")["main.rms"]
			pos := linePos(src, tc.line, tc.needle, 0)
			refs := NewResolver(fakeSource{}).References(context.Background(), uris["main.rms"], pos)

			if !tc.onWord {
				assert.Empty(t, refs, "a non-word position must answer empty (AC5)")

				return
			}

			assert.Equal(t, baseline[tc.name], refs,
				"the on-disk CRLF answer must equal the LF twin's set (AC1)")
		})
	}
}

// sectionTree is the uppercase-section fixture (issue #204, AC2): the
// corpus spells section headers <UPPERCASE>, while the parser records
// the section word lowercase — whatever the source spelling, References
// on either tag token must answer the full occurrence set.
func sectionTree() map[string]string {
	return map[string]string{
		"main.rms": "<TERRAIN_GENERATION>\nbase_terrain GRASS\n</TERRAIN_GENERATION>\n",
	}
}

// TestResolver_ReferencesSectionNameCase pins issue #204 scenarios (c)
// and (d): References queried on an uppercase section word — on an LF
// file (c) and on a CRLF file (d), on the opening and on the closing
// tag token — returns the two occurrences of the recorded (lowercase)
// name. A name sliced from the raw text keeps the written uppercase
// spelling, matches no indexed word and silently empties the answer.
func TestResolver_ReferencesSectionNameCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	const sectionWord = "TERRAIN_GENERATION"

	tree := sectionTree()
	uris := writeTree(t, dir, tree)
	mainURI := uris["main.rms"]

	// the expected set: the section word's opening-tag and closing-tag
	// occurrences, computed from the fixture text itself so the columns
	// never drift from it
	lines := strings.Split(tree["main.rms"], "\n")
	want := []Target{
		{URI: mainURI, Range: rangeAt(tree["main.rms"], 0, strings.Index(lines[0], sectionWord), len(sectionWord))},
		{URI: mainURI, Range: rangeAt(tree["main.rms"], 2, strings.Index(lines[2], sectionWord), len(sectionWord))},
	}

	entries := []struct {
		label string
		line  int
	}{
		{"opening tag", 0},
		{"closing tag", 2},
	}

	// both twins answer before the subtests spawn: sequential
	// editor-state queries, then parallel assertions
	answers := make(map[string][]Target, 2*len(entries))

	for _, eol := range []struct{ label, s string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		texts := eolTree(tree, eol.s)
		writeTree(t, dir, texts)

		r := NewResolver(fakeSource{mainURI: texts["main.rms"]})

		for _, e := range entries {
			pos := linePos(texts["main.rms"], e.line, sectionWord, 0)
			answers[eol.label+"/"+e.label] = r.References(context.Background(), mainURI, pos)
		}
	}

	for _, eol := range []string{"LF", "CRLF"} {
		for _, e := range entries {
			t.Run(eol+"/"+e.label, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, want, answers[eol+"/"+e.label],
					"the uppercase section word must answer its full occurrence set (AC2)")
			})
		}
	}
}

// inlineXsTree is the scenario-(e) fixture: an inline XS block inside
// the RMS map, navTree-shaped, in either EOL spelling.
func inlineXsTree() map[string]string {
	return map[string]string{
		"main.rms":     "#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n",
		"parts/lib.xs": "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n",
	}
}

// TestResolver_ReferencesInlineXsCRLFParity pins issue #204 scenario
// (e): the inline XS block of a CRLF file answers exactly like its LF
// twin's block. The block path never touches the defect — the XS parser
// works on the block's own LF-normalized code and containment compares
// Line/Column — so this pin guards the fix against regressing the
// untouched XS/inline branches; it is expected green before the fix.
func TestResolver_ReferencesInlineXsCRLFParity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	lfTexts := eolTree(inlineXsTree(), "\n")
	uris := writeTree(t, dir, lfTexts)

	open := func(texts map[string]string) fakeSource {
		return fakeSource{uris["main.rms"]: texts["main.rms"], uris["parts/lib.xs"]: texts["parts/lib.xs"]}
	}

	// the cursor sits on the sharedFn call inside main.rms's inline
	// block (line 1)
	posLF := linePos(lfTexts["main.rms"], 1, "sharedFn", 0)

	rLF := NewResolver(open(lfTexts))
	baseline := rLF.References(context.Background(), uris["main.rms"], posLF)

	require.NotEmpty(t, baseline, "LF baseline: the block call resolves")
	require.True(t,
		slices.ContainsFunc(baseline, func(tg Target) bool { return tg.Range.Contains(posLF) }),
		"LF baseline: the answer contains the queried occurrence")

	crlfTexts := eolTree(inlineXsTree(), "\r\n")
	writeTree(t, dir, crlfTexts)

	t.Run("CRLF/inline block call", func(t *testing.T) {
		t.Parallel()

		pos := linePos(crlfTexts["main.rms"], 1, "sharedFn", 0)
		refs := NewResolver(open(crlfTexts)).References(context.Background(), uris["main.rms"], pos)

		assert.Equal(t, baseline, refs,
			"the CRLF inline-block answer must equal the LF twin's set")
	})
}

// declTree is the scenario-(f) fixture: an XS library declaring sharedFn
// next to an RMS map whose #const declares the same name — the cursor
// sits on the RMS word, the declaration lives in lib.xs.
func declTree() map[string]string {
	return map[string]string{
		"main.rms":     "#includeXS parts/lib.xs\n#const sharedFn 1\n",
		"parts/lib.xs": "void sharedFn(int n) { }\n",
	}
}

// TestResolver_DefinitionCRLFParity pins issue #204 scenario (f): on a
// CRLF map, Definition on the RMS word sharedFn resolves the top-level
// declaration in lib.xs exactly like the LF twin (AC3) — symbolNameAt
// feeds the closure lookup, so a raw-text slice with normalized offsets
// poisons the name and the answer silently degrades to found=false. A
// cursor on a non-word (the numeric literal) stays found=false in both
// spellings (AC3 negative, AC5).
func TestResolver_DefinitionCRLFParity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	lfTexts := eolTree(declTree(), "\n")
	uris := writeTree(t, dir, lfTexts)

	tests := []struct {
		name   string
		line   int
		needle string
		onWord bool
	}{
		{"RMS word resolves the closure declaration", 1, "sharedFn", true},
		{"number literal stays not-found", 1, "1", false},
	}

	// the LF twin answers from disk first (nothing open — the
	// Definition tests run on the bare resolver), then the CRLF twin
	// overwrites the same paths
	rLF := NewResolver(fakeSource{})

	baseline := make(map[string]Target, len(tests))
	baselineFound := make(map[string]bool, len(tests))
	for _, tc := range tests {
		pos := linePos(lfTexts["main.rms"], tc.line, tc.needle, 0)
		baseline[tc.name], baselineFound[tc.name] = rLF.Definition(context.Background(), uris["main.rms"], pos)

		if !tc.onWord {
			require.False(t, baselineFound[tc.name], "LF baseline: a non-word stays not-found")

			continue
		}

		require.True(t, baselineFound[tc.name], "LF baseline: the RMS word resolves")
		require.Equal(t, uris["parts/lib.xs"], baseline[tc.name].URI,
			"LF baseline: the declaration is in lib.xs")
	}

	crlfTexts := eolTree(declTree(), "\r\n")
	writeTree(t, dir, crlfTexts)

	for _, tc := range tests {
		t.Run("CRLF/"+tc.name, func(t *testing.T) {
			t.Parallel()

			pos := linePos(crlfTexts["main.rms"], tc.line, tc.needle, 0)
			target, found := NewResolver(fakeSource{}).
				Definition(context.Background(), uris["main.rms"], pos)

			if !tc.onWord {
				assert.False(t, found, "a non-word position must stay not-found (AC5)")

				return
			}

			require.True(t, found, "the RMS word must resolve on CRLF (AC3)")
			assert.Equal(t, baseline[tc.name], target,
				"the CRLF answer must equal the LF twin's target (AC3)")
		})
	}
}
