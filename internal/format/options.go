// Package format prints RMS sources in the cell's canonical style from
// parsed ASTs: indentation and blank lines are rebuilt, semantics are
// unchanged. Stateless and protocol-independent — no LSP types.
package format

// Options carries the indentation parameters of printing; they are shared
// by the cell's entry points. Construct-and-use data, no methods — and
// the zero value is valid, so callers can pass Options{} when they have
// no preferences of their own.
type Options struct {
	// TabSize is the width of one indentation level in spaces; 0 means
	// the default width of 4.
	TabSize int
	// IndentTabs prints one tab per level instead of TabSize spaces.
	IndentTabs bool
}
