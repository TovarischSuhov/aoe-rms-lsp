package include

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource is a hand-written Source over a map of open documents.
type fakeSource map[string]string

func (f fakeSource) Text(uri string) (string, bool) {
	text, ok := f[uri]

	return text, ok
}

func (f fakeSource) URIs() []string {
	uris := make([]string, 0, len(f))
	for uri := range f {
		uris = append(uris, uri)
	}

	slices.Sort(uris)

	return uris
}

// compile-time check: the fake satisfies Source.
var _ Source = fakeSource{}

// TestSource_FakeSatisfiesInterface checks the contract shape through the
// fake used by the resolver tests.
func TestSource_FakeSatisfiesInterface(t *testing.T) {
	src := fakeSource{"file:///a": "text-a", "file:///b": "text-b"}

	text, found := src.Text("file:///a")
	require.True(t, found)
	assert.Equal(t, "text-a", text)

	_, found = src.Text("file:///c")
	assert.False(t, found)

	assert.Equal(t, []string{"file:///a", "file:///b"}, src.URIs())
}
