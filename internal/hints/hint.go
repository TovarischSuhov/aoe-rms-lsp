// Package hints computes signature-help hints over parsed RMS/XS ASTs
// and the knowledge base: truth model (source declarations win over kb),
// the conflict rule and protocol-agnostic label rendering.
package hints

// Hint is the rendered hint for one call site — protocol-agnostic data
// (mapping to protocol.SignatureInformation is the server's
// responsibility). Construct-and-use data: no mutation.
type Hint struct {
	// Label is the full one-line signature, e.g.
	// "vector xsVectorSet(float x, float y, float z)" or
	// "percent_chance(%: percent 0..99)".
	Label string
	// Params are the parameter labels in list order; optional ones are
	// bracketed — "[float z]", "[MaxHeight: number 1..16]",
	// "[set_circular_base]".
	Params []string
	// Active is the index of the active parameter in Params; -1 — none.
	Active int
}
