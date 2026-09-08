package common

// Diagnostic severities following the LSP numbering; no other values exist.
const (
	// SeverityError marks an error.
	SeverityError = 1
	// SeverityWarning marks a warning.
	SeverityWarning = 2
	// SeverityInfo marks an informational message.
	SeverityInfo = 3
	// SeverityHint marks a hint or suggestion.
	SeverityHint = 4
)

// Diagnostic is a problem found in a source file: the single representation
// shared by rms.Parse, xs.Parse and the Analyzer.
type Diagnostic struct {
	// Range is the span the problem refers to.
	Range Range
	// Severity is the importance of the problem, 1..4 by LSP numbering.
	Severity int
	// Message is the human-readable problem text.
	Message string
	// Code is the stable identifier of the rule that produced the problem.
	Code string
}
