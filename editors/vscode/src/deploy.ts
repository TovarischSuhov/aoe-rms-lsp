// Map deployment to the game's mod folder (issue #83, slot of
// .goga/history/2026/ux-wave-2/task.md).
//
// Pure Node module: no vscode import, every filesystem touch goes through
// the DeployEnv seam (the src/gamedetect.ts pattern) so the unit tests
// run against real tmpdir fixtures. The mod folder layout lives in the
// aoe2-game-paths cook (.goga/usages/cooks/aoe2-game-paths.md) — verified
// against a live install on 2026-11-19.

import { copyFile, mkdir, readFile, readdir, stat } from "node:fs/promises";
import { homedir } from "node:os";
import * as path from "node:path";

// The only filesystem surface this module needs. extension.ts wires the
// real implementations; tests pass node:fs/promises over tmp fixtures.
export interface DeployEnv {
	home: string;
	exists(p: string): Promise<boolean>;
	readTextFile(p: string): Promise<string>;
	readDir(p: string): Promise<string[]>;
	ensureDir(p: string): Promise<void>;
	copyFile(src: string, dst: string): Promise<void>;
}

export function realDeployEnv(): DeployEnv {
	return {
		home: homedir(),
		exists: async (p) => {
			try {
				await stat(p);
				return true;
			} catch {
				return false;
			}
		},
		readTextFile: (p) => readFile(p, "utf8"),
		readDir: (p) => readdir(p),
		ensureDir: async (p) => {
			await mkdir(p, { recursive: true });
		},
		copyFile: (src, dst) => copyFile(src, dst),
	};
}

// Directive lines in .rms/.xs text: #include, #includeXS and #include_drs
// (quoted or bare path; for #include_drs the trailing legacy archive id
// is not part of the path). Comment lines never match — directives start
// the line.
const INCLUDE_LINE = /^[ \t]*#include(?:_drs|XS)?[ \t]+(?:"([^"]+)"|([^\s"']+))/gm;

// scanIncludes extracts the include paths as written, in document order,
// deduplicated.
export function scanIncludes(text: string): string[] {
	const out: string[] = [];
	const seen = new Set<string>();
	for (let m = INCLUDE_LINE.exec(text); m !== null; m = INCLUDE_LINE.exec(text)) {
		const p = m[1] ?? m[2];
		if (!seen.has(p)) {
			seen.add(p);
			out.push(p);
		}
	}
	INCLUDE_LINE.lastIndex = 0;
	return out;
}

// collectDependencies walks the include graph from the map file:
// every directive resolves against the including file's directory; only
// files inside the map's own directory tree are collected (deps served
// from the game install — includeRoots territory — must not be copied;
// ../-escapes and missing targets are skipped silently, mirroring the
// resolver's MissingInclude). Cycles terminate on the visit set. The map
// itself is the first entry.
export async function collectDependencies(mapPath: string, env: DeployEnv): Promise<string[]> {
	const mapDir = path.dirname(mapPath);
	const out: string[] = [mapPath];
	const seen = new Set<string>([path.normalize(mapPath)]);

	const walk = async (file: string): Promise<void> => {
		let text: string;
		try {
			text = await env.readTextFile(file);
		} catch {
			return;
		}

		for (const inc of scanIncludes(text)) {
			const target = path.resolve(path.dirname(file), inc);
			const key = path.normalize(target);
			if (seen.has(key)) continue;
			seen.add(key);

			// Only the map's own tree is deployed; anything resolved
			// elsewhere (game roots, ../-escapes) stays where it is.
			if (!key.startsWith(path.normalize(mapDir) + path.sep)) continue;
			if (!(await env.exists(target))) continue;

			out.push(target);
			await walk(target);
		}
	};

	await walk(mapPath);
	return out;
}

// deployMap copies the map and its local dependencies into targetDir,
// recreating the relative directory structure (a dep at <mapdir>/sub/x
// lands at <targetDir>/sub/x so relative include resolution survives the
// move). Existing files are overwritten — deploy means "the map I am
// editing wins". Returns the number of files written.
export async function deployMap(mapPath: string, targetDir: string, env: DeployEnv): Promise<number> {
	const files = await collectDependencies(mapPath, env);
	const mapDir = path.dirname(mapPath);

	await env.ensureDir(targetDir);

	for (const file of files) {
		const rel = path.relative(mapDir, file);
		const dst = path.join(targetDir, rel);
		await env.ensureDir(path.dirname(dst));
		await env.copyFile(file, dst);
	}

	return files.length;
}

// detectDeployTarget finds the game's local-mod random-map-scripts folder
// under %USERPROFILE%\Games\Age of Empires 2 DE\<account>\mods\local\<mod>\
// resources\_common\random-map-scripts. One candidate wins; zero or many
// (several accounts or mods) need the explicit aoe2lsp.deploy.targetDir
// setting — auto-detect refuses to guess. Missing intermediate dirs are
// normal (no local mods installed).
export async function detectDeployTarget(env: DeployEnv): Promise<string | undefined> {
	const accountsRoot = path.join(env.home, "Games", "Age of Empires 2 DE");
	const found: string[] = [];

	let accounts: string[];
	try {
		accounts = await env.readDir(accountsRoot);
	} catch {
		return undefined;
	}

	for (const account of accounts) {
		const localMods = path.join(accountsRoot, account, "mods", "local");
		let mods: string[];
		try {
			mods = await env.readDir(localMods);
		} catch {
			continue;
		}

		for (const mod of mods) {
			const target = path.join(localMods, mod, "resources", "_common", "random-map-scripts");
			if (await env.exists(target)) {
				found.push(target);
			}
		}
	}

	if (found.length === 1) {
		return found[0];
	}

	return undefined;
}
