import * as esbuild from "esbuild";

// Bundle the extension into a single CJS file: VS Code loads
// dist/extension.js with plain node require, no bundler at runtime.
await esbuild.build({
	entryPoints: ["src/extension.ts"],
	bundle: true,
	outfile: "dist/extension.js",
	platform: "node",
	format: "cjs",
	target: "node18",
	external: ["vscode"],
	minify: true,
	sourcemap: true,
});
