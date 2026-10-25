package format

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/xs"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// xsGoldenInputs lists the XS testdata cases; every one is both printed
// against its golden and re-formatted for the idempotency invariant.
func xsGoldenInputs(t *testing.T) []string {
	t.Helper()

	inputs, err := filepath.Glob(filepath.Join("testdata", "*.xs"))
	if err != nil {
		t.Fatalf("glob testdata: %v", err)
	}

	if len(inputs) == 0 {
		t.Fatal("no golden inputs in testdata")
	}

	return inputs
}

// TestXS_Golden formats every XS testdata input with the default options
// and compares with the matching .golden byte for byte; -update writes
// the goldens instead.
func TestXS_Golden(t *testing.T) {
	t.Parallel()

	for _, in := range xsGoldenInputs(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			got, err := XS(string(src), Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			golden := strings.TrimSuffix(in, ".xs") + ".golden"

			if *updateGolden {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}

				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}

			if got != string(want) {
				t.Errorf("XS(%s) mismatch:\n--- got ---\n%s\n--- want ---\n%s", in, got, want)
			}
		})
	}
}

// TestXS_IdempotentGolden checks the contract invariant on every XS golden
// input: formatting an already formatted document changes nothing.
func TestXS_IdempotentGolden(t *testing.T) {
	t.Parallel()

	for _, in := range xsGoldenInputs(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			once, err := XS(string(src), Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(formatted): %v", err)
			}

			if twice != once {
				t.Errorf("XS(XS(%s)) differs:\n--- once ---\n%q\n--- twice ---\n%q", in, once, twice)
			}
		})
	}
}

// TestXS_RefusalOnError checks the refusal contract: error-severity
// diagnostics answer the shared sentinel and an empty formatted value,
// never a partial print. Each row's input is also parsed directly, tying
// the refusal to its cause — at least one error-severity diagnostic — so
// an implementation refusing everything does not pass either. The
// warning-only row pins the other half: warnings format, they do not
// refuse.
func TestXS_RefusalOnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			// the ternary `?:` is outside the parsed expression grammar:
			// as a statement it dies at the `?` with a missing `;`
			name: "ternary-statement",
			src:  "void f() {\na ? b : c;\n}\n",
		},
		{
			// the lexer has no exponent form: `1e5` is the literal 1
			// glued to the identifier e5, a missing `;` in between
			name: "exponent-literal",
			src:  "void f() {\nint x = 1e5;\n}\n",
		},
		{
			// a stray operator token at top level (bytes outside the
			// operator set, like `@`, are skipped by the scanner without
			// a diagnostic and refuse nothing)
			name: "garbage-token",
			src:  "?\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := XS(tt.src, Options{TabSize: 4})
			if !errors.Is(err, ErrParseErrors) {
				t.Fatalf("XS error = %v, want ErrParseErrors", err)
			}

			if got != "" {
				t.Errorf("XS refusal output = %q, want empty", got)
			}

			_, diags := xs.XsParse(tt.src, "case.xs")

			errs := 0

			for _, d := range diags {
				if d.Severity == common.SeverityError {
					errs++
				}
			}

			require.Positive(t, errs, "xs.XsParse(%q) diagnostics = %v, want at least one error", tt.src, diags)
		})
	}

	t.Run("warning-only", func(t *testing.T) {
		t.Parallel()

		// a two-component vector literal is a warning, not an error: the
		// input formats instead of refusing
		src := "void f() {\n(1, 2);\n}\n"

		out, err := XS(src, Options{TabSize: 4})
		if err != nil {
			t.Fatalf("XS refused a warning-only input: %v", err)
		}

		if out == "" {
			t.Errorf("XS warning-only output = %q, want formatted text", out)
		}
	})
}

// TestXS_EmptyInput checks that nothing printable answers nothing: the
// empty document and whitespace-only text format to the empty string.
func TestXS_EmptyInput(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		"empty":        "",
		"whitespace":   "  \n\t \n\n",
		"only newline": "\n\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := XS(src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			if got != "" {
				t.Errorf("XS(%q) = %q, want empty", src, got)
			}
		})
	}
}

// TestXS_ZeroOptions checks that the zero Options format like the default
// width: TabSize 0 means 4.
func TestXS_ZeroOptions(t *testing.T) {
	t.Parallel()

	src := "int f(int size) {\nint x = size * 2;\nreturn x;\n}\n"

	want, err := XS(src, Options{TabSize: 4})
	if err != nil {
		t.Fatalf("XS: %v", err)
	}

	got, err := XS(src, Options{})
	if err != nil {
		t.Fatalf("XS zero options: %v", err)
	}

	if got != want {
		t.Errorf("XS(Options{}) = %q, want %q", got, want)
	}
}

// TestXS_IndentTabs checks the tab indentation mode: one tab per level
// regardless of TabSize.
func TestXS_IndentTabs(t *testing.T) {
	t.Parallel()

	src := "int f(int size) {\nint x = 1;\nreturn x;\n}\n"

	got, err := XS(src, Options{TabSize: 8, IndentTabs: true})
	if err != nil {
		t.Fatalf("XS: %v", err)
	}

	want := "int f(int size) {\n" +
		"\tint x = 1;\n" +
		"\treturn x;\n" +
		"}\n"

	if got != want {
		t.Errorf("XS tabs = %q, want %q", got, want)
	}
}

// TestXS_StructureInvariant checks the published guarantee on every golden
// input: parsing the formatted output yields the same tree as parsing the
// input — same shape without positions (declaration kind, type and name;
// statement kind and expression count; expression kind, value, callee and
// children). Refused inputs (error-severity diagnostics) promise nothing.
// The corpus-wide sweep arrives with the corpus stage of the pipeline.
func TestXS_StructureInvariant(t *testing.T) {
	t.Parallel()

	for _, in := range xsGoldenInputs(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			src := string(raw)

			out, err := XS(src, Options{TabSize: 4})
			if err != nil {
				t.Skipf("refused input: %v", err)
			}

			want, _ := xs.XsParse(src, "")
			got, gotDiags := xs.XsParse(out, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors — a // comment
			// collapsed into a head line shows up here first
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%s) output no longer parses: %s (line %d)", in, d.Message, d.Range.Start.Line+1)
				}
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" {
				t.Errorf("XS(%s) changes the parse:\n%s", in, diff)
			}
		})
	}
}

// xsTreeShape renders the position-free shape of a parsed XS file:
// declarations with their kinds, types, names and parameters, then
// statements and expressions in document order. Positions never enter —
// the printer moves them by construction.
func xsTreeShape(f xs.XsFile) []string {
	out := []string{}

	for _, d := range f.Decls {
		line := fmt.Sprintf("decl %s type=%q name=%q", d.Kind, d.Type, d.Name)

		for _, prm := range d.Params {
			line += fmt.Sprintf(" %s %s", prm.Type, prm.Name)
		}

		out = append(out, line)
		out = append(out, xsStmtsShape(d.Body, 1)...)
	}

	return out
}

// xsStmtsShape renders one statement list at an indent level.
func xsStmtsShape(list []xs.Stmt, level int) []string {
	out := []string{}

	for _, s := range list {
		out = append(out, fmt.Sprintf("%sstmt %s exprs=%d", strings.Repeat("  ", level), s.Kind, len(s.Exprs)))

		for _, e := range s.Exprs {
			out = append(out, xsExprShape(e, level+1)...)
		}

		out = append(out, xsStmtsShape(s.Body, level+1)...)
	}

	return out
}

// xsExprShape renders one expression subtree.
func xsExprShape(e xs.Expr, level int) []string {
	line := fmt.Sprintf("%sexpr %s", strings.Repeat("  ", level), e.Kind)

	if e.Value != "" {
		line += fmt.Sprintf(" value=%q", e.Value)
	}

	if e.Callee != "" {
		line += fmt.Sprintf(" callee=%q", e.Callee)
	}

	out := []string{line}

	for _, c := range e.Children {
		out = append(out, xsExprShape(c, level+1)...)
	}

	return out
}

// TestXS_FixtureDiagnostics ties every fixture to the parser: zero
// error-severity diagnostics, so the golden pass is a printing question
// and never a refusal. The vector case pins its own cause — the warning
// diagnostics the statement slice detection keys on.
func TestXS_FixtureDiagnostics(t *testing.T) {
	t.Parallel()

	for _, in := range xsGoldenInputs(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			_, diags := xs.XsParse(string(raw), "")

			for _, d := range diags {
				if d.Severity == common.SeverityError {
					t.Errorf("xs.XsParse(%s): %s (line %d)", in, d.Message, d.Range.Start.Line+1)
				}
			}
		})
	}

	t.Run("vector-warning-present", func(t *testing.T) {
		t.Parallel()

		raw, err := os.ReadFile(filepath.Join("testdata", "case48.xs"))
		if err != nil {
			t.Fatalf("read input: %v", err)
		}

		_, diags := xs.XsParse(string(raw), "")

		warns := 0

		for _, d := range diags {
			if d.Severity == common.SeverityWarning {
				warns++
			}
		}

		require.Positive(t, warns, "case48 vector literals carry no warning diagnostics: %v", diags)
	})
}

// TestXS_SliceCommentsOnce checks the hybrid-slice comment contract (FR4)
// on every golden input: each comment of the input reaches the output
// exactly once — riding inside a verbatim slice or anchored from the
// global stream, never both and never lost. Texts are cut from the source
// by the parser's own extents; the fixtures keep them distinct and
// single-spaced so a substring hit or a whitespace collapse cannot fake
// the count.
func TestXS_SliceCommentsOnce(t *testing.T) {
	t.Parallel()

	for _, in := range xsGoldenInputs(t) {
		t.Run(filepath.Base(in), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			src := string(raw)

			out, err := XS(src, Options{TabSize: 4})
			if errors.Is(err, ErrParseErrors) {
				t.Skipf("refused input: %v", err)
			}

			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			file, _ := xs.XsParse(src, "")

			for _, r := range file.Comments {
				text := src[r.Start.Offset:r.End.Offset]
				if n := strings.Count(out, text); n != 1 {
					t.Errorf("comment %q from %s appears %d times in the output, want exactly once:\n%s", text, in, n, out)
				}
			}
		})
	}
}

// TestXS_WarnEmptyBody checks the empty-body brace contract on the
// warning statements whose extent ends on a zero-width block (blockEnd
// answers the brace's own start): the verbatim cut must carry the body's
// {} pair — an output that drops it does not reparse. Each row formats,
// reparses its own output and checks the fixed point, the three
// properties the fuzz target pins, plus the pair's presence itself.
func TestXS_WarnEmptyBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			// a wrong-arity vector condition over an empty while body
			name: "while",
			src:  "void f(){while((0,0)){}}",
		},
		{
			// the same vector in a for header, both sections empty
			name: "for",
			src:  "void f(){for((0,0);;){}}",
		},
		{
			// the then pair rides the cut's bytes; the else pair is the
			// one the extent's end leaves outside
			name: "if-else",
			src:  "void f(){if((0,0)){}else{}}",
		},
		{
			// a non-empty then leaves the cut's brackets balanced, so the
			// empty else pair is the extension's to carry — the gate's
			// open edge
			name: "if-else-empty-else",
			src:  "void f(){if((0,0)){1;}else{}}",
		},
		{
			// a non-empty else turns its brace into the closer loop's own
			// closer — the gate's closed edge, the pair must not double
			name: "if-else-empty-then",
			src:  "void f(){if((0,0)){}else{2;}}",
		},
		{
			// an empty block as the body's last statement ends the extent
			// right on its opener: the closer loop must step over the
			// pair whole and still reach the statement's own closing
			// brace — or the cut ends inside the body
			name: "nested-empty-block",
			src:  "void f(){if((0,0)){ {} }}",
		},
		{
			// the same boundary shape with an inner warning if as the
			// last statement: its empty body's pair is the boundary one
			name: "nested-if",
			src:  "void f(){if((0,0)){ if((1,1)){} }}",
		},
		{
			// the while form of the nested-if boundary
			name: "nested-while",
			src:  "void f(){while((0,0)){ while((1,1)){} }}",
		},
		{
			// the for form: the body's last statement ends the extent on
			// its empty body's opener
			name: "nested-for-if",
			src:  "void f(){for((0,0);;){ if((1,1)){} }}",
		},
		{
			// both branches of the boundary statement must survive: the
			// then pair is the boundary one, the else pair rides behind
			// the statement's own closer
			name: "nested-if-else",
			src:  "void f(){if((0,0)){ if((1,1)){}else{} }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if twice != once || !strings.Contains(once, "{}") {
				t.Errorf("XS(%q) = %q (reprint %q): empty body braces lost or unstable", tt.src, once, twice)
			}
		})
	}
}

// TestXS_WarnGroupedVectorStmt checks the grouped wrong-arity vector
// contract: a bare statement whose vector rides inside dropped groupings
// keeps every wrapping "(" in its verbatim cut — a cut opening on the
// warning's own start leaves the group's ")" behind unowned and the
// output stops parsing. Each row formats (the warning is accepted, never
// refused), reparses its own output, checks the fixed point and the
// grouping's presence — the fuzz target's properties plus the bytes the
// AST alone cannot keep.
func TestXS_WarnGroupedVectorStmt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			// the fuzz crasher: one dropped grouping around the vector
			name: "single-group",
			src:  "void A(){((0,0));}",
		},
		{
			// two groupings: the cut must open two parens back
			name: "double-group",
			src:  "void A(){(((0,0)));}",
		},
		{
			// depth is parametric: every layer's "(" is its own to lose
			name: "triple-group",
			src:  "void A(){((((0,0))));}",
		},
		{
			// whitespace inside the body: the gap between the body brace
			// and the grouping still lies in the cut's reach
			name: "space-before-group",
			src:  "void A(){ ((0,0)); }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS refused a warning-only input: %v", err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if twice != once || !strings.Contains(once, "((0,0))") {
				t.Errorf("XS(%q) = %q (reprint %q): outer grouping lost or unstable", tt.src, once, twice)
			}
		})
	}
}

// TestXS_WarnTruncatedOperandOnce pins the hanging-closer walk-back in
// warnCutStart (#170): the warned operand folds to a node inside a
// grouping, so the statement's cut carries the grouping's ")" while its
// "(" sits left of the extent — and the cut's own brackets then net zero,
// the closing hunt saw no depth to close, and pass one printed a hanging
// ")" with the tail operand lost (`0)%(0` from `(0)%(0,0)`), unparseable.
// The cut now opens at the closer's own opener, and warnCutEnd closes the
// depth the widened cut counts.
func TestXS_WarnTruncatedOperandOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// the class: the "%" LHS keeps its grouping's ")",
			// the warned vector RHS loses the tail past the
			// collapsed first operand
			name: "binary with truncated rhs",
			src:  "void A(){(0);(0)%(0,0)}",
			want: "void A() {\n    0;\n    (0)%(0,0)\n}\n",
		},
		{
			// the issue's second member: the warned vector sits
			// inside a grouping of its own
			name: "statement in grouping",
			src:  "void A(){(0) (0%(0,0))}",
			want: "void A() {\n    (0) (0%(0,0))\n}\n",
		},
		{
			// the walk-back is per grouping: two layers, two
			// hanging closers
			name: "nested grouping lhs",
			src:  "void A(){((0))%(0,0)}",
			want: "void A() {\n    ((0))%(0,0)\n}\n",
		},
		{
			// the warned operand's own grouping closes past the
			// collapsed extent
			name: "grouped rhs",
			src:  "void A(){(0)%((0,0))}",
			want: "void A() {\n    (0)%((0,0))\n}\n",
		},
		{
			// a float operand prints its dot byte-exact
			name: "float lhs",
			src:  "void A(){(0.0)%(0,0)}",
			want: "void A() {\n    (0.0)%(0,0)\n}\n",
		},
		{
			// the mirror: the warned vector carries the "%"
			name: "truncated lhs",
			src:  "void A(){(0);(0,0)%(0)}",
			want: "void A() {\n    0;\n    (0,0)%(0)\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			_, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if once != tt.want || twice != once {
				t.Errorf("XS(%q) =\n%q\n(reprint %q)\nwant\n%q", tt.src, once, twice, tt.want)
			}
		})
	}
}

// TestXS_WarnLiteralBodyOnce pins the typed closer pairing in warnCutEnd
// (#163): a block literal tolerates an unclosed "(" in its recovery bytes,
// and the cut hunt used to spend the literal's own "}" on it — the cut ran
// on to the enclosing body's brace, and the printed body grew a stray "}"
// at top level. The closer is now spent on an opener of its own kind; one
// nothing opened of its kind closes the opener the depth counted.
func TestXS_WarnLiteralBodyOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// the class: the "(" is unowned junk inside the
			// literal, the statement's cut stops on the literal's
			// own brace
			name: "unclosed paren in literal",
			src:  "void A(){0.!%{(}}",
			want: "void A() {\n    0.!%{(}\n}\n",
		},
		{
			name: "operand before the paren",
			src:  "void A(){0.!%{(0}}",
			want: "void A() {\n    0.!%{(0}\n}\n",
		},
		{
			// a matched pair past the extent steps whole — the
			// guard the typed pairing must not disturb
			name: "balanced pair in literal",
			src:  "void A(){0.!%{()}}",
			want: "void A() {\n    0.!%{()}\n}\n",
		},
		{
			// the closed form was correct before the fix
			name: "closed literal",
			src:  "void A(){0.!%{}}",
			want: "void A() {\n    0.!%{}\n}\n",
		},
		{
			// the parent prints canonically since #171: a body-grown
			// extent no longer forces the whole-if verbatim, the
			// warn statement's cut rides inside the block (the same
			// discipline as the top-level row)
			name: "nested body",
			src:  "void A(){if(1){0.!%{(}}}",
			want: "void A() {\n    if (1) {\n        0.!%{(}\n    }\n}\n",
		},
		{
			// the declaration-level cut (warnInit) has its own
			// terminator discipline and was never bitten
			name: "declaration-level literal",
			src:  "vector A=0.!%{(}",
			want: "vector A=0.!%{(}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			wantTree, _ := xs.XsParse(tt.src, "")
			gotTree, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(wantTree), xsTreeShape(gotTree)); diff != "" {
				t.Errorf("XS(%q) tree drift:\n%s\noutput:\n%s", tt.src, diff, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_WarnDegenerateTailOnce pins the degenerate-tail literal cut
// (#173): an unclosed "(" inside a CLOSED block literal has no closer
// anywhere, and a typeless residual spent the first closer past the
// extent on it — the enclosing body's own brace, once a degenerate
// member (`.0`) carried the extent past the literal's closer. The cut
// then printed the body's `}` and the body printer printed it again, and
// the output did not reparse (`unexpected "}" at top level`). The
// residual skips closed literals whole (the scanner's own discipline);
// the #163 open-literal shapes are untouched.
func TestXS_WarnDegenerateTailOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// the issue repro: the cut stops before the body's
			// own brace
			name: "degenerate tail",
			src:  "void A(){in%A0%{00000000000(0000}.0}",
			want: "void A() {\n    in%A0%{00000000000(0000}.0\n}\n",
		},
		{
			// the closed literal without the tail prints
			// canonically and always did
			name: "closed literal canonical",
			src:  "void A(){in%A0%{00000000}}",
			want: "void A() {\n    in % A0 % {00000000};\n}\n",
		},
		{
			// the open literal at the extent edge keeps the #163
			// cut: the hunt still spends the literal's own closer
			name: "open literal edge",
			src:  "void A(){0.!%{(}}",
			want: "void A() {\n    0.!%{(}\n}\n",
		},
		{
			// the parent prints canonically since #171 (the same
			// discipline as the #163 pin's row); the compound
			// statement's cut still carries its own body brace
			// after the literal's
			name: "nested body",
			src:  "void A(){if(1){0.!%{(}}}",
			want: "void A() {\n    if (1) {\n        0.!%{(}\n    }\n}\n",
		},
		{
			// the warnInit decl path has its own terminator
			// discipline and was never bitten
			name: "decl literal control",
			src:  "vector A=0.!%{(}",
			want: "vector A=0.!%{(}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			wantTree, _ := xs.XsParse(tt.src, "")
			gotTree, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(wantTree), xsTreeShape(gotTree)); diff != "" {
				t.Errorf("XS(%q) tree drift:\n%s\noutput:\n%s", tt.src, diff, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_WarnInitUnpairedBrace pins the warnInit terminator rule (#151): a
// block literal with an unpaired "(" keeps the bracket scan positive, the
// separator search finds no end, and the fallback cut reaches the extent's
// end — where expectSemi's consumed ";" rides. The cut must not carry it:
// the head synthesizes the declarator's own, and a cut that kept one grew
// the output by a ";" on every pass — fixed point from the third, never
// idempotent, format-on-save growing the file.
func TestXS_WarnInitUnpairedBrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			// the fuzz find: no ';' in the source at all
			name: "unterminated-init",
			src:  "vector A=(0,0)%{(0}",
		},
		{
			// the once-formatted shape: the consumed ';' is in the source
			name: "consumed-terminator",
			src:  "vector A = (0,0)%{(0};",
		},
		{
			// a second declarator past the unbalanced literal: the region
			// ends at the split, the terminator rule must hold there too
			name: "second-declarator",
			src:  "vector A=(0,0)%{(0},B=2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS refused a warning-only input: %v", err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if twice != once {
				t.Errorf("XS(XS(%q)) differs:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}

			if strings.Contains(once, "};;") {
				t.Errorf("XS(%q) synthesized a doubled terminator:\n%s", tt.src, once)
			}

			_, diags := xs.XsParse(once, "")
			warned := false

			for _, d := range diags {
				switch d.Severity {
				case common.SeverityError:
					t.Fatalf("XS(%q) output carries an error: %s", tt.src, d.Message)
				case common.SeverityWarning:
					warned = true
				}
			}

			if !warned {
				t.Errorf("XS(%q) output lost the arity warning", tt.src)
			}
		})
	}
}

// TestXS_ZeroCallChainOnce pins the zero-call chain recovery (#165): the
// parser folds every zero-argument call suffix into its callee — one Call
// node whose extent stops at the callee — and the canonical print spends
// at most one "(" pair past it. A chain of two or more zero suffixes lost
// the extra pairs silently (byte drift, no diagnostic anywhere), and a
// truncation that landed inside an argument list printed an unbalanced
// callee slice that does not reparse. The chain's own bytes print as one
// cut; the single-suffix and argument-capped shapes stay canonical.
func TestXS_ZeroCallChainOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// the grouped repro: one suffix consumed canonically,
			// the second lost — the cut carries both
			name: "grouped chain",
			src:  "vector A=(A)()()",
			want: "vector A = (A)()();\n",
		},
		{
			// the truncation repro: the tail suffix collapsed the
			// node mid-arglist, the cut printed `0(0()` and the
			// output stopped parsing
			name: "truncated arglist",
			src:  "vector A=0(0())()",
			want: "vector A = 0(0())();\n",
		},
		{
			// a plain-ident callee folds the same way: one `()`
			// pair short, no diagnostic
			name: "plain callee chain",
			src:  "vector A=A()()",
			want: "vector A = A()();\n",
		},
		{
			// the truncation can land in an inner call's own
			// arglist
			name: "inner arglist truncation",
			src:  "vector A=f(A())()",
			want: "vector A = f(A())();\n",
		},
		{
			// suffix pairs in a statement body — the statement
			// boundary ends the cut; riding into the next
			// statement's leading "(" duplicated it on every pass
			name: "chain in body",
			src:  "void A(){(A)()()();}",
			want: "void A() {\n    (A)()()();\n}\n",
		},
		{
			// one suffix: the canonical print was never wrong
			name: "single suffix",
			src:  "vector A=f(A)()",
			want: "vector A = f(A)();\n",
		},
		{
			// an argument suffix after the zero one is represented
			// — the canonical print already carried it
			name: "argument suffix",
			src:  "vector A=A()(0)",
			want: "vector A = A()(0);\n",
		},
		{
			// a // comment between the callee and the suffix lifted
			// out of the cut: collapsed inside it, the chain's tail
			// re-read as part of the comment and the shape drifted
			// on the next pass (#177)
			name: "comment before suffix",
			src:  "void A(){00//\n()()}",
			want: "void A() {\n    //\n    00 ()();\n}\n",
		},
		{
			// a block comment between the pairs rides the cut — it
			// closes itself, the chain bytes around it stay one line
			name: "block comment rides",
			src:  "vector A=(0)/*c*/()",
			want: "vector A = (0)/*c*/();\n",
		},
		{
			// a // comment between the pairs lifts above the
			// declaration; the pairs stay on the decl line
			name: "line comment between pairs",
			src:  "vector A=(0)//c\n()",
			want: "//c\nvector A = (0)();\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			wantTree, _ := xs.XsParse(tt.src, "")
			gotTree, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(wantTree), xsTreeShape(gotTree)); diff != "" {
				t.Errorf("XS(%q) tree drift:\n%s\noutput:\n%s", tt.src, diff, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_DegenerateChains checks the degenerate member-name contract: a
// chain whose name after "." came off a non-identifier token has no
// canonical spelling, so the construct prints as a byte-exact slice of its
// own source. The swallowed shape is the crux — a postfix call on the
// chain replaces the nodes with one Call whose callee name is empty, so
// the AST-based gate alone sees nothing and the canonical fallback glues
// the callee cut with synthesized "()" into output that no longer parses.
// The structural-closer rows pin the other half: a name's "(" is not an
// opener, so the verbatim cuts must not spend the body's "}" closing it.
// Each row formats, requires the slice's bytes in the output, reparses
// the output and checks the fixed point — the fuzz target's properties.
func TestXS_DegenerateChains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		slice string
	}{
		{
			// two degenerate names deep, the trailing empty-args call
			// swallowing the chain into its callee; the initializer
			// declaration carries it
			name:  "decl-vector",
			src:   "vector A=(0).(.(()",
			slice: "vector A=(0).(.(()",
		},
		{
			// the same shape on a plain int initializer
			name:  "decl-int",
			src:   "int A=(0).(.(()",
			slice: "int A=(0).(.(()",
		},
		{
			// the extern initializer form of the same chain
			name:  "decl-extern",
			src:   "extern int A=(0).(.(();",
			slice: "extern int A=(0).(.(();",
		},
		{
			// a comma-adjacent declarator group rides one line; the
			// swallowed chain belongs to its last declarator
			name:  "decl-group",
			src:   "int a=1, b=(0).(.(();",
			slice: "int a=1, b=(0).(.(();",
		},
		{
			// the assignment statement form: the statement extent reaches
			// its own ";", the chain bytes stay whole
			name:  "stmt-assign",
			src:   "void f(){x=(0).(.(();}",
			slice: "x=(0).(.(();",
		},
		{
			// the bare statement starts on the operand a dropped grouping
			// wrapped: the cut must open at the grouping's own "(" or its
			// ")" arrives unowned
			name:  "stmt-bare-grouped",
			src:   "void f(){(0).(.(();}",
			slice: "(0).(.(();",
		},
		{
			// no ";" behind the call: the expectSemi recovery leaves the
			// argument pair outside the extent and the cut follows it out
			name:  "stmt-callee-stopped",
			src:   "void f(){x=(0).(.(()}",
			slice: "x=(0).(.(()",
		},
		{
			// a single degenerate name: the pinned neighbor of the
			// swallowed family, now byte-exact like its deeper kin
			name:  "decl-one-name",
			src:   "vector A=(0).(()",
			slice: "vector A=(0).(()",
		},
		{
			// degenerate names nested inside the swallowed call's
			// arguments: the pair matching steps over them
			name:  "nested-names",
			src:   "vector A=(0).((0).(()",
			slice: "vector A=(0).((0).(()",
		},
		{
			// the name's "(" must not count as an opener: the body's
			// closing brace is the block's own, never the slice's
			name:  "stmt-closer-theft",
			src:   "void A(){0.( }",
			slice: "0.(",
		},
		{
			// a warning statement carrying a degenerate name: the warn
			// cut's closer accounting takes the same name discipline
			name:  "warn-chain",
			src:   "void f(){x=(0,0).(()}",
			slice: "x=(0,0).(()",
		},
		{
			// a dot right after a dot is the member name itself (the
			// parser's slot takes the next token verbatim) and the call
			// after it swallows the chain: the mirror reading the second
			// dot as another separator turns the call's "(" into a
			// nobody's name and the output stops parsing
			name:  "decl-dot-dot",
			src:   "vector A=(0)..((0))",
			slice: "vector A=(0)..((0))",
		},
		{
			// the bare-statement form of the dot-dot chain: the cut opens
			// on the grouping's own "(" with the name-dot accounted
			name:  "stmt-dot-dot",
			src:   "void f(){(0)..((0));}",
			slice: "(0)..((0));",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			if !strings.Contains(once, tt.slice) {
				t.Errorf("XS(%q) = %q: the construct's own bytes are not a byte-exact slice of the output", tt.src, once)
			}

			_, diags := xs.XsParse(once, "")
			for _, d := range diags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if twice != once {
				t.Errorf("XS(%q) not idempotent: first pass %q, second pass %q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_HeaderDegenerateScopeOnce pins the header scope of the
// degenerate-name gate (#171): a degenerate member name in the BYTES OF A
// BODY used to pull the whole header+body statement verbatim, and the
// multi-line cut's per-line collapse stranded the interior indentation —
// the for body printed at two levels on the first pass and at one space
// on the second. The gate now scopes to the header, the body prints
// through branch with its own degenerate checks, and every row is a fixed
// point from the first pass. A degenerate in the header itself keeps the
// whole-statement verbatim (single-line cut, own bytes).
func TestXS_HeaderDegenerateScopeOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "for body degenerate",
			src:  "void A(){switch(0%0%0){0%0}for(0%0;0)!{.00}}",
			want: "void A() {\n    switch (0 % 0 % 0) {\n        0 % 0;\n    }\n    for (0 % 0; 0;)\n        !{.00};\n}\n",
		},
		{
			name: "while body degenerate",
			src:  "void A(){while(0)!{.00}}",
			want: "void A() {\n    while (0)\n        !{.00};\n}\n",
		},
		{
			name: "if body degenerate",
			src:  "void A(){if(0)!{.00}}",
			want: "void A() {\n    if (0)\n        !{.00};\n}\n",
		},
		{
			name: "switch case degenerate",
			src:  "void A(){switch(0){0%!{.00}}}",
			want: "void A() {\n    switch (0) {\n        0 % !{.00};\n    }\n}\n",
		},
		{
			name: "typed init body degenerate",
			src:  "void A(){for(int i=0;i<3;)!{.00}}",
			want: "void A() {\n    for (int i = 0; i < 3;)\n        !{.00};\n}\n",
		},
		{
			name: "header degenerate stays whole-statement verbatim",
			src:  "void A(){for(0.!;0)0;}",
			want: "void A() {\n    for(0.!;0)0;\n}\n",
		},
		{
			name: "canonical for",
			src:  "void A(){for(0%0;0)0;}",
			want: "void A() {\n    for (0 % 0; 0;)\n        0;\n}\n",
		},
		{
			name: "canonical typed init",
			src:  "void A(){for(int i=0;i<3;)0;}",
			want: "void A() {\n    for (int i = 0; i < 3;)\n        0;\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			_, gotDiags := xs.XsParse(once, "")
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_ZeroExtentSwitch checks the zero-width-extent switch contract: a
// subject-less switch with an empty body keeps only its keyword in the
// AST — the extent starts and ends on it while the block sits past — and
// a verbatim cut without the block prints an indent-only line the next
// pass drops. Each row pins the block's presence, the reparse and the
// fixed point; the case-label row pins the closer loop's own path, where
// the switch's "{" already counts inside the extent.
func TestXS_ZeroExtentSwitch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		slice string
	}{
		{
			name:  "empty-body",
			src:   "void A(){switch{}}",
			slice: "switch{}",
		},
		{
			// a sibling statement behind the empty block: the cut must
			// stop at the block's own closer
			name:  "empty-body-with-sibling",
			src:   "void f(){switch{}x=1;}",
			slice: "switch{}",
		},
		{
			// a case label ends the extent past the "{", so the closer
			// loop spends the "}" — the shape that already worked
			name:  "case-only-body",
			src:   "void f(){switch{case 1:}}",
			slice: "switch{case 1:}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			if !strings.Contains(once, tt.slice) {
				t.Errorf("XS(%q) = %q: the switch's own bytes are not a byte-exact slice of the output", tt.src, once)
			}

			_, diags := xs.XsParse(once, "")
			for _, d := range diags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if twice != once {
				t.Errorf("XS(%q) not idempotent: first pass %q, second pass %q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_IncludeTrailingCR checks the include line's page-ending rule: a
// lone \r an unterminated literal carries out of its line is the page's
// own artifact, not the string's content — the scanner re-absorbs it into
// the token on the reparse, so the printed line trims it and stays a
// fixed point instead of growing one \r per pass.
func TestXS_IncludeTrailingCR(t *testing.T) {
	t.Parallel()

	tests := []string{
		"include\"\r",
		"include \"a\r",
	}

	for _, src := range tests {
		t.Run(fmt.Sprintf("%q", src), func(t *testing.T) {
			t.Parallel()

			once, err := XS(src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%q", err, once)
			}

			if twice != once {
				t.Errorf("XS(%q) not idempotent: first pass %q, second pass %q", src, once, twice)
			}
		})
	}
}

// TestXS_IncludeTrailingCommentOnce checks the include line's trailing
// comment contract (FR4): a comment spilling past the directive line
// anchors on its own line from the first pass — an unterminated-to-EOF
// block whose "*/" sits in its own opener ("/*/") is still unterminated,
// and the line's cut must stop before it instead of gluing it; a closed
// block is the line's own bytes and rides.
func TestXS_IncludeTrailingCommentOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// #164: the opener's "*/" fooled the closed check and
			// the cut swallowed the comment — pass 1 glued it to the
			// last include, pass 2 anchored it
			name: "unterminated block, two includes",
			src:  `include"";include"";/*/0`,
			want: "include\"\";\n\ninclude\"\";\n/*/0\n",
		},
		{
			name: "unterminated block, one include",
			src:  `include"";/*/0`,
			want: "include\"\";\n/*/0\n",
		},
		{
			// the variable path never cuts past its extent — the
			// stability guard for the same tail
			name: "unterminated block after a variable",
			src:  `int A;/*/0`,
			want: "int A;\n/*/0\n",
		},
		{
			// the opener plus one byte ends on "*/" all the same —
			// the closer's "*" must sit past the opener's
			name: "unterminated opener-plus-one",
			src:  `include""/*/`,
			want: "include\"\"\n/*/\n",
		},
		{
			name: "unterminated block with terminator bytes",
			src:  `include"";/*/0;`,
			want: "include\"\";\n/*/0;\n",
		},
		{
			// a closed block at EOF ends on its "*/": the line's
			// trailing comment, a fixed point as written
			name: "closed block rides the line",
			src:  `include"";/*c*/`,
			want: "include\"\";/*c*/\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			want, _ := xs.XsParse(tt.src, "")
			got, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" {
				t.Errorf("XS(%q) tree drift:\n%s\noutput:\n%s", tt.src, diff, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}

// TestXS_EOLDominance checks the line-ending contract: the dominant EOL of
// the input survives to the output, and exactly one final newline closes
// the document.
func TestXS_EOLDominance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "lf",
			src:  "int one() {\nreturn 1;\n}\nint two() {\nreturn 2;\n}\n",
		},
		{
			name: "crlf",
			src:  "int one() {\r\nreturn 1;\r\n}\r\nint two() {\r\nreturn 2;\r\n}\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			wantEOL := "\n"
			if strings.Contains(tt.src, "\r\n") {
				wantEOL = "\r\n"
			}

			// dominance: in a CRLF document every newline rides a \r\n
			// pair in both directions; an LF document carries no stray
			// carriage returns
			dominant := !strings.Contains(out, "\r")

			if wantEOL == "\r\n" {
				dominant = strings.Count(out, "\n") == strings.Count(out, "\r\n") &&
					strings.Count(out, "\r") == strings.Count(out, "\r\n")
			}

			single := strings.HasSuffix(out, wantEOL) && !strings.HasSuffix(out, wantEOL+wantEOL)

			if !dominant || !single {
				t.Errorf("XS EOL handling:\n--- src ---\n%q\n--- out ---\n%q\n(dominant=%v, single-final=%v)", tt.src, out, dominant, single)
			}
		})
	}
}

// TestXS_ZeroExtentSiblingOnce checks the zero-extent sibling contract: a
// block the parser records with a zero-width extent (blockEnd finds an
// empty body, so the extent ends on the block's own opener) standing right
// behind a statement whose verbatim cut emptyBodyEnd widens through the
// pair must print exactly once — through the cut or through its own
// StmtBlock node, never both. The crasher row is the fuzz finding: every
// pass printed the sibling twice, growing the document by one block. The
// warn-chain rows pin the other half of the ownership walk: the zero-width
// block owning a warning-carrying statement's extent can sit levels down a
// brace-less body chain, and a cut that stops short of it loses its bytes —
// the output must keep the whole chain and stay idempotent. The structural
// assert is the primary discriminator: the doubling reparses without error
// diagnostics and is byte-stable from the second pass on, so the reparse
// check and the fixed point alone accept it — and a {} substring count
// cannot see the synthesized node either, StmtBlock prints its braces on
// separate lines.
func TestXS_ZeroExtentSiblingOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{
			// the fuzz crasher 6481cc9454be3769: the sibling hides
			// behind the while's empty body inside a declaration
			name: "crasher",
			src:  "int A(){{while;{}}}",
		},
		{
			// the pair straight in a function body: the doubling does
			// not need the crasher's extra nesting block (a truly bare
			// top-level `while;{}` is refused before the printer runs —
			// top level takes declarations only)
			name: "body-while",
			src:  "void A(){while;{}}",
		},
		{
			// the ownership walk must descend: a wrong-arity vector in
			// the head of a brace-less body chain leaves the chain's
			// zero-width block levels below the warning-carrying
			// statement, and a cut stopping short of it loses the pair —
			// the output dropped the inner braces and no longer parsed.
			// One warning per row is the parser's own verdict, the
			// shapes stay accepted
			name: "warn-chain-if-while",
			src:  "void A(){if((1,2,3,4))while(c){}}",
		},
		{
			name: "warn-chain-if-else-if",
			src:  "void A(){if((1,2,3,4)){}else if(c){}}",
		},
		{
			name: "warn-chain-if-if",
			src:  "void A(){if((1,2,3,4))if(c){}}",
		},
		{
			name: "warn-chain-while-while",
			src:  "void A(){while((1,2,3,4))while(c){}}",
		},
		{
			name: "warn-chain-for-while",
			src:  "void A(){for(;(1,2,3,4);)while(c){}}",
		},
		{
			name: "warn-chain-if-else-while",
			src:  "void A(){if((1,2,3,4)){}else while(c){}}",
		},
		{
			name: "warn-chain-if-for",
			src:  "void A(){if((1,2,3,4))for(;;){}}",
		},
		{
			name: "warn-chain-do-while",
			src:  "void A(){do while((1,2,3,4)){}}",
		},
		{
			// depth three: the walk crosses two brace-less bodies
			name: "warn-chain-if-while-for",
			src:  "void A(){if((1,2,3,4))while(c)for(;;){}}",
		},
		{
			name: "warn-chain-if-elseif-while",
			src:  "void A(){if((1,2,3,4)){}else if(c)while(d){}}",
		},
		{
			// the do loop's accepted forms pin today's contract: the
			// loop keeps its own closer after the body (`do{}while;`)
			// or ends without one (`do;`, `do{}`) — none of them
			// leaves a zero-extent sibling behind the body's extent
			// for a cut to swallow
			name: "do-while-empty-cond",
			src:  "void A(){do{}while;}",
		},
		{
			name: "do-empty-stmt-body",
			src:  "void A(){do;}",
		},
		{
			name: "do-block-body",
			src:  "void A(){do{}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			want, _ := xs.XsParse(tt.src, "")
			got, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" || twice != once {
				t.Errorf("XS(%q) prints the zero-extent sibling twice or grows:\n%s\n--- once ---\n%q\n--- twice ---\n%q", tt.src, diff, once, twice)
			}
		})
	}

	t.Run("do-with-sibling-refused", func(t *testing.T) {
		t.Parallel()

		// `do;` spends the loop's own `;` on the empty body statement,
		// so the `{` behind it reports the parser error: the formatter
		// cannot accept what the parser rejects — the do-plus-sibling
		// shape refuses instead of printing, and the tie below keeps an
		// implementation that refuses everything from passing
		src := "void A(){do;{}}"

		_, err := XS(src, Options{TabSize: 4})
		if !errors.Is(err, ErrParseErrors) {
			t.Fatalf("XS(%q) error = %v, want ErrParseErrors", src, err)
		}

		_, diags := xs.XsParse(src, "")

		errs := 0

		for _, d := range diags {
			if d.Severity == common.SeverityError {
				errs++
			}
		}

		require.Positive(t, errs, "xs.XsParse(%q) diagnostics = %v, want at least one error", src, diags)
	})
}

// TestXS_DeclAdjacentOnce checks the decl-path sibling contract: a rule
// declaration printed through its verbatim header cut must own only its
// own extent — the cut stops at the rule's closing brace, and the next
// top-level declaration prints once through its own Decl, never both.
// The crasher row is the fuzz finding df7bb1183f5bb08: the junk-string
// header's cut ran through the neighbor rule's opener and body, and the
// neighbor then printed again canonically — every format pass appended
// one more rule block and the document grew without bound. The structural
// assert is the primary discriminator: the duplicate reparses without
// error diagnostics and the document is byte-stable from the second pass
// on, so reparse and fixed point alone accept it. The count pins are
// calibrated on the sibling-once fix shape: the input's two rule keywords
// are the whole budget of the output — the pins were red until the fix
// landed (the doubling pass printed a third, a fresh copy of the neighbor
// per pass); on the fixed tree the output carries exactly two.
func TestXS_DeclAdjacentOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		rules int
	}{
		{
			// the fuzz crasher df7bb1183f5bb08: the neighbor is an empty
			// rule whose body the cut swallows wholesale
			name: "crasher",
			src: `rule """""""""0000000
({}rule {}`,
			rules: 2,
		},
		{
			// the same class with content in the neighbor: the duplicate
			// carries the neighbor's body, so the growth reaches the
			// declaration shape too
			name: "adj5",
			src: `rule """""""""0000000
({}rule R {int x=1;}`,
			rules: 2,
		},
		{
			// the partial-overlap subclass: the junk header's cut used to
			// stop strictly inside the neighbor — on the first closer past
			// its head (`a[0]`'s bracket) — printing the head twice, once
			// inside the cut and once from the neighbor's own node
			name: "partial",
			src: `rule """""""""0000000
({}rule R {int x=a[0];}`,
			rules: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			want, _ := xs.XsParse(tt.src, "")
			got, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" || twice != once {
				t.Errorf("XS(%q) prints the adjacent rule twice or grows:\n%s\n--- once ---\n%q\n--- twice ---\n%q", tt.src, diff, once, twice)
			}

			// calibrated on the sibling-once fix shape: two rule
			// declarations in, two rule keywords out — the doubling pass
			// printed a third
			if n := strings.Count(once, "rule"); n != tt.rules {
				t.Errorf("XS(%q) output carries %d rule declarations, want %d:\n%s", tt.src, n, tt.rules, once)
			}
		})
	}
}

func TestXS_DeclSplitHeadOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// #162: the scanner skips the '#' byte, so the tail
			// declarator's extent starts at its bare name and the
			// gap is not a bare comma — the group test used to leave
			// it standalone and the degenerate cut printed a bare
			// statement. The gap still reduces to the comma: the
			// pair groups and the line is its own fixed point.
			name: "junk gap, degenerate init",
			src:  `void A,#A=0.`,
			want: "void A,#A=0.\n",
		},
		{
			name: "junk gap with terminator",
			src:  `void A,#A=0.;`,
			want: "void A,#A=0.;\n",
		},
		{
			// the name equals the type word: grouping is the only
			// reparseable print (a standalone head would read both
			// words as the type)
			name: "junk gap, name is a type word",
			src:  `void A,#void=1.`,
			want: "void A,#void=1.\n",
		},
		{
			name: "multi-byte junk gap",
			src:  `void A,# @#B=0.;`,
			want: "void A,# @#B=0.;\n",
		},
		{
			// a clean initializer takes the canonical merged print;
			// the invisible junk has no tree to preserve
			name: "junk gap, clean init",
			src:  `int A,#B=1;`,
			want: "int A, B = 1;\n",
		},
		{
			// a comment in the gap must anchor on its own line, so
			// the split declarator prints standalone — the cut
			// carries the synthesized type words ahead of its bytes
			name: "comment gap, typed head",
			src:  `void A,/*c*/B=0.;`,
			want: "void A;\n\n/*c*/\nvoid B=0.;\n",
		},
		{
			name: "comment gap before a group",
			src:  `int A,/*c*/B,C=0.;`,
			want: "int A;\n\n/*c*/\nint B,C=0.;\n",
		},
		{
			name: "comment gap, unterminated literal",
			src:  "int A,/*c*/B=\"x",
			want: "int A;\n\n/*c*/\nint B=\"x\n",
		},
		{
			// junk inside the first declarator's extent rides in the
			// verbatim bytes — the extent opens at the type words
			name: "junk before the first name",
			src:  `void #A=0.;`,
			want: "void #A=0.;\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(tt.src, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS: %v", err)
			}

			want, _ := xs.XsParse(tt.src, "")
			got, gotDiags := xs.XsParse(once, "")

			// the published guarantee: replacing a document with its
			// formatted form never introduces syntax errors
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%q) output no longer parses: %s (line %d), output:\n%s", tt.src, d.Message, d.Range.Start.Line+1, once)
				}
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(output) does not reparse (%v); output:\n%s", err, once)
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" {
				t.Errorf("XS(%q) tree drift:\n%s\noutput:\n%s", tt.src, diff, once)
			}

			if once != tt.want {
				t.Errorf("XS(%q) =\n%q\nwant\n%q", tt.src, once, tt.want)
			}

			if twice != once {
				t.Errorf("XS(%q) not a fixed point:\n--- once ---\n%q\n--- twice ---\n%q", tt.src, once, twice)
			}
		})
	}
}
