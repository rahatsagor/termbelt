#!/usr/bin/env bun
"use strict";
// Prints the Homebrew formula for the release described by a SHA256SUMS file.
// Usage: bun scripts/homebrew.cjs [dist/releases/SHA256SUMS]
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const root = join(__dirname, "..");
const sums = process.argv[2] || join(root, "dist", "releases", "SHA256SUMS");
if (process.argv.length > 3) throw new Error("usage: bun scripts/homebrew.cjs [SHA256SUMS]");
const targets = ["darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"];
const digests = new Map();
let version;
for (const line of readFileSync(sums, "utf8").trim().split(/\r?\n/)) {
  const match = /^([a-f0-9]{64})  termbelt_(\d+\.\d+\.\d+)_([a-z]+_[a-z0-9]+)\.(?:tar\.gz|zip)$/.exec(line);
  if (!match) throw new Error(`unexpected SHA256SUMS entry: ${line}`);
  if (version && version !== match[2]) throw new Error("SHA256SUMS lists more than one version");
  version = match[2];
  digests.set(match[3], match[1]);
}
let formula = readFileSync(join(root, "packaging", "homebrew", "termbelt.rb"), "utf8")
  .split("\n").filter(line => !line.startsWith("# ")).join("\n")
  .replaceAll("{{VERSION}}", version);
for (const target of targets) {
  if (!digests.has(target)) throw new Error(`SHA256SUMS has no ${target} archive`);
  formula = formula.replaceAll(`{{SHA256_${target.toUpperCase()}}}`, digests.get(target));
}
if (formula.includes("{{")) throw new Error("formula template has an unfilled placeholder");
process.stdout.write(formula);
