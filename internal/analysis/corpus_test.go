package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCorpus_NoDuplicateDecls runs the duplicate-declaration check over
// the real corpus sample (make corpus): a community-written script
// tripping the warning is a false positive to fix, not a map to shame.
// The gate reruns for every analysis slot over its own code; it skips
// when the corpus has not been fetched.
func TestCorpus_NoDuplicateDecls(t *testing.T) {
	t.Parallel()

	rmsFiles, err := filepath.Glob(filepath.Join("..", "..", ".corpus", "*.rms"))
	if err != nil || len(rmsFiles) == 0 {
		t.Skip("corpus not fetched")
	}
	xsFiles, err := filepath.Glob(filepath.Join("..", "..", ".corpus", "*.xs"))
	if err != nil {
		t.Skip("corpus not fetched")
	}

	store, err := kb.NewStore()
	require.NoError(t, err)
	a := NewAnalyzer(store)

	// inline XS blocks inside RMS maps
	for _, path := range rmsFiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			file, _ := rms.Parse(string(raw), path)

			for _, block := range file.XsBlocks {
				xsFile, _ := xs.XsParse(block.Code, "inline:"+filepath.Base(path))
				assertNoDuplicateDecls(t, a.AnalyzeXs(xsFile, nil))
			}
		})
	}

	// standalone XS scripts
	for _, path := range xsFiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			xsFile, _ := xs.XsParse(string(raw), path)
			assertNoDuplicateDecls(t, a.AnalyzeXs(xsFile, nil))
		})
	}
}

// assertNoDuplicateDecls fails the corpus subtest when the check fires.
func assertNoDuplicateDecls(t *testing.T, diags []common.Diagnostic) {
	t.Helper()

	for _, d := range diags {
		if d.Code == CodeDuplicateDeclaration {
			t.Errorf("duplicate-declaration fired: %s (line %d)", d.Message, d.Range.Start.Line)
		}
	}
}
