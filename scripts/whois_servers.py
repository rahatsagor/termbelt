#!/usr/bin/env python3
"""Refresh the bundled WHOIS server map for root-zone TLDs that publish no RDAP service.

Usage: python3 scripts/whois_servers.py
Reads internal/core/iana-rdap.json and internal/core/iana-tlds.txt, queries
whois.iana.org once per uncovered TLD, and writes internal/core/iana-whois.json
with the IANA servers plus the hand-checked supplemental servers below that
are still needed.
"""
import concurrent.futures
import json
import pathlib
import re
import socket
import time

root = pathlib.Path(__file__).resolve().parent.parent
core = root / "internal" / "core"
bootstrap = json.loads((core / "iana-rdap.json").read_text())
covered = {tld.lower() for service in bootstrap["services"] for tld in service[0]}
tlds = [line.strip().lower() for line in (core / "iana-tlds.txt").read_text().splitlines() if line.strip() and not line.startswith("#")]
missing = [tld for tld in tlds if tld not in covered and tld != "arpa"]
# Registry WHOIS servers that IANA does not list. Keys may be public suffixes
# such as co.za when the suffix has its own registry. Each entry was checked
# by hand with a registered and an unregistered name.
supplemental = {
    "cd": "whois.nic.cd",
    "co.za": "whois.registry.net.za",
    "ga": "whois.nic.ga",
    "kw": "whois.nic.kw",
    "net.za": "whois.registry.net.za",
    "org.za": "whois.registry.net.za",
    "ps": "whois.registry.ps",
    "sl": "whois.nic.sl",
    "web.za": "whois.registry.net.za",
}

hostname = re.compile(r"^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$")


def query(tld):
    for attempt in range(3):
        try:
            with socket.create_connection(("whois.iana.org", 43), timeout=10) as conn:
                conn.settimeout(15)
                conn.sendall(tld.encode("ascii") + b"\r\n")
                chunks = []
                while True:
                    chunk = conn.recv(65536)
                    if not chunk:
                        break
                    chunks.append(chunk)
            text = b"".join(chunks).decode("utf-8", "replace")
            for line in text.splitlines():
                key, _, value = line.partition(":")
                if key.strip().lower() == "whois":
                    server = value.strip().lower().rstrip(".")
                    return tld, server if hostname.match(server) else ""
            return tld, ""
        except OSError:
            time.sleep(1 + attempt)
    raise RuntimeError(f"whois.iana.org did not answer for .{tld}")


servers = {}
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
    for tld, server in pool.map(query, missing):
        if server:
            servers[tld] = server
extra = {suffix: server for suffix, server in supplemental.items() if suffix not in covered and suffix not in servers}
output = {
    "description": "Legacy WHOIS servers published by IANA for root-zone TLDs without an RDAP bootstrap entry",
    "rdap_publication": bootstrap.get("publication", ""),
    "servers": dict(sorted(servers.items())),
    "supplemental_description": "Registry WHOIS servers that IANA does not list, for TLDs and public suffixes without RDAP; checked by hand against each registry",
    "supplemental": dict(sorted(extra.items())),
}
(core / "iana-whois.json").write_text(json.dumps(output, indent=2) + "\n")
print(f"{len(missing)} TLDs without RDAP; {len(servers)} publish a WHOIS server; {len(extra)} supplemental servers")
