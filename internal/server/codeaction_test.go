package server

import (
	"aoe2-lsp/internal/analysis"
	"aoe2-lsp/internal/complete"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestInitialize_AdvertisesCodeAction pins the capability surface: the
// code-action provider must be advertised with the quickfix kind.
func TestInitialize_AdvertisesCodeAction(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	res, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	opts, ok := res.Capabilities.CodeActionProvider.(*protocol.CodeActionOptions)
	require.True(t, ok, "codeAction must be advertised with its kinds")
	require.Contains(t, opts.CodeActionKinds, protocol.CodeActionKindQuickFix)
}

// TestServerCodeAction_APIShape pins the empty-result contract: no
// context diagnostics answer with an empty slice, never nil, never an
// error.
func TestServerCodeAction_APIShape(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	actions, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
	})

	require.NoError(t, err, "an empty result is not an error")
	require.NotNil(t, actions)
	require.Empty(t, actions)
}

// codeActionAt requests actions with a query range spanning the given
// diagnostics — how an editor asks for the squiggle under the cursor.
func codeActionAt(
	t *testing.T,
	s *Server,
	docURI uri.URI,
	diags ...protocol.Diagnostic,
) []protocol.CommandOrCodeAction {
	t.Helper()

	require.NotEmpty(t, diags)
	query := protocol.Range{Start: diags[0].Range.Start, End: diags[len(diags)-1].Range.End}

	res, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Range:        query,
		Context:      protocol.CodeActionContext{Diagnostics: diags},
	})
	require.NoError(t, err)

	return res
}

// TestServerCodeAction_RenameFix pins the did-you-mean quickfix: the
// edit covers exactly the misspelled word and inserts the suggested
// name.
func TestServerCodeAction_RenameFix(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	t.Run("rms command typo", func(t *testing.T) {
		t.Parallel()

		s.docs.Put("file:///t.rms", "creat_object\n", 1)

		actions := codeActionAt(t, s, "file:///t.rms", protocol.Diagnostic{
			Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
			Code:    protocol.String(analysis.CodeUnknownCommand),
			Message: protocol.String(`unknown command "creat_object"; did you mean "create_object"?`),
		})

		require.Len(t, actions, 1)

		action := actions[0].(*protocol.CodeAction)
		require.Equal(t, "Change to 'create_object'", action.Title)
		require.Equal(t, protocol.CodeActionKindQuickFix, *action.Kind)

		edits := action.Edit.Changes["file:///t.rms"]
		require.Len(t, edits, 1)
		require.Equal(t, "create_object", edits[0].NewText)
		require.Equal(t, uint32(0), edits[0].Range.Start.Line)
		require.Equal(t, uint32(0), edits[0].Range.Start.Character)
		require.Equal(t, uint32(len("creat_object")), edits[0].Range.End.Character)
	})

	t.Run("xs symbol typo", func(t *testing.T) {
		t.Parallel()

		src := "void f() { xsGetMapSeedd(); }"
		s.docs.Put("file:///t.xs", src, 1)

		actions := codeActionAt(t, s, "file:///t.xs", protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: uint32(len("void f() { "))},
				End:   protocol.Position{Line: 0, Character: uint32(len("void f() { xsGetMapSeed"))},
			},
			Code:    protocol.String(analysis.CodeUndefinedSymbol),
			Message: protocol.String(`undefined symbol "xsGetMapSeedd"; did you mean "xsGetMapSeed"?`),
		})

		require.Len(t, actions, 1)

		edits := actions[0].(*protocol.CodeAction).Edit.Changes["file:///t.xs"]
		require.Equal(t, "xsGetMapSeed", edits[0].NewText)
		require.Equal(t, uint32(len("void f() { ")), edits[0].Range.Start.Character)
	})
}

// TestServerCodeAction_EffectPercentFix pins the deprecation quickfix:
// the command token is replaced, the arguments stay untouched.
func TestServerCodeAction_EffectPercentFix(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms", "effect_percent 5\n", 1)

	actions := codeActionAt(t, s, "file:///t.rms", protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Code:    protocol.String(analysis.CodeDeprecatedEffectPercent),
		Message: protocol.String("effect_percent is deprecated; use effect_amount instead"),
	})

	require.Len(t, actions, 1)

	action := actions[0].(*protocol.CodeAction)
	require.Equal(t, "Replace with effect_amount", action.Title)

	edits := action.Edit.Changes["file:///t.rms"]
	require.Len(t, edits, 1)
	require.Equal(t, "effect_amount", edits[0].NewText)
	require.Equal(t, uint32(len("effect_percent")), edits[0].Range.End.Character)
}

// TestServerCodeAction_CreateIncludeFix pins the missing-include
// quickfix: an idempotent create-file operation next to the document.
func TestServerCodeAction_CreateIncludeFix(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	dir := t.TempDir()
	docURI := uri.File(dir + "/main.rms")
	s.docs.Put(docURI.String(), "#include \"nope.rms\"\n", 1)

	actions := codeActionAt(t, s, docURI, protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 10},
			End:   protocol.Position{Line: 0, Character: 19},
		},
		Code:    protocol.String("missing-include"),
		Message: protocol.String("include not found: nope.rms"),
	})

	require.Len(t, actions, 1)

	action := actions[0].(*protocol.CodeAction)
	require.Equal(t, "Create 'nope.rms'", action.Title)
	require.Len(t, action.Edit.DocumentChanges, 1)

	create, ok := action.Edit.DocumentChanges[0].(*protocol.CreateFile)
	require.True(t, ok, "the fix is a create-file operation")
	require.Equal(t, uri.File(dir+"/nope.rms"), create.URI)
	require.NotNil(t, create.Options.IgnoreIfExists)
}

// TestServerCodeAction_MissingSectionFix pins the map-structure
// quickfix: the section skeleton inserts zero-width before the first
// section header (the diagnostic points there).
func TestServerCodeAction_MissingSectionFix(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms", "<PLAYER_SETUP>\nrandom_placement\n<LAND_GENERATION>\nbase_terrain GRASS\n", 1)

	actions := codeActionAt(t, s, "file:///t.rms", protocol.Diagnostic{
		Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Code:  protocol.String(analysis.CodeMissingSection),
		Message: protocol.String(
			"missing required section <TERRAIN_GENERATION>; without it the map generates no terrain"),
	})

	require.Len(t, actions, 1)

	action := actions[0].(*protocol.CodeAction)
	require.Equal(t, "Insert <TERRAIN_GENERATION> section", action.Title)

	edits := action.Edit.Changes["file:///t.rms"]
	require.Len(t, edits, 1)
	require.Equal(t, "<TERRAIN_GENERATION>\nbase_terrain GRASS\n\n", edits[0].NewText)
	require.Equal(t, uint32(0), edits[0].Range.Start.Line)
	require.Equal(t, uint32(0), edits[0].Range.Start.Character)
	require.Equal(t, edits[0].Range.Start, edits[0].Range.End, "zero-width insert")
}

// TestServerCodeAction_Filters pins the silence shape: a kind filter
// without quickfix, diagnostics without a suggestion and foreign codes
// produce no actions.
func TestServerCodeAction_Filters(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.rms", "creat_object\n", 1)

	res, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///t.rms"},
		Range:        protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{{
				Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
				Code:    protocol.String(analysis.CodeUnknownCommand),
				Message: protocol.String(`unknown command "creat_object"`),
			}},
			Only: []protocol.CodeActionKind{protocol.CodeActionKindRefactor},
		},
	})
	require.NoError(t, err)
	require.Empty(t, res, "Only without quickfix silences the provider")

	// no did-you-mean suffix — nothing to apply
	actions := codeActionAt(t, s, "file:///t.rms", protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Code:    protocol.String(analysis.CodeUnknownCommand),
		Message: protocol.String(`unknown command "creat_object"`),
	})
	require.Empty(t, actions)

	// a foreign code never maps to a fix
	actions = codeActionAt(t, s, "file:///t.rms", protocol.Diagnostic{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
		Code:    protocol.String(analysis.CodeBadArgument),
		Message: protocol.String(`command "creat_object" takes at most 0 argument(s), got 1`),
	})
	require.Empty(t, actions)
}

// TestServerCodeAction_AddIncludeFix pins the auto-include quickfix: an
// undefined symbol declared by exactly one available file gains an
// Add-include action; two owners and non-matching codes stay silent.
func TestServerCodeAction_AddIncludeFix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainURI := uri.File(dir + "/main.xs")

	require.NoError(t, os.WriteFile(filepath.FromSlash(dir+"/lib.xs"),
		[]byte("int uniqueName = 1;\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.FromSlash(dir+"/other.xs"),
		[]byte("int twinName = 1;\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.FromSlash(dir+"/second.xs"),
		[]byte("int twinName = 2;\n"), 0o644))

	s := newNavigationServer(t)
	s.docs.Put(mainURI.String(), "void main() {\n\ttwinName = 1;\n}\n", 1)

	t.Run("single owner inserts the include", func(t *testing.T) {
		t.Parallel()

		actions := codeActionAt(t, s, mainURI, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 1},
				End:   protocol.Position{Line: 1, Character: 9},
			},
			Code:    protocol.String(analysis.CodeUndefinedSymbol),
			Message: protocol.String(`undefined symbol "uniqueName"; did you mean "uniqueName"?`),
		})

		var add *protocol.CodeAction

		for _, a := range actions {
			if ca, ok := a.(*protocol.CodeAction); ok && strings.HasPrefix(ca.Title, "Add include '") {
				add = ca
			}
		}

		require.NotNil(t, add, "the unique-owner quickfix fires")
		require.Equal(t, "Add include 'lib.xs'", add.Title)

		edits := add.Edit.Changes[uri.URI(mainURI.String())]
		require.Len(t, edits, 1)
		require.Equal(t, "#include \"lib.xs\"\n", edits[0].NewText)
		require.Equal(t, uint32(0), edits[0].Range.Start.Line)
		require.Equal(t, uint32(0), edits[0].Range.Start.Character)
	})

	t.Run("two owners stay silent", func(t *testing.T) {
		t.Parallel()

		actions := codeActionAt(t, s, mainURI, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 1},
				End:   protocol.Position{Line: 1, Character: 9},
			},
			Code:    protocol.String(analysis.CodeUndefinedSymbol),
			Message: protocol.String(`undefined symbol "twinName"`),
		})

		for _, a := range actions {
			ca, ok := a.(*protocol.CodeAction)
			require.True(t, ok)
			assert.NotContains(t, ca.Title, "twinName", "ambiguous candidates get no include fix")
		}
	})
}

// TestToCompletionItems_AutoInclude pins the auto-include completion
// item shape: the include path rides as AdditionalTextEdits at the
// document start.
func TestToCompletionItems_AutoInclude(t *testing.T) {
	t.Parallel()

	items := toCompletionItems([]complete.Candidate{
		{Label: "exportedFn", Kind: complete.KindFunction, Detail: "int exportedFn() · auto-include lib.xs", Sort: "1exportedFn", Include: "lib.xs"},
	}, "#include")

	require.Len(t, items, 1)
	require.Len(t, items[0].AdditionalTextEdits, 1)

	edit := items[0].AdditionalTextEdits[0]
	require.Equal(t, "#include \"lib.xs\"\n", edit.NewText)
	require.Equal(t, uint32(0), edit.Range.Start.Line)
	require.Equal(t, uint32(0), edit.Range.Start.Character)
	require.Equal(t, edit.Range.Start, edit.Range.End, "zero-width insert")
}

// TestServerCodeAction_RenamedSymbolFix pins the changelog rename
// quickfix: the old name under the diagnostic is replaced by its
// knowledge-base replacement, taken from the data, not a hardcode.
func TestServerCodeAction_RenamedSymbolFix(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	s.docs.Put("file:///t.xs", "void main() {\n\txsGetLocalPlayerId();\n}\n", 1)

	actions := codeActionAt(t, s, "file:///t.xs", protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 25},
		},
		Code:    protocol.String(analysis.CodeRenamedSymbol),
		Message: protocol.String(`"xsGetLocalPlayerId" was renamed to "xsUnsyncGetLocalPlayerId" — the game rejects the old name`),
	})

	require.Len(t, actions, 1)

	action := actions[0].(*protocol.CodeAction)
	require.Equal(t, "Rename to 'xsUnsyncGetLocalPlayerId'", action.Title)

	edits := action.Edit.Changes["file:///t.xs"]
	require.Len(t, edits, 1)
	require.Equal(t, "xsUnsyncGetLocalPlayerId", edits[0].NewText)
}
