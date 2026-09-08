package complete

import (
	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
	"strings"
)

// Candidate kinds — the complete cell's protocol-agnostic dictionary.
const (
	// KindCommand is an RMS command name from the knowledge base.
	KindCommand = "command"
	// KindAttribute is an attribute of the owning RMS command.
	KindAttribute = "attribute"
	// KindConstant is an XS/RMS constant from the knowledge base.
	KindConstant = "constant"
	// KindFunction is an XS function (kb builtin or source declaration).
	KindFunction = "function"
	// KindVariable is a source variable declaration.
	KindVariable = "variable"
	// KindParam is a parameter of the enclosing XS function.
	KindParam = "param"
	// KindLocal is a local variable of the enclosing XS block.
	KindLocal = "local"
)

// Sort groups fix the candidate ordering: context attributes first,
// then commands, then constants; the label orders inside a group.
const (
	sortAttr    = "0"
	sortCommand = "1"
	sortConst   = "2"
)

// XS sort groups: source symbols first, then kb functions and kb
// constants; the label orders inside a group.
const (
	sortSource     = "0"
	sortKbFunction = "1"
	sortKbConstant = "2"
)

// Completer computes completion candidates over parsed ASTs and the
// knowledge base: stateless, protocol-agnostic. The zero value is not
// usable — construct with NewCompleter.
type Completer struct {
	store *kb.Store
}

// NewCompleter builds a Completer over the knowledge base (constructor DI).
func NewCompleter(store *kb.Store) *Completer {
	return &Completer{store: store}
}

// RmsAt returns the completion candidates for pos in an RMS file by
// the context matrix:
//
//   - no owning statement → the section's commands (the synthetic
//     "global" section widens to every command, no section → silence);
//   - on a command name or a statement tail → the section's commands
//     plus the owning command's kb attributes;
//   - on a positional argument value → kb constants;
//   - on an attribute value → kb constants, on an attribute name →
//     the owning command's kb attributes.
//
// An empty result is the designed silence: candidates are never
// invented, only names from the knowledge base.
func (c *Completer) RmsAt(file rms.RmsFile, pos common.Pos) []Candidate {
	site, ok := file.ArgAt(pos)
	if !ok {
		return c.sectionCommands(sectionQuery(file, pos))
	}

	switch site.Kind {
	case rms.KindArg:
		return c.constants()

	case rms.KindAttr:
		if attrValueAt(site, pos) {
			return c.constants()
		}

		return c.ownerAttributes(site.Stmt.Name)

	default: // KindNone: command name, tail or an ambiguous position
		attrs := c.ownerAttributes(site.Stmt.Name)

		return append(attrs, c.sectionCommands(sectionQuery(file, pos))...)
	}
}

// sectionQuery returns the kb section filter for the section containing
// pos; found=false answers a nil filter and no candidates.
func sectionQuery(file rms.RmsFile, pos common.Pos) (string, bool) {
	sec, ok := file.SectionAt(pos)
	if !ok {
		return "", false
	}

	if sec.Name == "global" {
		return "", true
	}

	return sec.Name, true
}

// sectionCommands lists the commands of a kb section as candidates
// ("" — every section).
func (c *Completer) sectionCommands(section string, ok bool) []Candidate {
	if !ok {
		return nil
	}

	list := c.store.Commands(section)
	out := make([]Candidate, 0, len(list))

	for _, cmd := range list {
		out = append(out, Candidate{
			Label:  cmd.Name,
			Kind:   KindCommand,
			Detail: cmd.Section,
			Sort:   sortCommand + cmd.Name,
		})
	}

	return out
}

// ownerAttributes lists the kb attributes of the named command; an
// unknown command answers nil — no invented attributes.
func (c *Completer) ownerAttributes(command string) []Candidate {
	cmd, ok := c.store.Command(command)
	if !ok {
		return nil
	}

	out := make([]Candidate, 0, len(cmd.Attributes))

	for _, a := range cmd.Attributes {
		out = append(out, Candidate{
			Label:  a.Name,
			Kind:   KindAttribute,
			Detail: attrDetail(a),
			Sort:   sortAttr + a.Name,
		})
	}

	return out
}

// constants lists every kb constant as a candidate.
func (c *Completer) constants() []Candidate {
	list := c.store.Constants("")
	out := make([]Candidate, 0, len(list))

	for _, k := range list {
		out = append(out, Candidate{
			Label:  k.Name,
			Kind:   KindConstant,
			Detail: k.Value,
			Sort:   sortConst + k.Name,
		})
	}

	return out
}

// attrDetail renders one attribute line: "Kind Min..Max" when the
// mined range is present, "Kind" for a plain kind, "" for flag
// attributes — kinds and defaults are never invented.
func attrDetail(a kb.CommandArg) string {
	if a.Kind == "" {
		return ""
	}

	if a.Range.Min != "" && a.Range.Max != "" {
		return a.Kind + " " + a.Range.Min + ".." + a.Range.Max
	}

	return a.Kind
}

// attrValueAt reports whether pos sits on the value expression of the
// attribute identified by site: an attribute without a value has an
// empty value range and answers false (name position). The last
// matching attribute wins — same-name repeats resolve to the nearest.
func attrValueAt(site rms.ArgSite, pos common.Pos) bool {
	for i := len(site.Stmt.Attributes) - 1; i >= 0; i-- {
		a := site.Stmt.Attributes[i]
		if a.Name != site.Name || !a.Range.Contains(pos) {
			continue
		}

		return a.Value.Range.Contains(pos)
	}

	return false
}

// XsAt returns the completion candidates for pos in an XS file (a
// document or an inline block in block-local coordinates — the caller
// translates). Source symbols — the file's visible symbols plus the
// include-closure declarations — win over same-name kb entries (truth
// model); same-name source symbols of different kinds are both kept.
// Rule and event names are skipped: they are not addressable in
// expressions. Positions inside strings and comments answer silence.
func (c *Completer) XsAt(file xs.XsFile, pos common.Pos, external []xs.Decl) []Candidate {
	syms, ok := file.VisibleAt(pos)
	if !ok {
		return nil
	}

	source := make(map[string]bool, len(syms)+len(external))
	out := make([]Candidate, 0, len(syms)+len(external))

	addSource := func(name string, kind string, detail string) {
		source[name] = true
		out = append(out, Candidate{
			Label:  name,
			Kind:   kind,
			Detail: detail,
			Sort:   sortSource + name,
		})
	}

	for _, s := range syms {
		var kind string

		switch s.Kind {
		case KindFunction, KindVariable, KindParam, KindLocal:
			kind = s.Kind
		case xs.DeclExtern:
			kind = KindFunction
		default: // rule, event: not addressable by name
			continue
		}

		detail := ""
		if kind == KindFunction {
			detail = fileSignature(file.Decls, s.Name)
		}

		addSource(s.Name, kind, detail)
	}

	for _, d := range external {
		var kind string

		switch d.Kind {
		case xs.DeclFunction, xs.DeclExtern:
			kind = KindFunction
		case xs.DeclVariable:
			kind = KindVariable
		default: // rule, event, include: not addressable by name
			continue
		}

		addSource(d.Name, kind, declSignature(d))
	}

	for _, fn := range c.store.Functions() {
		if source[fn.Name] {
			continue
		}

		types := make([]string, 0, len(fn.Params))

		for _, p := range fn.Params {
			types = append(types, p.Type)
		}

		out = append(out, Candidate{
			Label:  fn.Name,
			Kind:   KindFunction,
			Detail: kbSignature(fn.ReturnType, fn.Name, types),
			Sort:   sortKbFunction + fn.Name,
		})
	}

	for _, k := range c.store.Constants("") {
		if source[k.Name] {
			continue
		}

		out = append(out, Candidate{
			Label:  k.Name,
			Kind:   KindConstant,
			Detail: k.Value,
			Sort:   sortKbConstant + k.Name,
		})
	}

	return out
}

// fileSignature returns the mini signature of the named function or
// extern declaration in decls, "" when no such declaration exists.
func fileSignature(decls []xs.Decl, name string) string {
	for _, d := range decls {
		if d.Name == name && (d.Kind == xs.DeclFunction || d.Kind == xs.DeclExtern) {
			return declSignature(d)
		}
	}

	return ""
}

// declSignature renders the mini signature of a source declaration:
// "[ret ]name(types…)" — types only as declared, never invented.
func declSignature(d xs.Decl) string {
	return signature(d.Type, d.Name, paramTypes(d.Params))
}

// kbSignature renders the mini signature of a kb function.
func kbSignature(ret string, name string, types []string) string {
	return signature(ret, name, types)
}

// signature composes "[ret ]name(types…)"; an empty return type
// omits the prefix.
func signature(ret string, name string, types []string) string {
	call := name + "(" + strings.Join(types, ", ") + ")"

	if ret == "" {
		return call
	}

	return ret + " " + call
}

// paramTypes extracts the declared parameter types.
func paramTypes(params []xs.Param) []string {
	types := make([]string, 0, len(params))

	for _, p := range params {
		types = append(types, p.Type)
	}

	return types
}
