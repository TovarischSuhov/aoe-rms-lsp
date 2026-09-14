package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesRename pins the capability surface: rename is
// advertised through RenameOptions with the prepare provider set.
func TestInitialize_AdvertisesRename(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.Capabilities.RenameProvider, "rename must be advertised")

	opts, ok := res.Capabilities.RenameProvider.(*protocol.RenameOptions)
	require.True(t, ok, "RenameProvider must carry RenameOptions, not a bare boolean")

	require.NotNil(t, opts.PrepareProvider, "prepareProvider must be set")
	assert.True(t, *opts.PrepareProvider)
}

// TestServerPrepareRename_Placeholder pins the happy path: the site
// under the cursor answers the placeholder arm with its exact range and
// the current name as the placeholder.
func TestServerPrepareRename_Placeholder(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs", "void f() {}\nvoid g() { f(); }", 1)

	res, err := s.PrepareRename(context.Background(), &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     protocol.Position{Line: 1, Character: 11},
		},
	})
	require.NoError(t, err)

	ph, ok := res.(*protocol.PrepareRenamePlaceholder)
	require.True(t, ok, "a renameable position answers the placeholder arm")

	assert.Equal(t, "f", ph.Placeholder)
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 1, Character: 11},
		End:   protocol.Position{Line: 1, Character: 12},
	}, ph.Range)
}

// TestServerPrepareRename_NotRenameable pins the silence contract:
// builtins and unopened documents answer nil, nil — never a guessed
// range.
func TestServerPrepareRename_NotRenameable(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	tests := []struct {
		name string
		uri  uri.URI
		src  string
		open bool
		pos  protocol.Position
	}{
		{
			name: "builtin callee",
			uri:  "file:///b.xs",
			src:  "void h() { xsSetWorldGravity(1.0); }",
			open: true,
			pos:  protocol.Position{Line: 0, Character: 11},
		},
		{
			name: "closed document",
			uri:  "file:///closed.xs",
			open: false,
			pos:  protocol.Position{Line: 0, Character: 0},
		},
	}

	for _, tt := range tests {
		if tt.open {
			s.docs.Put(string(tt.uri), tt.src, 1)
		}

		res, err := s.PrepareRename(context.Background(), &protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: tt.uri},
				Position:     tt.pos,
			},
		})

		require.NoError(t, err, "case=%s: silence is not an error", tt.name)
		assert.Nil(t, res, "case=%s: not renameable answers nil, nil", tt.name)
	}
}

// TestServerPrepareRename_CrlfText pins the placeholder extraction on
// CRLF documents: the RMS parser counts offsets after normalization
// while the editor state keeps the raw bytes — the placeholder must
// still be the site's name, not a shifted garbage slice.
func TestServerPrepareRename_CrlfText(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms",
		"#const FOO 5\r\n<LAND_GENERATION>\r\ncreate_land\r\nland_percent FOO\r\n</LAND_GENERATION>\r\n", 1)

	res, err := s.PrepareRename(context.Background(), &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
			Position:     protocol.Position{Line: 3, Character: 13}, // on the FOO use
		},
	})
	require.NoError(t, err)

	ph, ok := res.(*protocol.PrepareRenamePlaceholder)
	require.True(t, ok, "the value use of a declared #const is renameable")

	assert.Equal(t, "FOO", ph.Placeholder)
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 3, Character: 13},
		End:   protocol.Position{Line: 3, Character: 16},
	}, ph.Range)
}

// TestServerRename_CoversUnopenedClosureFile pins the epic criterion:
// the edit covers every site of the closure — including files that are
// not open documents (here the included part.rms using the #const the
// open main.rms declares).
func TestServerRename_CoversUnopenedClosureFile(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	dir := t.TempDir()
	partPath := filepath.Join(dir, "part.rms")
	require.NoError(t, os.WriteFile(partPath,
		[]byte("<LAND_GENERATION>\ncreate_land\nland_percent FOO\n</LAND_GENERATION>\n"), 0o644))

	mainURI := uri.File(filepath.Join(dir, "main.rms"))
	s.docs.Put(string(mainURI), "#const FOO 5\n#include \"part.rms\"\n", 1)

	edit, err := s.Rename(context.Background(), &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: mainURI},
			Position:     protocol.Position{Line: 0, Character: 8}, // on FOO
		},
		NewName: "BAR",
	})
	require.NoError(t, err)
	require.NotNil(t, edit)
	assert.Empty(t, edit.DocumentChanges, "plain WorkspaceEdit{Changes}")

	changes := edit.Changes
	require.Len(t, changes, 2, "the declaring file plus the unopened includer target")

	mainEdits := changes[mainURI]
	require.Len(t, mainEdits, 1)
	assert.Equal(t, protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 7},
			End:   protocol.Position{Line: 0, Character: 10},
		},
		NewText: "BAR",
	}, mainEdits[0])

	partEdits := changes[uri.File(partPath)]
	require.Len(t, partEdits, 1, "the unopened closure file gets its edit")
	assert.Equal(t, protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 13},
			End:   protocol.Position{Line: 2, Character: 16},
		},
		NewText: "BAR",
	}, partEdits[0])
}

// TestServerRename_InvalidNewName pins the lexical gate: an empty or
// ill-formed NewName is a request error with no edit. The positive
// control first proves the position itself renames — the rejections
// below are about the name.
func TestServerRename_InvalidNewName(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs", "void f() {}\nvoid g() { f(); }", 1)

	okEdit, err := s.Rename(context.Background(), &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
			Position:     protocol.Position{Line: 1, Character: 11},
		},
		NewName: "h",
	})
	require.NoError(t, err, "the control position is renameable")
	require.NotNil(t, okEdit)

	for _, newName := range []string{"", "1bad", "bad-name"} {
		edit, err := s.Rename(context.Background(), &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.xs"},
				Position:     protocol.Position{Line: 1, Character: 11},
			},
			NewName: newName,
		})

		require.Error(t, err, "newName=%q", newName)
		assert.Nil(t, edit, "newName=%q", newName)
	}
}

// TestServerRename_NotRenameable pins the found=false gate: a position
// without a renameable binding is a request error, not an empty edit.
// The positive control first proves the document is served — h renames.
func TestServerRename_NotRenameable(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///b.xs", "void h() { xsSetWorldGravity(1.0); }", 1)

	okEdit, err := s.Rename(context.Background(), &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///b.xs"},
			Position:     protocol.Position{Line: 0, Character: 5}, // on the h declaration
		},
		NewName: "i",
	})
	require.NoError(t, err, "the control position is renameable")
	require.NotNil(t, okEdit)

	edit, err := s.Rename(context.Background(), &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///b.xs"},
			Position:     protocol.Position{Line: 0, Character: 11}, // on the builtin callee
		},
		NewName: "okName",
	})

	require.Error(t, err)
	assert.Nil(t, edit)
}

// TestServerRename_IntegrationStdio runs the rename request through the
// real stdio entrypoint: the dispatcher routes textDocument/rename to
// the server and a multi-file edit comes back.
func TestServerRename_IntegrationStdio(t *testing.T) {
	h := startHarness(t)
	ctx := context.Background()

	_, err := h.disp.Initialize(ctx, &protocol.InitializeParams{})
	require.NoError(t, err)

	f := crossFileFixture(t)

	mainText := "#include \"parts/econ.rms\"\n#includeXS parts/lib.xs\nvoid main() { sharedFn(1); }\n"
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: f["maps/main.rms"], LanguageID: "aoe2rms", Version: 1, Text: mainText,
		},
	}))
	h.waitDiagnostics(f["maps/main.rms"])

	libText := "void sharedFn(int n) { }\nvoid caller() { sharedFn(2); }\n"
	require.NoError(t, h.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: f["maps/parts/lib.xs"], LanguageID: "aoe2xs", Version: 1, Text: libText,
		},
	}))
	h.waitDiagnostics(f["maps/parts/lib.xs"])

	edit, err := h.disp.Rename(ctx, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: f["maps/parts/lib.xs"]},
			Position:     protocol.Position{Line: 0, Character: 8}, // on the sharedFn declaration
		},
		NewName: "renamedFn",
	})
	require.NoError(t, err, "the rename request must dispatch to an implemented handler")
	require.NotNil(t, edit)

	changes := edit.Changes
	require.Len(t, changes, 2, "a multi-file edit: lib.xs plus the open includer main.rms")

	libEdits := changes[f["maps/parts/lib.xs"]]
	require.Len(t, libEdits, 2, "the declaration plus the call in lib.xs")

	mainEdits := changes[f["maps/main.rms"]]
	require.Len(t, mainEdits, 1, "the inline call in main.rms")
	assert.Equal(t, protocol.Range{
		Start: protocol.Position{Line: 2, Character: 14},
		End:   protocol.Position{Line: 2, Character: 22}, // "sharedFn" spans 8 columns
	}, mainEdits[0].Range, "the inline site in .rms coordinates")

	for _, e := range libEdits {
		assert.Equal(t, "renamedFn", e.NewText)
	}

	assert.Equal(t, "renamedFn", mainEdits[0].NewText)
}
