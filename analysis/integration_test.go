package analysis

import (
	"fmt"
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

	diags := a.AnalyzeXs(file, nil)

	require.Empty(t, diags, "prelude.xs must not trigger analyzer diagnostics: %v", messagesWithPos(diags))
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

			base := filepath.Base(path)
			require.Contains(t, want, base, "pin the baseline for the new fixture")

			file, _ := rms.Parse(string(raw), base)
			got := codes(a.AnalyzeRms(file))

			assert.Equal(t, want[base], got)
			assert.NotContains(t, got, CodeBadArgumentValue, "fixtures must not gain value diagnostics")
		})
	}
}

// TestPipeline_MergedDiagnostics mirrors the server composition: syntax
// diagnostics of both parsers plus analyzer output for the RMS file and
// every inline XS block, shifted to document coordinates — one batch,
// sorted before publishing.
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

		diags = append(diags, shiftDiags(xdiags, block.Range.Start)...)
		diags = append(diags, shiftDiags(a.AnalyzeXs(xfile, nil), block.Range.Start)...)
	}

	sortDiags(diags) // the server sorts the merged batch the same way

	got := codes(diags)

	assert.Contains(t, got, CodeUnknownCommand)
	assert.Contains(t, got, CodeBadArgumentValue)
	assert.Contains(t, got, CodeBadType)
	assert.NotContains(t, got, CodeBadArity)

	assertSorted(t, diags)
}

// messagesWithPos renders diagnostics as line:col message for readable
// failures.
func messagesWithPos(diags []common.Diagnostic) []string {
	out := make([]string, 0, len(diags))

	for _, d := range diags {
		out = append(out, fmt.Sprintf("%d:%d %s", d.Range.Start.Line, d.Range.Start.Column, d.Message))
	}

	return out
}

// shiftDiags moves block-relative diagnostics into document coordinates.
// It mirrors the server implementation (server.shiftPos) verbatim.
func shiftDiags(diags []common.Diagnostic, base common.Pos) []common.Diagnostic {
	out := make([]common.Diagnostic, len(diags))

	for i, d := range diags {
		out[i] = d
		out[i].Range.Start = shiftPos(d.Range.Start, base)
		out[i].Range.End = shiftPos(d.Range.End, base)
	}

	return out
}

// shiftPos maps a block-relative position into the document by adding the
// block start; only the first block line also gains the start column.
func shiftPos(p common.Pos, base common.Pos) common.Pos {
	shifted := common.Pos{
		Line:   p.Line + base.Line,
		Column: p.Column,
		Offset: p.Offset + base.Offset,
	}

	if p.Line == 0 {
		shifted.Column += base.Column
	}

	return shifted
}
