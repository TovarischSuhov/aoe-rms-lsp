package include

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// benchEditorSource mirrors an editing session: every chain document is
// open, so resolver.load takes the editor-state path (Source.Text wins
// over the disk copy) on every closure member.
type benchEditorSource map[string]string

func (s benchEditorSource) Text(uri string) (string, bool) {
	text, ok := s[uri]

	return text, ok
}

func (s benchEditorSource) URIs() []string {
	uris := make([]string, 0, len(s))
	for u := range s {
		uris = append(uris, u)
	}

	slices.Sort(uris)

	return uris
}

// compile-time check: the fake satisfies Source.
var _ Source = benchEditorSource{}

// benchOpenChain reads every chain document written into dir (the docs
// and the shared library) from disk into an editor source — the
// open-document set of a real session. Returns the sorted open URIs.
func benchOpenChain(b *testing.B, dir string) (benchEditorSource, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		b.Fatal(err)
	}

	src := benchEditorSource{}
	uris := make([]string, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rms") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			b.Fatal(err)
		}

		u := string(uri.File(filepath.Join(dir, e.Name())))
		src[u] = string(raw)
		uris = append(uris, u)
	}

	slices.Sort(uris)

	return src, uris
}

// BenchmarkClosureEditor measures the same closure as BenchmarkClosure
// with every chain document open in the editor: each member loads
// through Source.Text and is re-parsed per request today — the
// steady-state keystroke cost behind publishDiagnostics and
// SignatureHelp (a memoized parse turns this into the warm path).
func BenchmarkClosureEditor(b *testing.B) {
	for _, size := range []struct {
		name string
		docs int
	}{
		{"chain_5", 5},
		{"chain_20", 20},
	} {
		dir := b.TempDir()
		root := benchChainOnDisk(b, dir, size.docs)
		source, _ := benchOpenChain(b, dir)
		resolver := NewResolver(source)
		ctx := context.Background()

		// warm once — with a memoized editor parse this fills the memo
		// so the loop measures the steady-state (memo-hit) path
		_ = resolver.Closure(ctx, root)

		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				_ = resolver.Closure(ctx, root)
			}
		})
	}
}

// BenchmarkClosureEditorFanout measures the References/renameRoots
// fan-out: one full closure per open document per request (the loop at
// resolver.go:419-431 / 650-666) — O(open documents x closure size),
// the References latency the issue reports.
func BenchmarkClosureEditorFanout(b *testing.B) {
	for _, size := range []struct {
		name string
		docs int
	}{
		{"chain_5", 5},
		{"chain_20", 20},
	} {
		dir := b.TempDir()
		root := benchChainOnDisk(b, dir, size.docs)
		source, _ := benchOpenChain(b, dir)
		resolver := NewResolver(source)
		ctx := context.Background()

		// warm once — steady state with a memoized editor parse
		_ = resolver.Closure(ctx, root)

		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				// exactly the References/renameRoots loop: the queried
				// file's own closure plus the closures of the other
				// open documents that include it
				closures := []Closure{resolver.Closure(ctx, root)}

				for _, u := range source.URIs() {
					if u == root {
						continue
					}

					cl := resolver.Closure(ctx, u)

					if closureHas(cl, root) {
						closures = append(closures, cl)
					}
				}

				_ = closures
			}
		})
	}
}
