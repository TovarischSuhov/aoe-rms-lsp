package include

// Editor-parse memo battery (issue #211): the memo of open documents'
// parsed files must be unobservable except through Source-call counts —
// cold == warm on every query, a changed text re-parses even without a
// version change, a closed document falls to disk, Drop clears the
// entry, roots never freeze resolution. The white-box asserts use the
// plan's pinned shape (r.memo keyed by canonical path next to r.cache).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/uri"
)

// memoMainLF is the memo fixture's RMS map in its LF form: three include
// directives (an open member, a disk-only member, a missing target), an
// #includeXS library directive, a #const whose name is referenced in a
// value position, a section, and a trailing bare #includeXS opening the
// inline XS block (a local declaration plus a call into the library).
// The open editor texts use the CRLF twin — RMS offsets address the
// normalized text, so every answer must be identical for both spellings.
const memoMainLF = "#include \"parts/econ.rms\"\n" +
	"#include \"parts/disk.rms\"\n" +
	"#include \"parts/gone.rms\"\n" +
	"#includeXS parts/lib.xs\n" +
	"#const MEMO_C 5\n" +
	"<LAND_GENERATION>\n" +
	"create_land\n" +
	"land_percent MEMO_C\n" +
	"#includeXS\n" +
	"void localFn() { }\n" +
	"void main() { sharedFn(1); }\n"

// memoEconLF is the fixture's open RMS member: it references the map's
// #const name in a value position, giving the References probe a
// cross-file occurrence.
const memoEconLF = "base_terrain GRASS\nland_percent MEMO_C\n"

// memoLibSrc is the fixture's open XS library (raw LF — XS is parsed on
// the file's own bytes).
const memoLibSrc = "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"

// memoDiskSrc is the fixture's disk-only RMS member: never opened, so
// every closure loads it through the stat-keyed disk cache.
const memoDiskSrc = "base_terrain GRASS\n"

// memoTree writes the memo battery's fixture tree into a temp dir and
// returns the URIs plus the exact editor texts of the open documents
// (main and econ as the CRLF twins of the LF forms, lib as written).
// parts/gone.rms stays absent — the Missing probe.
func memoTree(t *testing.T) (map[string]string, map[string]string) {
	t.Helper()

	uris := writeTree(t, t.TempDir(), map[string]string{
		"main.rms":       memoMainLF,
		"parts/econ.rms": memoEconLF,
		"parts/lib.xs":   memoLibSrc,
		"parts/disk.rms": memoDiskSrc,
	})

	texts := map[string]string{
		uris["main.rms"]:       strings.ReplaceAll(memoMainLF, "\n", "\r\n"),
		uris["parts/econ.rms"]: strings.ReplaceAll(memoEconLF, "\n", "\r\n"),
		uris["parts/lib.xs"]:   memoLibSrc,
	}

	return uris, texts
}

// aliasedSource serves one document under several URI spellings
// (dot-segment twins of the same path) while URIs() lists the canonical
// keys only — the requester-spelling probe of the R4 pin without
// polluting the reverse-search open set.
type aliasedSource struct {
	base    fakeSource
	aliases map[string]string // spelling → the canonical key of the same document
}

func (s aliasedSource) Text(u string) (string, bool) {
	if canon, ok := s.aliases[u]; ok {
		u = canon
	}

	return s.base.Text(u)
}

func (s aliasedSource) URIs() []string {
	return s.base.URIs()
}

// compile-time check: the aliasing fake satisfies Source.
var _ Source = aliasedSource{}

// countingSource wraps a fakeSource and counts Text/URIs calls — the
// probe counter proving load makes no Source round-trips beyond its
// single Text per loaded document (R1). Not for concurrent use.
type countingSource struct {
	inner  fakeSource
	textBy map[string]int
	uris   int
}

func newCountingSource(inner fakeSource) *countingSource {
	return &countingSource{inner: inner, textBy: make(map[string]int, len(inner))}
}

func (s *countingSource) Text(u string) (string, bool) {
	s.textBy[u]++

	return s.inner.Text(u)
}

func (s *countingSource) URIs() []string {
	s.uris++

	return s.inner.URIs()
}

// textCalls is the number of Text calls seen for one URI so far.
func (s *countingSource) textCalls(u string) int {
	return s.textBy[u]
}

// textTotal is the number of Text calls seen so far over all URIs.
func (s *countingSource) textTotal() int {
	total := 0

	for _, n := range s.textBy {
		total += n
	}

	return total
}

// urisCalls is the number of URIs calls seen so far.
func (s *countingSource) urisCalls() int {
	return s.uris
}

// compile-time check: the counting fake satisfies Source.
var _ Source = (*countingSource)(nil)

// TestResolver_EditorMemoColdEqualsWarm pins the memo equivalence (R1,
// R2, R4, R9): with every closure member open under CRLF editor texts,
// the warm closure/references/rename answers equal the cold run's
// pairwise — DFS order, Resolved/Missing content and the sorted Targets
// included. The dual-spelling sub-run keeps the requester's URI: a warm
// entry must be served under the current query's spelling (R4), and a
// shared entry mutated by a twin query must not leak into the next one.
func TestResolver_EditorMemoColdEqualsWarm(t *testing.T) {
	t.Parallel()

	uris, texts := memoTree(t)

	r := NewResolver(fakeSource(texts))
	ctx := context.Background()

	// the CRLF editor text of the map: probe positions address its lines
	mainCRLF := strings.ReplaceAll(memoMainLF, "\n", "\r\n")
	posConst := linePos(mainCRLF, 7, "MEMO_C", 0) // the value use of the #const name

	coldClosure := r.Closure(ctx, uris["main.rms"])
	coldLib := r.Closure(ctx, uris["parts/lib.xs"])
	coldRefs := r.References(ctx, uris["main.rms"], posConst)
	coldSites, coldFound := r.RenameSites(ctx, uris["main.rms"], posConst)

	// non-vacuity guards: the equivalence must not pass empty-vs-empty
	require.Len(t, coldClosure.Rms, 3, "cold baseline: main, econ and the disk member in DFS order")
	require.Equal(t, uris["main.rms"], coldClosure.Rms[0].URI)
	require.Equal(t, uris["parts/econ.rms"], coldClosure.Rms[1].URI)
	require.Equal(t, uris["parts/disk.rms"], coldClosure.Rms[2].URI)
	require.Len(t, coldClosure.Xs, 1)
	require.Equal(t, uris["parts/lib.xs"], coldClosure.Xs[0].URI)
	require.Len(t, coldClosure.Resolved, 3, "cold baseline: econ, disk and lib resolve; gone is missing")
	require.Len(t, coldClosure.Missing, 1)
	require.Equal(t, "parts/gone.rms", coldClosure.Missing[0].Path)
	require.Len(t, coldLib.Xs, 1, "cold baseline: the open lib is its own closure")
	require.Len(t, coldRefs, 3, "cold baseline: the #const name answers in main (decl + use) and econ")
	require.True(t, slicesSortedTargets(coldRefs), "cold baseline: references are sorted")
	require.True(t, coldFound, "cold baseline: the declared name is renameable")
	require.Len(t, coldSites, 3, "cold baseline: main's decl + use and econ's use")

	// the warm pass over the same resolver: every load is a memo hit
	warmClosure := r.Closure(ctx, uris["main.rms"])
	warmLib := r.Closure(ctx, uris["parts/lib.xs"])
	warmRefs := r.References(ctx, uris["main.rms"], posConst)
	warmSites, warmFound := r.RenameSites(ctx, uris["main.rms"], posConst)

	assert.Equal(t, coldClosure, warmClosure, "warm closure equals the cold run (deep, DFS order included)")
	assert.Equal(t, coldLib, warmLib, "warm lib closure equals the cold run")
	assert.Equal(t, coldRefs, warmRefs, "warm references equal the cold run (sorted targets included)")
	assert.True(t, warmFound)
	assert.Equal(t, coldSites, warmSites, "warm rename sites equal the cold run")

	t.Run("dual spelling keeps the requester's URI", func(t *testing.T) {
		t.Parallel()

		uris, texts := memoTree(t)

		dotted := strings.Replace(uris["main.rms"], "/main.rms", "/parts/../main.rms", 1)
		require.NotEqual(t, uris["main.rms"], dotted)
		require.Equal(t, canonicalPath(uris["main.rms"]), canonicalPath(dotted),
			"the twin spells the same document")

		src := aliasedSource{base: fakeSource(texts), aliases: map[string]string{dotted: uris["main.rms"]}}
		r := NewResolver(src)
		ctx := context.Background()

		mainCRLF := strings.ReplaceAll(memoMainLF, "\n", "\r\n")
		posLocal := linePos(mainCRLF, 9, "localFn", 0) // the inline block's own declaration

		// warm the memo through the canonical spelling
		warm := r.Closure(ctx, uris["main.rms"])
		require.Len(t, warm.Rms, 3)

		defPlain, found := r.Definition(ctx, uris["main.rms"], posLocal)
		require.True(t, found, "the inline block declaration resolves")
		require.Equal(t, uris["main.rms"], defPlain.URI, "the plain query is served under its own spelling")

		// the same warm memo under the dot-segment spelling
		dottedClosure := r.Closure(ctx, dotted)
		require.Len(t, dottedClosure.Rms, 3)
		assert.Equal(t, dotted, dottedClosure.Root)
		assert.Equal(t, dotted, dottedClosure.Rms[0].URI,
			"the root entry carries the requesting spelling, not the first requester's")
		assert.Equal(t, uris["parts/econ.rms"], dottedClosure.Rms[1].URI,
			"non-root members resolve through their canonical paths")

		defDotted, found := r.Definition(ctx, dotted, posLocal)
		require.True(t, found, "the inline block declaration resolves under the twin spelling")
		assert.Equal(t, dotted, defDotted.URI,
			"the inline-block answer carries the requesting spelling (R4: the memo entry's URI is rewritten)")

		// and the canonical spelling keeps answering under itself — a
		// shared entry mutated by the dotted query would leak the twin
		defAgain, found := r.Definition(ctx, uris["main.rms"], posLocal)
		require.True(t, found)
		assert.Equal(t, uris["main.rms"], defAgain.URI,
			"the shared memo entry stays unmutated by the twin query")
	})
}

// slicesSortedTargets reports whether targets are sorted by (URI, start
// position) — the References/RenameSites output contract.
func slicesSortedTargets(targets []Target) bool {
	for i := 1; i < len(targets); i++ {
		if targets[i-1].URI > targets[i].URI {
			return false
		}

		if targets[i-1].URI == targets[i].URI &&
			targets[i-1].Range.Start.After(targets[i].Range.Start) {
			return false
		}
	}

	return true
}

// TestResolver_EditorMemoReparseOnChange pins the text validation (R2):
// Source carries no version, so the memo must compare the bytes — a
// changed text under the same key re-parses, the same text stays
// stable, and reverting the text restores the original answer.
func TestResolver_EditorMemoReparseOnChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		"main.rms": "#include \"member.rms\"\n",
		"old.rms":  "base_terrain GRASS\n",
		"new.rms":  "base_terrain DIRT\n",
	})

	src := fakeSource{uris["main.rms"]: "#include \"old.rms\"\n"}
	r := NewResolver(src)
	ctx := context.Background()

	cold := r.Closure(ctx, uris["main.rms"])
	require.Len(t, cold.Resolved, 1)
	require.Equal(t, uris["old.rms"], cold.Resolved[0].Target, "the cold run includes old.rms")

	// the text changes under the same key and without a version — the
	// classic stale-AST hazard the full-text validation exists for
	src[uris["main.rms"]] = "#include \"new.rms\"\n"

	changed := r.Closure(ctx, uris["main.rms"])
	require.Len(t, changed.Resolved, 1)
	assert.Equal(t, uris["new.rms"], changed.Resolved[0].Target,
		"a changed text re-parses despite the warm memo")

	again := r.Closure(ctx, uris["main.rms"])
	assert.Equal(t, changed, again, "the same text keeps the answer stable")

	src[uris["main.rms"]] = "#include \"old.rms\"\n"

	reverted := r.Closure(ctx, uris["main.rms"])
	assert.Equal(t, cold, reverted, "reverting the text restores the original closure")
}

// TestResolver_EditorMemoNoExtraSourceCalls pins the Source budget (R1):
// load consults Source exactly once per loaded document — one Text per
// member, URIs only from the reverse-search loops. A memo validated by
// a second Text/URIs probe, or checked before Text (stealing the
// editor-priority decision), changes these counts.
func TestResolver_EditorMemoNoExtraSourceCalls(t *testing.T) {
	t.Parallel()

	uris, texts := memoTree(t)

	src := newCountingSource(fakeSource(texts))
	r := NewResolver(src)
	ctx := context.Background()

	mainCRLF := strings.ReplaceAll(memoMainLF, "\n", "\r\n")
	posConst := linePos(mainCRLF, 7, "MEMO_C", 0)

	// the cold closure: one Text per loaded document, no URIs
	r.Closure(ctx, uris["main.rms"])
	coldText, coldURIs := src.textTotal(), src.urisCalls()

	require.Equal(t, 4, coldText, "cold closure: one Text per member (main, econ, lib and the disk miss)")
	require.Equal(t, 0, coldURIs, "a closure never lists the open set")
	for _, u := range []string{uris["main.rms"], uris["parts/econ.rms"], uris["parts/lib.xs"], uris["parts/disk.rms"]} {
		require.Equal(t, 1, src.textCalls(u), "cold closure: exactly one Text per document")
	}

	// the warm closure: same budget — the memo hit must not re-probe
	r.Closure(ctx, uris["main.rms"])
	warmText, warmURIs := src.textTotal()-coldText, src.urisCalls()-coldURIs

	assert.Equal(t, coldText, warmText, "a warm closure makes no more Text calls than the cold one")
	assert.Equal(t, coldURIs, warmURIs, "a warm closure makes no URIs calls")

	// references: the reverse search lists the open set exactly once,
	// cold and warm alike
	r.References(ctx, uris["main.rms"], posConst)
	refsText, refsURIs := src.textTotal()-coldText-warmText, src.urisCalls()-coldURIs-warmURIs

	require.Equal(t, 1, refsURIs, "references list the open set exactly once")

	beforeText, beforeURIs := src.textTotal(), src.urisCalls()
	r.References(ctx, uris["main.rms"], posConst)
	warmRefsText, warmRefsURIs := src.textTotal()-beforeText, src.urisCalls()-beforeURIs

	assert.Equal(t, refsText, warmRefsText, "a warm references run makes no more Text calls")
	assert.Equal(t, refsURIs, warmRefsURIs, "a warm references run still lists the open set exactly once")
}

// TestResolver_EditorMemoCloseFallsToDisk pins the close transition
// (R3): the memo is consulted only on the editor path — once
// Source.Text misses, the member reads the disk copy even though its
// editor-state entry is still warm.
func TestResolver_EditorMemoCloseFallsToDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uris := writeTree(t, dir, map[string]string{
		"main.rms": "#includeXS lib.xs\n",
		"lib.xs":   "void diskFn() { }\n",
	})

	src := fakeSource{
		uris["main.rms"]: "#includeXS lib.xs\n",
		uris["lib.xs"]:   "void editorFn() { }\n",
	}
	r := NewResolver(src)
	ctx := context.Background()

	warm := r.Closure(ctx, uris["main.rms"])
	require.Len(t, warm.Xs, 1)
	require.Equal(t, "editorFn", warm.Xs[0].File.Decls[0].Name,
		"the open lib answers with the editor text")

	// the document closes: Source.Text misses from now on while the
	// editor memo entry is still warm
	delete(src, uris["lib.xs"])

	closed := r.Closure(ctx, uris["main.rms"])
	require.Len(t, closed.Xs, 1)
	assert.Equal(t, "diskFn", closed.Xs[0].File.Decls[0].Name,
		"a closed member reads the disk copy, never the editor memo")

	repeat := r.Closure(ctx, uris["main.rms"])
	assert.Equal(t, closed, repeat, "the closed member keeps reading the disk copy")
}

// TestResolver_EditorMemoDrop pins Drop over both caches (R5, D3):
// the watched-files path (FsPath spelling, no Clean) clears the editor
// memo entry and the disk cache entry of each dropped path, unknown
// paths stay a no-op, other documents keep their entries — and the
// dropped disk member re-reads the same-size mtime-fooling rewrite the
// stat fingerprint would otherwise serve stale.
func TestResolver_EditorMemoDrop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	libPath := filepath.Join(dir, "lib.xs")
	require.NoError(t, os.WriteFile(libPath, []byte("void aaaa() {}\n"), 0o644))

	st, err := os.Stat(libPath)
	require.NoError(t, err)

	// lib.xs is written by hand (the same-size rewrite below needs its
	// path); its URI comes from the same conversion writeTree uses
	libURI := uri.File(libPath).String()

	uris := writeTree(t, dir, map[string]string{
		"main.rms": "#include \"a.rms\"\n#includeXS lib.xs\n",
		"a.rms":    "#include \"old.rms\"\n",
		"old.rms":  "base_terrain GRASS\n",
		"new.rms":  "base_terrain DIRT\n",
	})

	src := fakeSource{
		uris["main.rms"]: "#include \"a.rms\"\n#includeXS lib.xs\n",
		uris["a.rms"]:    "#include \"old.rms\"\n",
	}
	r := NewResolver(src)
	ctx := context.Background()

	warm := r.Closure(ctx, uris["main.rms"])
	require.Len(t, warm.Rms, 3, "main, the open a.rms and the disk old.rms join the closure")
	require.Len(t, warm.Xs, 1)
	require.Equal(t, "aaaa", warm.Xs[0].File.Decls[0].Name)

	// both caches are warm after one pass
	canonical := func(u string) string { return canonicalPath(u) }

	_, ok := r.memo[canonical(uris["main.rms"])]
	require.True(t, ok, "the editor memo holds the main entry after the warm pass")
	_, ok = r.memo[canonical(uris["a.rms"])]
	require.True(t, ok, "the editor memo holds the editor member's entry")
	_, ok = r.cache[canonical(libURI)]
	require.True(t, ok, "the disk cache holds the disk member's entry")

	// Drop by watched-files paths: the editor member, the disk member
	// and an unknown path in one call
	r.Drop([]string{
		uri.URI(uris["a.rms"]).FsPath(),
		uri.URI(libURI).FsPath(),
		uri.File(filepath.Join(dir, "unknown.xs")).FsPath(),
	})

	_, ok = r.memo[canonical(uris["a.rms"])]
	require.False(t, ok, "Drop clears the editor memo of the dropped path")
	_, ok = r.cache[canonical(libURI)]
	require.False(t, ok, "Drop clears the disk cache of the dropped path")
	_, ok = r.memo[canonical(uris["main.rms"])]
	require.True(t, ok, "the undropped document keeps its memo entry (unknown paths are a no-op)")

	// an unknown-only Drop stays a no-op
	r.Drop([]string{uri.File(filepath.Join(dir, "unknown.xs")).FsPath()})
	_, ok = r.memo[canonical(uris["main.rms"])]
	require.True(t, ok, "an unknown path alone drops nothing")

	// swap the texts: the editor member's text changes and the disk
	// member is rewritten same-size with the mtime wound back — the
	// stat fingerprint would keep serving the stale copy without Drop
	src[uris["a.rms"]] = "#include \"new.rms\"\n"
	require.NoError(t, os.WriteFile(libPath, []byte("void bbbb() {}\n"), 0o644))
	require.NoError(t, os.Chtimes(libPath, st.ModTime(), st.ModTime()))

	after := r.Closure(ctx, uris["main.rms"])
	require.Len(t, after.Rms, 3)
	require.Len(t, after.Resolved, 3)
	assert.Equal(t, uris["new.rms"], after.Resolved[1].Target,
		"the swapped editor member's closure reflects the new text")
	require.Len(t, after.Xs, 1)
	assert.Equal(t, "bbbb", after.Xs[0].File.Decls[0].Name,
		"Drop forces the disk re-read past the fooled fingerprint")
}

// TestResolver_EditorMemoSetRootsKeepsResolution pins the SetRoots half
// of R5/R8: the parse memo stays valid across SetRoots — roots steer
// target resolution, not parsing — and a warm-memo closure re-resolves
// its directives through the new roots instead of freezing the first
// root's answer.
func TestResolver_EditorMemoSetRootsKeepsResolution(t *testing.T) {
	t.Parallel()

	mapDir := t.TempDir()
	first := t.TempDir()
	second := t.TempDir()

	uris := writeTree(t, mapDir, map[string]string{
		"main.rms": "#include \"shared.rms\"\n",
	})
	writeTree(t, first, map[string]string{"shared.rms": "base_terrain GRASS\n"})
	writeTree(t, second, map[string]string{"shared.rms": "base_terrain DIRT\n"})

	src := fakeSource{uris["main.rms"]: "#include \"shared.rms\"\n"}
	r := NewResolver(src)
	ctx := context.Background()

	// warm the parse memo before any root is configured
	preWarm := r.Closure(ctx, uris["main.rms"])
	require.Len(t, preWarm.Missing, 1, "without roots the directive stays missing")
	_, ok := r.memo[canonicalPath(uris["main.rms"])]
	require.True(t, ok, "the warm closure fills the editor memo")

	r.SetRoots([]string{first})
	_, ok = r.memo[canonicalPath(uris["main.rms"])]
	require.True(t, ok, "SetRoots leaves the editor memo in place")

	warm := r.Closure(ctx, uris["main.rms"])
	require.Len(t, warm.Resolved, 1)
	require.Contains(t, warm.Resolved[0].Target, uri.File(first).String(),
		"the first root resolves the directive")

	r.SetRoots([]string{second})
	_, ok = r.memo[canonicalPath(uris["main.rms"])]
	require.True(t, ok, "SetRoots still leaves the editor memo in place on replacement")

	moved := r.Closure(ctx, uris["main.rms"])
	require.Len(t, moved.Resolved, 1)
	assert.Contains(t, moved.Resolved[0].Target, uri.File(second).String(),
		"the warm-memo closure re-resolves through the new roots")

	r.SetRoots(nil)
	_, ok = r.memo[canonicalPath(uris["main.rms"])]
	require.True(t, ok, "SetRoots(nil) keeps the parse memo as well")

	cleared := r.Closure(ctx, uris["main.rms"])
	require.Len(t, cleared.Missing, 1, "clearing the roots returns the directive to Missing")
}

// TestResolver_EditorMemoConcurrent pins the shared-resolver safety
// (R6): K goroutines hammer Closure/References over intersecting roots
// on the warm resolver and every answer equals the sequential baseline
// (the data-race side is pinned by the -race run of this battery).
func TestResolver_EditorMemoConcurrent(t *testing.T) {
	t.Parallel()

	uris, texts := memoTree(t)

	r := NewResolver(fakeSource(texts))
	ctx := context.Background()

	posFn := linePos(memoLibSrc, 0, "sharedFn", 0) // the library's declaration

	// the sequential baselines — also the warm-up pass
	wantMain := r.Closure(ctx, uris["main.rms"])
	wantLib := r.Closure(ctx, uris["parts/lib.xs"])
	wantRefs := r.References(ctx, uris["parts/lib.xs"], posFn)

	require.Len(t, wantMain.Rms, 3)
	require.Len(t, wantLib.Xs, 1)
	require.NotEmpty(t, wantRefs, "the reverse search answers across the open includer")

	const workers = 8
	const iters = 25

	errs := make(chan string, workers)

	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range iters {
				if got := r.Closure(ctx, uris["main.rms"]); !assert.ObjectsAreEqual(wantMain, got) {
					errs <- "Closure(main) differs from the sequential baseline"

					return
				}

				if got := r.Closure(ctx, uris["parts/lib.xs"]); !assert.ObjectsAreEqual(wantLib, got) {
					errs <- "Closure(lib) differs from the sequential baseline"

					return
				}

				if got := r.References(ctx, uris["parts/lib.xs"], posFn); !assert.ObjectsAreEqual(wantRefs, got) {
					errs <- "References(lib) differs from the sequential baseline"

					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for msg := range errs {
		t.Error(msg)
	}
}

// TestResolver_EditorMemoSharedAcrossRoots pins the per-file memo
// (R8): one open library joins the closures of two maps from different
// directories — different closure rootDirs — and both answers carry the
// structurally identical lib member: the parse never depended on the
// asking root, so the shared memo serves both without changing either.
func TestResolver_EditorMemoSharedAcrossRoots(t *testing.T) {
	t.Parallel()

	libDir := t.TempDir()
	dirA := t.TempDir()
	dirB := t.TempDir()

	libText := "void libFn(int n) { }\nint libVar = 1;\nvoid user() { libFn(libVar); }\n"
	uris := writeTree(t, libDir, map[string]string{"lib.xs": libText})
	urisA := writeTree(t, dirA, map[string]string{"mapA.rms": "#includeXS lib.xs\n"})
	urisB := writeTree(t, dirB, map[string]string{"mapB.rms": "#includeXS lib.xs\n#const B_ONLY 1\n"})

	src := fakeSource{
		uris["lib.xs"]:    libText,
		urisA["mapA.rms"]: "#includeXS lib.xs\n",
		urisB["mapB.rms"]: "#includeXS lib.xs\n#const B_ONLY 1\n",
	}
	r := NewResolver(src)
	r.SetRoots([]string{libDir})
	ctx := context.Background()

	cA := r.Closure(ctx, urisA["mapA.rms"])
	cB := r.Closure(ctx, urisB["mapB.rms"])

	require.Len(t, cA.Xs, 1)
	require.Len(t, cB.Xs, 1)
	assert.Equal(t, uris["lib.xs"], cA.Xs[0].URI, "mapA's closure resolves the shared lib")
	assert.Equal(t, uris["lib.xs"], cB.Xs[0].URI, "mapB's closure resolves the same shared lib")
	assert.Equal(t, cA.Xs[0].File, cB.Xs[0].File,
		"the shared lib member is structurally identical for both roots (the parse is rootDir-free)")

	require.Len(t, cA.Rms, 1)
	require.Len(t, cB.Rms, 1)
	assert.Equal(t, urisA["mapA.rms"], cA.Rms[0].URI, "each closure keeps its own map as the RMS entry")
	assert.Equal(t, urisB["mapB.rms"], cB.Rms[0].URI)
	assert.NotEqual(t, cA.Rms[0].File, cB.Rms[0].File, "the two maps' own entries stay distinct")
}
