import * as vscode from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind, ErrorAction, CloseAction, State } from "vscode-languageclient/node";
import * as path from "node:path";
import { deployMap, detectDeployTarget, realDeployEnv } from "./deploy.ts";
import { detectGameIncludeRoots, realSteamEnv } from "./gamedetect.ts";
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

	// Steam auto-detect (issue #123): fill includeRoots from the detected
	// game install before the client starts, so the first session already
	// resolves game includes. An explicit user setting at any level wins —
	// a not-found game stays silent, and failures never block activation:
	// includeRoots is a convenience, not a dependency.
	try {
		const detected = await detectGameIncludeRoots(realSteamEnv());
		const inspected = config.inspect<string[]>("includeRoots");
		const explicit =
			inspected?.globalValue ?? inspected?.workspaceValue ?? inspected?.workspaceFolderValue;
		if (detected.length > 0 && explicit === undefined) {
			await config.update("includeRoots", detected, vscode.ConfigurationTarget.Global);
			output.appendLine(`includeRoots auto-detected: ${detected.join(", ")}`);
		} else {
			output.appendLine(
				detected.length > 0
					? "includeRoots kept: an explicit setting wins over auto-detection"
					: "no AoE2 DE Steam install found, includeRoots untouched",
			);
		}
	} catch (err) {
		output.appendLine(`includeRoots auto-detection skipped: ${err instanceof Error ? err.message : String(err)}`);
	}

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

	// Deploy-to-game (issue #83): copy the active .rms and its local
	// include dependencies into the game's mod folder. The explicit
	// aoe2lsp.deploy.targetDir wins; without it a single local mod under
	// %USERPROFILE%\Games\Age of Empires 2 DE is used, anything else asks
	// for the setting instead of guessing.
	const deployMapCommand = vscode.commands.registerCommand("aoe2lsp.deployMap", async () => {
		const editor = vscode.window.activeTextEditor;
		if (!editor || !editor.document.uri.fsPath.toLowerCase().endsWith(".rms")) {
			void vscode.window.showErrorMessage("aoe2-lsp: open a .rms map file to deploy it to the game.");
			return;
		}

		const mapPath = editor.document.uri.fsPath;
		let targetDir = config.get<string>("deploy.targetDir", "").trim();
		if (!targetDir) {
			const detected = await detectDeployTarget(realDeployEnv());
			if (!detected) {
				void vscode.window.showErrorMessage(
					'aoe2-lsp: no local AoE2 DE mod found under %USERPROFILE%\\Games\\Age of Empires 2 DE — set the aoe2lsp.deploy.targetDir setting to the mod\'s resources/_common/random-map-scripts folder.',
				);
				return;
			}
			targetDir = detected;
		}

		try {
			const count = await vscode.window.withProgress(
				{ location: vscode.ProgressLocation.Window, title: `aoe2-lsp: deploying ${path.basename(mapPath)}`, cancellable: false },
				() => deployMap(mapPath, targetDir, realDeployEnv()),
			);
			output.appendLine(`deploy: ${count} file(s) → ${targetDir}`);
			void vscode.window.showInformationMessage(
				`aoe2-lsp: deployed ${count} file(s) to the game mod folder (${path.basename(targetDir)}). The mod folder is read when a match starts.`,
			);
		} catch (err) {
			output.appendLine(`deploy failed: ${err instanceof Error ? err.message : String(err)}`);
			void vscode.window.showErrorMessage(`aoe2-lsp: deploy failed: ${err instanceof Error ? err.message : String(err)}`);
		}
	});
	context.subscriptions.push(deployMapCommand);

	void client.start().catch((err: unknown) => {
		void vscode.window.showErrorMessage(
			`aoe2-lsp failed to start: ${String(err)}. Check the aoe2lsp.serverPath setting, or set aoe2lsp.download.mode to "auto" so the server is downloaded on activation.`,
		);
	});
}

export function deactivate(): Thenable<void> | undefined {
	return client?.stop();
}
