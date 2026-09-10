package rms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzParse pins the robustness contract the server depends on: any
// input — truncated, binary, hostile — parses without a panic (the
// NeverNil tests state it, fuzzing explores it). Seeds: hand-picked
// edges, every testdata fixture, and the real corpus sample under
// .corpus when it has been fetched.
func FuzzParse(f *testing.F) {
	f.Add("")
	f.Add("\x00\x01 @@ <<<< \x02")
	f.Add("<LAND_GENERATION>\ncreate_land {\nif\n")
	f.Add("#includeXS \n<XS>\nrule 1 {")

	seedFuzzFixtures(f, "testdata", ".rms")
	seedFuzzFixtures(f, filepath.Join("..", "..", ".corpus"), ".rms")

	// The contract is totality: parsing any byte sequence terminates
	// without a panic (the returned file is a value, never nil, and
	// the diagnostics are advisory — the server relies on both).
	f.Fuzz(func(t *testing.T, src string) {
		Parse(src, "fuzz.rms")
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
