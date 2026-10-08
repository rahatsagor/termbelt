#!/usr/bin/env node
"use strict";
const { statSync } = require("node:fs");
const { dirname, join } = require("node:path");
const { spawn } = require("node:child_process");
const { current, packageName, executable } = require("./platforms.cjs");
const version = require("../package.json").version;
function fail(error) {
  const message = String(error.message || error).replace(/[\x00-\x1f\x7f-\x9f]/g, " ");
  console.error(`termbelt: ${message}`);
  process.exitCode = 1;
}
function nativePath() {
  const platform = current();
  const name = packageName(platform);
  let manifest;
  try { manifest = require.resolve(`${name}/package.json`); }
  catch { throw new Error(`missing ${name}@${version}. Reinstall termbelt with optional dependencies enabled (do not use --omit=optional).`); }
  if (require(manifest).version !== version) throw new Error(`${name} version does not match termbelt ${version}; reinstall the package.`);
  const binary = join(dirname(manifest), "bin", executable(platform));
  let file;
  try { file = statSync(binary); }
  catch { throw new Error(`native executable is missing from ${name}; reinstall termbelt.`); }
  if (!file.isFile() || (process.platform !== "win32" && !(file.mode & 0o111))) throw new Error(`native executable in ${name} is not executable; reinstall termbelt.`);
  return binary;
}
function run() {
  const binary = nativePath();
  const args = process.argv.slice(2);
  if (process.platform !== "win32") {
    if (typeof process.execve !== "function") throw new Error("Node.js 22.15 or later is required. Bun users can run: bunx --bun termbelt");
    // Replace the runtime while preserving PID, foreground terminal and signals.
    process.execve(binary, [binary, ...args], process.env);
    return;
  }
  const child = spawn(binary, args, { stdio: "inherit", windowsHide: false });
  const interrupt = () => {}; // Windows delivers Ctrl+C to both console processes.
  const terminate = () => { if (child.exitCode === null) child.kill("SIGTERM"); };
  process.on("SIGINT", interrupt);
  process.on("SIGTERM", terminate);
  const cleanup = () => {
    process.removeListener("SIGINT", interrupt);
    process.removeListener("SIGTERM", terminate);
  };
  child.once("error", error => { cleanup(); fail(error); });
  child.once("exit", (code, signal) => {
    cleanup();
    process.exitCode = code ?? ({ SIGINT: 130, SIGTERM: 143 }[signal] || 1);
  });
}
try { run(); } catch (error) { fail(error); }
