package hints

import (
	"aoe2-lsp/common"
	"aoe2-lsp/kb"
	"aoe2-lsp/rms"
	"aoe2-lsp/xs"
	"strings"
)

// Computer computes signature-help hints over parsed ASTs and the
// knowledge base: the truth model (source declarations — document and
// closure form one pool — win over kb), the conflict rule and label
// rendering. It is stateless: the only field is the immutable store, so
// concurrent handler use is safe.
type Computer struct {
	store *kb.Store
}

// NewComputer builds a Computer over the knowledge base (DI).
func NewComputer(store *kb.Store) *Computer {
	return &Computer{store: store}
}

// XsAt returns the hint for pos in an XS file (a document or an inline
// block in block-local coordinates — the caller performs the
// translation). Silence (found=false) is the failure mode: no call
// context, conflicting source declarations of one name, or no source and
// no kb entry.
func (c *Computer) XsAt(file xs.XsFile, pos common.Pos, external []xs.Decl) (Hint, bool) {
	cs, ok := file.CallAt(pos)
	if !ok {
		return Hint{}, false
	}

	// Truth model: document decls in source order, then closure decls in
	// caller order — one pool.
	pool := sourcePool(file.Decls, external, cs.Callee)

	if len(pool) > 0 {
		// Conflict rule: any parameter mismatch silences; equal
		// duplicates collapse onto the pool-first declaration.
		for _, other := range pool[1:] {
			if !paramsEqual(other.Params, pool[0].Params) {
				return Hint{}, false
			}
		}

		return renderSource(pool[0], cs), true
	}

	fn, found := c.store.Function(cs.Callee)
	if !found {
		return Hint{}, false
	}

	return renderKb(fn, cs)
}

// RmsAt returns the hint for pos in an RMS file: the full kb-ordered
// list of the owning command (Args then Attributes, no truncation) with
// the active-element mapping. Silence on no argument context or unknown
// command.
func (c *Computer) RmsAt(file rms.RmsFile, pos common.Pos) (Hint, bool) {
	site, ok := file.ArgAt(pos)
	if !ok {
		return Hint{}, false
	}

	cmd, found := c.store.Command(site.Stmt.Name)
	if !found {
		return Hint{}, false
	}

	labels := make([]string, 0, len(cmd.Args)+len(cmd.Attributes))

	for _, arg := range cmd.Args {
		labels = append(labels, commandArgLabel(arg))
	}

	for _, attr := range cmd.Attributes {
		labels = append(labels, commandArgLabel(attr))
	}

	active := -1

	switch site.Kind {
	case rms.KindArg:
		active = site.Index // pass-through, never clamped
	case rms.KindAttr:
		if i := attrPosition(cmd.Attributes, site.Name); i >= 0 {
			active = len(cmd.Args) + i
		}
	}

	return Hint{
		Label:  cmd.Name + "(" + strings.Join(labels, ", ") + ")",
		Params: labels,
		Active: active,
	}, true
}

// sourcePool collects Kind=function declarations of name from the
// document (source order) and then from the external closure decls.
func sourcePool(decls []xs.Decl, external []xs.Decl, name string) []xs.Decl {
	var pool []xs.Decl

	for i := range decls {
		if decls[i].Kind == xs.DeclFunction && decls[i].Name == name {
			pool = append(pool, decls[i])
		}
	}

	for i := range external {
		if external[i].Kind == xs.DeclFunction && external[i].Name == name {
			pool = append(pool, external[i])
		}
	}

	return pool
}

// paramsEqual reports equal parameter sequences: same length and
// pairwise (Type, Name) equality.
func paramsEqual(a []xs.Param, b []xs.Param) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Type != b[i].Type || a[i].Name != b[i].Name {
			return false
		}
	}

	return true
}

// renderSource renders the hint from a source declaration: labels are
// "<Type> <Name>" (no type — bare name), all parameters required (no
// brackets); the return type prefixes the label only when declared.
func renderSource(decl xs.Decl, cs xs.CallSite) Hint {
	labels := make([]string, 0, len(decl.Params))

	for _, p := range decl.Params {
		labels = append(labels, paramLabel(p.Type, p.Name))
	}

	return Hint{
		Label:  signatureLabel(decl.Type, decl.Name, labels),
		Params: labels,
		Active: activeOf(cs, len(labels)),
	}
}

// renderKb renders the hint from a kb function: optional parameters
// (Required=false) are bracketed.
func renderKb(fn kb.Function, cs xs.CallSite) (Hint, bool) {
	labels := make([]string, 0, len(fn.Params))

	for _, p := range fn.Params {
		label := paramLabel(p.Type, p.Name)
		if !p.Required {
			label = "[" + label + "]"
		}

		labels = append(labels, label)
	}

	return Hint{
		Label:  signatureLabel(fn.ReturnType, fn.Name, labels),
		Params: labels,
		Active: activeOf(cs, len(labels)),
	}, true
}

// paramLabel builds one XS parameter label: "<Type> <Name>", falling
// back to the bare name when the type is unset (permissive sources).
func paramLabel(typ string, name string) string {
	if typ == "" {
		return name
	}

	return typ + " " + name
}

// signatureLabel assembles the one-line signature; the return type
// prefixes the label only when non-empty.
func signatureLabel(ret string, name string, labels []string) string {
	label := name + "(" + strings.Join(labels, ", ") + ")"

	if ret != "" {
		label = ret + " " + label
	}

	return label
}

// activeOf maps the call site onto the rendered list: no active
// parameter on the callee name and never past the list end (silence
// instead of a wrong highlight — clamping is a consumer decision).
func activeOf(cs xs.CallSite, count int) int {
	if !cs.OnArg || cs.ArgIndex >= count {
		return -1
	}

	return cs.ArgIndex
}

// commandArgLabel builds one RMS element label: bare name for flag
// attributes, "Name: Kind" plus " Min..Max" for mined bounds; the whole
// label is bracketed when the element is optional.
func commandArgLabel(arg kb.CommandArg) string {
	label := arg.Name

	if arg.Kind != "" {
		label = arg.Name + ": " + arg.Kind

		if arg.Range != (kb.ValueRange{}) {
			label += " " + arg.Range.Min + ".." + arg.Range.Max
		}
	}

	if !arg.Required {
		label = "[" + label + "]"
	}

	return label
}

// attrPosition returns the kb position of the named attribute, -1 when
// the document attribute is absent from the kb list.
func attrPosition(attrs []kb.CommandArg, name string) int {
	for i := range attrs {
		if attrs[i].Name == name {
			return i
		}
	}

	return -1
}
