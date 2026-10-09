#!/usr/bin/env python3
"""Exercise the built executable. Live mode uses modest, bounded network requests."""
import argparse
import concurrent.futures
import json
import os
import pathlib
import subprocess
import tempfile
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("--binary", default="build/termbelt")
parser.add_argument("--live", action="store_true")
parser.add_argument("--all-tlds", action="store_true")
args = parser.parse_args()
binary = str(pathlib.Path(args.binary).resolve())
temp = tempfile.TemporaryDirectory(prefix="termbelt-smoke-")
env = dict(os.environ, TERMBELT_CONFIG=temp.name + "/config.json", TERMBELT_CACHE_DIR=temp.name + "/cache")

def run(label, command, predicate, stdin=None, timeout=60):
    result = subprocess.run([binary, *command, "--json"], input=stdin, capture_output=True, text=True, env=env, timeout=timeout)
    if result.returncode:
        raise AssertionError(f"{label}: {result.stderr.strip()}")
    data = json.loads(result.stdout)
    assert predicate(data), f"{label}: unexpected response: {data.get('summary', data.get('title'))}"
    return f"PASS {label} ({data['duration_ms']} ms)"

cases = [
    ("JSON pipe / precision", ["json", "--path", "id"], lambda d: d["output"] == "9007199254740993", '{"id":9007199254740993}'),
    ("Base64", ["base64", "aGVsbG8=", "--decode"], lambda d: d["output"] == "hello"),
    ("UUID", ["uuid", "--count", "3"], lambda d: len(d["data"]) == 3),
    ("Secure password", ["password", "--length", "32"], lambda d: len(d["data"][0]) == 32),
    ("IPv6 subnet", ["cidr", "2001:db8::/64"], lambda d: d["data"]["addresses"] == "18446744073709551616"),
    ("Cron / Dhaka", ["cron", "*/15 * * * *", "--zone", "Asia/Dhaka"], lambda d: len(d["data"]["next_runs"]) == 8),
    ("Local IP classification", ["ip", "192.168.1.1"], lambda d: d["data"]["source"] == "local classification"),
    ("JSON key order / HTML", ["json"], lambda d: d["output"].index('"z"') < d["output"].index('"a"') and "<b>" in d["output"], '{"z":1,"a":"<b>"}'),
    ("Bare address subnet", ["cidr", "10.0.0.1"], lambda d: d["data"]["network"] == "10.0.0.1/32"),
    ("All checksums", ["hash", "abc", "--algorithm", "all"], lambda d: len(d["data"]["hashes"]) == 6),
    ("Local port inspection", ["ports"], lambda d: d["title"] == "Local listening ports" and isinstance(d.get("data", []), list)),
]
if args.live:
    cases += [
        ("Public IP", ["ip"], lambda d: bool(d["data"]["ip"])),
        ("Specific IP / ASN", ["ip", "8.8.8.8"], lambda d: d["data"]["ip"] == "8.8.8.8" and bool(d["data"]["asn"])),
        ("Registered .com", ["domains", "example.com"], lambda d: d["data"]["counts"]["registered"] == 1),
        ("Unregistered .com", ["domains", "termbelt-smoke-" + uuid.uuid4().hex + ".com"], lambda d: d["data"]["counts"]["unregistered"] == 1),
        ("RDAP detail", ["whois", "example.com"], lambda d: d["data"]["objectClassName"] == "domain"),
        ("DNS / all record types", ["dns", "example.com"], lambda d: any(r["type"] == "A" for r in d["data"])),
        ("DNS / custom resolver", ["dns", "example.com", "--type", "A", "--resolver", "1.1.1.1"], lambda d: bool(d["data"])),
        ("TLS / trusted certificate", ["tls", "example.com"], lambda d: d["data"]["trusted"] is True),
        ("TLS / URL input", ["tls", "https://example.com"], lambda d: d["data"]["port"] == "443" and bool(d["data"]["chain"])),
        ("WHOIS fallback / registered .io", ["domains", "google.io"], lambda d: d["data"]["counts"]["registered"] == 1 and d["data"]["domains"][0]["protocol"] == "whois"),
        ("WHOIS fallback / unregistered .io", ["domains", "termbelt-smoke-" + uuid.uuid4().hex[:16] + ".io"], lambda d: d["data"]["counts"]["unregistered"] == 1),
        ("DNS / reverse lookup", ["dns", "1.1.1.1"], lambda d: any(r["type"] == "PTR" for r in d["data"])),
        ("HTTP / timings", ["http", "https://example.com"], lambda d: d["data"]["status"] == 200 and d["data"]["timings_ms"]["total"] > 0),
        ("Website / CDN evidence", ["site", "github.com"], lambda d: bool(d["data"]["addresses"]) and d["data"]["http"]["status"] == 200),
        ("TCP latency", ["ping", "example.com", "--count", "2"], lambda d: d["data"]["failed"] == 0),
        ("Fast.com / 8 MiB cap", ["speed", "--duration", "3", "--max-mb", "8"], lambda d: 0 < d["data"]["downloaded_bytes"] <= 8 * 1024 * 1024),
        ("Cloudflare / download + upload", ["speed", "--provider", "cloudflare", "--duration", "3", "--max-mb", "8"], lambda d: d["data"].get("upload_mbps", 0) > 0 and d["data"]["download_mbps"] > 0),
    ]
failures = []
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
    futures = [pool.submit(run, *case) for case in cases if not (args.live and case[0].startswith(("Fast.com", "Cloudflare")))]
    for future in concurrent.futures.as_completed(futures):
        try:
            print(future.result(), flush=True)
        except Exception as exc:
            failures.append(str(exc))
            print(f"FAIL {exc}", flush=True)
# Speed measurements run alone, so other smoke requests don't compete for bandwidth.
if args.live:
    for case in cases:
        if case[0].startswith(("Fast.com", "Cloudflare")):
            try:
                print(run(*case), flush=True)
            except Exception as exc:
                failures.append(str(exc))
                print(f"FAIL {exc}", flush=True)
if args.all_tlds:
    try:
        print(run("Every IANA TLD", ["domains", "termbelt-smoke-" + uuid.uuid4().hex[:12], "--all"], lambda d: len(d["data"]["domains"]) > 1000 and sum(d["data"]["counts"].values()) == len(d["data"]["domains"]), timeout=420), flush=True)
    except Exception as exc:
        failures.append(str(exc))
        print(f"FAIL {exc}", flush=True)
print(f"{len(cases) + int(args.all_tlds) - len(failures)} checks passed; {len(failures)} failed", flush=True)
temp.cleanup()
raise SystemExit(bool(failures))
