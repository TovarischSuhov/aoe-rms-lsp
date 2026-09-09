package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestServerDidChangeWatchedFiles_Republishes pins the end-to-end
// reaction: a disk edit of an included file plus the watch event
// republish the includer's diagnostics with the updated closure.
func TestServerDidChangeWatchedFiles_Republishes(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.xs")
	mainURI := uri.File(dir + "/main.rms")

	require.NoError(t, os.WriteFile(libPath, []byte("void bar() {}\n"), 0o644))

	rc := &recordingClient{notify: make(chan struct{}, 4)}
	ctx := protocol.WithClient(context.Background(), rc)

	s.docs.Put(mainURI.String(), "#includeXS lib.xs\nvoid main() { foo(); }\n", 1)
	s.publishDiagnostics(ctx, mainURI)

	require.Equal(t, 1, rc.batchCount())
	require.NotEmpty(t, rc.batchAt(0).Diagnostics, "foo is undefined in v1")

	// the disk edit and the watch event
	require.NoError(t, os.WriteFile(libPath, []byte("void foo() {}\n"), 0o644))

	require.NoError(t, s.DidChangeWatchedFiles(ctx, &protocol.DidChangeWatchedFilesParams{
		Changes: []protocol.FileEvent{{URI: uri.File(libPath), Type: protocol.FileChangeTypeChanged}},
	}))

	require.Equal(t, 2, rc.batchCount(), "the open includer is republished")

	for _, d := range rc.batchAt(1).Diagnostics {
		require.NotEqual(t, "undefined-symbol", fmt.Sprint(d.Code),
			"the closure reloaded the declaration: %v", rc.batchAt(1).Diagnostics)
	}
}

// TestServerInitialized_RegistersWatchers pins the registration: with
// the dynamicRegistration capability the server registers RMS/XS
// watchers; without it there is no registration call.
func TestServerInitialized_RegistersWatchers(t *testing.T) {
	t.Parallel()

	t.Run("with capability", func(t *testing.T) {
		t.Parallel()

		s := newNavigationServer(t)

		dynamic := true
		_, err := s.Initialize(context.Background(), &protocol.InitializeParams{
			Capabilities: protocol.ClientCapabilities{
				Workspace: &protocol.WorkspaceClientCapabilities{
					DidChangeWatchedFiles: &protocol.DidChangeWatchedFilesClientCapabilities{
						DynamicRegistration: &dynamic,
					},
				},
			},
		})
		require.NoError(t, err)

		rc := &recordingClient{notify: make(chan struct{}, 4)}
		ctx := protocol.WithClient(context.Background(), rc)

		require.NoError(t, s.Initialized(ctx, &protocol.InitializedParams{}))

		require.Len(t, rc.registrations, 1)

		reg := rc.registrations[0].Registrations
		require.Len(t, reg, 1)
		require.Equal(t, "workspace/didChangeWatchedFiles", reg[0].Method)

		var opts protocol.DidChangeWatchedFilesRegistrationOptions
		require.NoError(t, unmarshalOptions(reg[0].RegisterOptions, &opts))
		require.Len(t, opts.Watchers, 2)
	})

	t.Run("without capability", func(t *testing.T) {
		t.Parallel()

		s := newNavigationServer(t)

		_, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
		require.NoError(t, err)

		rc := &recordingClient{notify: make(chan struct{}, 4)}
		ctx := protocol.WithClient(context.Background(), rc)

		require.NoError(t, s.Initialized(ctx, &protocol.InitializedParams{}))

		require.Empty(t, rc.registrations, "no capability — no registration")
	})
}

// unmarshalOptions decodes a registration-options LSPAny payload with
// the union-aware json/v2 (plain encoding/json cannot decode GlobPattern).
func unmarshalOptions(raw protocol.LSPAny, opts any) error {
	return jsonv2.Unmarshal([]byte(raw), opts)
}
