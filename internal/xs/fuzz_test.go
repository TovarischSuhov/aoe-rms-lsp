package xs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzXsParse pins the robustness contract the server depends on: any
// input — truncated, binary, hostile — parses without a panic. Seeds:
// hand-picked edges, every testdata fixture, and the real corpus
// sample under .corpus when it has been fetched (its .xs files,
// including maps' inline XS unpacked by the fetch script).
func FuzzXsParse(f *testing.F) {
	f.Add("")
	f.Add("\x00\x01 @@ <<<< \x02")
	f.Add("rule 1 {\nif(")
	f.Add("void f() {\n  int a = ")

	seedFuzzFixtures(f, "testdata", ".xs")
	seedFuzzFixtures(f, filepath.Join("..", "..", ".corpus"), ".xs")

	// The contract is totality: parsing any byte sequence terminates
	// without a panic (the returned file is a value, never nil, and
	// the diagnostics are advisory — the server relies on both).
	f.Fuzz(func(t *testing.T, src string) {
		XsParse(src, "fuzz.xs")
	})
}

// seedFuzzFixtures registers every file under dir with the given
// extension as a fuzz seed; a missing directory (corpus not fetched)
// is not an error.
func seedFuzzFixtures(f *testing.F, dir, ext string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		f.Add(string(raw))
	}
}
