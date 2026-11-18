package format

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRMS_IdempotentCorpus pins the contract's idempotency invariant on
// the corpus: formatting an already formatted document changes nothing,
// byte for byte. The golden inputs alone are too tame — corpus maps carry
// the pathological parses (attribute extents across sections, glued
// braces, chains crossing section headers) where a format that has not
// reached its fixed point would show. Refused inputs promise nothing.
func TestRMS_IdempotentCorpus(t *testing.T) {
	t.Parallel()

	corpus, err := filepath.Glob(filepath.Join("..", "..", ".corpus", "*.rms"))
	if err != nil || len(corpus) == 0 {
		t.Skip("corpus not fetched")
	}

	for _, in := range corpus {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			once, err := RMS(string(raw), Options{TabSize: 4})
			if err != nil {
				t.Skipf("refused input: %v", err)
			}

			twice, err := RMS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("RMS(formatted): %v", err)
			}

			if twice != once {
				t.Errorf("RMS(RMS(%s)) differs:\n--- once ---\n%q\n--- twice ---\n%q", in, once, twice)
			}
		})
	}
}
