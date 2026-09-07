package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aoe2-lsp/include"
)

func TestDocStore_GetNotOpen(t *testing.T) {
	s := NewDocStore()

	_, _, found := s.Get("file:///missing.rms")

	require.False(t, found)
}

func TestDocStore_PutGet(t *testing.T) {
	s := NewDocStore()

	s.Put("file:///map.rms", "text v1", 1)

	text, version, found := s.Get("file:///map.rms")

	require.True(t, found)
	require.Equal(t, "text v1", text)
	require.Equal(t, 1, version)
}

func TestDocStore_StaleVersionIgnored(t *testing.T) {
	s := NewDocStore()

	s.Put("file:///map.rms", "new", 5)
	s.Put("file:///map.rms", "old", 3)

	text, version, found := s.Get("file:///map.rms")

	require.True(t, found)
	require.Equal(t, "new", text)
	require.Equal(t, 5, version)

	s.Put("file:///map.rms", "same", 5)

	text, _, _ = s.Get("file:///map.rms")
	require.Equal(t, "same", text, "equal version must not be ignored")
}

func TestDocStore_Remove(t *testing.T) {
	s := NewDocStore()

	s.Put("file:///map.rms", "text", 1)
	s.Remove("file:///map.rms")

	_, _, found := s.Get("file:///map.rms")

	require.False(t, found)
}

// TestDocStore_SatisfiesSource checks the compile-time structural
// satisfaction of include.Source (Text + URIs).
func TestDocStore_SatisfiesSource(t *testing.T) {
	var _ include.Source = testDocStore()
}

// testDocStore builds a store with two open documents.
func testDocStore() *DocStore {
	s := NewDocStore()
	s.Put("file:///b.rms", "text-b", 2)
	s.Put("file:///a.rms", "text-a", 1)

	return s
}

// TestDocStore_TextAndURIs checks the Source projection: Text without a
// version, URIs sorted for determinism.
func TestDocStore_TextAndURIs(t *testing.T) {
	s := testDocStore()

	text, found := s.Text("file:///a.rms")
	require.True(t, found)
	assert.Equal(t, "text-a", text)

	_, found = s.Text("file:///c.rms")
	assert.False(t, found)

	assert.Equal(t, []string{"file:///a.rms", "file:///b.rms"}, s.URIs())

	s.Remove("file:///a.rms")
	assert.Equal(t, []string{"file:///b.rms"}, s.URIs())
}
