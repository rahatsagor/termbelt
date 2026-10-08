#!/usr/bin/env bun
"use strict";

const { spawnSync } = require("node:child_process");
const { readFileSync, writeFileSync, existsSync } = require("node:fs");
const path = require("node:path");

const goBinary = process.env.GO_BINARY || "go";
const check = process.argv.slice(2);
if (check.length > 1 || (check.length === 1 && check[0] !== "--check")) {
  console.error("Usage: bun scripts/notices.cjs [--check]");
  process.exit(2);
}

const targets = [
  ["darwin", "arm64"], ["darwin", "amd64"],
  ["linux", "arm64"], ["linux", "amd64"],
  ["windows", "arm64"], ["windows", "amd64"],
];
const mainPath = "github.com/rahatsagor/termbelt";
const modules = new Map();

for (const [GOOS, GOARCH] of targets) {
  const result = spawnSync(goBinary, ["list", "-deps", "-json", "./..."], {
    encoding: "utf8",
    env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: "0" },
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`go list failed for ${GOOS}/${GOARCH}: ${result.stderr.trim()}`);
  }
  // go list emits a stream of adjacent JSON objects, not a JSON array.
  let depth = 0;
  let quoted = false;
  let escaped = false;
  let start = -1;
  for (let i = 0; i < result.stdout.length; i++) {
    const ch = result.stdout[i];
    if (quoted) {
      if (escaped) escaped = false;
      else if (ch === "\\") escaped = true;
      else if (ch === '"') quoted = false;
      continue;
    }
    if (ch === '"') quoted = true;
    else if (ch === "{") {
      if (depth++ === 0) start = i;
    } else if (ch === "}" && --depth === 0) {
      const pkg = JSON.parse(result.stdout.slice(start, i + 1));
      const mod = pkg.Module;
      if (mod && mod.Path !== mainPath && mod.Dir) {
        const previous = modules.get(mod.Path);
        if (previous && previous.version !== mod.Version) {
          throw new Error(`conflicting versions for ${mod.Path}: ${previous.version} and ${mod.Version}`);
        }
        modules.set(mod.Path, { version: mod.Version, dir: mod.Dir });
      }
    }
  }
  if (depth !== 0) throw new Error(`could not parse go list output for ${GOOS}/${GOARCH}`);
}

const goEnv = spawnSync(goBinary, ["env", "GOROOT"], { encoding: "utf8" });
if (goEnv.status !== 0) throw new Error(`go env GOROOT failed: ${goEnv.stderr.trim()}`);
const goRoot = goEnv.stdout.trim();
const sdkFiles = new Map();
for (const name of ["LICENSE", "PATENTS"]) {
  const source = [goRoot, path.dirname(goRoot)].map(dir => path.join(dir, name)).find(existsSync);
  if (source) sdkFiles.set(name, source);
}
if (!sdkFiles.has("LICENSE")) throw new Error("Go SDK LICENSE not found in GOROOT or its parent");

function normalize(text) {
  return text.replace(/\r\n/g, "\n").replace(/\r/g, "\n").replace(/\s+$/, "") + "\n";
}
function asciiCompare(a, b) { return a < b ? -1 : a > b ? 1 : 0; }

const entries = [{ path: "Go standard library", version: "", files: [...sdkFiles.keys()], sources: sdkFiles }];
for (const [modulePath, mod] of [...modules].sort(([a], [b]) => asciiCompare(a, b))) {
  const names = ["LICENSE", "LICENCE", "COPYING", "LICENSE.txt"];
  const found = names.filter(name => existsSync(path.join(mod.dir, name)));
  if (!found.length) {
    throw new Error(`missing LICENSE, LICENCE, COPYING, or LICENSE.txt for ${modulePath}@${mod.version} (${mod.dir})`);
  }
  for (const optional of ["PATENTS", "NOTICE"]) {
    if (existsSync(path.join(mod.dir, optional))) found.push(optional);
  }
  entries.push({ path: modulePath, version: mod.version, dir: mod.dir, files: found });
}

const sections = [];
for (const entry of entries) {
  const title = entry.version ? `${entry.path} ${entry.version}` : entry.path;
  for (const file of entry.files) {
    sections.push(`${title} — ${file}\n${"=".repeat(title.length + file.length + 3)}\n\n${normalize(readFileSync(entry.sources?.get(file) || path.join(entry.dir, file), "utf8"))}`);
  }
}
const output = sections.join("\n");
const destination = path.resolve(__dirname, "../THIRD_PARTY_NOTICES");
if (check[0] === "--check") {
  if (!existsSync(destination) || readFileSync(destination, "utf8") !== output) {
    console.error("THIRD_PARTY_NOTICES is missing or out of date; run bun scripts/notices.cjs");
    process.exit(1);
  }
  console.log(`THIRD_PARTY_NOTICES is current (${modules.size} external modules)`);
} else {
  writeFileSync(destination, output);
  console.log(`Wrote THIRD_PARTY_NOTICES (${modules.size} external modules)`);
}
