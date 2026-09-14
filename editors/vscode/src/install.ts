// Server binary delivery for the VS Code extension (task
// .goga/history/2026/task-vscode-autodownload/task.md, issue #52).
//
// Zero npm dependencies on purpose: fetch + node:crypto + the system tar
// cover download, checksum and extraction. The "vscode" import stays
// type-only so this module also loads outside the extension host (unit
// tests run it on Node's type stripping); every VS Code API touchpoint
// goes through the wireEnv seam instead.

import { execFile } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { createReadStream } from "node:fs";
import { access, chmod, constants, mkdir, open, readdir, rename, rm, stat } from "node:fs/promises";
import * as path from "node:path";
import { promisify } from "node:util";
import type * as vscode from "vscode";

const execFileAsync = promisify(execFile);

const REPO = "TovarischSuhov/aoe-rms-lsp";
const LATEST_URL = `https://api.github.com/repos/${REPO}/releases/latest`;
const USER_AGENT = "aoe2-lsp-vscode-extension";
const SERVER_COMMAND = "aoe2-lsp";
// One small request per activation; a stuck API must not pin the
// extension host for long.
const API_TIMEOUT_MS = 10_000;
const DOWNLOAD_TIMEOUT_MS = 120_000;

// release.yml asset matrix: `aoe2-lsp-<goos>-<goarch>` zipped on windows,
// tarred elsewhere. Node reports x64 where Go says amd64.
const RELEASE_ASSETS: Record<string, string> = {
	"win32/amd64": "aoe2-lsp-windows-amd64.zip",
	"linux/amd64": "aoe2-lsp-linux-amd64.tar.gz",
	"darwin/amd64": "aoe2-lsp-darwin-amd64.tar.gz",
	"darwin/arm64": "aoe2-lsp-darwin-arm64.tar.gz",
};

export type DownloadMode = "auto" | "off";

export interface ServerResolution {
	command: string; // команда для LanguageClient
	origin: "setting" | "cache" | "path" | "download";
}

export interface ReleaseAsset {
	tagName: string; // напр. "v0.3.0"
	archiveUrl: string; // browser_download_url для os/arch
}

export type InstallErrorKind =
	| "rate-limit"
	| "network"
	| "unsupported-platform"
	| "checksum"
	| "archive"
	| "spawn";

export class InstallError extends Error {
	readonly kind: InstallErrorKind;

	constructor(kind: InstallErrorKind, message: string) {
		super(message);
		this.name = "InstallError";
		this.kind = kind;
	}
}

// The only VS Code surface this module touches. extension.ts wires the
// real implementations; tests substitute stubs through wireEnv.
export interface VscodeEnv {
	readServerPath(): { value: string; explicit: boolean };
	withProgress<T>(title: string, job: (report?: (fraction: number) => void) => Promise<T>): Promise<T>;
	showMessage(text: string): void;
	log(text: string): void;
}

// Headless defaults: bare command resolution, transparent progress,
// silent messaging — safe outside a running extension host.
let env: VscodeEnv = {
	readServerPath: () => ({ value: SERVER_COMMAND, explicit: false }),
	withProgress: <T>(_title: string, job: (report?: (fraction: number) => void) => Promise<T>): Promise<T> => job(),
	showMessage: () => {},
	log: () => {},
};

export function wireEnv(wired: VscodeEnv): void {
	env = wired;
}

function describeError(err: unknown): string {
	return err instanceof Error ? err.message : String(err);
}

function normalizeArch(arch: string): string {
	return arch === "x64" ? "amd64" : arch;
}

function serverBinaryName(): string {
	return process.platform === "win32" ? `${SERVER_COMMAND}.exe` : SERVER_COMMAND;
}

// Numeric per-segment comparison: v0.10.0 must outrank v0.9.0, which a
// lexical sort gets wrong.
function compareTags(a: string, b: string): number {
	const segmentsOf = (tag: string): number[] =>
		tag.replace(/^v/, "").split(".").map((s) => Number(s) || 0);
	const sa = segmentsOf(a);
	const sb = segmentsOf(b);
	for (let i = 0; i < Math.max(sa.length, sb.length); i++) {
		const diff = (sa[i] ?? 0) - (sb[i] ?? 0);
		if (diff !== 0) return diff;
	}
	return 0;
}

// Latest-релиз → asset под платформу; ошибка на неподдерживаемой платформе.
export async function latestReleaseAsset(
	platform: NodeJS.Platform,
	arch: string,
): Promise<ReleaseAsset> {
	const assetName = RELEASE_ASSETS[`${platform}/${normalizeArch(arch)}`];
	if (!assetName) {
		throw new InstallError(
			"unsupported-platform",
			`no aoe2-lsp release asset for ${platform}/${arch} (see the release matrix: windows/amd64, linux/amd64, darwin/amd64, darwin/arm64)`,
		);
	}

	let res: Response;
	try {
		// GitHub answers UA-less API requests with 403, so both headers are
		// part of the contract, not decoration.
		res = await fetch(LATEST_URL, {
			headers: {
				accept: "application/vnd.github+json",
				"user-agent": USER_AGENT,
			},
			signal: AbortSignal.timeout(API_TIMEOUT_MS),
		});
	} catch (err) {
		throw new InstallError("network", `GitHub API request failed: ${describeError(err)}`);
	}

	if (res.status === 403 || res.status === 429) {
		throw new InstallError("rate-limit", `GitHub API rate limit (HTTP ${res.status})`);
	}
	if (!res.ok) {
		throw new InstallError("network", `GitHub API HTTP ${res.status}`);
	}

	let body: { tag_name?: unknown; assets?: { name?: unknown; browser_download_url?: unknown }[] };
	try {
		body = (await res.json()) as typeof body;
	} catch (err) {
		throw new InstallError("network", `GitHub API returned malformed JSON: ${describeError(err)}`);
	}

	if (typeof body.tag_name !== "string" || body.tag_name === "") {
		throw new InstallError("network", "GitHub API response carries no usable tag_name");
	}
	const asset = (body.assets ?? []).find((a) => a?.name === assetName);
	if (typeof asset?.browser_download_url !== "string") {
		throw new InstallError(
			"unsupported-platform",
			`the latest release has no ${assetName} asset for ${platform}/${arch}`,
		);
	}
	return { tagName: body.tag_name, archiveUrl: asset.browser_download_url };
}

function serversDirOf(context: vscode.ExtensionContext): string {
	return path.join(context.globalStorageUri.fsPath, "servers");
}

async function cachedBinary(context: vscode.ExtensionContext, tag: string): Promise<string | undefined> {
	const bin = path.join(serversDirOf(context), tag, serverBinaryName());
	try {
		if ((await stat(bin)).isFile()) return bin;
	} catch {
		// no cached binary for this tag
	}
	return undefined;
}

async function newestCachedBinary(context: vscode.ExtensionContext): Promise<string | undefined> {
	let entries: string[];
	try {
		entries = await readdir(serversDirOf(context));
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ENOENT") return undefined;
		throw err;
	}
	const tags = entries.filter((name) => !name.startsWith(".")).sort(compareTags).reverse();
	for (const tag of tags) {
		const bin = await cachedBinary(context, tag);
		if (bin) return bin;
	}
	return undefined;
}

// A successful install is the only version we want to keep: drop older
// tags and stale .tmp-* leftovers from interrupted runs.
async function pruneServers(context: vscode.ExtensionContext, keepTag: string): Promise<void> {
	let entries: string[];
	try {
		entries = await readdir(serversDirOf(context));
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ENOENT") return;
		throw err;
	}
	for (const name of entries) {
		if (name === keepTag) continue;
		await rm(path.join(serversDirOf(context), name), { recursive: true, force: true });
	}
}

// Scan process.env.PATH by hand: zero deps, and the result is exactly
// what a bare server command would resolve to anyway.
async function findInPath(command: string): Promise<string | undefined> {
	const names = process.platform === "win32" ? [`${command}.exe`, command] : [command];
	for (const dir of (process.env.PATH ?? "").split(path.delimiter)) {
		if (!dir) continue;
		for (const name of names) {
			const full = path.join(dir, name);
			try {
				if (!(await stat(full)).isFile()) continue;
				if (process.platform !== "win32") {
					await access(full, constants.X_OK);
				}
				return full;
			} catch {
				// not present/not executable in this dir — keep scanning
			}
		}
	}
	return undefined;
}

async function pathResolution(): Promise<ServerResolution> {
	return { command: (await findInPath(SERVER_COMMAND)) ?? SERVER_COMMAND, origin: "path" };
}

async function degradeResolution(context: vscode.ExtensionContext): Promise<ServerResolution> {
	const cached = await newestCachedBinary(context);
	if (cached) return { command: cached, origin: "cache" };
	return pathResolution();
}

function parseChecksum(sums: string, fileName: string): string {
	for (const line of sums.split("\n")) {
		const match = /^([0-9a-fA-F]{64})[ \t]+\*?(.+?)\s*$/.exec(line);
		if (match && match[2] === fileName) return match[1].toLowerCase();
	}
	throw new InstallError("checksum", `SHA256SUMS has no entry for ${fileName}`);
}

// release.yml stores the defluxed member name (`aoe2-lsp-<os>-<arch>[.exe]`)
// at the archive root; tolerate the directory variants tar can emit.
async function findExtractedBinary(dir: string): Promise<string> {
	const entries = await readdir(dir, { withFileTypes: true });
	const files = entries.filter((e) => e.isFile());
	if (files.length === 1) return path.join(dir, files[0].name);
	const dirs = entries.filter((e) => e.isDirectory());
	if (files.length === 0 && dirs.length === 1) return findExtractedBinary(path.join(dir, dirs[0].name));
	throw new InstallError(
		"archive",
		`expected a single binary in the archive, found: ${entries.map((e) => e.name).join(", ")}`,
	);
}

// Скачивание + SHA256 + распаковка в globalStorage/servers/<tag>/;
// возвращает путь к бинарнику; rename-атомарно, при неудаче — чистый откат.
export async function installFromRelease(
	context: vscode.ExtensionContext,
	asset: ReleaseAsset,
	progress?: (fraction: number) => void,
): Promise<string> {
	const report = progress ?? ((): void => {});
	const serversDir = serversDirOf(context);
	// Temp lives inside servers/ so the final publish is a same-filesystem
	// rename; the dot prefix keeps it out of the tag namespace.
	const tmpDir = path.join(serversDir, `.tmp-${randomBytes(6).toString("hex")}`);
	const extractDir = path.join(tmpDir, "extracted");
	const archivePath = path.join(tmpDir, "archive");
	const assetName = path.basename(asset.archiveUrl);
	const destDir = path.join(serversDir, asset.tagName);
	const dest = path.join(destDir, assetName.includes("windows") ? `${SERVER_COMMAND}.exe` : SERVER_COMMAND);

	try {
		await mkdir(extractDir, { recursive: true });
		report(0);

		// Download, streaming: bytes → fraction 0 .. 0.7.
		let res: Response;
		try {
			res = await fetch(asset.archiveUrl, { signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
		} catch (err) {
			throw new InstallError("network", `download of ${assetName} failed: ${describeError(err)}`);
		}
		if (!res.ok) throw new InstallError("network", `download of ${assetName} failed: HTTP ${res.status}`);
		if (!res.body) throw new InstallError("network", `download of ${assetName} returned no body`);
		const total = Number(res.headers.get("content-length") ?? 0);
		const reader = res.body.getReader();
		const file = await open(archivePath, "w");
		try {
			let received = 0;
			for (;;) {
				const { done, value } = await reader.read();
				if (done) break;
				if (!value) continue;
				await file.write(value);
				received += value.byteLength;
				if (total > 0) report(0.7 * Math.min(1, received / total));
			}
		} catch (err) {
			// Mid-stream failures (timeout, reset) must classify as network,
			// not surface as a raw TimeoutError.
			throw new InstallError("network", `download of ${assetName} broke mid-stream: ${describeError(err)}`);
		} finally {
			await file.close();
		}

		// Checksum: fetch SHA256SUMS from the release, hash the archive
		// stream — fraction 0.7 .. 0.8.
		const sumsUrl = `${asset.archiveUrl.slice(0, asset.archiveUrl.lastIndexOf("/"))}/SHA256SUMS`;
		let sumsRes: Response;
		try {
			sumsRes = await fetch(sumsUrl, { signal: AbortSignal.timeout(API_TIMEOUT_MS) });
		} catch (err) {
			throw new InstallError("network", `download of SHA256SUMS failed: ${describeError(err)}`);
		}
		if (!sumsRes.ok) throw new InstallError("network", `download of SHA256SUMS failed: HTTP ${sumsRes.status}`);
		let sumsText: string;
		try {
			sumsText = await sumsRes.text();
		} catch (err) {
			throw new InstallError("network", `download of SHA256SUMS broke mid-stream: ${describeError(err)}`);
		}
		const expected = parseChecksum(sumsText, assetName);

		report(0.7);
		const hasher = createHash("sha256");
		let hashed = 0;
		for await (const chunk of createReadStream(archivePath)) {
			hasher.update(chunk);
			hashed += chunk.length;
			if (total > 0) report(0.7 + 0.1 * Math.min(1, hashed / total));
		}
		if (hasher.digest("hex") !== expected) {
			throw new InstallError("checksum", `SHA256 mismatch for ${assetName}: expected ${expected}`);
		}

		// Extract with the system tar (bsdtar on Windows 10 1803+ reads
		// .zip as well) — fraction 0.8 .. 0.95.
		report(0.8);
		try {
			await execFileAsync("tar", ["-xf", archivePath, "-C", extractDir]);
		} catch (err) {
			const code = (err as NodeJS.ErrnoException).code;
			const stderr = (err as { stderr?: string }).stderr ?? "";
			if (code === "ENOENT") {
				throw new InstallError(
					"spawn",
					`tar not found on PATH (on Windows it ships as bsdtar since 10 1803+) — install tar or set aoe2lsp.serverPath to a binary built from the repository`,
				);
			}
			throw new InstallError(
				"archive",
				`tar could not extract ${assetName} (exit ${String(code)}): ${stderr.trim() || "no stderr"} — the download may be corrupted or tar is missing`,
			);
		}
		report(0.95);

		// Publish: rename the extracted member under the suffix-free name,
		// atomically. An existing binary means another window won the race —
		// not an error; a tag dir without a binary is stale and goes away.
		const extracted = await findExtractedBinary(extractDir);
		let alreadyThere = false;
		try {
			alreadyThere = (await stat(dest)).isFile();
		} catch {
			alreadyThere = false;
		}
		if (!alreadyThere) {
			await rm(destDir, { recursive: true, force: true });
			await mkdir(destDir, { recursive: true });
			try {
				await rename(extracted, dest);
			} catch (err) {
				const code = (err as NodeJS.ErrnoException).code;
				const lostRace =
					code === "EEXIST" || code === "ENOTEMPTY" || code === "EPERM";
				if (lostRace && await stat(dest).then((s) => s.isFile(), () => false)) {
					// another window published first — its binary wins
				} else {
					throw err;
				}
			}
		}
		if (process.platform !== "win32") {
			await chmod(dest, 0o755);
		}
		report(1);

		await pruneServers(context, asset.tagName);
		return dest;
	} catch (err) {
		// A concurrent window's prune can delete our temp dir mid-install
		// (two cold caches racing). If it published this tag, the race is
		// lost but the outcome is good: its verified binary wins.
		const won = await stat(dest).then((s) => s.isFile(), (): boolean => false);
		if (won) {
			report(1);
			return dest;
		}
		throw err;
	} finally {
		// Clean rollback: nothing partial survives a failed install.
		await rm(tmpDir, { recursive: true, force: true });
	}
}

// Резолв по приоритету setting → cache → path → (mode=auto) download.
export async function resolveServer(
	context: vscode.ExtensionContext,
	mode: DownloadMode,
): Promise<ServerResolution> {
	const setting = env.readServerPath();
	if (setting.explicit) {
		env.log(`serverPath override: ${setting.value}`);
		return { command: setting.value, origin: "setting" };
	}

	if (mode === "off") {
		return degradeResolution(context);
	}

	// mode auto: exactly one network check per activation.
	let asset: ReleaseAsset;
	try {
		asset = await latestReleaseAsset(process.platform, process.arch);
	} catch (err) {
		if (err instanceof InstallError && err.kind === "rate-limit") {
			// A rate-limited IP is routine (60 req/h unauthenticated): fall
			// back silently, no error surface.
			env.log(`latest check rate-limited, staying offline: ${err.message}`);
			return degradeResolution(context);
		}
		env.showMessage(degradeMessage(err));
		return degradeResolution(context);
	}

	const cached = await cachedBinary(context, asset.tagName);
	if (cached) {
		env.log(`server from cache: ${cached}`);
		await pruneServers(context, asset.tagName);
		return { command: cached, origin: "cache" };
	}

	const inPath = await findInPath(SERVER_COMMAND);
	if (inPath) {
		env.log(`server from PATH: ${inPath}`);
		return { command: inPath, origin: "path" };
	}

	try {
		const bin = await env.withProgress(
			`aoe2-lsp: downloading server ${asset.tagName}`,
			(report) => installFromRelease(context, asset, report),
		);
		env.log(`server downloaded: ${bin}`);
		return { command: bin, origin: "download" };
	} catch (err) {
		env.showMessage(degradeMessage(err));
		// Same degrade ladder as a failed latest-check: with a warm cache
		// and broken networking the cached server must keep the LSP alive,
		// not fall through to a bare command.
		return degradeResolution(context);
	}
}

function degradeMessage(err: unknown): string {
	const reason = err instanceof InstallError ? err.message : describeError(err);
	return `aoe2-lsp: could not download the server (${reason}). Falling back to the "${SERVER_COMMAND}" command on PATH — set aoe2lsp.serverPath or build/install the binary manually if this keeps failing.`;
}
