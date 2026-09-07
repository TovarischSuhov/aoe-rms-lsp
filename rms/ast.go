// Package rms parses Age of Empires II Random Map Scripts into a
// position-carrying AST with error recovery.
package rms

import (
	"slices"

	"aoe2-lsp/common"
)

// Statement kinds.
const (
	// KindCommand is a plain command statement.
	KindCommand = "command"
	// KindRandom is a start_random/percent_chance block.
	KindRandom = "random"
	// KindConditional is an if/elseif/else branch.
	KindConditional = "conditional"
)

// Expression kinds.
const (
	// KindNumber is an int or float literal.
	KindNumber = "number"
	// KindPercent is a number literal followed by %.
	KindPercent = "percent"
	// KindConst is an ALL-CAPS constant identifier.
	KindConst = "const"
	// KindIdent is any other identifier (helper calls included).
	KindIdent = "ident"
	// KindBinary is a binary operation: Value is the operator.
	KindBinary = "binary"
	// KindUnary is a unary operation: Value is the operator.
	KindUnary = "unary"
)

// RmsFile is the root of the RMS AST.
type RmsFile struct {
	// Name is the file name passed to Parse.
	Name string
	// Sections are the script sections in file order; an implicit
	// "global" section holds statements outside any section.
	Sections []Section
	// Includes are the paths of #include directives.
	Includes []string
	// XsBlocks are the embedded XS blocks started by #includeXS.
	XsBlocks []XsBlock

	// words are all word-token occurrences (section, command and
	// attribute names, identifier/constant values) recorded at parse
	// time; ReferencesAt answers from here.
	words []wordOcc
}

// wordOcc is one word-token occurrence.
type wordOcc struct {
	name string
	at   common.Range
}

// SectionAt returns the section containing pos (for the completion
// context); found=false outside any section.
func (f RmsFile) SectionAt(pos common.Pos) (Section, bool) {
	for _, sec := range f.Sections {
		if sec.Range.Contains(pos) {
			return sec, true
		}
	}

	return Section{}, false
}

// StatementAt returns the closest statement containing pos, or the last
// statement preceding it (for hover). A position on an attribute inside
// a command block returns the owning command.
func (f RmsFile) StatementAt(pos common.Pos) (Statement, bool) {
	var preceding *Statement

	for i := range f.Sections {
		for j := range f.Sections[i].Statements {
			stmt := &f.Sections[i].Statements[j]

			if stmt.Range.Contains(pos) {
				return *deepestAt(stmt, pos), true
			}

			if endsBefore(stmt, pos) {
				preceding = stmt
			}
		}
	}

	if preceding != nil {
		return *preceding, true
	}

	return Statement{}, false
}

// deepestAt descends into children while they contain pos and returns the
// innermost containing statement.
func deepestAt(stmt *Statement, pos common.Pos) *Statement {
	for i := range stmt.Children {
		if stmt.Children[i].Range.Contains(pos) {
			return deepestAt(&stmt.Children[i], pos)
		}
	}

	return stmt
}

// endsBefore reports whether the statement (its whole block) ends before
// pos; used to track the preceding statement.
func endsBefore(stmt *Statement, pos common.Pos) bool {
	return stmt.Range.End.Line < pos.Line
}

// Symbols returns the outline tree of the file (LSP
// textDocument/documentSymbol): section nodes with statement children,
// random/conditional blocks recursing into Children, #includeXS nodes
// placed into their containing section. Statements outside sections —
// the flattened "global" section — become root nodes.
func (f RmsFile) Symbols() []common.Symbol {
	roots := make([]common.Symbol, 0)

	for _, sec := range f.Sections {
		if sec.Name == "global" {
			for _, stmt := range sec.Statements {
				roots = append(roots, f.stmtSymbol(stmt))
			}

			continue
		}

		node := common.Symbol{
			Kind:      "section",
			Name:      sec.Name,
			Range:     sec.Range,
			Selection: f.wordNear(sec.Name, sec.Range.Start, sec.Range),
		}

		for _, stmt := range sec.Statements {
			node.Children = append(node.Children, f.stmtSymbol(stmt))
		}

		for _, block := range f.XsBlocks {
			if sec.Range.Contains(block.Range.Start) {
				node.Children = insertSymbol(node.Children, f.xsBlockSymbol(block))
			}
		}

		roots = append(roots, node)
	}

	for _, block := range f.XsBlocks {
		if !f.inNamedSection(block.Range.Start) {
			roots = insertSymbol(roots, f.xsBlockSymbol(block))
		}
	}

	return roots
}

// stmtSymbol converts one statement into an outline node, recursing
// into random/conditional children.
func (f RmsFile) stmtSymbol(stmt Statement) common.Symbol {
	node := common.Symbol{
		Kind:      "command",
		Name:      stmt.Name,
		Range:     stmt.Range,
		Selection: f.wordNear(stmt.Name, stmt.Range.Start, stmt.Range),
	}

	for _, child := range stmt.Children {
		node.Children = append(node.Children, f.stmtSymbol(child))
	}

	return node
}

// xsBlockSymbol builds the outline node of an embedded XS block.
func (f RmsFile) xsBlockSymbol(block XsBlock) common.Symbol {
	return common.Symbol{
		Kind:      "xs",
		Name:      "#includeXS",
		Range:     block.Range,
		Selection: block.Range,
	}
}

// inNamedSection reports whether at lies inside a named (non-global)
// section span.
func (f RmsFile) inNamedSection(at common.Pos) bool {
	for _, sec := range f.Sections {
		if sec.Name != "global" && sec.Range.Contains(at) {
			return true
		}
	}

	return false
}

// insertSymbol inserts node into the position-ordered node list.
func insertSymbol(nodes []common.Symbol, node common.Symbol) []common.Symbol {
	i := 0

	for i < len(nodes) && !node.Range.Start.Before(nodes[i].Range.Start) {
		i++
	}

	return slices.Insert(nodes, i, node)
}

// wordNear returns the recorded word range of name closest at or after
// start; fallback is returned when no such word exists (recovered
// declarations, structural keywords).
func (f RmsFile) wordNear(name string, start common.Pos, fallback common.Range) common.Range {
	var best common.Range
	found := false

	for _, w := range f.words {
		if w.name != name || w.at.Start.Before(start) {
			continue
		}

		if !found || w.at.Start.Before(best.Start) {
			best = w.at
			found = true
		}
	}

	if !found {
		return fallback
	}

	return best
}

// ReferencesAt returns every occurrence of the word under pos — section,
// command, attribute and identifier words — sorted by position (LSP
// textDocument/references). RMS has no local declarations: all
// occurrences are equal.
func (f RmsFile) ReferencesAt(pos common.Pos) []common.Range {
	var name string
	found := false

	for _, w := range f.words {
		if w.at.Contains(pos) {
			name, found = w.name, true

			break
		}
	}

	if !found {
		return nil
	}

	out := make([]common.Range, 0)

	for _, w := range f.words {
		if w.name == name {
			out = append(out, w.at)
		}
	}

	slices.SortFunc(out, func(a, b common.Range) int {
		switch {
		case a.Start.Before(b.Start):
			return -1
		case b.Start.Before(a.Start):
			return 1
		default:
			return 0
		}
	})

	return out
}

// XsBlock is embedded XS code (after #includeXS) with its source range.
// The rms parser does not interpret the code; consumers pass Code to
// xs.Parse and shift diagnostics by Range.Start.
type XsBlock struct {
	// Code is the raw block text; positions are relative to the block.
	Code string
	// Range is the span of the block in the RMS file.
	Range common.Range
}

// Section is one script section (e.g. land_generation); Name has no angle
// brackets.
type Section struct {
	// Name is the section name.
	Name string
	// Statements are the section's statements in file order.
	Statements []Statement
	// Range is the span from the section header to the next header (or EOF).
	Range common.Range
}

// Statement is a command or block. Positional semantics: Attributes
// belong to the command after which they are written (inside its braces).
// Kind=random nests Children; Kind=conditional covers if/elseif/else.
type Statement struct {
	// Kind is one of command / random / conditional.
	Kind string
	// Name is the command name.
	Name string
	// Args are the positional argument expressions.
	Args []Expr
	// Attributes are the attributes written after the command.
	Attributes []Attribute
	// Children are the nested statements of random/conditional blocks.
	Children []Statement
	// Range is the span of the statement including its block.
	Range common.Range
}

// Attribute is one attribute line inside a command block: a name and its
// (first) value expression. Flag attributes have a zero Value.
type Attribute struct {
	// Name is the attribute name.
	Name string
	// Value is the attribute's value expression.
	Value Expr
	// Range is the span of the attribute line.
	Range common.Range
}

// Expr is a DE math expression tree. Kind=binary: Value is the operator
// and Children are the operands; Kind=const/ident: Value is the name
// (Children hold call arguments for helper calls).
type Expr struct {
	// Kind is one of number / percent / const / ident / binary / unary.
	Kind string
	// Value is the literal, name or operator.
	Value string
	// Children are the operands (or call arguments).
	Children []Expr
	// Range is the span of the expression.
	Range common.Range
}
