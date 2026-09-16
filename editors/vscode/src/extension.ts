import * as vscode from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind, ErrorAction, CloseAction, State } from "vscode-languageclient/node";
import { resolveServer, wireEnv, type DownloadMode, type ServerResolution } from "./install.ts";
import { statusView, type ServerPhase } from "./statusbar.ts";

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
	const config = vscode.workspace.getConfiguration("aoe2lsp");
	const output = vscode.window.createOutputChannel("aoe2-lsp");
	context.subscriptions.push(output);

	// Real implementations of the install.ts seam: everything VS
	// Code-specific (settings, progress UI, messages, logging) stays here
	// so src/install.ts keeps its vscode import type-only and stays
	// testable outside the extension host.
	wireEnv({
		readServerPath: (): { value: string; explicit: boolean } => {
			const value = config.get<string>("serverPath", "aoe2-lsp");
			// Only a user-set value overrides the resolve chain; the
			// "aoe2-lsp" default is a fall-through, not an override.
			const inspected = config.inspect<string>("serverPath");
			const explicit =
				(inspected?.globalValue ??
					inspected?.workspaceValue ??
					inspected?.workspaceFolderValue) !== undefined;
			return { value, explicit };
		},
		withProgress: <T>(
			title: string,
			job: (report?: (fraction: number) => void) => Promise<T>,
		): Promise<T> =>
			Promise.resolve(
				vscode.window.withProgress<T>(
					// Cancellation is not wired through the installer (see the
					// vscode-extension cook); the download runs to completion.
					{ location: vscode.ProgressLocation.Window, title, cancellable: false },
					(progress) => {
						let reported = 0;
						return job((fraction) => {
							// withProgress wants increments, install.ts reports
							// absolute fractions of the whole pipeline.
							progress.report({ increment: Math.max(0, fraction - reported) * 100 });
							reported = fraction;
						});
					},
				),
			),
		showMessage: (text: string): void => {
			void vscode.window.showErrorMessage(text);
		},
		log: (text: string): void => {
			output.appendLine(text);
		},
	});

	// Async activation: the client starts once the server command is
	// resolved (explicit setting → cache → PATH → download).
	const mode = config.get<DownloadMode>("download.mode", "auto");
	let resolution: ServerResolution;
	try {
		resolution = await resolveServer(context, mode);
	} catch (err) {
		// resolveServer degrades internally; reaching this means an
		// unexpected failure — keep activation alive with the bare command.
		output.appendLine(`server resolve failed: ${err instanceof Error ? err.message : String(err)}`);
		resolution = { command: "aoe2-lsp", origin: "path" };
	}
	output.appendLine(`server: ${resolution.command} (origin: ${resolution.origin})`);

	// The server is an external binary over stdio; the resolved command is
	// client-side and never reaches the server itself.
	const serverOptions: ServerOptions = {
		command: resolution.command,
		args: [],
		transport: TransportKind.stdio,
	};

	const clientOptions: LanguageClientOptions = {
		documentSelector: [{ language: "aoe2rms" }, { language: "aoe2xs" }],
		// One channel for install logs and protocol logs: the status bar
		// click target is unambiguous.
		outputChannel: output,
		// A missing binary is a setup problem, not a crash loop: stop
		// cleanly and tell the user where to fix the path.
		errorHandler: {
			error: () => ({ action: ErrorAction.Continue }),
			closed: () => ({ action: CloseAction.DoNotRestart }),
		},
	};

	client = new LanguageClient("aoe2lsp", "AoE2 RMS/XS Language Server", serverOptions, clientOptions);

	// Server indicator: the client state machine drives it, the version
	// arrives with initializeResult once running ("dev" for local builds).
	const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
	const showOutput = vscode.commands.registerCommand("aoe2lsp.showOutput", () => output.show());
	context.subscriptions.push(status, showOutput);
	status.command = "aoe2lsp.showOutput";

	const renderStatus = (phase: ServerPhase): void => {
		const view = statusView(phase, client?.initializeResult?.serverInfo?.version);
		status.text = view.text;
		status.tooltip = view.tooltip;
	};

	client.onDidChangeState(({ newState }) => {
		renderStatus(newState === State.Running ? "running" : newState === State.Stopped ? "stopped" : "starting");
	});

	renderStatus("starting");
	status.show();

	void client.start().catch((err: unknown) => {
		void vscode.window.showErrorMessage(
			`aoe2-lsp failed to start: ${String(err)}. Check the aoe2lsp.serverPath setting, or set aoe2lsp.download.mode to "auto" so the server is downloaded on activation.`,
		);
	});
}

export function deactivate(): Thenable<void> | undefined {
	return client?.stop();
}
