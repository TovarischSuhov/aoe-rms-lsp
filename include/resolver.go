package include

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.lsp.dev/uri"

	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
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

	r.expand(ctx, &c, uriArg, 0, visited)

	return c
}

// expand loads uriArg, appends its entry and recurses into its include
// directives. Guards: ctx cancellation, depth, file count and the
// visit-set (cycles terminate, each canonical path expands once).
func (r *Resolver) expand(ctx context.Context, c *Closure, uriArg string, depth int, visited map[string]bool) {
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
		r.expandDirectives(ctx, c, uriArg, path, f.rms, depth, visited)
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
	file *rms.RmsFile,
	depth int,
	visited map[string]bool,
) {
	directives := make([]rms.Include, 0, len(file.Includes)+len(file.XsIncludes))
	directives = append(directives, file.Includes...)
	directives = append(directives, file.XsIncludes...)

	for _, inc := range directives {
		target := filepath.Join(filepath.Dir(ownerPath), inc.Path)

		if _, err := os.Stat(target); err != nil {
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

		r.expand(ctx, c, targetURI, depth+1, visited)
	}
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
		return cached.file, true
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
