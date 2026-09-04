// Package common provides positional types and diagnostics shared by the
// parsers, analysis and server of the aoe2-lsp language server.
package common

// Pos is a position in a source file: zero-based, LSP-aligned coordinates.
//
// Ordering follows (Line, Column); Offset is carried alongside for O(1)
// slicing and does not take part in comparisons.
type Pos struct {
	// Line is the zero-based line number.
	Line uint32
	// Column is the zero-based offset within the line.
	Column uint32
	// Offset is the byte offset from the start of the file.
	Offset int
}

// Before reports whether p lies strictly before other in (Line, Column) order.
func (p Pos) Before(other Pos) bool {
	if p.Line != other.Line {
		return p.Line < other.Line
	}

	return p.Column < other.Column
}

// After reports whether p lies strictly after other in (Line, Column) order.
func (p Pos) After(other Pos) bool {
	return other.Before(p)
}
