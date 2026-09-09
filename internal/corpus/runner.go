package corpus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// Session budgets: one file gets a fixed wall-clock budget, the first
// diagnostics batch its own wait, and the process a grace period after
// stdin closes before the harness kills it. sessionTimeout is a var so
// tests can tighten it; consumers cannot configure it.
var (
	sessionTimeout = 60 * time.Second
	diagWait       = 10 * time.Second
	exitGrace      = 5 * time.Second
)

// stderrCap bounds the captured server stderr per file — panic traces
// fit, document dumps cannot.
const stderrCap = 64 << 10

// maxSamples bounds the positions sampled per file; maxCompletions bounds
// the (heavier) completion requests.
const (
	maxSamples     = 8
	maxCompletions = 2
)

// Runner drives corpus runs: one fresh aoe2-lsp subprocess per file, the
// full LSP cycle, and a hard failure classification.
type Runner struct {
	bin string
}

// NewRunner builds a runner spawning bin once per corpus file.
func NewRunner(bin string) *Runner {
	return &Runner{bin: bin}
}

// Run walks dir for .rms/.xs files (sorted, deterministic) and runs each
// in a fresh session. The returned error covers harness infrastructure
// only — the binary cannot start, the directory cannot be read; results
// of the scripts themselves always land in the report.
func (r *Runner) Run(ctx context.Context, dir string) (Report, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Report{}, fmt.Errorf("stat corpus dir: %w", err)
	}

	if !info.IsDir() {
		return Report{}, fmt.Errorf("corpus dir %s is not a directory", dir)
	}

	files, err := collectScripts(dir)
	if err != nil {
		return Report{}, fmt.Errorf("collect corpus files: %w", err)
	}

	if len(files) == 0 {
		return Report{}, fmt.Errorf("no .rms/.xs files under %s", dir)
	}

	start := time.Now()

	report := Report{Files: make([]FileResult, 0, len(files))}

	for _, path := range files {
		if ctx.Err() != nil {
			break
		}

		res, err := r.runFile(ctx, dir, path)
		if err != nil {
			return report, err
		}

		if res.Status != statusOK {
			report.HardFailures++
		}

		report.Files = append(report.Files, res)

		slog.DebugContext(ctx, "corpus file done", "path", res.Path, "status", res.Status)
	}

	report.DurationMS = time.Since(start).Milliseconds()

	return report, nil
}

// collectScripts gathers .rms/.xs paths under root in deterministic
// (lexical walk) order.
func collectScripts(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := strings.ToLower(d.Name())

		if strings.HasSuffix(name, ".rms") || strings.HasSuffix(name, ".xs") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	sort.Strings(files)

	return files, nil
}

// runFile runs one session over one corpus file.
func (r *Runner) runFile(ctx context.Context, root string, path string) (FileResult, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return FileResult{}, fmt.Errorf("rel %s: %w", path, err)
	}

	rel = filepath.ToSlash(rel)

	text, err := os.ReadFile(path)
	if err != nil {
		// A single unreadable file is a file-level failure, not a reason
		// to abort the whole run.
		return FileResult{
			Path:   rel,
			Status: statusExit,
			Detail: truncate("read: "+err.Error(), detailCap),
		}, nil
	}

	start := time.Now()

	sess, err := startSession(ctx, r.bin, root)
	if err != nil {
		return FileResult{}, fmt.Errorf("start %s: %w", r.bin, err)
	}

	defer sess.close()

	res := sess.drive(ctx, path, string(text), rel)
	res.DurationMS = time.Since(start).Milliseconds()

	return res, nil
}

// session owns one server subprocess and its client-side connection.
type session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *cappedBuffer

	conn   jsonrpc2.Conn
	disp   protocol.Server
	client *diagClient

	root string

	exited     chan struct{}
	waitErr    error
	once       sync.Once
	classified bool
}

// startSession spawns the server binary and wires the client protocol
// over its stdio.
func startSession(ctx context.Context, bin string, root string) (*session, error) {
	// The child runs with Dir=root, so a relative binary path would
	// otherwise resolve against the corpus root instead of the caller's
	// working directory (the CI gate passes ./aoe2-lsp).
	abs, err := filepath.Abs(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", bin, err)
	}

	cmd := exec.Command(abs)
	cmd.Dir = root

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrBuf := &cappedBuffer{cap: stderrCap}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	s := &session{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderrBuf,
		root:   root,
		exited: make(chan struct{}),
	}

	go func() {
		s.waitErr = cmd.Wait()
		close(s.exited)
	}()

	client := &diagClient{notify: make(chan struct{}, 4)}

	connCtx, conn, disp := protocol.NewClient(ctx, client, jsonrpc2.NewStream(&procTransport{s: s}))
	_ = connCtx

	s.conn = conn
	s.disp = disp
	s.client = client

	return s, nil
}

// drive performs the full LSP cycle over one file and classifies the
// outcome.
func (s *session) drive(parent context.Context, absPath string, text string, rel string) FileResult {
	res := FileResult{Path: rel}

	ctx, cancel := context.WithTimeout(parent, sessionTimeout)
	defer cancel()

	rootURI := uri.File(s.root)

	initParams := &protocol.InitializeParams{
		WorkspaceFoldersInitializeParams: protocol.WorkspaceFoldersInitializeParams{
			WorkspaceFolders: protocol.NewNullable([]protocol.WorkspaceFolder{
				{URI: rootURI, Name: "corpus"},
			}),
		},
		Capabilities: protocol.ClientCapabilities{
			General: &protocol.GeneralClientCapabilities{
				PositionEncodings: []protocol.PositionEncodingKind{
					protocol.PositionEncodingKindUTF8,
				},
			},
		},
	}

	if _, err := s.disp.Initialize(ctx, initParams); err != nil {
		s.fail(&res, ctx, "initialize: "+err.Error())

		return res
	}

	if err := s.disp.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		s.fail(&res, ctx, "initialized: "+err.Error())

		return res
	}

	if err := s.disp.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri.File(absPath),
			LanguageID: protocol.LanguageKind(languageID(absPath)),
			Version:    1,
			Text:       text,
		},
	}); err != nil {
		s.fail(&res, ctx, "did_open: "+err.Error())

		return res
	}

	res.Diagnostics = s.awaitDiagnostics(ctx)

	for _, pos := range samplePositions(text, maxSamples) {
		p := pos

		if !s.request(&res, ctx, "hover", func() error {
			_, err := s.disp.Hover(ctx, &protocol.HoverParams{
				TextDocumentPositionParams: textDocPos(absPath, p),
			})

			return err
		}) {
			return res
		}

		if !s.request(&res, ctx, "signature_help", func() error {
			_, err := s.disp.SignatureHelp(ctx, &protocol.SignatureHelpParams{
				TextDocumentPositionParams: textDocPos(absPath, p),
			})

			return err
		}) {
			return res
		}
	}

	for _, pos := range firstPositions(samplePositions(text, maxSamples), maxCompletions) {
		p := pos

		if !s.request(&res, ctx, "completion", func() error {
			_, err := s.disp.Completion(ctx, &protocol.CompletionParams{
				TextDocumentPositionParams: textDocPos(absPath, p),
			})

			return err
		}) {
			return res
		}
	}

	if !s.request(&res, ctx, "document_symbol", func() error {
		_, err := s.disp.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(absPath)},
		})

		return err
	}) {
		return res
	}

	if err := s.disp.Shutdown(ctx); err != nil {
		s.fail(&res, ctx, "shutdown: "+err.Error())

		return res
	}

	// Exit is a notification: the handler may be gone already, the error
	// carries no signal.
	_ = s.disp.Exit(ctx)

	if !s.awaitExit(exitGrace) {
		res.Status = statusTimeout
		res.Detail = "server did not exit after exit notification"

		return res
	}

	switch {
	case s.stderr.hasPanic():
		res.Status = statusPanic
		res.Detail = s.stderr.panicSnippet()
	case s.exitCode() != 0:
		res.Status = statusExit
		res.Detail = fmt.Sprintf("exit code %d", s.exitCode())
	default:
		res.Status = statusOK
	}

	return res
}

// request performs one harness request: a JSON-RPC error counts as
// RequestErrors and the session continues; a panicked handler or a dead
// session classifies the hard failure and returns false.
func (s *session) request(res *FileResult, ctx context.Context, name string, call func() error) bool {
	res.Requests++

	err := call()
	if err == nil {
		return true
	}

	res.RequestErrors++

	// The jsonrpc2 layer answers a panicked request handler with an
	// internal error carrying the panic value — the precise signal.
	if strings.Contains(err.Error(), "panicked") {
		res.Status = statusPanic
		res.Detail = truncate(err.Error(), detailCap)
		s.classified = true

		return false
	}

	dead := ctx.Err() != nil || s.stderr.hasPanic() || s.processExited() || s.connDone()

	if !dead {
		slog.DebugContext(ctx, "corpus request error", "method", name, "err", err)

		return true
	}

	s.fail(res, ctx, name+": "+err.Error())

	return false
}

// fail classifies a hard failure from the observable session state.
func (s *session) fail(res *FileResult, ctx context.Context, detail string) {
	res.Detail = truncate(detail, detailCap)

	// A dead connection usually means a dying process: cmd.Wait drains
	// the stderr pipe before returning, so give it a moment — the panic
	// marker and the exit code land only after that.
	if s.connDone() && !s.processExited() {
		s.awaitExit(time.Second)
	}

	switch {
	case s.stderr.hasPanic():
		res.Status = statusPanic
		res.Detail = s.stderr.panicSnippet()
	case strings.Contains(res.Detail, "panicked"):
		res.Status = statusPanic
	case ctx.Err() != nil:
		res.Status = statusTimeout
	case s.processExited() && s.exitCode() != 0:
		res.Status = statusExit
		res.Detail = fmt.Sprintf("exit code %d: %s", s.exitCode(), res.Detail)
	default:
		res.Status = statusTransport
	}

	s.classified = true
}

// awaitDiagnostics waits for the first diagnostics batch of the open
// document; zero when it never arrives in time or the connection dies
// first (not a hard failure by itself).
func (s *session) awaitDiagnostics(ctx context.Context) int {
	select {
	case <-s.client.notify:
		return s.client.lastCount()
	case <-s.conn.Done():
		return 0
	case <-ctx.Done():
		return 0
	case <-time.After(diagWait):
		return 0
	}
}

// awaitExit waits for the process to finish on its own.
func (s *session) awaitExit(grace time.Duration) bool {
	select {
	case <-s.exited:
		return true
	case <-time.After(grace):
		return false
	}
}

// processExited reports whether the server process has finished.
func (s *session) processExited() bool {
	select {
	case <-s.exited:
		return true
	default:
		return false
	}
}

// connDone reports whether the jsonrpc2 connection is closed.
func (s *session) connDone() bool {
	select {
	case <-s.conn.Done():
		return true
	default:
		return false
	}
}

// exitCode returns the process exit code; -1 while running or when the
// process was killed by a signal.
func (s *session) exitCode() int {
	if !s.processExited() {
		return -1
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](s.waitErr); ok {
		return exitErr.ExitCode()
	}

	if s.waitErr == nil {
		return 0
	}

	return -1
}

// close tears the session down: a healthy session gets EOF on stdin and
// a grace period to exit on its own; an already classified one is killed
// outright — there is nothing left to wait for.
func (s *session) close() {
	s.once.Do(func() {
		_ = s.stdin.Close()

		if !s.classified && !s.awaitExit(exitGrace) {
			_ = s.cmd.Process.Kill()
			<-s.exited
		}

		if s.classified && !s.processExited() {
			_ = s.cmd.Process.Kill()
			<-s.exited
		}

		_ = s.stdout.Close()
		_ = s.conn.Close()
	})
}

// procTransport adapts the process pipes to a jsonrpc2 stream.
type procTransport struct {
	s *session
}

// Read reads protocol bytes from the server stdout.
func (t *procTransport) Read(p []byte) (int, error) {
	return t.s.stdout.Read(p)
}

// Write writes protocol bytes to the server stdin.
func (t *procTransport) Write(p []byte) (int, error) {
	return t.s.stdin.Write(p)
}

// Close closes both pipe halves.
func (t *procTransport) Close() error {
	if err := t.s.stdin.Close(); err != nil {
		return err
	}

	return t.s.stdout.Close()
}

// diagClient is the editor-side client: it records pushed diagnostics
// batches and wakes awaiting sessions.
type diagClient struct {
	protocol.UnimplementedClient

	mu      sync.Mutex
	batches []*protocol.PublishDiagnosticsParams
	notify  chan struct{}
}

// PublishDiagnostics records the batch and wakes waiting sessions.
func (c *diagClient) PublishDiagnostics(
	ctx context.Context,
	params *protocol.PublishDiagnosticsParams,
) error {
	c.mu.Lock()
	c.batches = append(c.batches, params)
	c.mu.Unlock()

	select {
	case c.notify <- struct{}{}:
	default:
	}

	return nil
}

// lastCount returns the size of the latest batch.
func (c *diagClient) lastCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.batches) == 0 {
		return 0
	}

	return len(c.batches[len(c.batches)-1].Diagnostics)
}

// cappedBuffer keeps the first cap bytes written to it.
type cappedBuffer struct {
	cap  int
	mu   sync.Mutex
	buf  bytes.Buffer
	full bool
}

// Write stores the prefix up to the cap.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.full {
		return len(p), nil
	}

	room := b.cap - b.buf.Len()

	if room <= 0 {
		b.full = true

		return len(p), nil
	}

	if len(p) > room {
		b.buf.Write(p[:room])
		b.full = true

		return len(p), nil
	}

	b.buf.Write(p)

	return len(p), nil
}

// String returns the captured prefix.
func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// hasPanic reports whether the captured stderr contains a panic marker —
// the runtime "panic:" line or the server's recovered-notification log.
func (b *cappedBuffer) hasPanic() bool {
	return strings.Contains(b.String(), "panic")
}

// panicSnippet extracts a one-line panic summary from the captured
// stderr: the runtime "panic:" line, or the server's recovered-panic
// log line.
func (b *cappedBuffer) panicSnippet() string {
	for line := range strings.SplitSeq(b.String(), "\n") {
		if strings.Contains(line, "panic") {
			return truncate(line, detailCap)
		}
	}

	return truncate("panic marker in stderr", detailCap)
}

// languageID picks the LSP language id by file extension.
func languageID(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".xs") {
		return "aoe2xs"
	}

	return "aoe2rms"
}

// textDocPos builds the shared TextDocumentPositionParams member.
func textDocPos(path string, pos protocol.Position) protocol.TextDocumentPositionParams {
	return protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(path)},
		Position:     pos,
	}
}

// samplePositions picks up to max deterministic positions from the text:
// lines spread evenly over the document, column at the first non-space
// byte (always a valid utf-8 boundary for space-indented scripts).
func samplePositions(text string, max int) []protocol.Position {
	lines := strings.Split(text, "\n")

	if len(lines) == 0 {
		return nil
	}

	seen := make(map[protocol.Position]struct{})
	out := make([]protocol.Position, 0, max)

	for i := range max {
		line := i * len(lines) / max

		if line >= len(lines) {
			line = len(lines) - 1
		}

		col := firstNonSpace(lines[line])

		pos := protocol.Position{Line: uint32(line), Character: uint32(col)}

		if _, dup := seen[pos]; dup {
			continue
		}

		seen[pos] = struct{}{}

		out = append(out, pos)
	}

	return out
}

// firstNonSpace returns the byte offset of the first non-space byte.
func firstNonSpace(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return i
		}
	}

	return 0
}

// firstPositions truncates the sample list for the heavier requests.
func firstPositions(positions []protocol.Position, max int) []protocol.Position {
	if len(positions) <= max {
		return positions
	}

	return positions[:max]
}

// truncate clamps s to at most max runes-free bytes on one line.
func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")

	if len(s) <= max {
		return s
	}

	return s[:max] + "…"
}
