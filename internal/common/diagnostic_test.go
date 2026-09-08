package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnostic_SeverityValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		severity int
		want     int
	}{
		{name: "error is 1", severity: SeverityError, want: 1},
		{name: "warning is 2", severity: SeverityWarning, want: 2},
		{name: "info is 3", severity: SeverityInfo, want: 3},
		{name: "hint is 4", severity: SeverityHint, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.severity)

			assert.GreaterOrEqual(t, tt.severity, SeverityError)
			assert.LessOrEqual(t, tt.severity, SeverityHint)
		})
	}
}

func TestDiagnostic_Fields(t *testing.T) {
	t.Parallel()
	d := Diagnostic{
		Range: Range{
			Start: Pos{Line: 3, Column: 0, Offset: 42},
			End:   Pos{Line: 3, Column: 13, Offset: 55},
		},
		Severity: SeverityError,
		Message:  "unknown command 'create_elefant'",
		Code:     "unknown-command",
	}

	require.Equal(t, Pos{Line: 3, Column: 0, Offset: 42}, d.Range.Start)
	require.Equal(t, Pos{Line: 3, Column: 13, Offset: 55}, d.Range.End)
	require.Equal(t, SeverityError, d.Severity)
	require.Equal(t, "unknown command 'create_elefant'", d.Message)
	require.Equal(t, "unknown-command", d.Code)
}
