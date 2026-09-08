package include

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/uri"
)

// benchNoSource mirrors the common case: no open documents, every
// include target loads from disk through the stat-keyed cache.
type benchNoSource struct{}

func (benchNoSource) Text(uri string) (string, bool) { return "", false }

func (benchNoSource) URIs() []string { return nil }

// benchChainOnDisk writes N documents into dir: doc_i includes doc_{i+1}
// plus a shared library — the include shape of real map packs.
func benchChainOnDisk(b *testing.B, dir string, docs int) string {
	var lib strings.Builder

	fmt.Fprintln(&lib, "#const SHARED_CONST 7")
	fmt.Fprintln(&lib, "<LAND_GENERATION>")
	fmt.Fprintln(&lib, "base_terrain GRASS")

	for i := range 40 {
		fmt.Fprintf(&lib, "create_object LIB_OBJ_%d {\n  number_of_objects %d\n}\n", i, i)
	}

	if err := os.WriteFile(filepath.Join(dir, "lib.rms"), []byte(lib.String()), 0o644); err != nil {
		b.Fatal(err)
	}

	for i := range docs {
		var doc strings.Builder

		fmt.Fprintf(&doc, "/* doc %d */\n", i)

		if i+1 < docs {
			fmt.Fprintf(&doc, "#include doc_%d.rms\n", i+1)
		}

		fmt.Fprintln(&doc, "#include lib.rms")
		fmt.Fprintln(&doc, "<OBJECTS_GENERATION>")

		for j := range 20 {
			fmt.Fprintf(&doc, "create_object OBJ_%d {\n  number_of_objects %d\n}\n", j, j)
		}

		name := filepath.Join(dir, fmt.Sprintf("doc_%d.rms", i))

		if err := os.WriteFile(name, []byte(doc.String()), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	return string(uri.File(filepath.Join(dir, "doc_0.rms")))
}

// BenchmarkClosure measures the include-closure computation behind
// diagnostics and cross-file navigation: the steady-state (warm disk
// cache) path the server takes on every request.
func BenchmarkClosure(b *testing.B) {
	for _, size := range []struct {
		name string
		docs int
	}{
		{"chain_5", 5},
		{"chain_20", 20},
	} {
		root := benchChainOnDisk(b, b.TempDir(), size.docs)
		resolver := NewResolver(benchNoSource{})
		ctx := context.Background()

		// warm the stat-keyed disk cache once — the server keeps one
		// resolver for the whole session
		_ = resolver.Closure(ctx, root)

		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				_ = resolver.Closure(ctx, root)
			}
		})
	}
}
