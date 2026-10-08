#!/usr/bin/env bun
"use strict";
const { chmodSync, copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } = require("node:fs");
const { join } = require("node:path");
const { spawnSync } = require("node:child_process");
const { createHash } = require("node:crypto");
const { platforms, current, packageName, binaryName, executable } = require("../npm/platforms.cjs");
const root = join(__dirname, "..");
const metadata = require("../package.json");
const currentOnly = process.argv.includes("--current");
if (process.argv.slice(2).some(arg => arg !== "--current")) throw new Error("usage: bun scripts/build.cjs [--current]");
function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: root, stdio: "inherit", ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} exited with ${result.status}`);
}
run(process.execPath, ["scripts/notices.cjs", "--check"]);
if (!currentOnly) rmSync(join(root, "dist"), { recursive: true, force: true });
mkdirSync(join(root, "build"), { recursive: true });
const checksumLines = [];
for (const platform of currentOnly ? [current()] : platforms) {
  const nativeDirectory = join(root, "dist", "native");
  mkdirSync(nativeDirectory, { recursive: true });
  const output = join(nativeDirectory, binaryName(platform));
  console.log(`Building ${platform.goos}/${platform.goarch}`);
  run(process.env.GO_BINARY || "go", ["build", "-buildvcs=false", "-trimpath", `-ldflags=-s -w -buildid= -X main.version=${metadata.version}`, "-o", output, "."], {
    env: { ...process.env, CGO_ENABLED: "0", GOOS: platform.goos, GOARCH: platform.goarch },
  });
  if (platform.os !== "win32") chmodSync(output, 0o755);
  if (platform.os === process.platform && platform.cpu === process.arch) {
    copyFileSync(output, join(root, "build", executable(platform)));
    if (platform.os !== "win32") chmodSync(join(root, "build", executable(platform)), 0o755);
  }
  checksumLines.push(`${createHash("sha256").update(readFileSync(output)).digest("hex")}  ${binaryName(platform)}`);
  const directory = join(root, "dist", "npm", `termbelt-${platform.os}-${platform.cpu}`);
  mkdirSync(join(directory, "bin"), { recursive: true });
  copyFileSync(output, join(directory, "bin", executable(platform)));
  if (platform.os !== "win32") chmodSync(join(directory, "bin", executable(platform)), 0o755);
  for (const file of ["LICENSE", "THIRD_PARTY_NOTICES"]) copyFileSync(join(root, file), join(directory, file));
  writeFileSync(join(directory, "README.md"), `# ${packageName(platform)}\n\nNative ${platform.os}/${platform.cpu} executable for [Termbelt](https://github.com/rahatsagor/termbelt). Installed automatically by the \`termbelt\` package.\n`);
  writeFileSync(join(directory, "package.json"), JSON.stringify({
    name: packageName(platform), version: metadata.version,
    description: `Termbelt native executable for ${platform.os}/${platform.cpu}`,
    license: metadata.license, repository: metadata.repository,
    os: [platform.os], cpu: [platform.cpu], files: ["bin", "LICENSE", "README.md", "THIRD_PARTY_NOTICES"],
    publishConfig: metadata.publishConfig,
  }, null, 2) + "\n");
}
if (!currentOnly) {
  writeFileSync(join(root, "dist", "native", "SHA256SUMS"), checksumLines.join("\n") + "\n");
  run(process.env.PYTHON || (process.platform === "win32" ? "python" : "python3"), ["scripts/archives.py", metadata.version]);
}
