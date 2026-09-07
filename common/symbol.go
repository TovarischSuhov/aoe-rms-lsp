package common

// Symbol is one node of a document outline tree: the shared shape for
// xs.Symbols and rms.Symbols; the consumer (server) maps it to the LSP
// DocumentSymbol.
type Symbol struct {
	// Kind is the node kind; the value vocabulary belongs to the
	// producer cell (xs: function/variable/rule/event/extern;
	// rms: section/command/xs).
	Kind string
	// Name is the display name of the node.
	Name string
	// Range is the span of the whole node.
	Range Range
	// Selection is the span of the node name (must be inside Range).
	Selection Range
	// Children are the nested nodes (empty for a leaf).
	Children []Symbol
}
