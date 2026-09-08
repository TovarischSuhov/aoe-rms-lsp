// Package corpus runs real-world RMS/XS scripts through the aoe2-lsp
// binary over the LSP protocol (one fresh subprocess per file) and
// aggregates hard failures: panics, crashes, hangs and transport breaks.
package corpus

import (
	"fmt"
	"strings"
)

// Hard status classes of one file session; anything but statusOK makes
// the run fail (the cmd wrapper exit code follows HardFailures).
const (
	statusOK        = "ok"
	statusPanic     = "panic"
	statusExit      = "exit"
	statusTimeout   = "timeout"
	statusTransport = "transport"
)

// detailCap bounds the failure detail kept per file — enough of a panic
// trace to act on, never a log firehose.
const detailCap = 200

// FileResult is the outcome of one corpus file session.
type FileResult struct {
	Path          string
	Status        string
	Detail        string
	Diagnostics   int
	Requests      int
	RequestErrors int
	DurationMS    int64
}

// Report aggregates a corpus run over a directory.
type Report struct {
	Files        []FileResult
	HardFailures int
	DurationMS   int64
}

// Summary renders the run for humans: one status line, then one line per
// hard failure (stdout of the cmd wrapper).
func (r Report) Summary() string {
	counts := make(map[string]int)

	for _, f := range r.Files {
		counts[f.Status]++
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d files: ok %d, panic %d, exit %d, timeout %d, transport %d (%.1fs)\n",
		len(r.Files),
		counts[statusOK], counts[statusPanic], counts[statusExit],
		counts[statusTimeout], counts[statusTransport],
		float64(r.DurationMS)/1000)

	for _, f := range r.Files {
		if f.Status == statusOK {
			continue
		}

		if f.Detail == "" {
			fmt.Fprintf(&b, "FAIL %s: %s\n", f.Path, f.Status)
		} else {
			fmt.Fprintf(&b, "FAIL %s: %s — %s\n", f.Path, f.Status, f.Detail)
		}
	}

	return b.String()
}
