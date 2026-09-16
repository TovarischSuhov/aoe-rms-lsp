// Status bar view model for the language server indicator (task
// .goga/history/2026/status-bar/task.md, issue #85).
//
// Pure mapping from the client state machine plus the server version to
// the status bar presentation. No vscode import at all — node:test runs
// it outside the extension host (the same testability pattern as
// src/install.ts, which keeps its vscode import type-only).

export type ServerPhase = "starting" | "running" | "stopped";

export interface StatusView {
	text: string;
	tooltip: string;
}

// statusView renders the indicator for a server phase. `version` is the
// initializeResult serverInfo version ("dev" for local builds without
// ldflags); an unknown version stays hidden instead of a placeholder.
export function statusView(phase: ServerPhase, version?: string): StatusView {
	switch (phase) {
		case "starting":
			return { text: "$(sync~spin) AoE2 LSP", tooltip: "aoe2-lsp is starting" };
		case "running":
			return {
				text: `$(zap) AoE2 LSP${version === undefined ? "" : ` ${version}`}`,
				tooltip: `aoe2-lsp${version === undefined ? "" : ` ${version}`} is running — click for logs`,
			};
		case "stopped":
			return { text: "$(circle-slash) AoE2 LSP stopped", tooltip: "aoe2-lsp has stopped — click for logs" };
	}
}
