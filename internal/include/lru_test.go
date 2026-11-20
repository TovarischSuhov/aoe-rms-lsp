package include

// LRU disk-cache battery (issue #214, spec scenarios 1-5): the entry
// bound holds, recency follows touches and stat replacements, Drop
// stays exact across evicted paths, and eviction never changes an
// answer — a re-read is the only effect. The pins are white-box: each
// test lowers its own resolver's cacheLimit (a per-resolver field, no
// global state) and runs on its own t.TempDir().

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/uri"
)

// newLimitedResolver builds a disk-only resolver whose cache bound is
// lowered white-box: cacheLimit is a per-resolver field exactly so the
// parallel tests never share or mutate global state.
func newLimitedResolver(limit int) *Resolver {
	r := NewResolver(fakeSource{})
	r.cacheLimit = limit

	return r
}

// cacheKeys returns the disk-cache keys (canonical paths) under the
// resolver's lock — the white-box view the bound and membership pins
// assert on. Sorted, so set comparisons are deterministic.
func cacheKeys(t *testing.T, r *Resolver) []string {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]string, 0, len(r.cache))
	for path := range r.cache {
		keys = append(keys, path)
	}

	slices.Sort(keys)

	return keys
}

// lruOrder walks the LRU list front to back (MRU first) under the lock.
func lruOrder(t *testing.T, r *Resolver) []string {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]string, 0, r.lru.Len())
	for e := r.lru.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(string))
	}

	return keys
}

// assertLRUInvariant checks the map<->list invariant (R1.5): the list
// holds exactly the cache's keys, each once — no orphaned nodes, no
// entries without an element.
func assertLRUInvariant(t *testing.T, r *Resolver) {
	t.Helper()

	order := lruOrder(t, r)
	sorted := slices.Clone(order)
	slices.Sort(sorted)

	assert.Equal(t, cacheKeys(t, r), sorted,
		"the LRU list holds exactly the cache's keys, once each (R1.5)")
}

// lruChainTree writes six linked disk files: chain_0.rms includes
// chain_1.rms, ..., chain_5.rms — one closure loads exactly six
// distinct paths, each with a distinct #const so the parsed ASTs
// differ and the equivalence below cannot pass on identical copies.
func lruChainTree(t *testing.T) map[string]string {
	t.Helper()

	tree := make(map[string]string, 6)
	for i := range 6 {
		content := fmt.Sprintf("#const CHAIN_FILE_%d %d\n", i, i)
		if i+1 < 6 {
			content += fmt.Sprintf("#include \"chain_%d.rms\"\n", i+1)
		}

		tree[fmt.Sprintf("chain_%d.rms", i)] = content
	}

	return writeTree(t, t.TempDir(), tree)
}

// TestResolver_LRUEvictionEquivalence pins scenario 1 (R6.1, R6.5): a
// closure over six disk files with cacheLimit = 3 keeps exactly three
// entries — the newest three loaded paths, the oldest evicted — while
// the answer equals the default-limit resolver's bit for bit, and a
// second identical closure re-reads the evicted files and answers the
// same (R5.1: eviction costs latency, never correctness).
func TestResolver_LRUEvictionEquivalence(t *testing.T) {
	t.Parallel()

	uris := lruChainTree(t)
	mainURI := uris["chain_0.rms"]
	ctx := context.Background()

	limited := newLimitedResolver(3)
	defaultLimit := NewResolver(fakeSource{})

	first := limited.Closure(ctx, mainURI)
	want := defaultLimit.Closure(ctx, mainURI)

	// non-vacuity: the chain really loads six files in DFS order and the
	// default resolver really keeps them all — the equivalence is not
	// empty-vs-empty and the bound below is the limit's work
	require.Len(t, first.Rms, 6)
	require.Len(t, first.Resolved, 5)
	require.Empty(t, first.Missing)
	require.Len(t, cacheKeys(t, defaultLimit), 6)

	assert.Equal(t, want, first,
		"the limited closure must equal the default one (equal DFS Rms/Xs, Resolved, Missing)")

	// the bound held: exactly three entries, the newest three loaded
	// paths survive, the oldest (the closure root) is evicted
	require.Len(t, cacheKeys(t, limited), 3, "cacheLimit = 3 keeps exactly three entries (R6.1)")

	for _, name := range []string{"chain_3.rms", "chain_4.rms", "chain_5.rms"} {
		assert.Contains(t, cacheKeys(t, limited), canonicalPath(uris[name]),
			"the (N+1)-th and later loaded paths survive")
	}

	assert.NotContains(t, cacheKeys(t, limited), canonicalPath(mainURI),
		"the oldest loaded path is evicted")
	assert.Equal(t,
		[]string{
			canonicalPath(uris["chain_5.rms"]),
			canonicalPath(uris["chain_4.rms"]),
			canonicalPath(uris["chain_3.rms"]),
		},
		lruOrder(t, limited),
		"inserts enter at MRU (front) and eviction removes the back")

	assertLRUInvariant(t, limited)

	again := limited.Closure(ctx, mainURI)
	assert.Equal(t, first, again,
		"a second identical closure re-reads the evicted files and answers identically (R5.1)")
}

// TestResolver_LRUTouchOrder pins scenario 2 (R6.2): with cacheLimit 3
// and A, B, C loaded in order, re-touching A must promote the cache hit
// to MRU (R1.3), so inserting D evicts B — not A. The re-touch is
// proven to be a hit by a stat-fooling same-size rewrite with the mtime
// wound back (the TestResolver_Drop pattern): a miss would re-read the
// rewritten disk copy and answer with its new declaration name.
func TestResolver_LRUTouchOrder(t *testing.T) {
	t.Parallel()

	uris := writeTree(t, t.TempDir(), map[string]string{
		"a.xs": "void aaa() { }\n",
		"b.xs": "void bbb() { }\n",
		"c.xs": "void ccc() { }\n",
		"d.xs": "void ddd() { }\n",
	})

	aPath := uri.URI(uris["a.xs"]).FsPath()
	st, err := os.Stat(aPath)
	require.NoError(t, err)

	r := newLimitedResolver(3)
	ctx := context.Background()

	// A, B, C in order — A is the LRU tail
	r.Closure(ctx, uris["a.xs"])
	r.Closure(ctx, uris["b.xs"])
	r.Closure(ctx, uris["c.xs"])

	// same-size rewrite with the mtime wound back: the fingerprint
	// cannot notice, so the next load of a.xs is a cache hit that still
	// serves the cached parse
	require.NoError(t, os.WriteFile(aPath, []byte("void zzz() { }\n"), 0o644))
	require.NoError(t, os.Chtimes(aPath, st.ModTime(), st.ModTime()))

	touched := r.Closure(ctx, uris["a.xs"])
	require.Len(t, touched.Xs, 1)
	require.Equal(t, "aaa", touched.Xs[0].File.Decls[0].Name,
		"the re-touch is a hit: the fooled fingerprint keeps the cached parse")

	// inserting D evicts the LRU tail B; A survived at MRU
	r.Closure(ctx, uris["d.xs"])

	keys := cacheKeys(t, r)
	require.Len(t, keys, 3, "cacheLimit = 3 keeps exactly three entries")
	assert.Contains(t, keys, canonicalPath(uris["a.xs"]),
		"the re-touched entry survived (a hit promotes to MRU, R1.3)")
	assert.Contains(t, keys, canonicalPath(uris["c.xs"]))
	assert.Contains(t, keys, canonicalPath(uris["d.xs"]), "the newest insert is cached")
	assert.NotContains(t, keys, canonicalPath(uris["b.xs"]),
		"the untouched middle entry is evicted")
	assert.Equal(t,
		[]string{canonicalPath(uris["d.xs"]), canonicalPath(uris["a.xs"]), canonicalPath(uris["c.xs"])},
		lruOrder(t, r), "MRU order is D, A (re-touched), C")

	assertLRUInvariant(t, r)
}

// TestResolver_LRUReplaceMovesToMRU pins scenario 3 (R6.3): reloading
// a cached file whose stat fingerprint changed replaces the entry and
// re-enters at MRU (R1.4), so the next insert evicts the other neighbor
// — not the replaced file. The rewrite changes the file SIZE, which is
// robust on Windows whose mtime granularity can hide a rewrite: a
// different size alone invalidates the fingerprint.
func TestResolver_LRUReplaceMovesToMRU(t *testing.T) {
	t.Parallel()

	uris := writeTree(t, t.TempDir(), map[string]string{
		"a.xs": "void aaa() { }\n",
		"b.xs": "void bbb() { }\n",
		"c.xs": "void ccc() { }\n",
	})

	r := newLimitedResolver(2)
	ctx := context.Background()

	// A and B cached; A is the LRU tail
	r.Closure(ctx, uris["a.xs"])
	r.Closure(ctx, uris["b.xs"])
	require.Len(t, cacheKeys(t, r), 2)

	// rewrite a.xs with a new size — the fingerprint cannot survive it
	aPath := uri.URI(uris["a.xs"]).FsPath()
	require.NoError(t, os.WriteFile(aPath, []byte("void aaaaaaa() { }\n"), 0o644))

	reloaded := r.Closure(ctx, uris["a.xs"])
	require.Len(t, reloaded.Xs, 1)
	require.Equal(t, "aaaaaaa", reloaded.Xs[0].File.Decls[0].Name,
		"the changed fingerprint forces the re-read; the new content is returned")

	// the replacement re-entered at MRU: inserting C evicts B, not A
	r.Closure(ctx, uris["c.xs"])

	keys := cacheKeys(t, r)
	require.Len(t, keys, 2, "cacheLimit = 2 keeps exactly two entries")
	assert.Contains(t, keys, canonicalPath(uris["a.xs"]),
		"the replaced file survived (a replacement re-enters at MRU, R1.4)")
	assert.Contains(t, keys, canonicalPath(uris["c.xs"]))
	assert.NotContains(t, keys, canonicalPath(uris["b.xs"]),
		"the untouched neighbor is evicted, not the replaced file")
	assert.Equal(t,
		[]string{canonicalPath(uris["c.xs"]), canonicalPath(uris["a.xs"])},
		lruOrder(t, r), "MRU order is C, A (the replacement)")

	// the cache keeps serving the new content on the next load
	again := r.Closure(ctx, uris["a.xs"])
	require.Len(t, again.Xs, 1)
	assert.Equal(t, "aaaaaaa", again.Xs[0].File.Decls[0].Name,
		"the replaced entry's new parse stays cached")

	assertLRUInvariant(t, r)
}

// TestResolver_LRUDropAfterEviction pins scenario 4 (R6.4, R3.1, R3.2):
// Drop on an already-evicted path and on an unknown path is a no-op —
// no panic, no list corruption — and the loads after it answer
// correctly. The map<->list invariant is walked white-box before and
// after the Drop.
func TestResolver_LRUDropAfterEviction(t *testing.T) {
	t.Parallel()

	uris := writeTree(t, t.TempDir(), map[string]string{
		"lib.xs":   "void libFn() { }\n",
		"other.xs": "void otherFn() { }\n",
	})

	r := newLimitedResolver(1)
	ctx := context.Background()

	// lib.xs cached, then evicted by the newer other.xs (limit 1)
	r.Closure(ctx, uris["lib.xs"])
	require.Len(t, cacheKeys(t, r), 1)

	r.Closure(ctx, uris["other.xs"])
	assertLRUInvariant(t, r)
	require.NotContains(t, cacheKeys(t, r), canonicalPath(uris["lib.xs"]),
		"lib.xs was evicted by the newer load")

	// Drop over the evicted path plus an unknown one: a no-op. Drop
	// takes FsPath spellings (cache keys are canonical paths — the
	// TestResolver_Drop pattern)
	r.Drop([]string{
		uri.URI(uris["lib.xs"]).FsPath(),
		uri.File(filepath.Join(filepath.Dir(uri.URI(uris["lib.xs"]).FsPath()), "unknown.xs")).FsPath(),
	})

	require.Len(t, cacheKeys(t, r), 1, "Drop on evicted and unknown paths removes nothing")
	assert.Equal(t, []string{canonicalPath(uris["other.xs"])}, lruOrder(t, r),
		"the surviving entry keeps its list node")
	assertLRUInvariant(t, r)

	// subsequent loads stay correct: the evicted lib re-reads, the
	// survivor keeps answering
	reloaded := r.Closure(ctx, uris["lib.xs"])
	require.Len(t, reloaded.Xs, 1)
	require.Equal(t, "libFn", reloaded.Xs[0].File.Decls[0].Name)

	survivor := r.Closure(ctx, uris["other.xs"])
	require.Len(t, survivor.Xs, 1)
	assert.Equal(t, "otherFn", survivor.Xs[0].File.Decls[0].Name)

	assertLRUInvariant(t, r)
}

// TestResolver_LRUAvailableBound pins scenario 5 (R6.6, R2.1): an
// Available scan over a directory with more .xs files than the lowered
// limit leaves at most cacheLimit disk-cache entries, and the returned
// entries (URI, Path, Decls) equal the default-limit resolver's —
// eviction changes only which files the next scan re-reads.
func TestResolver_LRUAvailableBound(t *testing.T) {
	t.Parallel()

	tree := map[string]string{"main.xs": "void mainFn() { }\n"}
	for i := range 10 {
		tree[fmt.Sprintf("s%02d.xs", i)] = fmt.Sprintf("void fn%02d() { }\nint var%02d = %d;\n", i, i, i)
	}

	uris := writeTree(t, t.TempDir(), tree)
	mainURI := uris["main.xs"]
	ctx := context.Background()

	limited := newLimitedResolver(4)
	defaultLimit := NewResolver(fakeSource{})

	entries := limited.Available(ctx, mainURI)
	want := defaultLimit.Available(ctx, mainURI)

	// non-vacuity: the scan sees all ten siblings with declarations, and
	// the default resolver retains every loaded file (main plus ten) —
	// the bound below is the lowered limit's work
	require.Len(t, entries, 10)
	require.NotEmpty(t, entries[0].Decls)
	require.Len(t, cacheKeys(t, defaultLimit), 11)

	assert.Equal(t, want, entries,
		"Available answers identically under the bound (URI, Path, Decls)")

	assert.LessOrEqual(t, len(cacheKeys(t, limited)), 4,
		"len(r.cache) <= cacheLimit after the scan (R2.1)")
	assertLRUInvariant(t, limited)
}
