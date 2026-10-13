package format

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/xs"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// xsCase is one invariant-sweep input kept beside its bytes: the comment
// and EOL checks need the source again after printing.
type xsCase struct {
	name string
	src  string
}

// xsCorpusMix reads the corpus .xs files and classifies them: the inputs
// XS accepts returned with their bytes, refusals counted. A refusal must
// carry the sentinel — a different error is a broken contract, not a
// refused input, and dies here. An unfetched corpus answers zero of both;
// the callers decide what that means for them.
func xsCorpusMix(t *testing.T) (accepted []xsCase, refused int) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", ".corpus", "*.xs"))
	if err != nil {
		t.Fatalf("glob corpus: %v", err)
	}

	for _, in := range files {
		raw, err := os.ReadFile(in)
		if err != nil {
			t.Fatalf("read %s: %v", in, err)
		}

		src := string(raw)

		if _, err := XS(src, Options{TabSize: 4}); err != nil {
			if !errors.Is(err, ErrParseErrors) {
				t.Fatalf("XS(%s): %v", in, err)
			}

			refused++

			continue
		}

		accepted = append(accepted, xsCase{name: filepath.Base(in), src: src})
	}

	return accepted, refused
}

// xsInvariantInputs gathers one sweep's rows: the golden fixtures first,
// then the accepted corpus files. The mix lands in the log; how balanced
// it is — and whether the corpus side exercises the refusal half of the
// contract at all — is TestXS_CorpusMixSanity's verdict, not a reason to
// drop rows here: with a fixed all-accepted corpus the refusal gate would
// silence the corpus invariants forever. A missing corpus simply
// contributes no rows and the sweep covers the fixtures.
func xsInvariantInputs(t *testing.T) []xsCase {
	t.Helper()

	var rows []xsCase

	for _, in := range xsGoldenInputs(t) {
		raw, err := os.ReadFile(in)
		if err != nil {
			t.Fatalf("read %s: %v", in, err)
		}

		rows = append(rows, xsCase{name: filepath.Base(in), src: string(raw)})
	}

	accepted, refused := xsCorpusMix(t)

	if len(accepted)+refused == 0 {
		t.Logf("corpus not fetched: the sweep covers the fixtures alone")
	} else {
		t.Logf("corpus mix: %d accepted, %d refused", len(accepted), refused)
	}

	return append(rows, accepted...)
}

// TestXS_InvariantStructure pins the contract's structural invariant on
// the goldens and the corpus: parsing the formatted output yields the same
// tree as parsing the input, compared without positions — declaration
// kind, type, name and parameters; statement kind and expression count;
// expression kind, value, callee and children — which is exactly what the
// printer is allowed to move. An accepted input whose output no longer
// parses is the invariant's hardest failure and dies before the shape
// comparison (a // comment collapsed into a head line shows up here
// first). Refused inputs promise nothing and skip.
func TestXS_InvariantStructure(t *testing.T) {
	t.Parallel()

	for _, r := range xsInvariantInputs(t) {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			out, err := XS(r.src, Options{TabSize: 4})
			if errors.Is(err, ErrParseErrors) {
				t.Skipf("refused input: %v", err)
			}

			if err != nil {
				t.Fatalf("XS(%s): %v", r.name, err)
			}

			want, _ := xs.XsParse(r.src, "")
			got, gotDiags := xs.XsParse(out, "")

			// the shape below is recovery noise when the output parses
			// with errors, so it cannot testify about structure
			for _, d := range gotDiags {
				if d.Severity == common.SeverityError {
					t.Fatalf("XS(%s) output no longer parses: %s (line %d)", r.name, d.Message, d.Range.Start.Line+1)
				}
			}

			if diff := linesDiff(xsTreeShape(want), xsTreeShape(got)); diff != "" {
				t.Errorf("XS(%s) changes the parse:\n%s", r.name, diff)
			}
		})
	}
}

// TestXS_InvariantIdempotent pins the fixed point on the goldens and the
// corpus: formatting an already formatted document changes nothing, byte
// for byte. The RMS suite's knownDivergent convergence escape is carried
// over deliberately NOT — an XS divergence is a finding to report, not to
// mask with a third pass.
func TestXS_InvariantIdempotent(t *testing.T) {
	t.Parallel()

	for _, r := range xsInvariantInputs(t) {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			once, err := XS(r.src, Options{TabSize: 4})
			if errors.Is(err, ErrParseErrors) {
				t.Skipf("refused input: %v", err)
			}

			if err != nil {
				t.Fatalf("XS(%s): %v", r.name, err)
			}

			twice, err := XS(once, Options{TabSize: 4})
			if err != nil {
				t.Fatalf("XS(XS(%s)): %v", r.name, err)
			}

			if twice != once {
				t.Errorf("XS(XS(%s)) differs:\n--- once ---\n%q\n--- twice ---\n%q", r.name, once, twice)
			}
		})
	}
}

// TestXS_InvariantCommentsOnce pins the comment-completeness invariant on
// the goldens and the corpus: every comment extent the parser records in
// the input reaches the output byte for byte — riding a verbatim slice or
// anchored from the global stream, never lost and never printed twice.
// Byte counting decides multiplicity only where the source makes the
// text unique: there the output must carry it exactly once, catching loss
// and double anchoring alike; a text the source itself repeats (the bare
// //, twin separator lines) is checked for presence alone — the fixtures
// keep their comments distinct precisely so the strict branch stays
// decidable on them.
func TestXS_InvariantCommentsOnce(t *testing.T) {
	t.Parallel()

	for _, r := range xsInvariantInputs(t) {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			out, err := XS(r.src, Options{TabSize: 4})
			if errors.Is(err, ErrParseErrors) {
				t.Skipf("refused input: %v", err)
			}

			if err != nil {
				t.Fatalf("XS(%s): %v", r.name, err)
			}

			file, _ := xs.XsParse(r.src, "")

			for _, ext := range file.Comments {
				if ext.Start.Offset < 0 || ext.End.Offset < ext.Start.Offset || ext.End.Offset > len(r.src) {
					// an extent off the source is a parser bug; failing this
					// row beats a slice panic taking the whole sweep down
					t.Errorf("comment extent [%d,%d) off the %d-byte source of %s", ext.Start.Offset, ext.End.Offset, len(r.src), r.name)

					continue
				}

				text := r.src[ext.Start.Offset:ext.End.Offset]
				n := strings.Count(out, text)

				switch {
				case strings.Count(r.src, text) == 1 && n != 1:
					t.Errorf("comment %q from %s appears %d times in the output, want exactly once:\n%s", text, r.name, n, out)
				case strings.Count(r.src, text) != 1 && n < 1:
					t.Errorf("comment %q from %s is missing from the output:\n%s", text, r.name, out)
				}
			}
		})
	}
}

// TestXS_InvariantEOLDominance pins the line-ending contract on the
// goldens and the corpus: the input's dominant EOL survives to the output
// — no carriage return in an LF document, no bare \n in a CRLF one. The
// other fixtures and the fetched corpus are LF on disk — case40 already
// is CRLF — so every LF input also runs as a CRLF-converted twin; an
// input the conversion leaves byte-identical gets no twin row, which
// would only duplicate the original.
func TestXS_InvariantEOLDominance(t *testing.T) {
	t.Parallel()

	var rows []xsCase

	for _, r := range xsInvariantInputs(t) {
		rows = append(rows, r)

		twin := strings.ReplaceAll(r.src, "\r\n", "\n")
		twin = strings.ReplaceAll(twin, "\n", "\r\n")

		if twin == r.src {
			continue
		}

		rows = append(rows, xsCase{name: r.name + "-crlf", src: twin})
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			out, err := XS(r.src, Options{TabSize: 4})
			if errors.Is(err, ErrParseErrors) {
				t.Skipf("refused input: %v", err)
			}

			if err != nil {
				t.Fatalf("XS(%s): %v", r.name, err)
			}

			// dominance as one assertion: in an LF document every newline
			// is bare, in a CRLF one every \n and every \r ride a \r\n pair
			kept := !strings.Contains(out, "\r")
			if dominantEOL(r.src) == "\r\n" {
				kept = strings.Count(out, "\n") == strings.Count(out, "\r\n") &&
					strings.Count(out, "\r") == strings.Count(out, "\r\n")
			}

			if !kept {
				t.Errorf("XS(%s) lost the input's dominant EOL %q:\n%q", r.name, dominantEOL(r.src), out)
			}
		})
	}
}

// TestXS_CorpusMixSanity is the meaningfulness check for the corpus side
// of the invariant suite: the fixed corpus must produce both classes —
// inputs the printer accepts and inputs it refuses — for the corpus rows
// to exercise the whole contract. An unbalanced corpus refuses this
// check's own run (skip, logged, never failed — a property of the fetch
// to notice, not a printer bug) while the sweeps above keep running the
// accepted rows: with a fixed all-accepted corpus they are still the
// real-world coverage the fixtures cannot replace. Every refusal has been
// pinned to the sentinel on the way through xsCorpusMix.
func TestXS_CorpusMixSanity(t *testing.T) {
	t.Parallel()

	accepted, refused := xsCorpusMix(t)
	if len(accepted) == 0 && refused == 0 {
		t.Skip("corpus not fetched")
	}

	t.Logf("corpus mix: %d accepted, %d refused of %d", len(accepted), refused, len(accepted)+refused)

	if len(accepted) == 0 || refused == 0 {
		t.Skipf("degenerate corpus mix (%d accepted / %d refused): the corpus sweeps would verify nothing", len(accepted), refused)
	}
}
