package kb

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMineKindRange_APIShape pins the contract surface of the mining
// foundation: the exported MineKindRange signature, ValueRange with its
// json tags (one type serves model and wire), and the Range field on
// CommandArg.
func TestMineKindRange_APIShape(t *testing.T) {
	// Signature: func MineKindRange(desc string) (kind string, r ValueRange);
	// unparseable prose yields the empty pair, never an error.
	kind, r := MineKindRange("nothing to mine here")

	assert.Empty(t, kind)
	assert.Equal(t, ValueRange{}, r)

	// ValueRange carries the min/max json tags.
	tags := jsonTags(ValueRange{})

	assert.Equal(t, "min", tags["Min"])
	assert.Equal(t, "max", tags["Max"])

	// CommandArg exposes the mined Range.
	arg := CommandArg{Name: "N"}

	assert.Equal(t, ValueRange{}, arg.Range)
}

// jsonTags maps field names to their json struct tag values.
func jsonTags(v any) map[string]string {
	rt := reflect.TypeOf(v)
	tags := make(map[string]string, rt.NumField())

	for f := range rt.Fields() {
		tags[f.Name] = f.Tag.Get("json")
	}

	return tags
}

// TestMineKindRange_BoundedNumber covers the canonical corpus pattern:
// bounds right after the kind word, with an unrelated "(default: …)"
// fragment behind it that must not match.
func TestMineKindRange_BoundedNumber(t *testing.T) {
	tests := []struct {
		name string
		desc string
		kind string
		min  string
		max  string
	}{
		{
			name: "canonical",
			desc: "number (0-99) (default: 12)",
			kind: "number",
			min:  "0",
			max:  "99",
		},
		{
			name: "large bounds",
			desc: "number (36-480)",
			kind: "number",
			min:  "36",
			max:  "480",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, r := MineKindRange(tt.desc)

			assert.Equal(t, tt.kind, kind)
			assert.Equal(t, ValueRange{Min: tt.min, Max: tt.max}, r)
		})
	}
}

// TestMineKindRange_DecimalAndNegativeBounds covers decimals and the
// optional leading minus on either bound.
func TestMineKindRange_DecimalAndNegativeBounds(t *testing.T) {
	kind, r := MineKindRange("float (-1.5-2.5) range")

	assert.Equal(t, "float", kind)
	assert.Equal(t, ValueRange{Min: "-1.5", Max: "2.5"}, r)
}

// TestMineKindRange_NoBounds covers prose without any bounds fragment.
func TestMineKindRange_NoBounds(t *testing.T) {
	kind, r := MineKindRange("plain description without bounds")

	assert.Empty(t, kind)
	assert.Equal(t, ValueRange{}, r)
}

// TestMineKindRange_DefaultAndSeeFragmentsIgnored covers non-numeric
// parentheticals: the parenthesized content must be exactly <num>-<num>.
// The third input is a real corpus string (create_elevation's Desc).
func TestMineKindRange_DefaultAndSeeFragmentsIgnored(t *testing.T) {
	tests := []string{
		"(default: 5)",
		"(see: create_elevation)",
		"number (default: 0 - not elevated)",
	}

	for _, desc := range tests {
		t.Run(desc, func(t *testing.T) {
			kind, r := MineKindRange(desc)

			assert.Empty(t, kind)
			assert.Equal(t, ValueRange{}, r)
		})
	}
}

// TestMineKindRange_EmptyDesc covers flag attributes (34 corpus entries
// carry empty Desc): empty result, no panic.
func TestMineKindRange_EmptyDesc(t *testing.T) {
	kind, r := MineKindRange("")

	assert.Empty(t, kind)
	assert.Equal(t, ValueRange{}, r)
}

// TestMineKindRange_BoundsWithoutKindWord covers bounds with no preceding
// word: kind stays empty while the bounds still mine.
func TestMineKindRange_BoundsWithoutKindWord(t *testing.T) {
	kind, r := MineKindRange("(0-5) picks randomly")

	assert.Empty(t, kind)
	assert.Equal(t, ValueRange{Min: "0", Max: "5"}, r)
}

// TestMineKindRange_FirstBoundsFragmentWins covers leftmost-match
// semantics; a refactor to last-match would flip this.
func TestMineKindRange_FirstBoundsFragmentWins(t *testing.T) {
	kind, r := MineKindRange("number (0-5) or (10-20)")

	assert.Equal(t, "number", kind)
	assert.Equal(t, ValueRange{Min: "0", Max: "5"}, r)
}
