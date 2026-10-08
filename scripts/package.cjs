#!/usr/bin/env bun
"use strict";
const { chmodSync, copyFileSync, mkdirSync, writeFileSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");
const { platforms, current } = require("../npm/platforms.cjs");
const { verify } = require("./verify.cjs");
const root = join(__dirname, "..");
const currentOnly = process.argv.includes("--current");
if (process.argv.slice(2).some(arg => arg !== "--current")) throw new Error("usage: bun scripts/package.cjs [--current]");
verify(currentOnly);
if (!currentOnly) {
  const result = spawnSync(process.env.PYTHON || (process.platform === "win32" ? "python" : "python3"), ["scripts/archives.py", "--verify"], { cwd: root, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error("release archive verification failed");
}
const destination = join(root, "dist", "packages");
mkdirSync(destination, { recursive: true });
const main = join(root, "dist", "npm", "termbelt");
mkdirSync(join(main, "npm"), { recursive: true });
for (const file of ["README.md", "LICENSE", "THIRD_PARTY_NOTICES", "npm/termbelt.cjs", "npm/platforms.cjs"]) copyFileSync(join(root, file), join(main, file));
chmodSync(join(main, "npm", "termbelt.cjs"), 0o755);
const metadata = { ...require("../package.json") };
delete metadata.scripts;
delete metadata.packageManager;
writeFileSync(join(main, "package.json"), JSON.stringify(metadata, null, 2) + "\n");
for (const directory of [main, ...(currentOnly ? [current()] : platforms).map(p => join(root, "dist", "npm", `termbelt-${p.os}-${p.cpu}`))]) {
  const result = spawnSync(process.execPath, ["pm", "pack", "--ignore-scripts", "--destination", destination], { cwd: directory, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error("bun pm pack failed");
}
