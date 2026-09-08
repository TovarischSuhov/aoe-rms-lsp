package include

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"aoe2-lsp/internal/xs"
)

// Closure is the result of resolving a document's include closure: the
// parsed files, the resolved directives and the missing ones, in DFS
// directive order.
type Closure struct {
	// Root is the URI of the queried document.
	Root string

	// Rms are the closure's RMS files (the root when it is RMS).
	Rms []RmsEntry

	// Xs are the closure's external XS scripts.
	Xs []XsEntry

	// Resolved lists every successfully resolved directive.
	Resolved []ResolvedInclude

	// Missing lists every directive whose target could not be loaded.
	Missing []MissingInclude
}

// ExternalDecls returns the top-level declarations of the closure's XS
// files for analysis; exclude drops the entry with that URI ("" keeps
// everything). The DFS order is preserved.
func (c Closure) ExternalDecls(exclude string) []xs.Decl {
	var out []xs.Decl

	for _, e := range c.Xs {
		if e.URI == exclude {
			continue
		}

		out = append(out, e.File.Decls...)
	}

	return out
}

// RmsEntry is one RMS file of the closure.
type RmsEntry struct {
	// URI identifies the file.
	URI string

	// File is the parsed AST.
	File rms.RmsFile
}

// XsEntry is one XS file of the closure.
type XsEntry struct {
	// URI identifies the file.
	URI string

	// File is the parsed AST.
	File xs.XsFile
}

// ResolvedInclude is one successfully resolved connection directive.
type ResolvedInclude struct {
	// Owner is the URI of the file containing the directive.
	Owner string

	// Inc is the directive itself.
	Inc rms.Include

	// Target is the URI of the resolved file.
	Target string
}

// MissingInclude is one connection directive whose target does not exist
// or cannot be read.
type MissingInclude struct {
	// Owner is the URI of the file containing the directive.
	Owner string

	// Path is the path as written.
	Path string

	// Range is the span of the path argument in the owner's coordinates.
	Range common.Range
}

// Target is one cross-file navigation point.
type Target struct {
	// URI is the file the point lives in.
	URI string

	// Range is the span within that file.
	Range common.Range
}
