#!/usr/bin/env bun
"use strict";
const { createHash } = require("node:crypto");
const { existsSync, readFileSync, renameSync, writeFileSync } = require("node:fs");
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
const receiptPath = join(root, "dist", "packages", ".publication-receipts.json");
const receipts = existsSync(receiptPath) ? JSON.parse(readFileSync(receiptPath, "utf8")) : {};
const packages = platforms.map(p => ({ name: packageName(p), filename: `rahatsagor-termbelt-${p.os}-${p.cpu}-${version}.tgz` }));
packages.push({ name: "termbelt", filename: `termbelt-${version}.tgz` });
for (const target of packages) {
  target.path = join(root, "dist", "packages", target.filename);
  target.integrity = `sha512-${createHash("sha512").update(readFileSync(target.path)).digest("base64")}`;
  target.key = `${target.name}@${version}`;
}
function accepted(target) {
  receipts[target.key] = target.integrity;
  const temporary = receiptPath + ".tmp";
  writeFileSync(temporary, JSON.stringify(receipts, null, 2) + "\n", { mode: 0o600 });
  renameSync(temporary, receiptPath);
}
async function published(name) {
  const response = await fetch(`https://registry.npmjs.org/${encodeURIComponent(name)}/${version}`, {
    headers: { "Cache-Control": "no-cache" }, signal: AbortSignal.timeout(20_000),
  });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`registry verification returned HTTP ${response.status} for ${name}`);
  return response.json();
}
function matching(target, metadata) {
  if (metadata.dist?.integrity !== target.integrity) {
    throw new Error(`${target.key} exists with different contents; published versions are immutable. Choose a new version.`);
  }
}
async function publish(target) {
  const existing = await published(target.name);
  if (existing) {
    matching(target, existing);
    console.log(`Verified existing ${target.key}; continuing`);
    return;
  }
  if (receipts[target.key] === target.integrity) {
    console.log(`Resuming accepted upload ${target.key}`);
    return;
  }
  const args = ci
    ? ["x", "npm@12.2.0", "publish", target.path, "--access", "public", "--provenance"]
    : ["publish", target.path, "--access", "public", "--tolerate-republish"];
  const result = spawnSync(process.execPath, args, { cwd: root, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`publication stopped at ${target.name}; rerun after resolving the reported error`);
  accepted(target);
}
async function confirm(targets) {
  const pending = new Map(targets.map(target => [target.key, target]));
  const deadline = performance.now() + 15 * 60_000;
  let waiting = false;
  while (pending.size) {
    await Promise.all([...pending.values()].map(async target => {
      const metadata = await published(target.name);
      if (!metadata) return;
      matching(target, metadata);
      pending.delete(target.key);
      console.log(`Published and verified ${target.key}`);
    }));
    if (!pending.size) return;
    if (!waiting) console.log(`Waiting for npm to process ${pending.size} package(s); accepted uploads are recorded for retries`);
    waiting = true;
    if (performance.now() >= deadline) throw new Error(`npm is still processing ${[...pending.keys()].join(", ")}; rerun to resume verification`);
    await Bun.sleep(10_000);
  }
}
async function main() {
  const native = packages.slice(0, -1);
  for (const target of native) await publish(target);
  await confirm(native);
  const launcher = packages.at(-1);
  await publish(launcher);
  await confirm([launcher]);
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
