// Unit tests for src/gamedetect.ts — Steam auto-detection (issue #123,
// slot of .goga/history/2026/ux-wave-2/task.md).
//
// Stock node:test runner, zero npm dependencies: Node's type stripping
// executes .ts directly (Node >= 22.18 locally and in CI). The fixtures
// are real tmpdir layouts (dirs, vdf files); only the platform identity
// and the environment variables go through the SteamEnv seam.

import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { detectGameIncludeRoots, parseLibraryFolders, steamLibraries, type SteamEnv } from "../src/gamedetect.ts";

// Real-world libraryfolders.vdf (Steam, 2026-11): a single library entry
// with the apps block, Windows doubled backslashes.
const VDF_ONE_LIB = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
		"contentid"		"-2359625428996037736"
		"apps"
		{
			"813780"		"25788200948"
		}
	}
}
`;

describe("parseLibraryFolders", () => {
	it("reads the modern format with the apps block", () => {
		assert.deepEqual(parseLibraryFolders(VDF_ONE_LIB), ["C:\\Program Files (x86)\\Steam"]);
	});

	it("reads several libraries in order, unescaping doubled backslashes", () => {
		const vdf = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
	"2"
	{
		"path"		"E:/Games/Steam"
	}
}
`;
		assert.deepEqual(parseLibraryFolders(vdf), [
			"C:\\Program Files (x86)\\Steam",
			"D:\\SteamLibrary",
			"E:/Games/Steam",
		]);
	});

	it("tolerates a vdf without path keys", () => {
		assert.deepEqual(parseLibraryFolders(`"libraryfolders"\n{\n}\n`), []);
		assert.deepEqual(parseLibraryFolders(""), []);
	});
});

describe("detectGameIncludeRoots", () => {
	let fixtureDir: string;

	before(async () => {
		fixtureDir = await mkdtemp(path.join(tmpdir(), "aoe2-gamedetect-"));
	});

	after(async () => {
		await rm(fixtureDir, { recursive: true, force: true });
	});

	// One Steam root fixture at <fixtureDir>/<...name>: real steamapps dir,
	// optional vdf listing real library paths, optional game with optional
	// resource folders. Everything below is real on-disk layout.
	async function makeSteamRoot(name: string[], opts: {
		vdfPaths?: string[];
		game?: boolean;
		resources?: boolean;
	}): Promise<string> {
		const root = path.join(fixtureDir, ...name);
		const steamapps = path.join(root, "steamapps");
		await mkdir(steamapps, { recursive: true });
		if (opts.vdfPaths !== undefined) {
			const entries = opts.vdfPaths
				.map((p) => `\t{\n\t\t"path"\t\t"${p.replaceAll("\\", "\\\\")}"\n\t}`)
				.join("\n");
			await writeFile(
				path.join(steamapps, "libraryfolders.vdf"),
				`"libraryfolders"\n{\n${entries}\n}\n`,
			);
		}
		if (opts.game) {
			const common = path.join(steamapps, "common", "AoE2DE");
			if (opts.resources) {
				for (const sub of ["random-map-scripts", "xs"]) {
					await mkdir(path.join(common, "resources", "_common", sub), { recursive: true });
				}
			} else {
				await mkdir(common, { recursive: true });
			}
		}
		return root;
	}

	// Real filesystem everywhere except the platform identity and the
	// Program Files roots: the same seam shape extension.ts wires.
	function envFor(platform: NodeJS.Platform, opts?: {
		programFiles?: string;
		programFilesX86?: string;
	}): SteamEnv {
		return {
			platform,
			home: fixtureDir,
			programFiles: opts?.programFiles,
			programFilesX86: opts?.programFilesX86,
			exists: async (p) => {
				try {
					await stat(p);
					return true;
				} catch {
					return false;
				}
			},
			readTextFile: (p) => readFile(p, "utf8"),
		};
	}

	const resourceRootsOf = (steamRoot: string): string[] => [
		path.join(steamRoot, "steamapps", "common", "AoE2DE", "resources", "_common", "random-map-scripts"),
		path.join(steamRoot, "steamapps", "common", "AoE2DE", "resources", "_common", "xs"),
	];

	it("finds the game under a standard Windows Steam root", async () => {
		const pf86 = path.join(fixtureDir, "pf86");
		const steam = await makeSteamRoot(["pf86", "Steam"], { game: true, resources: true });
		const env = envFor("win32", { programFilesX86: pf86 });

		assert.deepEqual(await detectGameIncludeRoots(env), resourceRootsOf(steam));
	});

	it("follows libraryfolders.vdf to a game on a secondary library", async () => {
		const pf86 = path.join(fixtureDir, "pf86b");
		const secondary = await makeSteamRoot(["library", "SteamLibrary"], { game: true, resources: true });
		await makeSteamRoot(["pf86b", "Steam"], { game: false, vdfPaths: [secondary] });
		const env = envFor("win32", { programFilesX86: pf86 });

		assert.deepEqual(
			await detectGameIncludeRoots(env),
			resourceRootsOf(secondary),
			"the standard root has no game; the vdf must lead to the secondary library",
		);
	});

	it("prefers the standard root over a vdf-listed secondary copy", async () => {
		const pf86 = path.join(fixtureDir, "pf86c");
		const secondary = await makeSteamRoot(["library", "SteamLibraryToo"], { game: true, resources: true });
		const primary = await makeSteamRoot(["pf86c", "Steam"], { game: true, resources: true, vdfPaths: [secondary] });
		const env = envFor("win32", { programFilesX86: pf86 });

		assert.deepEqual(await detectGameIncludeRoots(env), resourceRootsOf(primary));
	});

	it("falls through a game install without the resource folders", async () => {
		const pf86 = path.join(fixtureDir, "pf86d");
		const secondary = await makeSteamRoot(["library", "SteamLibraryBare"], { game: true, resources: true });
		await makeSteamRoot(["pf86d", "Steam"], { game: true, resources: false, vdfPaths: [secondary] });
		const env = envFor("win32", { programFilesX86: pf86 });

		assert.deepEqual(
			await detectGameIncludeRoots(env),
			resourceRootsOf(secondary),
			"a partial install must not shadow a complete one",
		);
	});

	it("stays silent when no library holds the game", async () => {
		await makeSteamRoot([".steam", "steam"], { game: false });
		await makeSteamRoot([".local", "share", "Steam"], { game: false });
		const env = envFor("linux");

		assert.deepEqual(await detectGameIncludeRoots(env), []);
	});

	it("scans the per-platform standard Steam roots", async () => {
		const pf86 = path.join(fixtureDir, "pf86e");
		const winSteam = await makeSteamRoot(["pf86e", "Steam"], {});
		const dotSteam = await makeSteamRoot([".steam", "steam"], {});
		const localSteam = await makeSteamRoot([".local", "share", "Steam"], {});

		assert.deepEqual(
			await steamLibraries(envFor("win32", { programFilesX86: pf86 })),
			[winSteam],
		);
		assert.deepEqual(await steamLibraries(envFor("linux")), [dotSteam, localSteam]);
		// A missing standard root is dropped silently.
		assert.deepEqual(await steamLibraries(envFor("darwin")), []);
		assert.deepEqual(await steamLibraries(envFor("freebsd")), []);
	});
});
