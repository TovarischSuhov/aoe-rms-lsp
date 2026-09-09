package common

// Token is one classified source span for semanticTokens/full: the
// shared shape for the analysis producer and the server consumer —
// the Symbol precedent. Type draws from the server legend vocabulary:
// known / unknown / deprecated / section / kind.
type Token struct {
	// Range is the token span in file coordinates.
	Range Range
	// Type is the token class from the legend vocabulary.
	Type string
}
