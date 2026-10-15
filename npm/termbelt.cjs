#!/usr/bin/env node
"use strict";
const { statSync } = require("node:fs");
const { dirname, join } = require("node:path");
const { spawn } = require("node:child_process");
const { constants } = require("node:os");
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
  if (process.platform !== "win32" && typeof process.execve === "function") {
    // Replace the runtime while preserving PID, foreground terminal and signals.
    process.execve(binary, [binary, ...args], process.env);
    return;
  }
  // Older Node.js releases and Windows have no execve; run the binary as a child instead.
  const child = spawn(binary, args, { stdio: "inherit", windowsHide: false });
  // Windows delivers Ctrl+C to both console processes, and kill() would terminate the child
  // instead of notifying it. On Unix, forwarding a signal the terminal already delivered is
  // harmless: the executable keeps listening until it exits.
  const forward = signal => () => { if (child.exitCode === null && child.signalCode === null) child.kill(signal); };
  const handlers = process.platform === "win32"
    ? new Map([["SIGINT", () => {}], ["SIGTERM", forward("SIGTERM")]])
    : new Map(["SIGINT", "SIGTERM", "SIGHUP", "SIGQUIT", "SIGWINCH"].map(signal => [signal, forward(signal)]));
  for (const [signal, handler] of handlers) process.on(signal, handler);
  const cleanup = () => { for (const [signal, handler] of handlers) process.removeListener(signal, handler); };
  child.once("error", error => { cleanup(); fail(error); });
  child.once("exit", (code, signal) => {
    cleanup();
    process.exitCode = code ?? (signal && constants.signals[signal] ? 128 + constants.signals[signal] : 1);
  });
}
try { run(); } catch (error) { fail(error); }
