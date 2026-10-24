package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"testing"

	"github.com/stretchr/testify/require"
)

// newNuanceAnalyzer builds an analyzer over the synthetic nuance fixture
// knowledge base in testdata/nuance-kb.
func newNuanceAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	store, err := kb.NewStoreFromDir("testdata/nuance-kb")
	require.NoError(t, err)

	return NewAnalyzer(store)
}

// diagSnap is a diagnostic without the range, so nuance cases compare
// severity, message and code in one assert.
type diagSnap struct {
	severity int
	message  string
	code     string
}

func diagSnaps(diags []common.Diagnostic) []diagSnap {
	out := make([]diagSnap, 0, len(diags))
	for _, d := range diags {
		out = append(out, diagSnap{d.Severity, d.Message, d.Code})
	}

	return out
}

// TestAnalyzeRms_DeprecatedFromData pins the migration equality: the
// data-driven deprecated rule emits the exact diagnostic the removed
// hardcoded effect_percent block produced — stable code, warning severity,
// verbatim message, zero-width range at the statement start.
func TestAnalyzeRms_DeprecatedFromData(t *testing.T) {
	t.Parallel()

	a := newAnalyzer(t)

	file, _ := rms.Parse("<PLAYER_SETUP>\neffect_percent SET_ATTRIBUTE HOUSE ATTR_STORAGE_VALUE 10\n", "test.rms")

	require.Equal(t, []common.Diagnostic{{
		Range:    common.Range{Start: common.Pos{Line: 1, Column: 0, Offset: 15}, End: common.Pos{Line: 1, Column: 0, Offset: 15}},
		Severity: common.SeverityWarning,
		Message:  "effect_percent is deprecated; use effect_amount instead",
		Code:     CodeDeprecatedEffectPercent,
	}}, a.AnalyzeRms(file))
}

// TestAnalyzeRms_Nuances covers the nuance ladder on the fixture KB: no-op
// pairs become hints, typo aliases become warnings, names without a nuance
// keep the previous errors, deprecated and legacy data markers surface
// verbatim.
func TestAnalyzeRms_Nuances(t *testing.T) {
	t.Parallel()

	a := newNuanceAnalyzer(t)

	tests := []struct {
		name string
		src  string
		want []diagSnap
	}{
		{
			name: "no-op attribute in braces is a hint",
			src: `<TERRAIN_GENERATION>
create_terrain {
	border_fuzziness 50
}
`,
			want: []diagSnap{{
				common.SeverityHint,
				`attribute "border_fuzziness" of "create_terrain" is probably ignored by the game`,
				CodeProbablyIgnored,
			}},
		},
		{
			name: "typo alias attribute in braces is a warning",
			src: `<OBJECTS_GENERATION>
create_object {
	group_varience 2
}
`,
			want: []diagSnap{{
				common.SeverityWarning,
				`unknown attribute "group_varience" of "create_object"; did you mean "group_variance"?`,
				CodeTypoAlias,
			}},
		},
		{
			name: "unknown attribute without a nuance stays an error",
			src: `<OBJECTS_GENERATION>
create_object {
	no_such_attr 1
}
`,
			want: []diagSnap{{
				common.SeverityError,
				`unknown attribute "no_such_attr" of "create_object"`,
				CodeUnknownAttribute,
			}},
		},
		{
			name: "no-op pair is scoped to its command",
			src: `<OBJECTS_GENERATION>
create_object {
	border_fuzziness 50
}
`,
			want: []diagSnap{{
				common.SeverityError,
				`unknown attribute "border_fuzziness" of "create_object"`,
				CodeUnknownAttribute,
			}},
		},
		{
			name: "typo alias command is a warning",
			src: `<TERRAIN_GENERATION>
group_varience 2
`,
			want: []diagSnap{{
				common.SeverityWarning,
				`unknown command "group_varience"; did you mean "group_variance"?`,
				CodeTypoAlias,
			}},
		},
		{
			name: "unknown command keeps its did-you-mean fallback",
			src: `<TERRAIN_GENERATION>
create_terain
`,
			want: []diagSnap{{
				common.SeverityError,
				`unknown command "create_terain"; did you mean "create_terrain"?`,
				CodeUnknownCommand,
			}},
		},
		{
			name: "bare no-op attribute attaches to the previous command",
			src: `<TERRAIN_GENERATION>
create_terrain
border_fuzziness 50
`,
			want: []diagSnap{{
				common.SeverityHint,
				`attribute "border_fuzziness" of "create_terrain" is probably ignored by the game`,
				CodeProbablyIgnored,
			}},
		},
		{
			name: "bare typo alias attribute attaches to the previous command",
			src: `<TERRAIN_GENERATION>
create_terrain
group_varience 2
`,
			want: []diagSnap{{
				common.SeverityWarning,
				`unknown attribute "group_varience" of "create_terrain"; did you mean "group_variance"?`,
				CodeTypoAlias,
			}},
		},
		{
			name: "legacy directive is a hint",
			src: `<PLAYER_SETUP>
#undefine FOO
`,
			want: []diagSnap{{
				common.SeverityHint,
				"#undefine is a legacy form",
				"legacy-undefine",
			}},
		},
		{
			name: "deprecated command from data is a warning",
			src: `<PLAYER_SETUP>
old_cmd
`,
			want: []diagSnap{{
				common.SeverityWarning,
				"old_cmd is deprecated",
				"deprecated-old-cmd",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := rms.Parse(tt.src, "test.rms")

			require.Equal(t, tt.want, diagSnaps(a.AnalyzeRms(file)))
		})
	}
}
