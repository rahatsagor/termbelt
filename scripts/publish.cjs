#!/usr/bin/env bun
"use strict";
const { createHash } = require("node:crypto");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");
const { platforms, packageName } = require("../npm/platforms.cjs");
const { verify } = require("./verify.cjs");
const root = join(__dirname, "..");
const version = require("../package.json").version;
const ci = process.argv.includes("--ci");
if (process.argv.slice(2).some(arg => arg !== "--ci")) throw new Error("usage: bun scripts/publish.cjs [--ci]");
if (ci && process.env.GITHUB_ACTIONS !== "true") throw new Error("--ci requires GitHub Actions trusted publishing");
if (process.platform === "win32") throw new Error("Publish all platforms from the GitHub release workflow or a Unix host to preserve executable permissions.");
verify();
const packages = platforms.map(p => ({ name: packageName(p), filename: `rahatsagor-termbelt-${p.os}-${p.cpu}-${version}.tgz` }));
packages.push({ name: "termbelt", filename: `termbelt-${version}.tgz` });
async function published(name) {
  const response = await fetch(`https://registry.npmjs.org/${encodeURIComponent(name)}/${version}`, {
    headers: { "Cache-Control": "no-cache" }, signal: AbortSignal.timeout(20_000),
  });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`registry verification returned HTTP ${response.status} for ${name}`);
  return response.json();
}
async function main() {
  for (const { name, filename } of packages) {
    const path = join(root, "dist", "packages", filename);
    const expected = `sha512-${createHash("sha512").update(readFileSync(path)).digest("base64")}`;
    const existing = await published(name);
    if (existing) {
      if (existing.dist?.integrity !== expected) throw new Error(`${name}@${version} already exists with different contents; published versions are immutable. Choose a new version.`);
      console.log(`Verified existing ${name}@${version}; continuing`);
      continue;
    }
    const args = ci
      ? ["x", "npm@12.2.0", "publish", path, "--access", "public", "--provenance"]
      : ["publish", path, "--access", "public"];
    const result = spawnSync(process.execPath, args, { cwd: root, stdio: "inherit" });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(`publication stopped at ${name}; rerun after resolving the reported error`);
    let confirmed = false;
    for (let attempt = 0; attempt < 6; attempt++) {
      const result = await published(name);
      if (result) {
        if (result.dist?.integrity !== expected) throw new Error(`published integrity mismatch for ${name}`);
        confirmed = true;
        break;
      }
      await new Promise(resolve => setTimeout(resolve, Math.min(1000 * 2 ** attempt, 8000)));
    }
    if (!confirmed) throw new Error(`${name} publication is not visible yet; rerun to verify before continuing`);
    console.log(`Published and verified ${name}@${version}`);
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
