package format

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzXS pins the printer's convergence contract the server relies on:
// anything XS accepts must reparse and reprint to the very same bytes —
// a document the formatter mangles on the second pass is a bug no golden
// test enumerates. Refusal (ErrParseErrors) is a legal outcome — the
// properties apply only to accepted input. Panics are the fuzz runner's
// to catch. Seeds: hand-picked edges, every .xs testdata fixture, and
// the real corpus sample under .corpus when it has been fetched.
func FuzzXS(f *testing.F) {
	f.Add("")
	f.Add("\x00\x01 @@ <<<< \x02")
	f.Add("rule 1 {\nif(")

	// reproducers pinned from the fuzzing rounds that drove the
	// printer's bug fixes — a regression on any of these fails the
	// properties below
	f.Add(`include""/*`)
	f.Add(`include "0";include A`)
	f.Add("int A(){\"0\n}")
	f.Add("rule {\"0\n}")
	f.Add(`vector A="0`)
	f.Add(`vector A = (0,0)%{;`)
	f.Add(`void A=0.`)
	f.Add(`int A(){(0)(0)}`)
	f.Add(`rule {("")("")}`)
	f.Add(`(0)(0)()`)
	f.Add(`vector A=(000) (000) (000) (000).0`)
	f.Add(`while{}0`)
	f.Add(`vector A=(000) (000) (000)%(0,0)/*`)
	f.Add(`vector A=(0)(#0)`)
	f.Add(`vector A=(0).(()`)
	f.Add("rule\"0000\n{/**/}")
	f.Add(`vector A=(0,0)(0)`)
	f.Add(`void A(){in%A00%A0000.0;0}`)
	// #163: the block literal's own "}" must not be spent on its
	// tolerated unclosed "(", or the cut eats the enclosing body's
	f.Add(`void A(){0.!%{(}}`)
	// #165: a zero-call chain folds to one callee-stopped node — the
	// cut must carry every suffix pair, and stop before the next ";"
	f.Add(`vector A=0(0())()`)
	f.Add(`void A(){get(1)();(1, 2, 3)();}`)
	f.Add(`vector A=(0(0))`)
	f.Add(`vector A=(0(0));`)
	f.Add(`void f(){while{x}{y}}`)
	f.Add(`vector A=(0.!)`)
	f.Add(`vector A=(0)((0))`)
	f.Add(`void f(){if((0,0)){ {} }}`)
	f.Add(`void f(){if((0,0)){ if((1,1)){} }}`)
	f.Add(`void f(){while((0,0)){ while((1,1)){} }}`)
	f.Add(`void f(){for((0,0);;){ if((1,1)){} }}`)
	f.Add(`void f(){if((0,0)){ if((1,1)){}else{} }}`)
	f.Add(`vector A=(0).(.(()`)
	f.Add(`vector A=(0)..((0))`)
	f.Add(`void A(){0.( }`)
	f.Add(`void A(){((0,0));}`)
	f.Add(`void A(){(((0,0)));}`)
	// the zero-extent sibling crasher 6481cc9454be3769: the block behind
	// the while's empty body must print once; the bare pair is refused at
	// top level — the refusal path must stay panic-free too
	f.Add(`int A(){{while;{}}}`)
	f.Add(`while;{}`)
	// the decl-path sibling crasher df7bb1183f5bb08 and its content
	// variant: a rule header whose verbatim cut overruns its own extent
	// drags the next declaration into the print — one extra rule block
	// per format pass until fixed (TestXS_DeclAdjacentOnce pins the
	// sibling-once shape)
	f.Add(`rule """""""""0000000
({}rule {}`)
	f.Add(`rule """""""""0000000
({}rule R {int x=1;}`)
	// the warnInit terminator crasher (#151): the unpaired "(" in the
	// block literal keeps the separator scan positive, the fallback cut
	// reached expectSemi's ";" and the head added its own — one ";" per
	// pass (TestXS_WarnInitUnpairedBrace pins the fix)
	f.Add(`vector A=(0,0)%{(0}`)
	// the #162 junk-gap split: the scanner skips the unknown byte, the
	// declarator extents split around it, and the gap test used to leave
	// the tail declarator standalone — a bare statement at top level
	// (TestXS_DeclSplitHeadOnce pins the group's own-bytes fixed point)
	f.Add(`void A,#A=0.`)

	seedFuzzXSFixtures(f, "testdata", ".xs")
	seedFuzzXSFixtures(f, filepath.Join("..", "..", ".corpus"), ".xs")

	f.Fuzz(func(t *testing.T, src string) {
		opts := Options{TabSize: 4}

		once, err := XS(src, opts)
		if err != nil {
			// refusal on error-severity input is the documented
			// contract — no idempotence claim to check
			if errors.Is(err, ErrParseErrors) {
				return
			}

			t.Fatalf("XS: non-sentinel error on %q: %v", src, err)
		}

		// an accepted document must survive its own output: the
		// formatted text parses again
		twice, err := XS(once, opts)
		if err != nil {
			t.Fatalf("XS: formatted output does not reparse (%v); first pass of %q:\n%s", err, src, once)
		}

		if twice != once {
			t.Fatalf("XS not idempotent: first pass %q, second pass %q", once, twice)
		}
	})
}

// seedFuzzXSFixtures registers every file under dir with the given
// extension as a fuzz seed; a missing directory (corpus not fetched) is
// not an error.
func seedFuzzXSFixtures(f *testing.F, dir, ext string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		f.Add(string(raw))
	}
}
