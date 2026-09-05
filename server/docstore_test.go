package server

import (
	"testing"

	"github.com/stretchr/testify/require"
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
