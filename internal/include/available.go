package include

import (
	"aoe2-lsp/internal/xs"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.lsp.dev/uri"
)

// AvailableEntry is one available-but-not-included XS file: a symbol
// source the document does not use yet (the auto-include feature).
type AvailableEntry struct {
	// URI identifies the file.
	URI string

	// Path is the include argument the document should use: relative
	// to the document's directory when possible, absolute otherwise.
	Path string

	// Decls are the file's top-level named declarations (functions,
	// variables, externs).
	Decls []xs.Decl
}

// Available scans the document's directory and the configured roots for
// XS files the document does not include: the completion sources for
// the auto-include feature. Documents with a non-file URI answer nil —
// the scan needs a real directory. The per-file parse goes through the
// resolver's caches (editor state wins, the disk copy is
// stat-fingerprinted), so repeated calls re-read nothing unchanged.
func (r *Resolver) Available(ctx context.Context, uriArg string) []AvailableEntry {
	rootPath := canonicalPath(uriArg)
	if rootPath == "" {
		return nil
	}
	rootDir := filepath.Dir(rootPath)

	included := map[string]bool{}
	for _, e := range r.Closure(ctx, uriArg).Xs {
		included[strings.ToLower(canonicalPath(e.URI))] = true
	}

	// the roots may overlap the document's directory: dedupe by path
	dirs := append([]string{rootDir}, r.rootsSnapshot()...)
	seen := map[string]bool{}

	var entries []AvailableEntry

	for _, dir := range dirs {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, de := range dirEntries {
			name := de.Name()
			if !strings.EqualFold(filepath.Ext(name), ".xs") || de.IsDir() {
				continue
			}

			path := filepath.Join(dir, name)
			key := strings.ToLower(path)
			if seen[key] || key == strings.ToLower(rootPath) || included[key] {
				continue
			}
			seen[key] = true

			fileURI := string(uri.File(path))
			file, ok := r.load(fileURI, path)
			if !ok || file.xs == nil {
				continue
			}

			entries = append(entries, AvailableEntry{
				URI:   fileURI,
				Path:  includeArgument(rootDir, path),
				Decls: namedDecls(file.xs.Decls),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries
}

// includeArgument renders path as an include argument relative to
// dir; an unrelativizable path (another drive, a root) stays as-is.
func includeArgument(dir string, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}

	return filepath.ToSlash(path)
}

// namedDecls keeps the declarations a user could reference from another
// file: named functions, variables and externs.
func namedDecls(decls []xs.Decl) []xs.Decl {
	out := make([]xs.Decl, 0, len(decls))

	for _, d := range decls {
		switch d.Kind {
		case xs.DeclFunction, xs.DeclVariable, xs.DeclExtern:
			if d.Name != "" {
				out = append(out, d)
			}
		}
	}

	return out
}
