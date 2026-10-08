# Termbelt

**One terminal. Fewer tabs.** Your daily developer tool belt, with a searchable terminal workbench and 21 direct commands. Built with Go, Bubble Tea v2 and Lip Gloss v2.

## Install

Run it directly with Bun 1.4.2 or later:

```sh
bunx --bun termbelt
bunx --bun termbelt ip
```

Or install globally with Node.js 22.15 or later:

```sh
npm install -g termbelt
termbelt
```

Both use the npm registry. Only the native package for your operating system and CPU is installed; no install scripts or additional downloads are required. Keep optional dependencies enabled. `bun add -g termbelt` also installs the command; its Node shebang requires Node when invoked directly, so Bun-only installations should use `bunx --bun`.

For a runtime-free installation, download the matching archive and `SHA256SUMS` from [GitHub Releases](https://github.com/rahatsagor/termbelt/releases), verify its checksum, and put the executable on your PATH. The native executable needs no Go, Bun, Node, or browser. Supported targets are macOS, Linux, and Windows on x64 and arm64. Linux binaries are statically linked; both glibc and musl systems are supported.

Build from source with Go 1.26 or later:

```sh
go install github.com/rahatsagor/termbelt@latest
```

## Quick start

```sh
termbelt                         # Open the interactive workbench
termbelt speed                   # Fast.com / Netflix download speed
termbelt speed --provider cloudflare  # Download + upload
termbelt ip                      # Your public IP and network
termbelt ip 8.8.8.8               # Another IP; hostnames work too
termbelt domains myidea           # Popular TLDs
termbelt domains myidea --all      # Every IANA root-zone TLD
termbelt whois example.com        # Registry + registrar registration data
termbelt site github.com          # Public endpoint owner, CDN and technology evidence
```

## The workbench

Type to search, use **↑ / ↓** to select a tool, and **Enter** to open it. **Tab / Shift+Tab** switches categories on the launcher and fields in a form. **Ctrl+F** toggles a favorite. The recent category remembers tool names only for the current session.

Use **Ctrl+R** to run any form. Enter also runs single-line fields; Enter inserts a line in multiline editors. JSON, JWT, Base64, hashing and regex support multiline paste. **Esc** cancels a running job and returns to its form. **Ctrl+C / Ctrl+Q** quits and restores the terminal.

On results: **↑ / ↓ / PageUp / PageDown** scroll, **c** copies, **j** toggles structured JSON, **e** edits inputs, **r** runs again, and **Esc** returns to the launcher. Clipboard support uses `pbcopy` on macOS, `wl-copy` / `xclip` on Linux, or `clip.exe` on Windows.

The layout adapts to the terminal. Wide windows show a tool preview; narrow windows show a compact list and keep the focused form field visible. Minimum size is 42 columns × 16 rows; 100 × 32 or larger gives the most comfortable layout.

Workbench multiline fields accept up to 1,048,576 characters; other fields accept 4,096. Oversized terminal paste is rejected with a message, and execution stays blocked until the input is edited. Use CLI stdin for larger text, and `hash --file` or piped stdin for byte-exact checksums. Terminal text editors normalize pasted control characters such as tabs and line endings. Returning to the launcher releases local input and result buffers.

## Commands

| Command | What it does | Example |
| --- | --- | --- |
| `speed` | Fast.com download; Cloudflare download + upload | `termbelt speed --provider cloudflare --duration 10` |
| `ip` | Public IP, country, city, timezone, ISP, organization, ASN | `termbelt ip 1.1.1.1` |
| `domains` | Authoritative registration checks across TLDs | `termbelt domains myidea --tlds com,io,dev,co.uk` |
| `whois` | Registrar, dates, status, nameservers, DNSSEC, public contacts | `termbelt whois example.com --json` |
| `site` | DNS, public network owner, hosting/CDN clues, headers, HTML technology evidence | `termbelt site https://github.com` |
| `dns` | A, AAAA, MX, NS, TXT, CNAME, SOA, CAA | `termbelt dns example.com --type MX --resolver 1.1.1.1` |
| `tls` | Certificate chain, issuer, validity, trust, SANs, fingerprints, cipher | `termbelt tls example.com:443` |
| `http` | HTTP status, redirects, headers, DNS/TCP/TLS/TTFB/total timings | `termbelt http https://example.com` |
| `ping` | Repeated TCP connection latency and failure rate | `termbelt ping example.com:443 --count 10` |
| `ports` | Which local processes listen on TCP ports | `termbelt ports --port 3000` |
| `json` | Validate, format, minify, extract dotted paths | `termbelt json --path users.0.name < data.json` |
| `base64` | Standard / URL-safe Base64 text encode and decode | `printf hello \| termbelt base64 --raw` |
| `url` | Encode, decode, inspect URL components | `termbelt url 'https://example.com?a=1' --mode inspect` |
| `jwt` | Decode header, claims, timestamps and expiration locally | `termbelt jwt < token.txt` |
| `hash` | SHA-256, SHA-512, SHA-1, MD5; stream large files | `termbelt hash --file archive.zip --algorithm sha256` |
| `uuid` | Secure random version 4 UUIDs | `termbelt uuid --count 5 --raw` |
| `password` | Secure random passwords with required character classes | `termbelt password --length 32 --no-symbols --raw` |
| `time` | Exact Unix s / ms / us / ns, fractional timestamps, dates and timezones | `termbelt time 1735689600123456789 --unit ns` |
| `cron` | Preview scheduled runs with timezone / daylight-saving rules | `termbelt cron '*/15 * * * *' --zone Asia/Dhaka` |
| `regex` | Go / RE2 matches, byte ranges, numbered / named captures | `termbelt regex '(?P<word>[a-z]+)' 'hello 123 world'` |
| `cidr` | IPv4 / IPv6 ranges, masks, counts, membership | `termbelt cidr 192.168.1.0/24 --contains 192.168.1.42` |

Run `termbelt COMMAND --help` for flags.

## Scripting

Every tool supports `--json`: a stable envelope with tool name, title, output, tables, metrics, notes, original data and elapsed time. `--raw` emits the transformed text, or JSON data when no text output exists. These two output modes are mutually exclusive. `--plain` and `NO_COLOR=1` disable colors in commands and the workbench. Non-terminal output is plain by default. Progress goes to stderr and is disabled for JSON / raw output.

```sh
termbelt ip --json > network.json
termbelt domains myidea --all --json > domains.json
termbelt uuid --count 10 --raw
termbelt json --minify --raw < data.json
printf '%s' 'hello' | termbelt hash --raw
termbelt base64 --decode < encoded.txt
termbelt completion zsh > _termbelt
```

Local text transformations accept up to 16 MiB on stdin. Hashes stream both stdin and regular files with bounded memory and support cancellation; use `hash --file` for a file or pipe a stream into `hash`. Inputs are treated as one quoted argument when passed on the command line. Negative timestamps can be passed after `--`, for example `termbelt time -- -1234567890`. Timestamp units auto-detect by magnitude; use `--unit s`, `ms`, `us`, or `ns` for historical or small values. Fractional timestamps preserve exact nanoseconds, including negative fractions.

JSON path syntax uses dot-separated object keys and numeric array indices, such as `users.0.name`; keys containing literal dots are not addressable through this simple path syntax. JSON numeric values are preserved without floating-point rounding. Base64 decoding of non-UTF-8 binary data displays hexadecimal. URL encoding escapes a single component; `--mode inspect` inspects an entire URL. JWTs are decoded **without signature verification** and their claims are explicitly marked untrusted.

## Provider behavior and limits

**Speed:** Fast.com mode discovers the current public token from Fast.com's script and measures download throughput against Netflix CDN targets. Cloudflare mode measures download and upload using its public speed-test endpoints. By default each direction runs for up to 8 seconds or 128 MiB of payload. `--duration 2–30`, `--max-mb 1–1024`, and `--download-only` control the test. Protocol headers, discovery, and latency requests add a small amount of data outside that payload budget. Rate is aggregate average HTTPS throughput across up to three connections, including when testing with a small byte limit. Upload requests adapt to connection throughput and reuse random payload buffers. JSON includes stopping reasons and a `partial` indicator; server failures also appear in notes. Tests that hit their download budget in under two seconds recommend a larger cap for stability. HTTP latency is measured before transfers; it includes server response time. These measurements are estimates, and this CLI does not reproduce a provider's complete browser algorithm or measure packet loss / loaded latency. Provider API changes produce a clear error.

**Domains:** Termbelt uses [IANA's RDAP bootstrap](https://www.iana.org/assignments/rdap-dns) to contact the authoritative registry. Registered records must match the requested domain. A valid authoritative not-found response means **unregistered**. Empty RDAP 404 responses such as Verisign's are accepted only when the response has the RDAP media type and remains on the authoritative service. Unsupported TLDs, rate limits, redirects to unrelated services, malformed responses and timeouts remain **unknown**.

An unregistered domain can be reserved, premium, restricted or blocked by registry policy. Confirm purchasability and price with a registrar. `--all` enumerates [every root-zone TLD published by IANA](https://data.iana.org/TLD/tlds-alpha-by-domain.txt), including brand, restricted and internationalized TLDs; it can take several minutes. Supply multi-label suffixes such as `co.uk`, `com.au` or `com.bd` explicitly with `--tlds`; `--all` and `--tlds` are mutually exclusive. Full domains are checked exactly; subdomains prompt you to use the registrable domain. Requests are bounded to six workers by default, interleaved across providers, and paced per registry host. The request deadline starts after pacing. A registry's HTTP 429 pauses further requests to that host for its bounded `Retry-After` window, preserving unknown outcomes instead of repeatedly hitting the rate limit. `--only-unregistered` filters the display while preserving all results in JSON data. `--refresh` updates registry metadata immediately even when the cache cannot be written. Invalid cached metadata falls back to the bundled snapshot.

**WHOIS:** Authoritative registry RDAP is preferred, with a bounded related registrar lookup for additional public contact data. When RDAP is inconclusive or unsupported, the tool discovers the registry WHOIS server through IANA and queries TCP port 43. That legacy protocol is unencrypted and can be blocked by a network. Redacted contact details cannot be recovered by this tool.

**Website intelligence:** Public IP ownership and hosting/CDN/technology labels are backed by the shown DNS, HTTP headers and HTML asset evidence. A reverse proxy or CDN can hide the origin provider. Labels are inferences, not definitive origin-host attribution.

**IP intelligence:** Uses HTTPS `ipwho.is` with HTTPS `free.freeipapi.com` as fallback. These anonymous providers can impose quotas or availability limits. Private and local IP addresses are classified locally without sending them to an API. Geolocation is approximate.

**TLS:** The inspection connection reads the presented chain even if it is expired or untrusted, then explicitly verifies system trust and hostname matching and reports the result. Normal HTTP / API requests retain certificate validation.

**Ports:** Uses the system's `lsof` on macOS, or `ss` on Linux. Shows listeners visible to your user account; it does not kill processes. This command is currently unsupported on Windows. Windows DNS lookups require `--resolver`, for example `--resolver 1.1.1.1`.

Provider references: [Cloudflare speed-test engine](https://github.com/cloudflare/speedtest), [IPWhois HTTPS API](https://ipwhois.io/documentation), [ICANN Lookup / RDAP](https://lookup.icann.org/).

## Custom IP API

An optional compatible API can provide `GET /me` and `GET /ip/:ip`. Responses must be JSON objects with an `ip` string; optional fields include `city`, `region`, `country`, `countryCode`, `continent`, `latitude`, `longitude`, `timezone`, `postal`, `isp`, `org`, and `asn`. Connect its base URL:

```sh
termbelt config set ip-api-url https://ip-api.example.com
termbelt ip
```

You can also use `TERMBELT_IP_API_URL` or pass `--ip-api-url` for a single invocation. A configured API is used explicitly; a failure is reported without silently switching to another provider. Clear it with `termbelt config set ip-api-url ''`.

## Privacy and configuration

Local utilities run entirely on your machine. There is no application telemetry. Termbelt never saves JWTs, text inputs, generated passwords, queried domains or lookup results. Favorites are saved to the OS user config directory (`~/Library/Application Support/termbelt/config.json` on macOS) with private permissions. Only public IANA metadata is cached in the OS cache directory. Network utilities contact the provider / target shown in the result and disclose the requested public domain or IP as needed for the lookup.

`termbelt config` shows the effective settings and location. `TERMBELT_CONFIG` and `TERMBELT_CACHE_DIR` override storage paths. Saving favorites preserves settings from disk, so temporary environment and command-line overrides are not accidentally saved. The default HTTP request timeout is 20 seconds; use `--timeout 3–120` to override it or `termbelt config set timeout-seconds 30` to save a default. Domain requests have an 8-second deadline after pacing. Values from network responses and errors are escaped before terminal rendering so they cannot inject terminal-control commands.

## Development

Go 1.26 or later builds the native executable. Go dependencies are locked by `go.mod` and `go.sum`. Bun 1.4.2 and Python 3.9 or later are used by package and release tooling. Node.js 22.15 or later is needed to test the npm launcher. No JavaScript dependencies need to be installed to build from source.

```sh
make build
make check                      # Tests with race detector + go vet
make smoke                      # Offline executable smoke checks
make edge-smoke                 # Large stdin, cancellation, malformed-input checks
make fuzz                       # Bounded domain and timestamp parser fuzzing
make vuln                       # Current Go vulnerability database scan
make live-smoke                 # Real provider / DNS / TLS / speed checks
python3 scripts/tui_smoke.py --binary build/termbelt # PTY interaction checks on Unix
make install                    # ~/.local/bin/termbelt; add ~/.local/bin to PATH
bun run build:release           # Six native binaries, platform packages, archives
bun run pack                    # Verify and pack npm tarballs
bun run test:package            # Isolated installs and Node/Bun launcher checks
```

Override the install directory with `make install PREFIX=/another/bin`. The build embeds timezone and initial IANA registry metadata. CI tests the native executable and package installation on macOS, Linux, and Windows. The release workflow builds all artifacts from the tagged source, publishes platform packages before the launcher, and uses npm trusted publishing with provenance. CI invokes the pinned npm OIDC client through `bunx`. All seven npm packages must configure the same trusted GitHub workflow before automated publication. An initial authenticated publication can use `bun run publish`; retries verify already published tarball integrity before continuing.

## License

[MIT](LICENSE). Bundled registry snapshots come from [IANA's RDAP bootstrap](https://data.iana.org/rdap/dns.json) and [root-zone TLD list](https://data.iana.org/TLD/tlds-alpha-by-domain.txt).
