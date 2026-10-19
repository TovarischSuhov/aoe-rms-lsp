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
