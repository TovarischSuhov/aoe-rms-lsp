// Unit tests for src/deploy.ts — map deployment to the game's mod folder
// (issue #83, slot of .goga/history/2026/ux-wave-2/task.md).
//
// Stock node:test runner, zero npm dependencies: Node's type stripping
// executes .ts directly (Node >= 22.18 locally and in CI). Fixtures are
// real tmpdir trees; the module touches the filesystem only through the
// DeployEnv seam (the src/gamedetect.ts pattern).

import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
	collectDependencies,
	deployMap,
	detectDeployTarget,
	realDeployEnv,
	scanIncludes,
	type DeployEnv,
} from "../src/deploy.ts";

describe("scanIncludes", () => {
	it("reads quoted and bare paths from the three directive forms", () => {
		const src = [
			"#include \"shared.rms\"",
			"#include_drs random_map.def 54000",
			"#includeXS \"Constants.xs\"",
			"  #include sub/extra.rms",
		].join("\n");
		assert.deepEqual(scanIncludes(src), ["shared.rms", "random_map.def", "Constants.xs", "sub/extra.rms"]);
	});

	it("ignores directives not at line start and deduplicates", () => {
		const src = "/* #include \"in-comment.rms\" */\nx = #include \"not-directive.rms\"\n#include \"a.rms\"\n#include \"a.rms\"\n";
		assert.deepEqual(scanIncludes(src), ["a.rms"]);
	});
});

describe("collectDependencies and deployMap", () => {
	let fixtureDir: string;
	let mapDir: string;

	before(async () => {
		fixtureDir = await mkdtemp(path.join(tmpdir(), "aoe2-deploy-"));
		mapDir = path.join(fixtureDir, "src");
		await mkdir(path.join(mapDir, "inc"), { recursive: true });

		// map.rms → inc/first.rms (→ sibling second.rms, cycle back to
		// first.rms), inc/only.xs, missing.rms (not written) and
		// Constants.xs — exists OUTSIDE the map dir (the game install
		// shape) and must not be collected.
		await writeFile(path.join(mapDir, "map.rms"),
			"#include \"inc/first.rms\"\n#include \"missing.rms\"\n#includeXS \"inc/only.xs\"\n#include \"../Constants.xs\"\n");
		await writeFile(path.join(mapDir, "inc", "first.rms"),
			"#include \"second.rms\"\n#include \"first.rms\"\n");
		await writeFile(path.join(mapDir, "inc", "second.rms"),
			"#include \"Constants.xs\"\n");
		await writeFile(path.join(mapDir, "inc", "only.xs"), "#include \"first.rms\"\n");
		await writeFile(path.join(fixtureDir, "Constants.xs"), "const int cGame = 1;\n");
	});

	after(async () => {
		await rm(fixtureDir, { recursive: true, force: true });
	});

	it("walks the include graph: nested deps, dedup, cycles, missing and outside-map targets skipped", async () => {
		const deps = await collectDependencies(path.join(mapDir, "map.rms"), realDeployEnv());
		assert.deepEqual(deps, [
			path.join(mapDir, "map.rms"),
			path.join(mapDir, "inc", "first.rms"),
			path.join(mapDir, "inc", "second.rms"),
			path.join(mapDir, "inc", "only.xs"),
		]);
	});

	it("copies the map and deps preserving the relative structure", async () => {
		const targetDir = path.join(fixtureDir, "deployed", "random-map-scripts");
		const count = await deployMap(path.join(mapDir, "map.rms"), targetDir, realDeployEnv());

		assert.equal(count, 4);
		const copied = await readFile(path.join(targetDir, "map.rms"), "utf8");
		assert.ok(copied.startsWith("#include \"inc/first.rms\""));
		await stat(path.join(targetDir, "inc", "first.rms"));
		await stat(path.join(targetDir, "inc", "second.rms"));
		await stat(path.join(targetDir, "inc", "only.xs"));
		await assert.rejects(stat(path.join(targetDir, "Constants.xs")), "outside-the-map files are not copied");
	});
});

describe("detectDeployTarget", () => {
	let fixtureDir: string;

	before(async () => {
		fixtureDir = await mkdtemp(path.join(tmpdir(), "aoe2-deploy-detect-"));
	});

	after(async () => {
		await rm(fixtureDir, { recursive: true, force: true });
	});

	// A DeployEnv whose Games tree is synthesized from the list of
	// existing random-map-scripts folders: the account and mod levels are
	// derived from those paths, the leaf check is real membership.
	function envWithRmsScripts(rmsScripts: string[]): DeployEnv {
		const env = realDeployEnv();
		const accountsRoot = path.join(fixtureDir, "Games", "Age of Empires 2 DE");
		return {
			...env,
			home: fixtureDir,
			exists: async (p) => rmsScripts.includes(p),
			readDir: async (p) => {
				if (p === accountsRoot) {
					return [...new Set(rmsScripts.map((r) => path.relative(accountsRoot, r).split(path.sep)[0]))];
				}
				if (p.startsWith(accountsRoot + path.sep) && p.endsWith(path.join("mods", "local"))) {
					return [...new Set(
						rmsScripts
							.filter((r) => r.startsWith(p + path.sep))
							.map((r) => path.relative(p, r).split(path.sep)[0]),
					)];
				}
				return [];
			},
		};
	}

	const rmsOf = (account: string, mod: string): string =>
		path.join(fixtureDir, "Games", "Age of Empires 2 DE", account, "mods", "local", mod, "resources", "_common", "random-map-scripts");

	it("returns the single local mod's random-map-scripts folder", async () => {
		const target = rmsOf("76561198060636836", "my-map-pack");
		assert.equal(await detectDeployTarget(envWithRmsScripts([target])), target);
	});

	it("refuses to guess with zero or several candidates", async () => {
		assert.equal(await detectDeployTarget(envWithRmsScripts([])), undefined);

		const env = envWithRmsScripts([
			rmsOf("76561198060636836", "map-pack-a"),
			rmsOf("76561198060636836", "map-pack-b"),
		]);
		assert.equal(await detectDeployTarget(env), undefined, "several local mods need the explicit setting");
	});

	it("stays silent when the game root is missing", async () => {
		const env = { ...realDeployEnv(), home: path.join(fixtureDir, "empty") };
		assert.equal(await detectDeployTarget(env), undefined);
	});
});
