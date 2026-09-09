package analysis

import (
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func newSuggestAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewAnalyzer(store)
}

// TestAnalyzeRms_UnknownCommandSuggestion pins the did-you-mean suffix:
// a typo of a known command suggests the fix; garbage stays plain.
func TestAnalyzeRms_UnknownCommandSuggestion(t *testing.T) {
	t.Parallel()

	a := newSuggestAnalyzer(t)

	tests := []struct {
		name    string
		src     string
		code    string
		wantSub string
	}{
		{
			name:    "typo of a known command suggests the fix",
			src:     "creat_object",
			code:    CodeUnknownCommand,
			wantSub: `unknown command "creat_object"; did you mean "create_object"?`,
		},
		{
			name:    "case-insensitive distance suggests the canonical spelling",
			src:     "CREATE_OBJECT",
			code:    CodeUnknownCommand,
			wantSub: `unknown command "CREATE_OBJECT"; did you mean "create_object"?`,
		},
		{
			name:    "garbage command stays plain",
			src:     "zzzzqqqxxx",
			code:    CodeUnknownCommand,
			wantSub: `unknown command "zzzzqqqxxx"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, _ := rms.Parse(tt.src, "t.rms")
			diags := a.AnalyzeRms(file)

			require.NotEmpty(t, diags)

			var msg string

			for _, d := range diags {
				if d.Code == tt.code {
					msg = d.Message
				}
			}

			require.Contains(t, msg, tt.wantSub)
			if tt.name == "garbage command stays plain" {
				require.NotContains(t, msg, "did you mean")
			}
		})
	}
}

// TestAnalyzeRms_UnknownSectionNoSuggestion pins the scope: sections
// never carry suggestions.
func TestAnalyzeRms_UnknownSectionNoSuggestion(t *testing.T) {
	t.Parallel()

	a := newSuggestAnalyzer(t)

	file, _ := rms.Parse("<LAND_GENERAT>\nbase_terrain GRASS\n", "t.rms")
	diags := a.AnalyzeRms(file)

	found := false

	for _, d := range diags {
		if d.Code == CodeUnknownSection {
			found = true
			require.NotContains(t, d.Message, "did you mean")
		}
	}

	require.True(t, found, "the unknown section is reported")
}

// TestAnalyzeRms_UnknownAttributeSuggestion pins the attribute
// candidates: only the known command's own attributes.
func TestAnalyzeRms_UnknownAttributeSuggestion(t *testing.T) {
	t.Parallel()

	a := newSuggestAnalyzer(t)

	file, _ := rms.Parse("create_land {\n	land_percen 20\n}\n", "t.rms")
	diags := a.AnalyzeRms(file)

	var msg string

	for _, d := range diags {
		if d.Code == CodeUnknownAttribute {
			msg = d.Message
		}
	}

	require.NotEmpty(t, msg, "the unknown attribute is reported")
	require.Contains(t, msg, `; did you mean "land_percent"?`)
}

// TestAnalyzeXs_UndefinedSymbolSuggestion pins the XS candidates: kb
// functions/constants and the file's own declared names — a typo of a
// local suggests the local.
func TestAnalyzeXs_UndefinedSymbolSuggestion(t *testing.T) {
	t.Parallel()

	a := newSuggestAnalyzer(t)

	t.Run("typo of a kb function", func(t *testing.T) {
		t.Parallel()

		file, _ := xs.XsParse("void f() { xsGetMapSeedd(); }", "t.xs")
		diags := a.AnalyzeXs(file, nil)

		for _, d := range diags {
			if d.Code == CodeUndefinedSymbol {
				require.Contains(t, d.Message, `; did you mean "xsGetMapSeed"?`)
				return
			}
		}

		t.Fatal("undefined symbol not reported")
	})

	t.Run("typo of a declared local", func(t *testing.T) {
		t.Parallel()

		file, _ := xs.XsParse("void f() {\n	int counter = 1;\n	countr = 2;\n}\n", "t.xs")
		diags := a.AnalyzeXs(file, nil)

		for _, d := range diags {
			if d.Code == CodeUndefinedSymbol {
				require.Contains(t, d.Message, `; did you mean "counter"?`)
				return
			}
		}

		t.Fatal("undefined symbol not reported")
	})

	t.Run("garbage symbol stays plain", func(t *testing.T) {
		t.Parallel()

		file, _ := xs.XsParse("void f() { zzzzqqqxxx(); }", "t.xs")
		diags := a.AnalyzeXs(file, nil)

		for _, d := range diags {
			if d.Code == CodeUndefinedSymbol {
				require.NotContains(t, d.Message, "did you mean")
				return
			}
		}

		t.Fatal("undefined symbol not reported")
	})
}

// TestSuggestSuffix_DeterministicTie pins the tie-break: equal
// distances resolve to the lexicographically smaller name regardless
// of candidate order.
func TestSuggestSuffix_DeterministicTie(t *testing.T) {
	t.Parallel()

	forward := []string{"create_y", "create_x"}
	reverse := slices.Clone(forward)
	slices.Reverse(reverse)

	// one substitution away from both candidates — a genuine tie
	require.Equal(t, suggestSuffix(forward, "create_z"), suggestSuffix(reverse, "create_z"))
	require.Contains(t, suggestSuffix(forward, "create_z"), "create_x")
}
