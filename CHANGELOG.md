# Changelog

## 1.2.1

- The workbench no longer stops responding to every key, including Ctrl+C, after the Mac sleeps. It reads keys from `/dev/tty` instead of polling stdin with kqueue, which can stop reporting a terminal as readable. If terminal input ever closes, the workbench exits and restores the terminal instead of hanging.
- The hash, Base64 and regex editors reject pasted tabs and carriage returns, which the editor would turn into spaces and newlines and so change the result. Use CLI stdin for exact input.
- The workbench JSON view escapes bidirectional control characters, like the other views.
- An error result no longer offers `c copy` and `j JSON`, which did nothing there.
- `time` reads a piped timestamp; it used to ignore stdin and print the current time.
- Commands that need input fail with a hint when run with no argument and nothing piped, instead of hashing or encoding an empty string.
- A command stopped with Ctrl+C or SIGTERM exits with status 130 and prints `canceled`.
- The regex summary reads "1,000+ matches" when results are capped.
- `dns` tries the next system resolver when the first one does not respond, such as a VPN resolver left behind after the network changes.
- Removing a favorite no longer rewrites other copies of the favorites list, and the RDAP metadata cache is flushed to disk before it replaces the old file.
## 1.2.0

- Install with Homebrew on macOS and Linux: `brew install rahatsagor/tap/termbelt`, with bash, zsh and fish completions. Releases update the tap formula automatically.
- `bunx termbelt` and `npx termbelt` work on any Node.js 18 or later: the launcher falls back to a child process with forwarded signals when `process.execve` is unavailable. `--bun` is no longer needed.
- New logo: a terminal window whose title bar and block cursor form a T. The workbench header draws it too.
- `dns` accepts underscore names such as `_dmarc.example.com` and `_sip._tcp.example.com`, so SRV, DMARC and DKIM lookups work.
- `http`, `site`, `tls` and `ping` accept bracketed IPv6 URLs such as `https://[2001:db8::1]:8443/`.
- `ports` shows IPv6 wildcard listeners as `::` on macOS and no longer mangles scoped IPv6 addresses from `ss` on Linux.
- Workbench multiline editors no longer stop accepting Enter after 99 lines, and reject pastes the editor would otherwise truncate (over 10,000 lines, or over the limit after tab expansion).
- Copy over SSH uses the terminal clipboard (OSC 52), falls back to it when a clipboard tool fails, and only uses `xclip` / `xsel` / `wl-copy` with a display. Windows copies non-ASCII text correctly.
- Saving a favorite repairs a damaged settings file instead of failing.
- `json --raw` prints an empty line for an extracted empty string instead of `""`.
- JWT status reports "not yet valid" for future `nbf` tokens without `exp`, and "invalid expiry" for a non-numeric `exp`.
- WHOIS disclaimers such as "this service is free of charge" no longer mark a registered domain as unregistered.
- `domains --all` interleaves WHOIS-only TLDs by WHOIS server, and the speed-test data-limit hint appears only when the limit was actually reached.
- Bidirectional control characters in network responses are escaped before rendering.
- The workbench preview wraps at word boundaries and uses the full width; form hints, progress and errors wrap at words too.
- Release archives build with Python 3.9.
- Updated Bubble Tea to 2.1.0, golang.org/x/net to 0.61.0, x/sys to 0.49.0, x/term to 0.47.0 and charmbracelet/x/ansi to 0.11.9.

## 1.1.1

- Build with Go 1.27.2 and golang.org/x/net 0.60.0 for GO-2026-6617 (HTTP/2 HPACK encoder race).
- Includes every change listed for 1.1.0, which was tagged but not published because the release vulnerability scan stopped it.

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
