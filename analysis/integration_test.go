package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aoe2-lsp/common"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
)

// TestAnalyzeXs_PreludeNoFalsePositives guards the acceptance criterion:
// the 5k-line prelude.xs fixture (882 externs) must yield zero analyzer
// diagnostics — in particular no bad-type from the type inference.
func TestAnalyzeXs_PreludeNoFalsePositives(t *testing.T) {
	raw, err := os.ReadFile("../docs/ref/ugc-guide/xs/prelude.xs")
	require.NoError(t, err)

	file, parseDiags := xs.XsParse(string(raw), "prelude.xs")
	require.Empty(t, parseDiags, "prelude.xs must parse without false errors")

	a := newAnalyzer(t)

	assert.Empty(t, codes(a.AnalyzeXs(file)), "prelude.xs must not trigger analyzer diagnostics")
}

// TestAnalyzeRms_FixturesRegression walks the parser fixtures and pins the
// analyzer baseline: the value checks add no diagnostics to real-world
// scripts (no bad-argument-value anywhere).
func TestAnalyzeRms_FixturesRegression(t *testing.T) {
	// baseline observed before the value checks landed
	want := map[string][]string{
		"broken.rms":       {CodeUnknownCommand},
		"random.rms":       {CodeUnknownAttribute},
		"conditionals.rms": {},
		"expressions.rms":  {},
		"includes.rms":     {},
		"positional.rms":   {},
		"sections.rms":     {},
	}

	files, err := filepath.Glob("../rms/testdata/*.rms")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	a := newAnalyzer(t)

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			file, _ := rms.Parse(string(raw), filepath.Base(path))
			got := codes(a.AnalyzeRms(file))

			assert.Equal(t, want[filepath.Base(path)], got)
			assert.NotContains(t, got, CodeBadArgumentValue, "fixtures must not gain value diagnostics")
		})
	}
}

// TestPipeline_MergedDiagnostics mirrors the server composition: syntax
// diagnostics of both parsers plus analyzer output for the RMS file and
// every inline XS block, shifted to document coordinates — one sorted
// batch.
func TestPipeline_MergedDiagnostics(t *testing.T) {
	src := `<LAND_GENERATION>
create_land_bogus
create_land
land_percent 150
#includeXS
void f() {
	sqrt("fast");
}
`
	a := newAnalyzer(t)

	file, diags := rms.Parse(src, "map.rms")
	diags = append(diags, a.AnalyzeRms(file)...)

	for _, block := range file.XsBlocks {
		xfile, xdiags := xs.XsParse(block.Code, "inline:map.rms")

		diags = append(diags, shiftDiags(a.AnalyzeXs(xfile), block.Range.Start)...)
		diags = append(diags, shiftDiags(xdiags, block.Range.Start)...)
	}

	got := codes(diags)

	assert.Contains(t, got, CodeUnknownCommand)
	assert.Contains(t, got, CodeBadArgumentValue)
	assert.Contains(t, got, CodeBadType)
	assert.NotContains(t, got, CodeBadArity)

	assertSorted(t, diags)
}

// shiftDiags moves block-relative diagnostics into document coordinates.
func shiftDiags(diags []common.Diagnostic, by common.Pos) []common.Diagnostic {
	out := make([]common.Diagnostic, len(diags))

	for i, d := range diags {
		start, end := d.Range.Start, d.Range.End

		if start.Line == 0 {
			start.Column += by.Column
		}

		if end.Line == 0 {
			end.Column += by.Column
		}

		start.Line += by.Line
		end.Line += by.Line

		out[i] = d
		out[i].Range = common.Range{Start: start, End: end}
	}

	return out
}
