"use strict";
const platforms = [
  { os: "darwin", cpu: "arm64", goos: "darwin", goarch: "arm64" },
  { os: "darwin", cpu: "x64", goos: "darwin", goarch: "amd64" },
  { os: "linux", cpu: "arm64", goos: "linux", goarch: "arm64" },
  { os: "linux", cpu: "x64", goos: "linux", goarch: "amd64" },
  { os: "win32", cpu: "arm64", goos: "windows", goarch: "arm64" },
  { os: "win32", cpu: "x64", goos: "windows", goarch: "amd64" },
];
function current() {
  const platform = platforms.find(p => p.os === process.platform && p.cpu === process.arch);
  if (!platform) throw new Error(`unsupported platform ${process.platform}/${process.arch}; supported: macOS, Linux, Windows on x64 or arm64`);
  return platform;
}
function packageName(platform) { return `@rahatsagor/termbelt-${platform.os}-${platform.cpu}`; }
function binaryName(platform) { return `termbelt_${platform.goos}_${platform.goarch}${platform.os === "win32" ? ".exe" : ""}`; }
function executable(platform) { return platform.os === "win32" ? "termbelt.exe" : "termbelt"; }
module.exports = { platforms, current, packageName, binaryName, executable };
