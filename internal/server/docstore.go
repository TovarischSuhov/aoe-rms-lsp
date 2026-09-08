// Package server implements the aoe2-lsp language server: LSP protocol
// handlers over the kb, rms, xs and analysis cells, served over stdio.
package server

import (
	"slices"
	"sync"
)

// document is one cached document: its full text and version.
type document struct {
	text    string
	version int
}

// DocStore is the cache of open documents uri → text + version.
type DocStore struct {
	mu        sync.Mutex
	documents map[string]document
}

// NewDocStore returns an empty document cache.
func NewDocStore() *DocStore {
	return &DocStore{documents: make(map[string]document)}
}

// Put saves the document text with the given version. A stale version
// (smaller than the currently stored one) is ignored.
func (s *DocStore) Put(uri string, text string, version int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cur, ok := s.documents[uri]; ok && version < cur.version {
		return
	}

	s.documents[uri] = document{text: text, version: version}
}

// Get returns the text and version of the document; found=false when the
// document is not open.
func (s *DocStore) Get(uri string) (text string, version int, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.documents[uri]
	if !ok {
		return "", 0, false
	}

	return d.text, d.version, true
}

// Remove deletes the document from the cache.
func (s *DocStore) Remove(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.documents, uri)
}

// Text returns the editor-state text without the version — the projection
// of Get that structurally satisfies include.Source.
func (s *DocStore) Text(uri string) (string, bool) {
	text, _, found := s.Get(uri)

	return text, found
}

// URIs lists every open document, sorted for determinism (the other half
// of satisfying include.Source).
func (s *DocStore) URIs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	uris := make([]string, 0, len(s.documents))
	for uri := range s.documents {
		uris = append(uris, uri)
	}

	slices.Sort(uris)

	return uris
}
