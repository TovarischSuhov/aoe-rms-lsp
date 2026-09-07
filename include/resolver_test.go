package include

import (
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
	r := NewResolver(fakeSource{})
	require.NotNil(t, r)

	c := r.Closure(context.Background(), "file:///none.rms")
	assert.Equal(t, "file:///none.rms", c.Root)
}

// TestResolver_ClosureDFSAndMissing checks the core closure: DFS order,
// resolved directives and one MissingInclude per missing directive.
func TestResolver_ClosureDFSAndMissing(t *testing.T) {
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
