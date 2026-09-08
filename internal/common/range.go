package common

// Range is a half-open span [Start, End) between two positions.
//
// A correctly built range satisfies End >= Start.
type Range struct {
	// Start is the beginning of the span, inclusive.
	Start Pos
	// End is the end of the span, exclusive.
	End Pos
}

// Contains reports whether p lies within the half-open span [Start, End).
func (r Range) Contains(p Pos) bool {
	return !p.Before(r.Start) && p.Before(r.End)
}
