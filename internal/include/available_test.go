package include

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/uri"
)

// TestResolver_Available checks the auto-include scan: sibling and
// root XS files show up with relative include arguments, the document
// itself and closure members stay out, editor state wins over the disk
// copy, and non-file URIs answer nil.
func TestResolver_Available(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(sub, 0o755))

	writeTree(t, dir, map[string]string{
		"main.xs":       "void main() {}\n",
		"neighbor.xs":   "int neighborVar = 1;\nvoid neighborFn() {}\n",
		"included.xs":   "int includedVar = 2;\n",
		"sub/nested.xs": "int nestedVar = 3;\n",
	})
	writeTree(t, root, map[string]string{"rootlib.xs": "int rootVar = 4;\n"})

	uris := map[string]string{
		"main": uri.File(filepath.Join(dir, "main.xs")).String(),
		// a .rms root resolves #includeXS into the closure: the map
		// shape auto-include lives in
		"map": uri.File(filepath.Join(dir, "map.rms")).String(),
	}

	t.Run("sibling files surface with relative paths", func(t *testing.T) {
		t.Parallel()

		r := NewResolver(fakeSource{})
		entries := r.Available(context.Background(), uris["main"])

		paths := make([]string, 0, len(entries))
		for _, e := range entries {
			paths = append(paths, e.Path)
		}

		assert.ElementsMatch(t, []string{"neighbor.xs", "included.xs"}, paths)
	})

	t.Run("closure members stay out", func(t *testing.T) {
		t.Parallel()

		// map.rms includes included.xs: it no longer counts as available
		r := NewResolver(fakeSource{
			uris["map"]: "#includeXS \"included.xs\"\n",
		})
		entries := r.Available(context.Background(), uris["map"])

		for _, e := range entries {
			assert.NotEqual(t, "included.xs", e.Path, "an included file is not available")
			assert.NotEqual(t, "map.rms", e.Path)
		}

		assert.Len(t, entries, 2, "main.xs and neighbor.xs remain available")
	})

	t.Run("roots contribute; out-of-tree paths stay absolute", func(t *testing.T) {
		t.Parallel()

		r := NewResolver(fakeSource{})
		r.SetRoots([]string{root})

		entries := r.Available(context.Background(), uris["main"])

		paths := make([]string, 0, len(entries))
		for _, e := range entries {
			paths = append(paths, e.Path)
		}

		// the root is a sibling temp tree: Rel yields a ..-prefixed
		// path, which stays absolute by design
		assert.Contains(t, paths, filepath.ToSlash(filepath.Join(root, "rootlib.xs")))
	})

	t.Run("decls carry named top-level declarations", func(t *testing.T) {
		t.Parallel()

		r := NewResolver(fakeSource{})
		entries := r.Available(context.Background(), uris["main"])

		for _, e := range entries {
			if e.Path != "neighbor.xs" {
				continue
			}

			names := make([]string, 0, len(e.Decls))
			for _, d := range e.Decls {
				names = append(names, d.Name)
			}

			assert.ElementsMatch(t, []string{"neighborVar", "neighborFn"}, names)
		}
	})

	t.Run("editor state wins over the disk copy", func(t *testing.T) {
		t.Parallel()

		r := NewResolver(fakeSource{
			uri.File(filepath.Join(dir, "neighbor.xs")).String(): "int freshVar = 9;\n",
		})

		entries := r.Available(context.Background(), uris["main"])

		for _, e := range entries {
			if e.Path != "neighbor.xs" {
				continue
			}

			require.Len(t, e.Decls, 1)
			assert.Equal(t, "freshVar", e.Decls[0].Name)
		}
	})

	t.Run("non-file URI answers nil", func(t *testing.T) {
		t.Parallel()

		r := NewResolver(fakeSource{})
		assert.Nil(t, r.Available(context.Background(), "untitled:buffer-1"))
	})
}
