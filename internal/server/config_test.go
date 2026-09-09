package server

import (
	"aoe2-lsp/internal/common"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
)

// mustJSON builds a settings payload for tests.
func mustJSON(t *testing.T, raw string) protocol.LSPAny {
	t.Helper()

	val := jsontext.Value(raw)
	require.True(t, val.IsValid(), "test settings must be valid JSON: %s", raw)

	return val
}

// TestServerDidChangeConfiguration_SeverityOverride pins the override
// semantics: hint downgrades, none suppresses, an invalid severity
// name keeps the analyzer's own severity.
func TestServerDidChangeConfiguration_SeverityOverride(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	err := s.DidChangeConfiguration(context.Background(), &protocol.DidChangeConfigurationParams{
		Settings: mustJSON(t, `{"diagnostics":{"severityOverrides":{
			"unknown-command": "hint",
			"undefined-symbol": "none",
			"deprecated-effect-percent": "loud"
		}}}`),
	})
	require.NoError(t, err)

	diags := s.toProtocolDiags("", []common.Diagnostic{
		{Code: "unknown-command", Severity: common.SeverityError, Message: "m"},
		{Code: "undefined-symbol", Severity: common.SeverityError, Message: "m"},
		{Code: "deprecated-effect-percent", Severity: common.SeverityWarning, Message: "m"},
	})

	require.Len(t, diags, 2, "none suppresses the diagnostic")
	require.Equal(t, protocol.DiagnosticSeverityHint, diags[0].Severity)
	require.Equal(t, protocol.DiagnosticSeverityWarning, diags[1].Severity,
		"an invalid override name keeps the analyzer severity")
}

// TestServerDidChangeConfiguration_IncludeRoots pins the wiring: pushed
// roots reach the resolver and closures resolve through them.
func TestServerDidChangeConfiguration_IncludeRoots(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	dir := t.TempDir()
	root := t.TempDir()
	mainURI := "file://" + dir + "/main.rms"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.rms"), []byte("#include \"shared.rms\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shared.rms"), []byte("base_terrain GRASS\n"), 0o644))

	s.docs.Put(mainURI, "#include \"shared.rms\"\n", 1)

	err := s.DidChangeConfiguration(context.Background(), &protocol.DidChangeConfigurationParams{
		Settings: mustJSON(t, `{"includeRoots":[`+strconv.Quote(root)+`]}`),
	})
	require.NoError(t, err)

	closure := s.resolver.Closure(context.Background(), mainURI)
	require.Empty(t, closure.Missing, "the include resolves through the configured root")
}

// TestServerInitialized_PullConfiguration pins the pull channel: with
// the client capability remembered from Initialize, Initialized asks
// the client for the aoe2lsp section and applies it.
func TestServerInitialized_PullConfiguration(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	configCap := true
	_, err := s.Initialize(context.Background(), &protocol.InitializeParams{
		Capabilities: protocol.ClientCapabilities{
			Workspace: &protocol.WorkspaceClientCapabilities{Configuration: &configCap},
		},
	})
	require.NoError(t, err)

	rc := &recordingClient{notify: make(chan struct{}, 4)}
	rc.configResult = []protocol.LSPAny{mustJSON(t,
		`{"diagnostics":{"severityOverrides":{"unknown-command":"hint"}}}`)}

	ctx := protocol.WithClient(context.Background(), rc)
	require.NoError(t, s.Initialized(ctx, &protocol.InitializedParams{}))

	diags := s.toProtocolDiags("", []common.Diagnostic{
		{Code: "unknown-command", Severity: common.SeverityError, Message: "m"},
	})
	require.Len(t, diags, 1)
	require.Equal(t, protocol.DiagnosticSeverityHint, diags[0].Severity, "the pulled settings apply")
}

// TestServerInitialized_NoCapabilityNoop pins the silence: without the
// client capability Initialized does nothing and never fails.
func TestServerInitialized_NoCapabilityNoop(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	_, err := s.Initialize(context.Background(), &protocol.InitializeParams{})
	require.NoError(t, err)

	// no client in the context and no capability — a plain no-op
	require.NoError(t, s.Initialized(context.Background(), &protocol.InitializedParams{}))

	require.Nil(t, s.settings.Load(), "nothing was applied")
}

// TestServerDidChangeConfiguration_Republishes pins the republish
// behavior: after a settings push every open document gets a fresh
// diagnostics batch.
func TestServerDidChangeConfiguration_Republishes(t *testing.T) {
	t.Parallel()

	s := newNavigationServer(t)

	rc := &recordingClient{notify: make(chan struct{}, 4)}
	ctx := protocol.WithClient(context.Background(), rc)

	s.docs.Put("file:///t.rms", "zzzzqqqxxx\n", 1)
	s.publishDiagnostics(ctx, "file:///t.rms")

	require.Equal(t, 1, rc.batchCount())

	err := s.DidChangeConfiguration(ctx, &protocol.DidChangeConfigurationParams{
		Settings: mustJSON(t, `{"diagnostics":{"severityOverrides":{"unknown-command":"none"}}}`),
	})
	require.NoError(t, err)

	require.Equal(t, 2, rc.batchCount(), "the open document is republished")
	require.Empty(t, rc.batchAt(1).Diagnostics, "the override suppressed the diagnostic")
}
