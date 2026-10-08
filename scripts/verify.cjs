#!/usr/bin/env bun
"use strict";
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { readFileSync, statSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");
const { platforms, current, packageName, binaryName, executable } = require("../npm/platforms.cjs");
const root = join(__dirname, "..");
const metadata = require("../package.json");
function checksum(contents) { return createHash("sha256").update(contents).digest("hex"); }
function checksums(directory, expected) {
  const entries = new Map();
  for (const line of readFileSync(join(directory, "SHA256SUMS"), "utf8").trim().split("\n")) {
    const match = /^([a-f0-9]{64})  ([A-Za-z0-9_.-]+)$/.exec(line);
    assert(match, "invalid checksum entry");
    assert(!entries.has(match[2]), "duplicate checksum entry");
    entries.set(match[2], match[1]);
  }
  assert.deepEqual([...entries.keys()].sort(), [...expected].sort(), "unexpected checksum file list");
  for (const [name, hash] of entries) assert.equal(checksum(readFileSync(join(directory, name))), hash, `checksum mismatch: ${name}`);
}
function architecture(data, platform) {
  assert(data.length > 1_000_000, "native executable is unexpectedly small");
  if (platform.os === "linux") {
    assert.equal(data.subarray(0, 6).toString("hex"), "7f454c460201", "expected little-endian 64-bit ELF");
    assert.equal(data.readUInt16LE(18), platform.cpu === "x64" ? 62 : 183, "wrong ELF architecture");
  } else if (platform.os === "darwin") {
    assert.equal(data.readUInt32LE(0), 0xfeedfacf, "expected Mach-O executable");
    assert.equal(data.readUInt32LE(4), platform.cpu === "x64" ? 0x01000007 : 0x0100000c, "wrong Mach-O architecture");
  } else {
    assert.equal(data.subarray(0, 2).toString(), "MZ", "expected PE executable");
    const offset = data.readUInt32LE(0x3c);
    assert(offset + 6 < data.length, "invalid PE header offset");
    assert.equal(data.subarray(offset, offset + 4).toString(), "PE\0\0", "invalid PE header");
    assert.equal(data.readUInt16LE(offset + 4), platform.cpu === "x64" ? 0x8664 : 0xaa64, "wrong PE architecture");
  }
}
function verify(currentOnly = false) {
  assert.equal(metadata.name, "termbelt");
  assert.match(metadata.version, /^\d+\.\d+\.\d+$/);
  assert.equal(metadata.license, "MIT");
  for (const name of ["preinstall", "install", "postinstall", "prepare"]) assert(!metadata.scripts?.[name], `install lifecycle script is not allowed: ${name}`);
  assert.deepEqual(metadata.files, ["npm/termbelt.cjs", "npm/platforms.cjs", "README.md", "LICENSE"]);
  assert(!metadata.dependencies && !metadata.devDependencies, "unexpected JavaScript dependencies");
  assert.deepEqual(Object.keys(metadata.optionalDependencies).sort(), platforms.map(packageName).sort());
  if (process.env.GITHUB_REF_TYPE === "tag") assert.equal(process.env.GITHUB_REF_NAME, `v${metadata.version}`, "tag and package version differ");
  for (const platform of currentOnly ? [current()] : platforms) {
    const name = packageName(platform);
    assert.equal(metadata.optionalDependencies[name], metadata.version, "native dependency version must be exact");
    const native = join(root, "dist", "native", binaryName(platform));
    const data = readFileSync(native);
    architecture(data, platform);
    // Windows filesystems do not expose POSIX permission bits; archive modes are checked separately.
    if (process.platform !== "win32" && platform.os !== "win32") assert(statSync(native).mode & 0o111, "native binary lacks executable permission");
    const directory = join(root, "dist", "npm", `termbelt-${platform.os}-${platform.cpu}`);
    const manifest = JSON.parse(readFileSync(join(directory, "package.json")));
    assert.equal(manifest.name, name);
    assert.equal(manifest.version, metadata.version);
    assert.deepEqual(manifest.os, [platform.os]);
    assert.deepEqual(manifest.cpu, [platform.cpu]);
    assert(!manifest.scripts && !manifest.dependencies && !manifest.optionalDependencies);
    assert.equal(checksum(readFileSync(join(directory, "bin", executable(platform)))), checksum(data));
  }
  const platform = current();
  const result = spawnSync(join(root, "dist", "native", binaryName(platform)), ["--version"], { encoding: "utf8" });
  assert.ifError(result.error);
  assert.equal(result.status, 0);
  assert.equal(result.stdout.trim(), `termbelt version ${metadata.version}`);
  if (!currentOnly) {
    checksums(join(root, "dist", "native"), platforms.map(binaryName));
    const archives = platforms.map(p => `termbelt_${metadata.version}_${p.goos}_${p.goarch}${p.os === "win32" ? ".zip" : ".tar.gz"}`);
    checksums(join(root, "dist", "releases"), archives);
  }
  console.log(`Verified Termbelt ${metadata.version}: ${currentOnly ? "current platform" : "six native targets, packages and checksums"}`);
}
module.exports = { verify };
if (require.main === module) {
  try { verify(process.argv.includes("--current")); }
  catch (error) { console.error(`Package verification failed: ${error.message}`); process.exitCode = 1; }
}
