import * as vscode from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind, ErrorAction, CloseAction } from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export function activate(_context: vscode.ExtensionContext): void {
	const serverPath = vscode.workspace
		.getConfiguration("aoe2lsp")
		.get<string>("serverPath", "aoe2-lsp");

	// The server is an external binary over stdio; the path setting is
	// client-side and never reaches the server itself.
	const serverOptions: ServerOptions = {
		command: serverPath,
		args: [],
		transport: TransportKind.stdio,
	};

	const clientOptions: LanguageClientOptions = {
		documentSelector: [{ language: "aoe2rms" }, { language: "aoe2xs" }],
		outputChannelName: "aoe2-lsp",
		// A missing binary is a setup problem, not a crash loop: stop
		// cleanly and tell the user where to fix the path.
		errorHandler: {
			error: () => ({ action: ErrorAction.Continue }),
			closed: () => ({ action: CloseAction.DoNotRestart }),
		},
	};

	client = new LanguageClient("aoe2lsp", "AoE2 RMS/XS Language Server", serverOptions, clientOptions);

	void client.start().catch((err: unknown) => {
		void vscode.window.showErrorMessage(
			`aoe2-lsp failed to start: ${String(err)}. Check the aoe2lsp.serverPath setting (the server binary must be built or installed separately).`,
		);
	});
}

export function deactivate(): Thenable<void> | undefined {
	return client?.stop();
}
