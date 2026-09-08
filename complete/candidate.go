// Package complete computes completion candidates for RMS and XS
// positions over parsed ASTs and the knowledge base: stateless,
// protocol-agnostic. An empty candidate list is the designed silence —
// never an error.
package complete

// Candidate is one protocol-agnostic completion candidate:
// construct-and-use data, no mutation. Mapping to
// protocol.CompletionItem (Label/Kind/Detail/SortText) is the server
// cell's responsibility.
type Candidate struct {
	// Label is the candidate text; the editor inserts it verbatim.
	Label string
	// Kind is the candidate nature: one of command / attribute /
	// constant / function / variable / param / local.
	Kind string
	// Detail is one short line (section, value bounds, mini signature,
	// constant value); empty when the data is unavailable — kinds and
	// defaults are never invented.
	Detail string
	// Sort is the SortText basis: a context-group digit followed by
	// Label, giving a deterministic ordering of candidate groups.
	Sort string
}
