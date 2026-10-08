#!/usr/bin/env python3
"""Verify input integrity, cancellation and output failures in the actual CLI."""
import argparse
import hashlib
import json
import os
import pathlib
import signal
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument("--binary", default="build/termbelt")
args = parser.parse_args()
binary = str(pathlib.Path(args.binary).resolve())

with tempfile.TemporaryDirectory(prefix="termbelt-edge-") as temporary:
    env = dict(os.environ, TERMBELT_CONFIG=temporary + "/config.json", TERMBELT_CACHE_DIR=temporary + "/cache", TERMBELT_IP_API_URL="")

    def run(command, data=None, succeeds=True):
        result = subprocess.run([binary, *command], input=data, capture_output=True, env=env, timeout=10)
        assert (result.returncode == 0) == succeeds, f"{command}: unexpected exit {result.returncode}: {result.stderr!r}"
        return result

    payload = (b"exact\tbytes\x00\r\n" * (17 * 1024 * 1024 // 14 + 1))[:17 * 1024 * 1024]
    result = run(["hash", "--raw"], payload)
    assert result.stdout.strip().decode() == hashlib.sha256(payload).hexdigest(), "large pipe changed bytes"
    print("PASS 17 MiB piped binary hash retains exact bytes", flush=True)

    for command in (["uuid", "unexpected"], ["uuid", "--json", "--raw"], ["domains", "brand", "--all", "--tlds", "com"], ["hash", "text", "--file", "missing"]):
        run(command, succeeds=False)
    print("PASS invalid arguments and conflicting flags return failure", flush=True)

    result = run(["time", "--unit", "s", "--json", "--", "-0.000000001"])
    data = json.loads(result.stdout)["data"]
    assert data["unix_nanoseconds"] == -1 and data["utc"] == "1969-12-31T23:59:59.999999999Z", "negative fraction lost precision"
    print("PASS exact negative fractional timestamp", flush=True)

    result = run(["unknown\x1b]52;c;not-a-clipboard-command\x07"], succeeds=False)
    assert b"\x1b" not in result.stderr and b"\x07" not in result.stderr, "error output contains terminal control bytes"
    print("PASS errors escape terminal control input", flush=True)

    if os.name == "posix":
        for command in ("hash", "json"):
            process = subprocess.Popen([binary, command, "--raw"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
            try:
                # Keep stdin open and empty; cancellation must interrupt the blocked read.
                time.sleep(0.15)
                process.send_signal(signal.SIGINT)
                process.wait(timeout=3)
                assert process.returncode != 0, f"{command} cancellation reported success"
                assert b"canceled" in process.stderr.read(), f"{command} cancellation was not reported"
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()
                for stream in (process.stdin, process.stdout, process.stderr):
                    stream.close()
        print("PASS Ctrl+C interrupts blocked hash and JSON stdin", flush=True)

print("Edge smoke checks passed", flush=True)
