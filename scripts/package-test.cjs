#!/usr/bin/env bun
"use strict";
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { createServer } = require("node:http");
const { chmodSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } = require("node:fs");
const { tmpdir } = require("node:os");
const { join } = require("node:path");
const { spawn } = require("node:child_process");
const { platforms, current, packageName, executable } = require("../npm/platforms.cjs");
const root = join(__dirname, "..");
const metadata = require("../package.json");
const platform = current();
const temporary = mkdtempSync(join(tmpdir(), "termbelt-package-"));
const downloaded = new Set();
const tarballs = new Map();
for (const filename of readdirSync(join(root, "dist", "packages"))) {
  if (filename.endsWith(".tgz")) tarballs.set(filename, readFileSync(join(root, "dist", "packages", filename)));
}
const mainFilename = `termbelt-${metadata.version}.tgz`;
assert(tarballs.has(mainFilename), "run bun run pack before package tests");
const tarballName = p => `rahatsagor-termbelt-${p.os}-${p.cpu}-${metadata.version}.tgz`;
const server = createServer((request, response) => {
  const path = decodeURIComponent(new URL(request.url, "http://localhost").pathname).slice(1);
  if (path.startsWith("tarballs/")) {
    const filename = path.slice(9);
    const data = tarballs.get(filename);
    if (!data) { response.writeHead(404).end(); return; }
    downloaded.add(filename);
    response.writeHead(200, { "content-type": "application/octet-stream", "content-length": data.length });
    response.end(data);
    return;
  }
  const target = platforms.find(p => packageName(p) === path);
  if (path !== "termbelt" && !target) { response.writeHead(404).end(); return; }
  const filename = target ? tarballName(target) : mainFilename;
  const data = tarballs.get(filename);
  const manifest = target ? {
    name: packageName(target), version: metadata.version, os: [target.os], cpu: [target.cpu],
  } : metadata;
  const dist = { tarball: `http://127.0.0.1:${server.address().port}/tarballs/${filename}` };
  if (data) {
    dist.shasum = createHash("sha1").update(data).digest("hex");
    dist.integrity = `sha512-${createHash("sha512").update(data).digest("base64")}`;
  }
  response.writeHead(200, { "content-type": "application/json" });
  response.end(JSON.stringify({ name: path, "dist-tags": { latest: metadata.version }, versions: { [metadata.version]: { ...manifest, dist } } }));
});
function run(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: temporary, ...options });
    const stdout = [];
    const stderr = [];
    child.stdout?.on("data", data => stdout.push(data));
    child.stderr?.on("data", data => stderr.push(data));
    const timeout = setTimeout(() => { child.kill("SIGKILL"); reject(new Error(`${command} timed out`)); }, 120_000);
    child.once("error", error => { clearTimeout(timeout); reject(error); });
    child.once("close", (code, signal) => {
      clearTimeout(timeout);
      resolve({ code, signal, stdout: Buffer.concat(stdout).toString(), stderr: Buffer.concat(stderr).toString() });
    });
    child.stdin?.end(options.input);
  });
}
function success(result) {
  assert.equal(result.code, 0, result.stderr || result.stdout);
  return result.stdout;
}
async function main() {
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const registry = `http://127.0.0.1:${server.address().port}`;
  writeFileSync(join(temporary, "package.json"), '{"name":"termbelt-install-test","private":true}\n');
  writeFileSync(join(temporary, ".npmrc"), `registry=${registry}\n@rahatsagor:registry=${registry}\n`);
  const environment = { ...process.env, TERMBELT_CONFIG: join(temporary, "config.json"), TERMBELT_CACHE_DIR: join(temporary, "cache"), TERMBELT_IP_API_URL: "" };
  success(await run(process.execPath, ["add", "--ignore-scripts", "--registry", registry, "--cache-dir", join(temporary, "bun-cache"), `termbelt@${metadata.version}`], { env: environment }));
  assert.deepEqual([...downloaded].sort(), [mainFilename, tarballName(platform)].sort(), "installation downloaded a foreign-platform binary");
  console.log("PASS clean Bun install downloads only launcher and current native package; no install scripts");
  const installed = join(temporary, "node_modules", "termbelt", "npm", "termbelt.cjs");
  const packageRoot = join(temporary, "node_modules", "termbelt");
  assert.deepEqual(readdirSync(packageRoot).sort(), ["LICENSE", "README.md", "THIRD_PARTY_NOTICES", "npm", "package.json"]);
  assert.deepEqual(readdirSync(join(packageRoot, "npm")).sort(), ["platforms.cjs", "termbelt.cjs"]);
  for (const runtime of [process.execPath, process.env.NODE_BINARY || "node"]) {
    assert.equal(success(await run(runtime, [installed, "--version"], { env: environment })).trim(), `termbelt version ${metadata.version}`);
    const json = JSON.parse(success(await run(runtime, [installed, "json", "--path", "id", "--json"], { env: environment, input: '{"id":9007199254740993}' })));
    assert.equal(json.output, "9007199254740993");
    assert.equal(success(await run(runtime, [installed, "base64", "space ; $ and ' quote", "--raw"], { env: environment })).trim(), Buffer.from("space ; $ and ' quote").toString("base64"));
    const invalid = await run(runtime, [installed, "does-not-exist"], { env: environment });
    assert.equal(invalid.code, 1);
    assert.match(invalid.stderr, /termbelt:/);
    if (process.platform !== "win32") {
      const wrapper = join(temporary, runtime === process.execPath ? "bun-launcher" : "node-launcher");
      const quote = value => "'" + value.replace(/'/g, "'\\''") + "'";
      writeFileSync(wrapper, `#!/bin/sh\nexec ${quote(runtime)} ${quote(installed)} "$@"\n`);
      chmodSync(wrapper, 0o755);
      success(await run(process.env.PYTHON || "python3", [join(root, "scripts", "edge_smoke.py"), "--binary", wrapper], { env: environment }));
      success(await run(process.env.PYTHON || "python3", [join(root, "scripts", "tui_smoke.py"), "--binary", wrapper, "--plain"], { env: environment }));
      console.log(`PASS ${runtime}: exact stdin, cancellation, TUI interactions, resize and terminal restoration`);
    } else {
      console.log(`PASS ${runtime}: version, piped input, arguments and failure status`);
    }
  }
  const nativeDirectory = join(temporary, "node_modules", "@rahatsagor", `termbelt-${platform.os}-${platform.cpu}`);
  for (const directory of [packageRoot, nativeDirectory]) assert.equal(readFileSync(join(directory, "THIRD_PARTY_NOTICES"), "utf8"), readFileSync(join(root, "THIRD_PARTY_NOTICES"), "utf8"));
  const manifestPath = join(nativeDirectory, "package.json");
  const originalManifest = readFileSync(manifestPath);
  writeFileSync(manifestPath, JSON.stringify({ ...JSON.parse(originalManifest), version: "0.0.0" }));
  const mismatch = await run(process.execPath, [installed, "uuid"], { env: environment });
  assert.equal(mismatch.code, 1);
  assert.match(mismatch.stderr, /version does not match/);
  writeFileSync(manifestPath, originalManifest);
  if (process.platform !== "win32") {
    const native = join(nativeDirectory, "bin", executable(platform));
    chmodSync(native, 0o644);
    const permissions = await run(process.execPath, [installed, "uuid"], { env: environment });
    assert.equal(permissions.code, 1);
    assert.match(permissions.stderr, /not executable/);
    chmodSync(native, 0o755);
  }
  rmSync(nativeDirectory, { recursive: true });
  const missing = await run(process.execPath, [installed, "uuid"], { env: environment });
  assert.equal(missing.code, 1);
  assert.match(missing.stderr, /optional dependencies enabled/);
  console.log("PASS missing package, mismatched versions and executable permissions produce actionable errors");
}
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => {
  server.close();
  rmSync(temporary, { recursive: true, force: true });
});
