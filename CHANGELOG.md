# Changelog

## 1.1.0

- Domain checks fall back to the registry's legacy WHOIS server for TLDs without RDAP, so `.io`, `.co`, `.me`, `.sh` and about 160 other TLDs report real results instead of unknown. WHOIS replies are classified conservatively.
- `tls` and `ping` accept URLs, and host:port inputs with internationalized names.
- JSON formatting keeps the original key order and HTML characters; paths support `["dotted.key"]` and negative indices; `--raw` prints extracted strings unquoted.
- Every CLI option is available in the workbench, with on/off options as Space-toggled checkboxes.
- `ports` works on Windows and returns structured port, process, PID and address data on every platform.
- DNS uses the system resolver on Windows, performs reverse lookups for IP input, keeps TXT values exact and supports any record type.
- TLS shows the presented chain, key type, signature algorithm, ALPN and expiry warnings.
- Speed tests report median latency on a warm connection plus jitter, and no longer under-report upload throughput.
- Hashes add SHA-384, SHA-224, `--algorithm all` and `--verify`.
- `cidr` accepts bare addresses and shows wildcard masks and host ranges; JWT reports not-yet-valid and unsigned tokens.
- A damaged settings file no longer blocks every command; added `config reset`, `config path` and `config set favorites`.
- Network tools, `url`, `cidr` and `cron` read piped input; `url` drops the trailing newline from `echo`.
- Legacy WHOIS output no longer shows escaped carriage returns; text wraps at word boundaries; `site` reports CNAMEs only when present and recognizes more hosting platforms and frameworks.
- Non-public ranges such as CGNAT and documentation addresses are classified locally; PowerShell completion; OSC 52 and `xsel` clipboard fallbacks; requests identify the running version.

## 1.0.1

- Include Go and third-party license notices in every npm package and native archive.
- Publish native packages as a batch before the launcher, with integrity-checked retries and bounded registry processing waits.
- Automate releases through the repository-scoped npm trusted publisher workflow.

## 1.0.0

- Searchable terminal workbench and 21 utilities for networking, domains, and everyday development.
- Script-friendly JSON and raw output, streaming file hashes, precise timestamps, and shell completions.
- Bun/npm distribution with prebuilt binaries for macOS, Linux, and Windows on x64 and arm64.
- Local transformations without telemetry, private settings, bounded requests, and cancellable operations.
