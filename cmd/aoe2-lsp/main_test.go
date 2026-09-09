package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

// newTestFlagSet mirrors the flags registered in main.
func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("debug", false, "")

	return fs
}

func TestSplitUnknownArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		known   []string
		unknown []string
	}{
		{name: "no args", args: nil, known: nil, unknown: nil},
		{name: "known flag", args: []string{"-debug"}, known: []string{"-debug"}},
		{name: "known with value", args: []string{"--debug=false"}, known: []string{"--debug=false"}},
		{name: "stdio is unknown", args: []string{"-stdio", "--stdio"}, unknown: []string{"-stdio", "--stdio"}},
		{name: "unknown with value", args: []string{"--transport=pipe"}, unknown: []string{"--transport=pipe"}},
		{name: "positional kept", args: []string{"map.rms", "-"}, known: []string{"map.rms", "-"}},
		{name: "terminator keeps the rest", args: []string{"-debug", "--", "-bogus"}, known: []string{"-debug", "--", "-bogus"}},
		{
			name:    "mixed",
			args:    []string{"--stdio", "-x", "-debug"},
			known:   []string{"-debug"},
			unknown: []string{"--stdio", "-x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			known, unknown := splitUnknownArgs(newTestFlagSet(), tt.args)

			assert.Equal(t, tt.known, known, "known args")
			assert.Equal(t, tt.unknown, unknown, "unknown args")
		})
	}
}
