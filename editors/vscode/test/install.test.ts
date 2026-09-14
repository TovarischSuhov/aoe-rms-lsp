// Unit tests for src/install.ts — server auto-download (task
// .goga/history/2026/task-vscode-autodownload/task.md, issue #52).
//
// Stock node:test runner, zero npm dependencies: Node's type stripping
// executes .ts directly (Node >= 22.18 locally and in CI), which is also
// why the import below spells out the .ts extension. src/install.ts must
// keep its "vscode" import type-only so the module loads outside the
// extension host — same for this file's own `vscode` import, which exists
// purely for the ExtensionContext type; the runtime seams (wireEnv,
// ExtensionContext stub) are exercised here instead.
//
// All networking goes through the mocked global fetch. The tar.gz fixture
// is built once with the system tar and hashed with node:crypto, so
// extraction and checksum verification run for real — POSIX-only, like
// the CI runner; Windows behaviour is a PR checklist item, not automated.

import { after, before, describe, it, mock } from "node:test";
import type { TestContext } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { ReadableStream } from "node:stream/web";
import type * as vscode from "vscode";
import * as install from "../src/install.ts";

const GH_LATEST = "https://api.github.com/repos/TovarischSuhov/aoe-rms-lsp/releases/latest";
const GH_DOWNLOAD = "https://github.com/TovarischSuhov/aoe-rms-lsp/releases/download";

// release.yml: every asset is `aoe2-lsp-<goos>-<goarch>` (defluxed binary
// name) zipped/tarred next to a single SHA256SUMS. The matrix test below
// pins these exact names so a typo cannot sneak into the client mapping.
const RELEASE_ASSETS = [
	"aoe2-lsp-windows-amd64.zip",
	"aoe2-lsp-linux-amd64.tar.gz",
	"aoe2-lsp-darwin-amd64.tar.gz",
	"aoe2-lsp-darwin-arm64.tar.gz",
	"SHA256SUMS",
];
const LINUX_ASSET = "aoe2-lsp-linux-amd64.tar.gz";
const LINUX_MEMBER = "aoe2-lsp-linux-amd64"; // archive member name per release.yml

interface Asset {
	tagName: string;
	archiveUrl: string;
}

interface EnvStub {
	readServerPath(): { value: string; explicit: boolean };
	withProgress<T>(title: string, job: () => Promise<T>): Promise<T>;
	showMessage(text: string): void;
	log(text: string): void;
}

let fixtureDir: string;
let fixturePayload: Buffer;
let archiveBytes: Buffer;
let archiveSums: string;
let corruptBytes: Buffer;
let corruptSums: string;
let tarDir: string;

function sha256(bytes: Buffer): string {
	return createHash("sha256").update(bytes).digest("hex");
}

before(async () => {
	// Directory of the system tar. The download test scopes PATH but the
	// implementation must still spawn the system tar via child_process
	// (task.md contract); a scoped PATH hiding it would turn every
	// download into an archive failure and make the test ungreenable.
	tarDir = path.dirname(execFileSync("sh", ["-c", "command -v tar"]).toString().trim());
	fixtureDir = await mkdtemp(path.join(tmpdir(), "aoe2lsp-fixture-"));
	const stage = path.join(fixtureDir, "stage");
	await mkdir(stage);
	// ~64 KiB so the chunked mock response yields several byte-progress
	// samples instead of a single 0 -> 1 jump.
	fixturePayload = Buffer.alloc(64 * 1024);
	for (let i = 0; i < fixturePayload.length; i++) {
		fixturePayload[i] = i % 251;
	}
	await writeFile(path.join(stage, LINUX_MEMBER), fixturePayload);
	const archivePath = path.join(fixtureDir, LINUX_ASSET);
	execFileSync("tar", ["czf", archivePath, "-C", stage, LINUX_MEMBER]);
	archiveBytes = await readFile(archivePath);
	archiveSums = `${sha256(archiveBytes)}  ${LINUX_ASSET}\n`;
	// Any buffer whose digest is listed but which tar cannot extract
	// exercises the archive-error branch.
	corruptBytes = Buffer.concat([archiveBytes.subarray(0, 64), Buffer.from("corrupted tail")]);
	corruptSums = `${sha256(corruptBytes)}  ${LINUX_ASSET}\n`;
});

after(async () => {
	await rm(fixtureDir, { recursive: true, force: true });
	// Leave the env seam on a silent default: cases that never call
	// wireEnv cannot inherit the previous test's stub, whatever order
	// the describe blocks run in.
	install.wireEnv({
		readServerPath: () => ({ value: "aoe2-lsp", explicit: false }),
		withProgress: <T>(_title: string, job: () => Promise<T>): Promise<T> => job(),
		showMessage: () => {},
		log: () => {},
	});
});

function mockFetch(t: TestContext, handler: (url: string) => Response | Promise<Response>) {
	const impl = async (input: unknown): Promise<Response> => {
		const url = input instanceof Request ? input.url : input instanceof URL ? input.href : String(input);
		return handler(url);
	};
	const m = mock.method(globalThis, "fetch", impl as unknown as typeof fetch);
	t.after(() => m.mock.restore());
	return m;
}

// Exact-URL routing; an unexpected fetch answers 500 so an accidental
// download fails loudly instead of silently succeeding.
function router(routes: Record<string, () => Response>): (url: string) => Response {
	return (url: string): Response => {
		const route = routes[url];
		if (!route) return new Response(`unexpected fetch: ${url}`, { status: 500 });
		return route();
	};
}

function releaseResponse(tag: string, assetNames?: string[]): Response {
	const names = assetNames ?? RELEASE_ASSETS;
	const body = {
		tag_name: tag,
		assets: names.map((name) => ({ name, browser_download_url: `${GH_DOWNLOAD}/${tag}/${name}` })),
	};
	return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

// Stream the payload in chunks with a Content-Length, mirroring a real
// download closely enough for byte-fraction progress to be observable.
// Pull-based on purpose: eager enqueue in start() lets the runtime
// coalesce the 64 KiB payload into one read on fast hosts, collapsing
// the progress samples this fixture exists to produce — one chunk per
// pull keeps the reads apart.
function chunked(bytes: Buffer, chunkSize = 16 * 1024): Response {
	let offset = 0;
	const stream = new ReadableStream<Uint8Array>({
		pull(controller) {
			const end = Math.min(offset + chunkSize, bytes.length);
			controller.enqueue(bytes.subarray(offset, end));
			offset = end;
			if (offset >= bytes.length) controller.close();
		},
	});
	return new Response(stream, { headers: { "content-length": String(bytes.length) } });
}

async function freshContext(t: TestContext): Promise<{ context: vscode.ExtensionContext; storage: string }> {
	const storage = await mkdtemp(path.join(tmpdir(), "aoe2lsp-storage-"));
	t.after(() => rm(storage, { recursive: true, force: true }));
	// The structural stub is all the module's fs-level contract needs;
	// the double cast keeps the plan's `vscode.ExtensionContext`
	// signature checkable instead of widening the helper's return type
	// (a bare structural literal would fail TS2345 against it).
	return { context: { globalStorageUri: { fsPath: storage } } as unknown as vscode.ExtensionContext, storage };
}

async function seedServer(storage: string, tag: string): Promise<string> {
	const dir = path.join(storage, "servers", tag);
	await mkdir(dir, { recursive: true });
	const bin = path.join(dir, "aoe2-lsp");
	await writeFile(bin, "#!/bin/sh\nexit 0\n");
	await chmod(bin, 0o755);
	return bin;
}

async function serversEntries(storage: string): Promise<string[]> {
	try {
		return await readdir(path.join(storage, "servers"));
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ENOENT") return [];
		throw err;
	}
}

function wireEnv(over?: Partial<EnvStub>): { messages: string[]; progressTitles: string[] } {
	const messages: string[] = [];
	const progressTitles: string[] = [];
	const stub: EnvStub = {
		readServerPath: over?.readServerPath ?? (() => ({ value: "aoe2-lsp", explicit: false })),
		withProgress:
			over?.withProgress ??
			(async <T>(title: string, job: () => Promise<T>): Promise<T> => {
				progressTitles.push(title);
				return job();
			}),
		showMessage: over?.showMessage ?? ((text: string) => { messages.push(text); }),
		log: over?.log ?? (() => {}),
	};
	install.wireEnv(stub);
	return { messages, progressTitles };
}

// Scoped PATH control for the findInPath cases; restored when the test ends.
function setPath(t: TestContext, value: string): void {
	const saved = process.env.PATH;
	process.env.PATH = value;
	t.after(() => { process.env.PATH = saved; });
}

async function makePathDir(t: TestContext, withBinary: boolean, includeTarDir = false): Promise<string> {
	const dir = await mkdtemp(path.join(tmpdir(), "aoe2lsp-path-"));
	t.after(() => rm(dir, { recursive: true, force: true }));
	if (withBinary) {
		const bin = path.join(dir, "aoe2-lsp");
		await writeFile(bin, "#!/bin/sh\nexit 0\n");
		await chmod(bin, 0o755);
	}
	// Hermetic by default: PATH contains only the temp dir, so ambient
	// aoe2-lsp binaries cannot leak into findInPath cases. Extraction
	// cases opt into the system tar's directory as well — the temp dir
	// comes first, so hermeticity over aoe2-lsp is preserved either way.
	setPath(t, includeTarDir ? [dir, tarDir].join(path.delimiter) : dir);
	return dir;
}

function installErrorKind(err: unknown): string {
	assert.ok(err instanceof install.InstallError, `expected InstallError, got: ${String(err)}`);
	return String((err as { kind?: unknown }).kind);
}

function assetV020(): Asset {
	return { tagName: "v0.2.0", archiveUrl: `${GH_DOWNLOAD}/v0.2.0/${LINUX_ASSET}` };
}

function v020Routes(archive: () => Response, sums: () => Response): Record<string, () => Response> {
	return {
		[`${GH_DOWNLOAD}/v0.2.0/${LINUX_ASSET}`]: archive,
		[`${GH_DOWNLOAD}/v0.2.0/SHA256SUMS`]: sums,
	};
}

describe("latestReleaseAsset", () => {
	const matrix = [
		{ platform: "win32", arch: "amd64", asset: "aoe2-lsp-windows-amd64.zip" },
		{ platform: "linux", arch: "amd64", asset: "aoe2-lsp-linux-amd64.tar.gz" },
		{ platform: "darwin", arch: "amd64", asset: "aoe2-lsp-darwin-amd64.tar.gz" },
		{ platform: "darwin", arch: "arm64", asset: "aoe2-lsp-darwin-arm64.tar.gz" },
	];

	it("maps platform/arch onto the release.yml asset names and sends GitHub API headers", async (t) => {
		const m = mockFetch(t, () => releaseResponse("v0.3.0"));
		for (const c of matrix) {
			const asset = await install.latestReleaseAsset(c.platform as NodeJS.Platform, c.arch);
			assert.deepEqual(asset, { tagName: "v0.3.0", archiveUrl: `${GH_DOWNLOAD}/v0.3.0/${c.asset}` });
		}
		assert.equal(m.mock.callCount(), matrix.length);
		for (const call of m.mock.calls) {
			// GitHub answers UA-less API requests with 403: both headers are
			// part of the contract, not decoration.
			const headers = new Headers((call.arguments[1] as RequestInit | undefined)?.headers);
			assert.match(headers.get("accept") ?? "", /vnd\.github\+json/);
			assert.ok((headers.get("user-agent") ?? "").length > 0, "User-Agent must be set");
		}
	});

	it("rejects linux/arm64 with kind unsupported-platform (AC: unsupported platform)", async (t) => {
		mockFetch(t, () => releaseResponse("v0.3.0"));
		await assert.rejects(
			install.latestReleaseAsset("linux" as NodeJS.Platform, "arm64"),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "unsupported-platform");
				return true;
			},
		);
	});

	it("maps 403/429 to kind rate-limit (AC: rate limit)", async (t) => {
		// One mutable handler, not a pair of mocks: t.after hooks restore
		// in registration order, so two consecutive mockFetch calls leave
		// globalThis.fetch mocked on the first one after the test ends.
		let status = 0;
		mockFetch(t, () => new Response("rate limit exceeded", { status }));
		for (status of [403, 429]) {
			await assert.rejects(
				install.latestReleaseAsset("linux" as NodeJS.Platform, "amd64"),
				(err: unknown) => {
					assert.equal(installErrorKind(err), "rate-limit");
					return true;
				},
			);
		}
	});

	it("maps other non-200 statuses and fetch rejections to kind network", async (t) => {
		// One mutable handler for the same reason as the rate-limit case
		// above: consecutive mockFetch calls restore out of order and
		// leave the first mock installed for the rest of the process.
		let refuse = false;
		mockFetch(t, () => {
			if (refuse) throw new Error("connection refused");
			return new Response("boom", { status: 500 });
		});
		await assert.rejects(
			install.latestReleaseAsset("linux" as NodeJS.Platform, "amd64"),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "network");
				return true;
			},
		);

		refuse = true;
		await assert.rejects(
			install.latestReleaseAsset("linux" as NodeJS.Platform, "amd64"),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "network");
				return true;
			},
		);
	});

	it("errors when the latest release lacks the platform asset", async (t) => {
		mockFetch(t, () => releaseResponse("v0.3.0", ["aoe2-lsp-darwin-arm64.tar.gz", "SHA256SUMS"]));
		await assert.rejects(
			install.latestReleaseAsset("linux" as NodeJS.Platform, "amd64"),
			(err: unknown) => {
				assert.ok(err instanceof install.InstallError, `expected InstallError, got: ${String(err)}`);
				return true;
			},
		);
	});
});

describe("installFromRelease", () => {
	it("installs the renamed binary under servers/<tag>, executable, with monotonic progress and no leftovers (AC: clean machine)", async (t) => {
		const { context, storage } = await freshContext(t);
		mockFetch(t, router(v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums))));

		const fractions: number[] = [];
		const bin = await install.installFromRelease(
			context,
			assetV020(),
			(f: number) => { fractions.push(f); },
		);

		// release.yml puts `aoe2-lsp-linux-amd64` at the archive root; the
		// install must publish it under the suffix-free name.
		const expected = path.join(storage, "servers", "v0.2.0", "aoe2-lsp");
		assert.equal(bin, expected);
		const st = await stat(expected);
		assert.ok(st.isFile(), "published path must be a regular file");
		assert.equal(st.mode & 0o111, 0o111, "binary must be executable on unix");
		assert.deepEqual(await readFile(expected), fixturePayload);

		// download -> hash -> extract -> publish must move monotonically 0 -> 1.
		assert.ok(fractions.length > 0, "progress must be reported");
		assert.ok(fractions[0] <= 0.1, `first fraction should start near 0, got ${fractions[0]}`);
		assert.ok(fractions[fractions.length - 1] >= 0.9, `last fraction should reach ~1, got ${fractions.join(", ")}`);
		for (let i = 1; i < fractions.length; i++) {
			assert.ok(fractions[i] >= fractions[i - 1], `fractions must be monotonic: ${fractions.join(", ")}`);
		}
		assert.deepEqual(await serversEntries(storage), ["v0.2.0"], "no .tmp-* leftovers");
	});

	it("prunes older tag dirs and stale .tmp-* dirs (AC: outdated cache cleanup)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.1.0");
		const staleTmp = path.join(storage, "servers", ".tmp-stale");
		await mkdir(staleTmp, { recursive: true });
		await writeFile(path.join(staleTmp, "junk"), "stale");
		mockFetch(t, router(v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums))));

		await install.installFromRelease(context, assetV020());

		assert.deepEqual(await serversEntries(storage), ["v0.2.0"]);
	});

	it("treats an existing <tag> binary as a lost race, not an error (AC: multi-window race)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.2.0");
		mockFetch(t, router(v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums))));

		const bin = await install.installFromRelease(context, assetV020());

		const expected = path.join(storage, "servers", "v0.2.0", "aoe2-lsp");
		assert.equal(bin, expected);
		await stat(expected);
	});

	it("replaces a stale <tag> dir without a binary (AC: atomic publish)", async (t) => {
		const { context, storage } = await freshContext(t);
		await mkdir(path.join(storage, "servers", "v0.2.0"), { recursive: true });
		mockFetch(t, router(v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums))));

		const bin = await install.installFromRelease(context, assetV020());

		const expected = path.join(storage, "servers", "v0.2.0", "aoe2-lsp");
		assert.equal(bin, expected);
		await stat(expected);
	});

	it("rejects a checksum mismatch with no partial state (AC: SHA256 mismatch)", async (t) => {
		const { context, storage } = await freshContext(t);
		const wrongSums = new Response(`${"0".repeat(64)}  ${LINUX_ASSET}\n`);
		mockFetch(t, router(v020Routes(() => chunked(archiveBytes), () => wrongSums)));

		await assert.rejects(
			install.installFromRelease(context, assetV020()),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "checksum");
				return true;
			},
		);

		const entries = await serversEntries(storage);
		assert.ok(!entries.includes("v0.2.0"), `partial tag dir left behind: ${entries.join(", ")}`);
		assert.ok(entries.every((e) => !e.startsWith(".tmp-")), `temp leftovers: ${entries.join(", ")}`);
	});

	it("rejects a corrupt archive whose digest matches with kind archive (AC: broken archive)", async (t) => {
		const { context, storage } = await freshContext(t);
		mockFetch(t, router(v020Routes(() => chunked(corruptBytes), () => new Response(corruptSums))));

		await assert.rejects(
			install.installFromRelease(context, assetV020()),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "archive");
				return true;
			},
		);

		const entries = await serversEntries(storage);
		assert.ok(!entries.includes("v0.2.0"), `partial tag dir left behind: ${entries.join(", ")}`);
		assert.ok(entries.every((e) => !e.startsWith(".tmp-")), `temp leftovers: ${entries.join(", ")}`);
	});

	it("maps a failing archive download to kind network (AC: unavailable release)", async (t) => {
		const { context } = await freshContext(t);
		mockFetch(t, (url) => {
			if (url.endsWith(LINUX_ASSET)) throw new Error("socket hang up");
			return new Response(archiveSums);
		});

		await assert.rejects(
			install.installFromRelease(context, assetV020()),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "network");
				return true;
			},
		);
	});

	it("maps a failing SHA256SUMS download to kind network (AC: unavailable release)", async (t) => {
		const { context } = await freshContext(t);
		mockFetch(t, (url) => {
			if (url.endsWith("SHA256SUMS")) throw new Error("socket hang up");
			return chunked(archiveBytes);
		});

		await assert.rejects(
			install.installFromRelease(context, assetV020()),
			(err: unknown) => {
				assert.equal(installErrorKind(err), "network");
				return true;
			},
		);
	});
});

describe("resolveServer", () => {
	it("explicit serverPath wins with zero network calls (AC: explicit serverPath)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.9.0");
		wireEnv({ readServerPath: () => ({ value: "/opt/aoe2-lsp-custom", explicit: true }) });
		const m = mockFetch(t, () => releaseResponse("v0.9.0"));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: "/opt/aoe2-lsp-custom", origin: "setting" });
		assert.equal(m.mock.callCount(), 0, "explicit setting must skip all networking");
	});

	it("mode off resolves the newest cached tag with zero network calls (AC: mode off)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.8.0");
		await seedServer(storage, "v0.9.0");
		wireEnv();
		const m = mockFetch(t, () => releaseResponse("v0.9.0"));

		const res = await install.resolveServer(context, "off");

		assert.deepEqual(res, { command: path.join(storage, "servers", "v0.9.0", "aoe2-lsp"), origin: "cache" });
		assert.equal(m.mock.callCount(), 0, "mode off must make no network calls");
	});

	it("mode off without cache falls back to PATH with zero network calls (AC: mode off)", async (t) => {
		const { context } = await freshContext(t);
		const dir = await makePathDir(t, true);
		wireEnv();
		const m = mockFetch(t, () => releaseResponse("v0.9.0"));

		const res = await install.resolveServer(context, "off");

		assert.deepEqual(res, { command: path.join(dir, "aoe2-lsp"), origin: "path" });
		assert.equal(m.mock.callCount(), 0, "mode off must make no network calls");
	});

	it("mode off with empty cache and PATH yields the bare command (AC: mode off)", async (t) => {
		const { context } = await freshContext(t);
		await makePathDir(t, false);
		wireEnv();
		const m = mockFetch(t, () => releaseResponse("v0.9.0"));

		const res = await install.resolveServer(context, "off");

		assert.deepEqual(res, { command: "aoe2-lsp", origin: "path" });
		assert.equal(m.mock.callCount(), 0, "mode off must make no network calls");
	});

	it("compares tags numerically: v0.10.0 is newer than v0.9.0", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.9.0");
		await seedServer(storage, "v0.10.0");
		wireEnv();
		const m = mockFetch(t, () => new Response("no network in this test", { status: 500 }));

		const res = await install.resolveServer(context, "off");

		// Lexical ordering would pick v0.9.0; the plan requires per-segment
		// numeric comparison.
		assert.deepEqual(res, { command: path.join(storage, "servers", "v0.10.0", "aoe2-lsp"), origin: "cache" });
		assert.equal(m.mock.callCount(), 0);
	});

	it("mode auto reuses the cache when it already holds the latest tag (AC: cache reuse, no re-download)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.3.0");
		wireEnv();
		const m = mockFetch(t, router({ [GH_LATEST]: () => releaseResponse("v0.3.0") }));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: path.join(storage, "servers", "v0.3.0", "aoe2-lsp"), origin: "cache" });
		assert.equal(m.mock.callCount(), 1, "exactly one network call: the latest check");
		assert.equal(String(m.mock.calls[0].arguments[0]), GH_LATEST);
	});

	it("mode auto with an empty cache but a PATH hit resolves from PATH without downloading (AC: resolve priority)", async (t) => {
		const { context } = await freshContext(t);
		const dir = await makePathDir(t, true);
		wireEnv();
		// Only the latest check is routed; the router answers any other
		// URL (i.e. an archive download) with 500, so skipping PATH in
		// the auto branch cannot stay green.
		const m = mockFetch(t, router({ [GH_LATEST]: () => releaseResponse("v0.3.0") }));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: path.join(dir, "aoe2-lsp"), origin: "path" });
		assert.equal(m.mock.callCount(), 1, "exactly the latest check; no download may follow");
	});

	it("mode auto downloads and installs when cache and PATH are empty (AC: clean machine)", async (t) => {
		const { context, storage } = await freshContext(t);
		// PATH stays scoped (no aoe2-lsp anywhere) but keeps the system tar
		// reachable: installFromRelease must extract with the real tar.
		await makePathDir(t, false, true);
		const { progressTitles } = wireEnv();
		mockFetch(t, router({
			[GH_LATEST]: () => releaseResponse("v0.2.0"),
			...v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums)),
		}));

		const res = await install.resolveServer(context, "auto");

		const expected = path.join(storage, "servers", "v0.2.0", "aoe2-lsp");
		assert.deepEqual(res, { command: expected, origin: "download" });
		await stat(expected);
		assert.equal(progressTitles.length, 1, "the download must run under withProgress");
	});

	it("rate-limited latest check degrades silently to the cache (AC: rate limit)", async (t) => {
		const { context, storage } = await freshContext(t);
		await seedServer(storage, "v0.9.0");
		const { messages } = wireEnv();
		const m = mockFetch(t, () => new Response("rate limit exceeded", { status: 403 }));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: path.join(storage, "servers", "v0.9.0", "aoe2-lsp"), origin: "cache" });
		assert.equal(m.mock.callCount(), 1, "exactly one network check per activation");
		assert.equal(messages.length, 0, `rate limit must stay silent, got: ${messages.join(" | ")}`);
	});

	it("network failure falls back to PATH with exactly one message (AC: graceful degradation)", async (t) => {
		const { context } = await freshContext(t);
		const dir = await makePathDir(t, true);
		const { messages } = wireEnv();
		mockFetch(t, () => { throw new Error("dns is down"); });

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: path.join(dir, "aoe2-lsp"), origin: "path" });
		assert.equal(messages.length, 1, `expected exactly one message, got: ${messages.join(" | ")}`);
	});

	it("release without the platform asset falls back to PATH with exactly one message (AC: unsupported platform)", async (t) => {
		const { context } = await freshContext(t);
		const dir = await makePathDir(t, true);
		const { messages } = wireEnv();
		mockFetch(t, () => releaseResponse("v0.3.0", ["aoe2-lsp-darwin-arm64.tar.gz", "SHA256SUMS"]));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: path.join(dir, "aoe2-lsp"), origin: "path" });
		assert.equal(messages.length, 1, `expected exactly one message, got: ${messages.join(" | ")}`);
	});

	it("tar missing from PATH degrades to the bare PATH command with one message (plan risk: spawn refusal)", async (t) => {
		const { context } = await freshContext(t);
		// PATH scoped to an empty dir: no aoe2-lsp anywhere (forces the
		// download branch) and no tar either, so the extraction spawn
		// itself fails with ENOENT — the Windows-without-bsdtar shape.
		await makePathDir(t, false);
		const { messages } = wireEnv();
		mockFetch(t, router({
			[GH_LATEST]: () => releaseResponse("v0.2.0"),
			...v020Routes(() => chunked(archiveBytes), () => new Response(archiveSums)),
		}));

		const res = await install.resolveServer(context, "auto");

		assert.deepEqual(res, { command: "aoe2-lsp", origin: "path" });
		assert.equal(messages.length, 1, `expected exactly one message, got: ${messages.join(" | ")}`);
	});
});
