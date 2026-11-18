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

// TestCorpus_NoDuplicateDecls runs the analysis gates over the real
// corpus sample (make corpus): duplicate-declaration and missing-section
// must stay at zero (a community-written script tripping them is a false
// positive to fix, not a map to shame); unused-declaration and
// empty-section are counted, not gated — true leftovers and empty
// sections in wild scripts are findings. Top-level function hints in
// library-shaped files (no rules or events) would be false positives —
// the include closure calls them — and fail the gate. The test skips
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

	// RMS maps: the structural section checks run here too
	for _, path := range rmsFiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			file, _ := rms.Parse(string(raw), path)
			assertAnalysisAcceptance(t, true, a.AnalyzeRms(file))

			for _, block := range file.XsBlocks {
				xsFile, _ := xs.XsParse(block.Code, "inline:"+filepath.Base(path))
				assertAnalysisAcceptance(t, xsShaped(xsFile), a.AnalyzeXs(xsFile, nil))
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
			assertAnalysisAcceptance(t, xsShaped(xsFile), a.AnalyzeXs(xsFile, nil))
		})
	}
}

// xsShaped reports whether the file has engine entry points (rules or
// events) — the opposite of library-shaped.
func xsShaped(file xs.XsFile) bool {
	for i := range file.Decls {
		if file.Decls[i].Kind == xs.DeclRule || file.Decls[i].Kind == xs.DeclEvent {
			return true
		}
	}

	return false
}

// assertAnalysisAcceptance gates duplicate-declaration and
// missing-section at zero, keeps top-level function hints out of
// library-shaped files and counts the rest of the findings for the
// slots' PR reports.
func assertAnalysisAcceptance(t *testing.T, scriptShaped bool, diags []common.Diagnostic) {
	t.Helper()

	unused, unusedFunctions, emptySections, misplaced, positional, ignored := 0, 0, 0, 0, 0, 0
	for _, d := range diags {
		switch d.Code {
		case CodeDuplicateDeclaration:
			t.Errorf("duplicate-declaration fired: %s (line %d)", d.Message, d.Range.Start.Line)
		case CodeMissingSection:
			t.Errorf("missing-section fired: %s (line %d)", d.Message, d.Range.Start.Line)
		case CodeReturnParens:
			t.Errorf("return-parens fired: %s (line %d)", d.Message, d.Range.Start.Line)
		case CodeUnusedDeclaration:
			unused++
			if isFunctionFlag(d.Message) {
				unusedFunctions++
				if !scriptShaped {
					t.Errorf("unused function in a library-shaped file: %s (line %d)",
						d.Message, d.Range.Start.Line)
				}
			}
		case CodeEmptySection:
			emptySections++
		case CodeMisplacedAttribute:
			misplaced++
		case CodeCreateLandPositional:
			positional++
		case CodeProbablyIgnored:
			ignored++
		}
	}

	if unused > 0 || emptySections > 0 || misplaced > 0 || positional > 0 || ignored > 0 {
		t.Logf("findings: unused %d (%d fn-level), empty %d, misplaced %d, land-positional %d, probably-ignored %d",
			unused, unusedFunctions, emptySections, misplaced, positional, ignored)
	}
}

// isFunctionFlag reports whether the unused-declaration message is about
// a top-level function rather than a local.
func isFunctionFlag(message string) bool {
	return strings.HasPrefix(message, "function ")
}
