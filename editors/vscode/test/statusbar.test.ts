// Unit tests for src/statusbar.ts — the server indicator view model
// (task .goga/history/2026/status-bar/task.md, issue #85). Pure module,
// no vscode import: node:test runs it outside the extension host.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { statusView } from "../src/statusbar.ts";

describe("statusView", () => {
	it("shows a spinner while starting", () => {
		const view = statusView("starting");
		assert.match(view.text, /^\$\(sync/);
		assert.equal(view.text.includes("v0.7.0"), false);
	});

	it("shows the server version when running", () => {
		const view = statusView("running", "v0.7.0");
		assert.equal(view.text, "$(zap) AoE2 LSP v0.7.0");
		assert.match(view.tooltip, /v0\.7\.0.*running/);
	});

	it("hides an unknown version when running", () => {
		const view = statusView("running");
		assert.equal(view.text, "$(zap) AoE2 LSP");
		assert.doesNotMatch(view.text, /undefined|\?/);
	});

	it("marks the server as stopped", () => {
		const view = statusView("stopped");
		assert.equal(view.text, "$(circle-slash) AoE2 LSP stopped");
		assert.match(view.tooltip, /stopped/);
	});
});
