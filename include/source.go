// Package include resolves the #include/#includeXS closure of a document
// and answers cross-file navigation over it: loading files from disk on
// demand while editor state stays authoritative for open documents.
package include

// Source supplies the editor-state text of open documents. It inverts the
// dependency on the server's document cache — the include cell knows
// nothing about the DocStore that implements it.
type Source interface {
	// Text returns the text of the open document; found=false signals
	// "look on disk".
	Text(uri string) (text string, found bool)

	// URIs lists every open document (used for the reverse search of
	// includers).
	URIs() (uris []string)
}
