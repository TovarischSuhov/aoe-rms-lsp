// Steam installation detection for the VS Code extension (issue #123,
// slot of .goga/history/2026/ux-wave-2/task.md).
//
// Pure Node module: no vscode import, every filesystem touch goes through
// the SteamEnv seam so the unit tests drive real tmpdir fixtures. The
// layout facts live in the aoe2-game-paths cook
// (.goga/usages/cooks/aoe2-game-paths.md) — verified against a live
// Steam install on 2026-11-19.

import { readFile, stat } from "node:fs/promises";
import { homedir } from "node:os";
import * as path from "node:path";

// Steam apps live under <library>/steamapps/common/<GAME_DIR>; the
// include-worthy resource folders hang off resources/_common (NOT the
// resources_common spelling that circulated in older docs — the live
// install has no such directory).
export const GAME_DIR = "AoE2DE";
export const RESOURCE_ROOTS = [
	"resources/_common/random-map-scripts",
	"resources/_common/xs",
];

const LIBRARY_FOLDERS_VDF = ["steamapps", "libraryfolders.vdf"].join("/");

// The only filesystem surface this module needs. extension.ts wires the
// real implementations; tests pass node:fs/promises over tmp fixtures.
export interface SteamEnv {
	platform: NodeJS.Platform;
	home: string;
	programFiles: string | undefined;
	programFilesX86: string | undefined;
	exists(p: string): Promise<boolean>;
	readTextFile(p: string): Promise<string>;
}

// Headless defaults over the real filesystem.
export function realSteamEnv(): SteamEnv {
	return {
		platform: process.platform,
		home: homedir(),
		programFiles: process.env["ProgramFiles"],
		programFilesX86: process.env["ProgramFiles(x86)"],
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

// Standard per-platform Steam roots, those that exist on disk. Windows
// ships Steam under both Program Files flavors depending on the installer
// era; the registry is deliberately not consulted (no extra deps).
export async function steamLibraries(env: SteamEnv): Promise<string[]> {
	let candidates: string[];
	switch (env.platform) {
		case "win32":
			candidates = [
				env.programFilesX86 && path.join(env.programFilesX86, "Steam"),
				env.programFiles && path.join(env.programFiles, "Steam"),
			].filter((p): p is string => Boolean(p));
			break;
		case "darwin":
			candidates = [path.join(env.home, "Library", "Application Support", "Steam")];
			break;
		case "linux":
			candidates = [
				path.join(env.home, ".steam", "steam"),
				path.join(env.home, ".local", "share", "Steam"),
			];
			break;
		default:
			return [];
	}

	const found: string[] = [];
	const seen = new Set<string>();
	for (const dir of candidates) {
		// Case-insensitive dedupe: one Windows machine can list the same
		// root through both Program Files flavors with differing case.
		const key = dir.toLowerCase();
		if (seen.has(key)) continue;
		if (await env.exists(dir)) {
			seen.add(key);
			found.push(dir);
		}
	}
	return found;
}

// Parse the modern libraryfolders.vdf: quoted "path" values carry doubled
// backslashes on Windows. The legacy direct-number format ("1" "D:\\...")
// predates 2020 and is not parsed. Values unescape \\ to \; forward
// slashes pass through untouched.
export function parseLibraryFolders(vdf: string): string[] {
	const out: string[] = [];
	const re = /"path"\s+"((?:[^"\\]|\\.)*)"/g;
	for (let m = re.exec(vdf); m !== null; m = re.exec(vdf)) {
		out.push(m[1].replace(/\\\\/g, "\\"));
	}
	return out;
}

// Every plausible library root: the standard Steam dir itself plus the
// paths listed in its libraryfolders.vdf (a missing vdf is normal —
// single-library installs still have one, extra ones only via vdf).
async function candidateLibraries(env: SteamEnv): Promise<string[]> {
	const bases = await steamLibraries(env);
	const out: string[] = [];
	const seen = new Set<string>();
	const push = (dir: string): void => {
		const key = dir.toLowerCase();
		if (!seen.has(key)) {
			seen.add(key);
			out.push(dir);
		}
	};

	for (const base of bases) {
		push(base);
		try {
			for (const lib of parseLibraryFolders(await env.readTextFile(path.join(base, "steamapps", "libraryfolders.vdf")))) {
				push(lib);
			}
		} catch {
			// no vdf or unreadable — the standard root alone stays a candidate
		}
	}
	return out;
}

// First existing AoE2DE install wins: standard roots are scanned before
// vdf-listed extra libraries, so a default install outranks a stale
// secondary copy on another drive. Roots that do not exist on disk are
// dropped (a partial install must not poison include resolution).
// No install → empty list: the caller stays silent.
export async function detectGameIncludeRoots(env: SteamEnv): Promise<string[]> {
	for (const lib of await candidateLibraries(env)) {
		const game = path.join(lib, "steamapps", "common", GAME_DIR);
		if (!(await env.exists(game))) continue;

		const roots: string[] = [];
		for (const sub of RESOURCE_ROOTS) {
			const dir = path.join(game, ...sub.split("/"));
			if (await env.exists(dir)) {
				roots.push(dir);
			}
		}
		if (roots.length > 0) {
			return roots;
		}
	}
	return [];
}
