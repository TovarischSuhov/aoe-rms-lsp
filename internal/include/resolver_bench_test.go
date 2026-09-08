package include

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// benchSource is an in-memory Source: a chain of N documents each
// including the next.
type benchSource struct {
	texts map[string]string
}

func (s benchSource) Text(uri string) (string, bool) {
	text, ok := s.texts[uri]

	return text, ok
}

func (s benchSource) URIs() []string {
	uris := make([]string, 0, len(s.texts))

	for uri := range s.texts {
		uris = append(uris, uri)
	}

	return uris
}

// benchChain builds N documents: doc_i includes doc_{i+1} plus a shared
// library — the include shape of real map packs.
func benchChain(docs int) benchSource {
	texts := map[string]string{}

	var lib strings.Builder

	fmt.Fprintln(&lib, "#const SHARED_CONST 7")
	fmt.Fprintln(&lib, "<LAND_GENERATION>")
	fmt.Fprintln(&lib, "base_terrain GRASS")

	for i := range 40 {
		fmt.Fprintf(&lib, "create_object LIB_OBJ_%d {\n  number_of_objects %d\n}\n", i, i)
	}

	texts["file:///bench/lib.rms"] = lib.String()

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

		texts[fmt.Sprintf("file:///bench/doc_%d.rms", i)] = doc.String()
	}

	return benchSource{texts: texts}
}

// BenchmarkClosure measures the include-closure computation behind
// diagnostics and cross-file navigation.
func BenchmarkClosure(b *testing.B) {
	for _, size := range []struct {
		name string
		docs int
	}{
		{"chain_5", 5},
		{"chain_20", 20},
	} {
		resolver := NewResolver(benchChain(size.docs))
		ctx := context.Background()

		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				_ = resolver.Closure(ctx, "file:///bench/doc_0.rms")
			}
		})
	}
}
