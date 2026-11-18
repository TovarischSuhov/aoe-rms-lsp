package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCorpus_NoDuplicateDecls runs the same-scope redeclaration check
// over the real corpus sample (make corpus): a community-written script
// tripping the warning is a false positive to fix, not a map to shame.
// Unused declarations are counted, not gated: true leftovers in wild
// scripts are findings, while top-level function flags in library-shaped
// files (no rules or events) would be false positives — the include
// closure calls them — and fail the gate. The test skips when the corpus
// has not been fetched.
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
				assertAnalysisAcceptance(t, xsFile, a.AnalyzeXs(xsFile, nil))
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
			assertAnalysisAcceptance(t, xsFile, a.AnalyzeXs(xsFile, nil))
		})
	}
}

// assertAnalysisAcceptance gates duplicate-declaration at zero, keeps
// top-level function hints out of library-shaped files and counts the
// rest of the unused findings for the slot's PR report.
func assertAnalysisAcceptance(t *testing.T, file xs.XsFile, diags []common.Diagnostic) {
	t.Helper()

	scriptShaped := false
	for i := range file.Decls {
		if file.Decls[i].Kind == xs.DeclRule || file.Decls[i].Kind == xs.DeclEvent {
			scriptShaped = true
		}
	}

	unused, unusedFunctions := 0, 0
	for _, d := range diags {
		switch d.Code {
		case CodeDuplicateDeclaration:
			t.Errorf("duplicate-declaration fired: %s (line %d)", d.Message, d.Range.Start.Line)
		case CodeUnusedDeclaration:
			unused++
			if isFunctionFlag(d.Message) {
				unusedFunctions++
				if !scriptShaped {
					t.Errorf("unused function in a library-shaped file: %s (line %d)",
						d.Message, d.Range.Start.Line)
				}
			}
		}
	}

	if unused > 0 {
		t.Logf("unused-declaration findings: %d (%d function-level)", unused, unusedFunctions)
	}
}

// isFunctionFlag reports whether the unused-declaration message is about
// a top-level function rather than a local.
func isFunctionFlag(message string) bool {
	return strings.HasPrefix(message, "function ")
}
