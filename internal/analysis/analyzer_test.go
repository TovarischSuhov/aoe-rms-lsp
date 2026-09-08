package analysis

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/kb"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAnalyzer builds the analyzer over the embedded knowledge base.
func newAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	store, err := kb.NewStore()
	require.NoError(t, err)

	return NewAnalyzer(store)
}

// codes extracts the diagnostic codes in order.
func codes(diags []common.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Code)
	}

	return out
}

// TestAnalyzeRms_Checks covers every RMS check with a positive and a
// negative case.
func TestAnalyzeRms_Checks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []string // diagnostic codes in order
	}{
		{
			name: "valid commands and attributes are clean",
			src: `<LAND_GENERATION>
base_terrain GRASS
create_land {
	land_percent 20
}`,
			want: []string{},
		},
		{
			name: "unknown command",
			src: `<LAND_GENERATION>
create_land_bogus
`,
			want: []string{CodeUnknownCommand},
		},
		{
			name: "unknown section",
			src: `<GENERATION_OF_WONK>
create_land
`,
			want: []string{CodeUnknownSection},
		},
		{
			name: "unknown attribute",
			src: `<LAND_GENERATION>
create_land {
	bogus_attribute 1
}
`,
			want: []string{CodeUnknownAttribute},
		},
		{
			name: "too many arguments",
			src: `<LAND_GENERATION>
base_terrain GRASS JUNK
`,
			want: []string{CodeBadArgument},
		},
		{
			name: "effect_percent is deprecated",
			src: `<PLAYER_SETUP>
effect_percent SET_ATTRIBUTE HOUSE ATTR_STORAGE_VALUE 10
`,
			want: []string{CodeDeprecatedEffectPercent},
		},
		{
			name: "bare attribute statement attaches to the previous command",
			src: `<OBJECTS_GENERATION>
create_object WOLF
number_of_objects 2
`,
			want: []string{},
		},
		{
			name: "nested commands inside random blocks are checked",
			src: `<OBJECTS_GENERATION>
start_random
	percent_chance 35
		create_object_bogus SHEEP
end_random
`,
			want: []string{CodeUnknownCommand},
		},
		{
			name: "percent attribute out of range",
			src: `<LAND_GENERATION>
create_land {
	land_percent 150
}
`,
			want: []string{CodeBadArgumentValue},
		},
		{
			name: "bare attribute value out of range",
			src: `<LAND_GENERATION>
create_land
land_percent 150
`,
			want: []string{CodeBadArgumentValue},
		},
		{
			name: "percent positional argument out of range",
			src: `percent_chance 150
`,
			want: []string{CodeBadArgumentValue},
		},
		{
			name: "helper call values are not flagged",
			src: `<LAND_GENERATION>
create_land
land_percent rand_float(10, 20)
`,
			want: []string{},
		},
		{
			name: "directives are not commands",
			src: `#const START_GOLD 800
<LAND_GENERATION>
base_terrain GRASS
`,
			want: []string{},
		},
	}

	a := newAnalyzer(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, _ := rms.Parse(tt.src, "test.rms")

			assert.Equal(t, tt.want, codes(a.AnalyzeRms(file)))
		})
	}
}

// TestAnalyzeRms_Invariants checks sorting, severity and that the input
// AST is not mutated.
func TestAnalyzeRms_Invariants(t *testing.T) {
	t.Parallel()
	a := newAnalyzer(t)

	src := `<GENERATION_OF_WONK>
create_land_bogus
create_land {
	bogus_attribute 1
}
`
	file, _ := rms.Parse(src, "test.rms")

	before, err := json.Marshal(file)
	require.NoError(t, err)

	diags := a.AnalyzeRms(file)

	after, err := json.Marshal(file)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after), "the AST must not be mutated")

	require.Len(t, diags, 3)
	assert.Equal(t, common.SeverityError, diags[0].Severity)
	assertSorted(t, diags)

	// re-analysis is stable (stateless per call)
	assert.Equal(t, codes(diags), codes(a.AnalyzeRms(file)))
}

// TestAnalyzeXs_Checks covers every XS check with a positive and a
// negative case.
func TestAnalyzeXs_Checks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "declared names and kb functions are clean",
			src: `int counter = 0;

void tick(int step) {
	int local = step + counter;
	int seed = xsGetMapSeed();
}
`,
			want: []string{},
		},
		{
			name: "undefined call",
			src: `void main() {
	xsGetMapSeedTypo();
}
`,
			want: []string{CodeUndefinedSymbol},
		},
		{
			name: "too many arguments",
			src: `void main() {
	xsGetMapSeed(1, 2);
}
`,
			want: []string{CodeBadArity},
		},
		{
			name: "vector members and builtins are clean",
			src: `void place(vector pos) {
	vector v = vector(0, 0, 0);
	v.x = pos.x;
	bool done = true;
}
`,
			want: []string{},
		},
		{
			name: "rules and externs declare names",
			src: `rule checker inactive {
	condition {
		ready()
	}
	action {
		int seed = xsGetMapSeed()
	}
}
`,
			want: []string{CodeUndefinedSymbol}, // ready is nowhere declared
		},
		{
			name: "call argument type mismatch",
			src: `void f() {
	sqrt("fast");
}
`,
			want: []string{CodeBadType}, // sqrt takes float
		},
		{
			name: "int widens to float silently",
			src: `void f() {
	sqrt(2);
}
`,
			want: []string{},
		},
		{
			name: "assignment to a typed top-level variable",
			src: `int x = 1.5;

void f() {
}
`,
			want: []string{CodeBadType},
		},
		{
			name: "return type mismatch",
			src: `int f() {
	return 1.5;
}
`,
			want: []string{CodeBadType},
		},
		{
			name: "untyped locals stay silent",
			src: `void g(int p) {
	int local = p;
	local = 1.5;
}
`,
			want: []string{},
		},
		{
			name: "bare local declarations are declared",
			src: `void h() {
	int x;
	x = 1;
}
`,
			want: []string{},
		},
	}

	a := newAnalyzer(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, _ := xs.XsParse(tt.src, "test.xs")

			assert.Equal(t, tt.want, codes(a.AnalyzeXs(file, nil)))
		})
	}
}

// TestAnalyzeXs_Invariants checks sorting, severity and AST immutability.
func TestAnalyzeXs_Invariants(t *testing.T) {
	t.Parallel()
	a := newAnalyzer(t)

	src := `void main() {
	int a = b + xsGetMapSeed(1, 2);
}
`
	file, _ := xs.XsParse(src, "test.xs")

	before, err := json.Marshal(file)
	require.NoError(t, err)

	diags := a.AnalyzeXs(file, nil)

	after, err := json.Marshal(file)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after), "the AST must not be mutated")

	require.Len(t, diags, 2)
	assert.Equal(t, common.SeverityError, diags[0].Severity)
	assertSorted(t, diags)
}

// assertSorted checks that diagnostics are ordered by position.
func assertSorted(t *testing.T, diags []common.Diagnostic) {
	t.Helper()

	for i := 1; i < len(diags); i++ {
		prev, cur := diags[i-1].Range.Start, diags[i].Range.Start

		if prev.Line == cur.Line {
			require.LessOrEqual(t, prev.Column, cur.Column)

			continue
		}

		require.Less(t, prev.Line, cur.Line)
	}
}

// TestAnalyzeXs_ExternalsSuppressUndefinedAndLocalsWin checks that closure
// declarations suppress undefined-symbol, provide types, and never
// override the file's own declarations.
func TestAnalyzeXs_ExternalsSuppressUndefinedAndLocalsWin(t *testing.T) {
	t.Parallel()
	a := newAnalyzer(t)

	// the file calls an external function declared only in the closure
	src := "void h() { extFn(1); }\n"
	file, _ := xs.XsParse(src, "t.xs")

	externals := []xs.Decl{{Kind: xs.DeclFunction, Name: "extFn", Type: "void"}}

	withExt := a.AnalyzeXs(file, externals)
	for _, d := range withExt {
		assert.NotEqual(t, "undefined-symbol", d.Code)
	}

	withoutExt := a.AnalyzeXs(file, nil)
	assert.Equal(t, []string{"undefined-symbol"}, codes(withoutExt))

	// externals provide types for bad-type checks: float param, int arg
	typed := []xs.Decl{{
		Kind: xs.DeclFunction, Name: "takeFloat", Type: "void",
		Params: []xs.Param{{Name: "v", Type: "float"}},
	}}
	tf, _ := xs.XsParse("void u() { takeFloat(1); }", "t.xs")
	assert.Equal(t, []string{}, codes(a.AnalyzeXs(tf, typed)))

	// a local declaration always wins: local extFn takes precedence
	both, _ := xs.XsParse("void extFn() { }\nvoid c() { extFn(); }", "t.xs")
	assert.Empty(t, codes(a.AnalyzeXs(both, externals)))

	// empty-name externals are skipped
	empty := []xs.Decl{{Kind: xs.DeclFunction, Name: ""}}
	e, _ := xs.XsParse("void q() { ghost(); }", "t.xs")
	assert.Equal(t, []string{"undefined-symbol"}, codes(a.AnalyzeXs(e, empty)))
}

func TestAnalyzeXs_ForLoopVarNoUndefined(t *testing.T) {
	t.Parallel()
	// the review repro: the loop variable used in init/cond/step/body
	// must not fire undefined-symbol (previously 4 false errors)
	src := "void main() { for (int i = 0; i < 10; i++) { int j = i + 1; } }"

	file, _ := xs.XsParse(src, "for.xs")
	a := newAnalyzer(t)

	assert.Equal(t, []string{}, codes(a.AnalyzeXs(file, nil)))
}

func TestAnalyzeXs_ForLoopVarUsedAfterNotFlagged(t *testing.T) {
	t.Parallel()
	// conservative semantics: a use after the loop is not marked —
	// same treatment as locals of nested blocks
	src := "void main() { for (int i = 0; i < 3; i++) { } i = 5; }"

	file, _ := xs.XsParse(src, "after.xs")
	a := newAnalyzer(t)

	assert.Equal(t, []string{}, codes(a.AnalyzeXs(file, nil)))
}

func TestAnalyzeXs_TwoSectionForImplicitVar(t *testing.T) {
	t.Parallel()
	// the real-world XS for (`for (i = 1; <= size)`): the implicit loop
	// variable is declared, the operator-headed condition parses
	// (xslibs corpus: 100+ false undefined-symbol/syntax before the fix)
	src := `void main() {
	int size = 10;
	for (i = 1; <= size) {
		xsSetGoal(1, i);
	}
}
`

	file, _ := xs.XsParse(src, "fortwo.xs")
	a := newAnalyzer(t)

	assert.Equal(t, []string{}, codes(a.AnalyzeXs(file, nil)))
}

func TestAnalyzeXs_ConstQualifierCompatible(t *testing.T) {
	t.Parallel()
	// const is a qualifier, not a distinct type: const declarations are
	// returned and assigned freely (xslibs: 490 false bad-type before)
	src := `extern const int cOk = 0;
int status() {
	return cOk;
}
void main() {
	int last = status();
	_stringLast = cOk;
}
int _stringLast = 0;
`

	file, _ := xs.XsParse(src, "const.xs")
	a := newAnalyzer(t)

	assert.Equal(t, []string{}, codes(a.AnalyzeXs(file, nil)))
}
