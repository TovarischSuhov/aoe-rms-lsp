package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLogger_DebugFlagControlsLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		debug bool
		want  bool // debug-level events enabled
	}{
		{name: "debug flag enables debug level", debug: true, want: true},
		{name: "default keeps info level", debug: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := newLogger(io.Discard, tt.debug)

			assert.Equal(t, tt.want, logger.Enabled(context.Background(), slog.LevelDebug))
		})
	}
}
