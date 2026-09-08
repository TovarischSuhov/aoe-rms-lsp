package kb

import "regexp"

// mineBoundsRe matches the first value-bounds fragment in Desc prose:
// an optional kind word immediately before "(<num>-<num>)". Only numeric
// parentheticals match — "(default: …)" and "(see: …)" cannot, because the
// parenthesized content must be exactly <num>-<num> (decimals and an
// optional leading minus on either bound are accepted).
var mineBoundsRe = regexp.MustCompile(
	`([A-Za-z_][A-Za-z0-9_]*)?\s*\((-?\d+(?:\.\d+)?)-(-?\d+(?:\.\d+)?)\)`,
)

// MineKindRange parses Desc prose into a structured kind word and value
// bounds. It is a pure, deterministic function of the input; unparseable
// prose yields the empty result — never an error.
func MineKindRange(desc string) (kind string, r ValueRange) {
	m := mineBoundsRe.FindStringSubmatch(desc)
	if m == nil {
		return "", ValueRange{}
	}

	// Bounds are kept exactly as written (string precision, no conversion).
	return m[1], ValueRange{Min: m[2], Max: m[3]}
}
