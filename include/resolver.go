package include

import (
	"aoe2-lsp/common"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
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
// limit stops the expansion without reporting an error.
const (
	maxDepth = 64
	maxFiles = 1024
)

// Resolver computes the #include/#includeXS closure of a document and
// answers cross-file navigation over it. Editor state (Source) always
// wins over the disk copy; disk files are cached by size and modtime.
type Resolver struct {
	source Source

	mu    sync.Mutex
	cache map[string]*cachedFile
}

// NewResolver builds a resolver over an editor-state source (DI).
func NewResolver(source Source) *Resolver {
	return &Resolver{
		source: source,
		cache:  make(map[string]*cachedFile),
	}
}

// cachedFile is one parsed disk file with its stat fingerprint.
type cachedFile struct {
	file file
	size int64
	mod  time.Time
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
		target := filepath.Join(filepath.Dir(ownerPath), inc.Path)

		if st, err := os.Stat(target); err != nil || !st.Mode().IsRegular() || !withinRoot(rootDir, target) {
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

// withinRoot reports whether target stays inside rootDir; an empty
// rootDir (non-disk root) disables the bound.
func withinRoot(rootDir string, target string) bool {
	if rootDir == "" {
		return true
	}

	rel, err := filepath.Rel(rootDir, target)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// load returns the file for uriArg: the editor-state text when open
// (never cached), otherwise the disk copy through the stat-keyed cache.
func (r *Resolver) load(uriArg string, path string) (file, bool) {
	if text, ok := r.source.Text(uriArg); ok {
		return parseFile(uriArg, text), true
	}

	return r.loadDisk(uriArg, path)
}

// loadDisk reads and caches the disk copy; a stale fingerprint reloads.
func (r *Resolver) loadDisk(uriArg string, path string) (file, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return file{}, false
	}

	r.mu.Lock()
	cached, ok := r.cache[path]
	r.mu.Unlock()

	if ok && cached.size == st.Size() && cached.mod.Equal(st.ModTime()) {
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
	r.cache[path] = &cachedFile{file: f, size: st.Size(), mod: st.ModTime()}
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

		target := filepath.Join(filepath.Dir(path), inc.Path)

		st, err := os.Stat(target)
		if err != nil || !st.Mode().IsRegular() || !withinRoot(filepath.Dir(path), target) {
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
// blocks translated), RMS words via ReferencesAt plus the file text.
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

	ranges := f.rms.ReferencesAt(pos)
	if len(ranges) == 0 {
		return ""
	}

	return f.text[ranges[0].Start.Offset:ranges[0].End.Offset]
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
