package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"testing"

	"github.com/stretchr/testify/require"
)

// Compile-time contract check: the exported surface must match
// analysis/CODEMANIFEST exactly.
var _ func(spec kb.CommandArg, kind, value string, r common.Range) (common.Diagnostic, bool) = CheckRmsValue

func TestCheckRmsValue_PercentOutOfRange(t *testing.T) {
	t.Parallel()
	r := common.Range{Start: common.Pos{Line: 1, Column: 2}, End: common.Pos{Line: 1, Column: 5}}
	spec := kb.CommandArg{Kind: "percent"}

	tests := []struct {
		name  string
		kind  string
		value string
	}{
		{name: "percent literal too large", kind: rms.KindPercent, value: "150"},
		{name: "percent literal with suffix too large", kind: rms.KindPercent, value: "150%"},
		{name: "number literal negative", kind: rms.KindNumber, value: "-1"},
		{name: "number literal too large", kind: rms.KindNumber, value: "100.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diag, reported := CheckRmsValue(spec, tt.kind, tt.value, r)

			require.True(t, reported)
			require.Equal(t, common.SeverityError, diag.Severity)
			require.Equal(t, CodeBadArgumentValue, diag.Code)
			require.Equal(t, r, diag.Range)
			require.Contains(t, diag.Message, "0..100")
		})
	}
}

func TestCheckRmsValue_PercentBoundaries(t *testing.T) {
	t.Parallel()
	spec := kb.CommandArg{Kind: "percent"}

	values := []struct {
		kind  string
		value string
	}{
		{kind: rms.KindNumber, value: "0"},
		{kind: rms.KindNumber, value: "100"},
		{kind: rms.KindPercent, value: "0%"},
		{kind: rms.KindPercent, value: "100%"},
		{kind: rms.KindNumber, value: "50"},
	}

	for _, tt := range values {
		t.Run(tt.value, func(t *testing.T) {
			_, reported := CheckRmsValue(spec, tt.kind, tt.value, common.Range{})
			require.False(t, reported)
		})
	}
}

func TestCheckRmsValue_ExpressionSkipped(t *testing.T) {
	t.Parallel()
	r := common.Range{}

	tests := []struct {
		name  string
		spec  kb.CommandArg
		kind  string
		value string
	}{
		{name: "binary expression", spec: kb.CommandArg{Kind: "percent"}, kind: rms.KindBinary, value: "1 + 2"},
		{name: "unary expression", spec: kb.CommandArg{Kind: "percent"}, kind: rms.KindUnary, value: "-1"},
		{name: "const name under percent spec", spec: kb.CommandArg{Kind: "percent"}, kind: rms.KindConst, value: "GRASS"},
		{name: "const spec not modelled", spec: kb.CommandArg{Kind: "const"}, kind: rms.KindConst, value: "GRASS"},
		{name: "number spec not checked", spec: kb.CommandArg{Kind: "number"}, kind: rms.KindNumber, value: "7"},
		{name: "float spec not checked", spec: kb.CommandArg{Kind: "float"}, kind: rms.KindNumber, value: "3.5"},
		{name: "empty spec kind", spec: kb.CommandArg{Kind: ""}, kind: rms.KindNumber, value: "150"},
		{name: "unparsable literal stays silent", spec: kb.CommandArg{Kind: "percent"}, kind: rms.KindNumber, value: "12x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reported := CheckRmsValue(tt.spec, tt.kind, tt.value, r)
			require.False(t, reported)
		})
	}
}
