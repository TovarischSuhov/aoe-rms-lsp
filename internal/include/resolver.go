package include

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
	"container/list"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.lsp.dev/uri"
)

// Depth and file-count limits guard against include bombs; reaching a
// limit stops the expansion without reporting an error. maxCacheEntries
// bounds the disk-cache memory instead: an evicted file costs a re-read,
// never a wrong answer.
const (
	maxDepth        = 64
	maxFiles        = 1024
	maxCacheEntries = 512
)

// Resolver computes the #include/#include_drs/#includeXS closure of a
// document and answers cross-file navigation over it. Editor state (Source)
// always wins over the disk copy; disk files are cached by size and modtime
// under an LRU entry bound (eviction trades a re-read, never correctness),
// and open documents' parses are memoized by their canonical path (keyed by
// byte-identical text, so an edit always re-parses).
type Resolver struct {
	source Source

	roots []string

	mu         sync.Mutex
	cache      map[string]*cachedFile
	lru        list.List // front = MRU; values are cache keys (string)
	memo       map[string]*file
	cacheLimit int // set to maxCacheEntries by NewResolver; same-package tests may lower it
}

// NewResolver builds a resolver over an editor-state source (DI).
func NewResolver(source Source) *Resolver {
	return &Resolver{
		source:     source,
		cache:      make(map[string]*cachedFile),
		memo:       make(map[string]*file),
		cacheLimit: maxCacheEntries,
	}
}

// cachedFile is one parsed disk file with its stat fingerprint.
type cachedFile struct {
	file file
	size int64
	mod  time.Time
	elem *list.Element // valid iff the entry is in r.cache (the map<->list invariant)
}

// file is one loaded closure member: its text and parsed AST (exactly one
// of rms/xs is set, by extension).
type file struct {
	uri  string
	text string
	rms  *rms.RmsFile
	xs   *xs.XsFile
}

// Closure resolves the include closure of uri in DFS directive order.
// A root that is neither open nor readable yields an empty closure —
// degradation, not an error.
func (r *Resolver) Closure(ctx context.Context, uriArg string) Closure {
	c := Closure{Root: uriArg}
	visited := make(map[string]bool)

	rootDir := ""

	if path := canonicalPath(uriArg); path != "" {
		rootDir = filepath.Dir(path)
	}

	r.expand(ctx, &c, uriArg, rootDir, 0, visited)

	return c
}

// expand loads uriArg, appends its entry and recurses into its include
// directives. Guards: ctx cancellation, depth, file count and the
// visit-set (cycles terminate, each canonical path expands once).
func (r *Resolver) expand(
	ctx context.Context,
	c *Closure,
	uriArg string,
	rootDir string,
	depth int,
	visited map[string]bool,
) {
	if ctx.Err() != nil || depth > maxDepth || len(c.Rms)+len(c.Xs) >= maxFiles {
		return
	}

	path := canonicalPath(uriArg)
	if path == "" || visited[path] {
		return
	}

	f, ok := r.load(uriArg, path)
	if !ok {
		return
	}

	visited[path] = true

	switch {
	case f.rms != nil:
		c.Rms = append(c.Rms, RmsEntry{URI: uriArg, File: *f.rms})
		r.expandDirectives(ctx, c, uriArg, path, rootDir, f.rms, depth, visited)
	default:
		c.Xs = append(c.Xs, XsEntry{URI: uriArg, File: *f.xs})
	}
}

// expandDirectives resolves every include directive of one RMS file:
// existing targets become ResolvedInclude and recurse; missing ones
// become MissingInclude (one per directive, duplicates kept).
func (r *Resolver) expandDirectives(
	ctx context.Context,
	c *Closure,
	ownerURI string,
	ownerPath string,
	rootDir string,
	file *rms.RmsFile,
	depth int,
	visited map[string]bool,
) {
	directives := make([]rms.Include, 0, len(file.Includes)+len(file.XsIncludes))
	directives = append(directives, file.Includes...)
	directives = append(directives, file.XsIncludes...)

	for _, inc := range directives {
		target, found := r.resolveTarget(ownerPath, inc.Path, rootDir)

		if !found {
			c.Missing = append(c.Missing, MissingInclude{
				Owner: ownerURI,
				Path:  inc.Path,
				Range: inc.Range,
			})

			continue
		}

		targetURI := string(uri.File(target))
		c.Resolved = append(c.Resolved, ResolvedInclude{
			Owner:  ownerURI,
			Inc:    inc,
			Target: targetURI,
		})

		r.expand(ctx, c, targetURI, rootDir, depth+1, visited)
	}
}

// resolveTarget picks the first existing include target: the owner's
// directory first, then the configured roots in priority order. A
// target must stay inside the closure root's directory or one of the
// configured roots (the ../-escape guard).
func (r *Resolver) resolveTarget(ownerPath string, incPath string, rootDir string) (string, bool) {
	roots := r.rootsSnapshot()

	candidates := make([]string, 0, len(roots)+1)
	candidates = append(candidates, filepath.Join(filepath.Dir(ownerPath), incPath))

	for _, root := range roots {
		candidates = append(candidates, filepath.Join(root, incPath))
	}

	for _, target := range candidates {
		if st, err := os.Stat(target); err != nil || !st.Mode().IsRegular() {
			continue
		}

		if withinRoot(rootDir, target) {
			return target, true
		}

		if slices.ContainsFunc(roots, func(root string) bool { return withinRoot(root, target) }) {
			return target, true
		}
	}

	return "", false
}

// SetRoots replaces the extra include search roots; later closures
// resolve through the new set (the stat-keyed disk cache stays valid).
func (r *Resolver) SetRoots(roots []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roots = slices.Clone(roots)
}

// rootsSnapshot copies the configured roots under the lock.
func (r *Resolver) rootsSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.roots)
}

// Drop force-invalidates the disk-cache and editor-memo entries for the
// given paths; later closures re-read these files regardless of their stat
// fingerprint. Unknown paths are a no-op.
func (r *Resolver) Drop(paths []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, path := range paths {
		// both halves of the entry go together: leaving the list node
		// would let an unrelated eviction remove it twice
		if old, ok := r.cache[path]; ok {
			delete(r.cache, path)
			r.lru.Remove(old.elem)
		}

		delete(r.memo, path)
	}
}

// withinRoot reports whether target stays inside rootDir; an empty
// rootDir (non-disk root) disables the bound. The escape check goes
// through filepath.IsLocal so the platform separator is honored: on
// Windows filepath.Rel yields `..\`-prefixed relatives, which a
// literal `../` prefix test lets through.
func withinRoot(rootDir string, target string) bool {
	if rootDir == "" {
		return true
	}

	rel, err := filepath.Rel(rootDir, target)

	return err == nil && filepath.IsLocal(rel)
}

// load returns the file for uriArg: the editor-state text when open
// (memoized by text, keyed by the canonical path), otherwise the disk
// copy through the stat-keyed cache.
func (r *Resolver) load(uriArg string, path string) (file, bool) {
	if text, ok := r.source.Text(uriArg); ok {
		return r.loadEditor(uriArg, path, text)
	}

	return r.loadDisk(uriArg, path)
}

// loadEditor serves the editor-state text through the parse memo keyed
// by the canonical path: a hit with a byte-identical text returns the
// memoized AST (a light copy — the URI spelling is the current query's),
// a different text re-parses and replaces the single entry of the path.
func (r *Resolver) loadEditor(uriArg string, path string, text string) (file, bool) {
	r.mu.Lock()
	m, ok := r.memo[path]
	r.mu.Unlock()

	if ok && m.text == text {
		copied := *m
		copied.uri = uriArg

		return copied, true
	}

	f := parseFile(uriArg, text)

	r.mu.Lock()
	r.memo[path] = &f
	r.mu.Unlock()

	return f, true
}

// loadDisk reads and caches the disk copy; a stale fingerprint reloads.
func (r *Resolver) loadDisk(uriArg string, path string) (file, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return file{}, false
	}

	r.mu.Lock()
	cached, ok := r.cache[path]
	hit := ok && cached.size == st.Size() && cached.mod.Equal(st.ModTime())
	if hit {
		// touched under the lock that validated the fingerprint: after an
		// unlock a parallel insert could evict this entry before an
		// unlocked MoveToFront and leave cached.elem dangling
		r.lru.MoveToFront(cached.elem)
	}
	r.mu.Unlock()

	if hit {
		// a light copy: the ASTs are read-only, the URI spelling is the
		// current query's — never the first requester's
		copied := cached.file
		copied.uri = uriArg

		return copied, true
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return file{}, false
	}

	f := parseFile(uriArg, string(raw))

	r.mu.Lock()
	// a fingerprint replacement re-enters at MRU: the stale element must
	// be unlinked first, or the list keeps a node the map no longer knows
	if old, ok := r.cache[path]; ok {
		r.lru.Remove(old.elem)
	}

	entry := &cachedFile{file: f, size: st.Size(), mod: st.ModTime()}
	entry.elem = r.lru.PushFront(path)
	r.cache[path] = entry

	// the bound holds before unlock; the inserted entry itself can only
	// be evicted when cacheLimit < 1 (white-box tests) — the caller owns
	// its copy either way
	for len(r.cache) > r.cacheLimit {
		back := r.lru.Back()
		key := back.Value.(string)
		delete(r.cache, key)
		r.lru.Remove(back)
	}
	r.mu.Unlock()

	return f, true
}

// parseFile parses text by extension: .xs via XsParse, everything else
// (text includes are RMS snippets of any extension) via rms.Parse.
// Parser diagnostics are dropped — the closure needs structure only.
func parseFile(uriArg string, text string) file {
	f := file{uri: uriArg, text: text}

	if strings.HasSuffix(strings.ToLower(uriArg), ".xs") {
		parsed, _ := xs.XsParse(text, uriArg)
		f.xs = &parsed

		return f
	}

	parsed, _ := rms.Parse(text, uriArg)
	f.rms = &parsed

	return f
}

// canonicalPath maps a URI to its cleaned absolute filesystem path;
// non-file schemes yield "" (unresolvable on disk).
func canonicalPath(uriArg string) string {
	u := uri.URI(uriArg)
	if !u.IsFile() {
		return ""
	}

	return filepath.Clean(u.FsPath())
}

// Definition resolves the cross-file definition under pos: an include
// path argument jumps to the target file start; an XS identifier resolves
// locally first, then through the closure's declarations. Builtins and
// unknown names yield found=false.
func (r *Resolver) Definition(
	ctx context.Context,
	uriArg string,
	pos common.Pos,
) (Target, bool) {
	if ctx.Err() != nil {
		return Target{}, false
	}

	path := canonicalPath(uriArg)
	if path == "" {
		return Target{}, false
	}

	f, ok := r.load(uriArg, path)
	if !ok {
		return Target{}, false
	}

	if f.rms != nil {
		if t, found := r.definitionInRms(f, path, pos); found {
			return t, true
		}
	} else if rr, found := f.xs.Definition(pos); found {
		return Target{URI: uriArg, Range: rr}, true
	}

	// closure-wide declaration lookup by the name under pos
	name := r.symbolNameAt(f, pos)
	if name == "" {
		return Target{}, false
	}

	for _, e := range r.Closure(ctx, uriArg).Xs {
		for _, sym := range e.File.Symbols() {
			if sym.Name == name {
				return Target{URI: e.URI, Range: sym.Selection}, true
			}
		}
	}

	return Target{}, false
}

// definitionInRms answers Definition inside an RMS file: the include
// directive hit-test, then the inline XS blocks.
func (r *Resolver) definitionInRms(f file, path string, pos common.Pos) (Target, bool) {
	directives := make([]rms.Include, 0, len(f.rms.Includes)+len(f.rms.XsIncludes))
	directives = append(directives, f.rms.Includes...)
	directives = append(directives, f.rms.XsIncludes...)

	for _, inc := range directives {
		if !inc.Range.Contains(pos) {
			continue
		}

		// definition jumps through the same resolution the closure
		// diagnostics used — a click answers exactly when the directive
		// is Resolved (issue #210)
		target, found := r.resolveTarget(path, inc.Path, filepath.Dir(path))
		if !found {
			return Target{}, false
		}

		return Target{URI: string(uri.File(target)), Range: common.Range{}}, true
	}

	for _, block := range f.rms.XsBlocks {
		if !block.Range.Contains(pos) {
			continue
		}

		xsFile, _ := xs.XsParse(block.Code, f.uri)

		if rr, found := xsFile.Definition(unshiftPos(pos, block.Range.Start)); found {
			return Target{URI: f.uri, Range: shiftRange(rr, block.Range.Start)}, true
		}

		return Target{}, false
	}

	return Target{}, false
}

// References returns every occurrence of the name under pos across the
// closure of the queried document plus the closures of open documents
// that include it (reverse direction). The declaration occurrence is
// included; the result is deduplicated and sorted by (URI, position).
func (r *Resolver) References(ctx context.Context, uriArg string, pos common.Pos) []Target {
	out := make([]Target, 0)

	if ctx.Err() != nil {
		return out
	}

	path := canonicalPath(uriArg)
	if path == "" {
		return out
	}

	f, ok := r.load(uriArg, path)
	if !ok {
		return out
	}

	name := r.nameAt(f, pos)
	if name == "" {
		return out
	}

	closures := []Closure{r.Closure(ctx, uriArg)}

	for _, u := range r.source.URIs() {
		if u == uriArg {
			continue
		}

		cl := r.Closure(ctx, u)

		if closureHas(cl, uriArg) {
			closures = append(closures, cl)
		}
	}

	seen := make(map[targetKey]bool)

	for _, c := range closures {
		for _, e := range c.Rms {
			r.addReferences(seen, &out, e.URI, e.File.References(name))

			for _, block := range e.File.XsBlocks {
				xsFile, _ := xs.XsParse(block.Code, e.URI)

				r.addShifted(seen, &out, e.URI, xsFile.References(name), block.Range.Start)
			}
		}

		for _, e := range c.Xs {
			r.addReferences(seen, &out, e.URI, e.File.References(name))
		}
	}

	slices.SortFunc(out, func(a, b Target) int {
		switch {
		case a.URI != b.URI:
			return strings.Compare(a.URI, b.URI)
		case a.Range.Start.Before(b.Range.Start):
			return -1
		case b.Range.Start.Before(a.Range.Start):
			return 1
		default:
			return 0
		}
	})

	return out
}

// RenameSites answers the rename sites of the binding under pos across
// the closure of the queried document plus the closures of open
// documents that include it (the reverse direction) — the merge the
// rename request needs. Scoped bindings (XS params/locals) are
// file-local: their answer stays inside the queried file. found is
// false when the position is not renameable at all (not an identifier,
// a builtin, the RMS vocabulary); there is no fallback guess. The
// result is deduplicated and sorted by (URI, position).
func (r *Resolver) RenameSites(ctx context.Context, uriArg string, pos common.Pos) ([]Target, bool) {
	if ctx.Err() != nil {
		return nil, false
	}

	path := canonicalPath(uriArg)
	if path == "" {
		return nil, false
	}

	f, ok := r.load(uriArg, path)
	if !ok {
		return nil, false
	}

	local, found := r.localRenameSites(f, pos)
	if !found {
		return nil, false
	}

	// The binding name is the site text under pos — every foreign file
	// of the merge is keyed on it alone.
	name := ""
	for _, rr := range local.sites {
		if rr.Contains(pos) {
			name = siteText(f, rr)
			break
		}
	}

	if name == "" {
		return nil, false
	}

	seen := make(map[targetKey]bool, len(local.sites))
	out := make([]Target, 0, len(local.sites))

	for _, rr := range local.sites {
		key := targetKey{uri: uriArg, r: rr}
		if seen[key] {
			continue
		}

		seen[key] = true
		out = append(out, Target{URI: uriArg, Range: rr})
	}

	// Scoped bindings never leave their file — the cross-file merge is
	// top-level only.
	scoped := local.kind == xs.KindParam || local.kind == xs.KindLocal

	// A top-level variable with an initializer answers with the
	// file-local kind when queried at its own declaration token (the
	// per-file resolver treats the initializer statement as a nested
	// scope); the binding itself is top-level — its declaration site
	// anchoring on the answering file's top-level symbols sees through
	// that per-file quirk.
	if scoped && xsDeclTopLevel(local.xsFile, name, local.xsSites) {
		scoped = false
	}

	if !scoped {
		for _, c := range r.renameRoots(ctx, uriArg) {
			for _, e := range c.Rms {
				r.addReferences(seen, &out, e.URI, e.File.RenameRefs(name))

				for _, block := range e.File.XsBlocks {
					xsFile, _ := xs.XsParse(block.Code, e.URI)

					r.addShifted(seen, &out, e.URI, xsRenameRefs(xsFile, name), block.Range.Start)
				}
			}

			for _, e := range c.Xs {
				r.addReferences(seen, &out, e.URI, xsRenameRefs(e.File, name))
			}
		}
	}

	slices.SortFunc(out, func(a, b Target) int {
		switch {
		case a.URI != b.URI:
			return strings.Compare(a.URI, b.URI)
		case a.Range.Start.Before(b.Range.Start):
			return -1
		case b.Range.Start.Before(a.Range.Start):
			return 1
		default:
			return 0
		}
	})

	return out, true
}

// localRename is the queried file's own half of the rename answer.
type localRename struct {
	// sites are the binding's sites in file coordinates — the local
	// part of the merged answer.
	sites []common.Range
	// kind is the binding's scope kind; "" for plain RMS positions (the
	// RMS vocabulary has no scoped bindings, every renameable RMS name
	// is top-level).
	kind string
	// xsFile is the XS file the binding answered from — the .xs document
	// itself or the parsed inline block; nil for plain RMS positions.
	xsFile *xs.XsFile
	// xsSites are the binding's sites in xsFile's own coordinates
	// (block-relative for inline blocks) — the anchor input of the
	// top-level guard.
	xsSites []common.Range
}

// localRenameSites resolves the binding under pos in the queried file
// itself. Inline XS blocks answer through the XS parser in block
// coordinates, translated back into file coordinates.
func (r *Resolver) localRenameSites(f file, pos common.Pos) (local localRename, found bool) {
	if f.rms != nil {
		for _, block := range f.rms.XsBlocks {
			if !block.Range.Contains(pos) {
				continue
			}

			xsFile, _ := xs.XsParse(block.Code, f.uri)
			blockSites, ok := xsFile.RenameSites(unshiftPos(pos, block.Range.Start))
			if !ok {
				return localRename{}, false
			}

			return localRename{
				sites:   shiftedSites(blockSites, block.Range.Start),
				kind:    blockSites[0].Kind,
				xsFile:  &xsFile,
				xsSites: siteRanges(blockSites),
			}, true
		}

		rmsSites, ok := f.rms.RenameSites(pos)
		if !ok {
			return localRename{}, false
		}

		return localRename{sites: rmsSites}, true
	}

	xsSites, ok := f.xs.RenameSites(pos)
	if !ok {
		return localRename{}, false
	}

	return localRename{
		sites:   siteRanges(xsSites),
		kind:    xsSites[0].Kind,
		xsFile:  f.xs,
		xsSites: siteRanges(xsSites),
	}, true
}

// siteText returns the name text of the site at rr in f. The RMS parser
// (and every inline block inside it) counts offsets after CRLF
// normalization, while XS files are parsed on the raw bytes and the
// editor state is raw — each answer is sliced against the text its own
// offsets were counted on.
func siteText(f file, rr common.Range) string {
	text := f.text
	if f.rms != nil {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}

	return text[rr.Start.Offset:rr.End.Offset]
}

// renameRoots collects the search roots exactly like References: the
// queried file's own closure plus the closures of open documents that
// include it (the reverse direction — "who includes me").
func (r *Resolver) renameRoots(ctx context.Context, uriArg string) []Closure {
	closures := []Closure{r.Closure(ctx, uriArg)}

	for _, u := range r.source.URIs() {
		if u == uriArg {
			continue
		}

		cl := r.Closure(ctx, u)

		if closureHas(cl, uriArg) {
			closures = append(closures, cl)
		}
	}

	return closures
}

// xsDeclTopLevel reports whether one of the binding's sites is the
// selection of a same-named top-level symbol of the file — the anchor
// test separating a genuinely scoped binding (a param or a block local
// is never a top-level symbol) from the initializer quirk of the
// per-file rename resolver.
func xsDeclTopLevel(xf *xs.XsFile, name string, sites []common.Range) bool {
	if xf == nil {
		return false
	}

	for _, sym := range xf.Symbols() {
		if sym.Name == name && slices.Contains(sites, sym.Selection) {
			return true
		}
	}

	return false
}

// xsRenameRefs classifies the name occurrences of a foreign XS file for
// a top-level rename: an occurrence without a local binding is an
// external reference to the closure's binding — the occurrence itself
// is included; an occurrence of a same-name top-level binding pulls in
// all of that binding's sites (name-based merge); occurrences of scoped
// bindings are skipped (they shadow the name in their scope only).
func xsRenameRefs(xf xs.XsFile, name string) []common.Range {
	var out []common.Range

	for _, occ := range xf.References(name) {
		sites, found := xf.RenameSites(occ.Start)
		if !found {
			out = append(out, occ)

			continue
		}

		if sites[0].Kind == xs.KindParam || sites[0].Kind == xs.KindLocal {
			// The initializer quirk reaches foreign files too: a
			// top-level declaration token answers with the file-local
			// kind when it is the queried occurrence. The same anchor
			// test rescues the declaration — without it a same-name
			// top-level binding without uses would silently drop out of
			// the merge.
			if !xsDeclTopLevel(&xf, name, siteRanges(sites)) {
				continue
			}
		}

		out = append(out, siteRanges(sites)...)
	}

	return out
}

// siteRanges projects rename sites onto their plain ranges.
func siteRanges(sites []xs.RenameSite) []common.Range {
	out := make([]common.Range, 0, len(sites))

	for _, s := range sites {
		out = append(out, s.Range)
	}

	return out
}

// shiftedSites maps block-relative rename sites into file coordinates.
func shiftedSites(sites []xs.RenameSite, base common.Pos) []common.Range {
	out := make([]common.Range, 0, len(sites))

	for _, rr := range siteRanges(sites) {
		out = append(out, shiftRange(rr, base))
	}

	return out
}

// targetKey deduplicates occurrences by (URI, Range).
type targetKey struct {
	uri string
	r   common.Range
}

// addReferences appends occurrences unless already seen.
func (r *Resolver) addReferences(seen map[targetKey]bool, out *[]Target, uriArg string, ranges []common.Range) {
	for _, rr := range ranges {
		key := targetKey{uri: uriArg, r: rr}
		if seen[key] {
			continue
		}

		seen[key] = true
		*out = append(*out, Target{URI: uriArg, Range: rr})
	}
}

// addShifted appends block-relative occurrences translated into file
// coordinates (mirrors the server's shiftPos semantics).
func (r *Resolver) addShifted(
	seen map[targetKey]bool,
	out *[]Target,
	uriArg string,
	ranges []common.Range,
	base common.Pos,
) {
	for _, rr := range ranges {
		shifted := shiftRange(rr, base)

		key := targetKey{uri: uriArg, r: shifted}
		if seen[key] {
			continue
		}

		seen[key] = true
		*out = append(*out, Target{URI: uriArg, Range: shifted})
	}
}

// closureHas reports whether target is among the closure's entries
// (canonical path comparison).
func closureHas(c Closure, target string) bool {
	tgt := canonicalPath(target)
	if tgt == "" {
		return false
	}

	for _, e := range c.Rms {
		if canonicalPath(e.URI) == tgt {
			return true
		}
	}

	for _, e := range c.Xs {
		if canonicalPath(e.URI) == tgt {
			return true
		}
	}

	return false
}

// nameAt resolves the name under pos: XS identifiers via SymbolAt (inline
// blocks translated), RMS words via the recorded word-index name (WordAt) —
// RMS offsets address the CRLF-normalized text, so the raw file text is
// never sliced with them (issue #204).
func (r *Resolver) nameAt(f file, pos common.Pos) string {
	if f.xs != nil {
		name, _ := f.xs.SymbolAt(pos)

		return name
	}

	for _, block := range f.rms.XsBlocks {
		if !block.Range.Contains(pos) {
			continue
		}

		xsFile, _ := xs.XsParse(block.Code, f.uri)
		name, _ := xsFile.SymbolAt(unshiftPos(pos, block.Range.Start))

		return name
	}

	name, _ := f.rms.WordAt(pos)

	return name
}

// symbolNameAt resolves the identifier name under pos for the closure-wide
// declaration lookup (XS files and inline blocks only).
func (r *Resolver) symbolNameAt(f file, pos common.Pos) string {
	return r.nameAt(f, pos)
}

// shiftRange translates a block-relative range into file coordinates.
func shiftRange(rr common.Range, base common.Pos) common.Range {
	return common.Range{Start: shiftPos(rr.Start, base), End: shiftPos(rr.End, base)}
}

// shiftPos maps a block-relative position into file coordinates; only the
// first block line also gains the start column.
func shiftPos(p common.Pos, base common.Pos) common.Pos {
	shifted := common.Pos{
		Line:   p.Line + base.Line,
		Column: p.Column,
		Offset: p.Offset + base.Offset,
	}

	if p.Line == 0 {
		shifted.Column += base.Column
	}

	return shifted
}

// unshiftPos maps a file position into block coordinates (the inverse of
// shiftPos); the caller guarantees p lies within the block.
func unshiftPos(p common.Pos, base common.Pos) common.Pos {
	unshifted := common.Pos{
		Line:   p.Line - base.Line,
		Column: p.Column,
		Offset: p.Offset - base.Offset,
	}

	if p.Line == base.Line {
		unshifted.Column -= base.Column
	}

	return unshifted
}
