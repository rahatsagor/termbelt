<p align="center">
  <a href="https://termbelt.rsapplication.com">
    <img src="https://raw.githubusercontent.com/rahatsagor/termbelt/main/docs/media/icon.svg" width="96" height="96" alt="Termbelt icon: a terminal window whose title bar and block cursor form a T">
  </a>
</p>

<h1 align="center">Termbelt</h1>

<h3 align="center">One terminal. Fewer tabs.</h3>

<p align="center">
  21 everyday developer tools in one native binary: speed tests, DNS, TLS, WHOIS,<br>
  domain checks, ports, JSON, JWT, hashes, timestamps, cron and more.
</p>

<p align="center">
  <b>A searchable workbench when you want to explore. Direct commands when you want to script.</b>
</p>

<p align="center">
  <a href="https://www.npmjs.com/package/termbelt"><img src="https://img.shields.io/npm/v/termbelt?style=for-the-badge&logo=npm&label=npm&color=5951ed&labelColor=16182a" alt="npm version"></a>
  <a href="#homebrew"><img src="https://img.shields.io/badge/brew-rahatsagor%2Ftap-5951ed?style=for-the-badge&logo=homebrew&logoColor=white&labelColor=16182a" alt="Homebrew tap"></a>
  <a href="https://termbelt.rsapplication.com"><img src="https://img.shields.io/badge/Website-termbelt.rsapplication.com-16182a?style=for-the-badge&logo=safari&logoColor=white" alt="Website"></a>
</p>

<p align="center">
  <a href="https://github.com/rahatsagor/termbelt/releases/latest"><img src="https://img.shields.io/github/v/release/rahatsagor/termbelt?style=flat-square&label=release&color=c9f394&labelColor=1f1f1f" alt="Latest release"></a>
  <a href="https://github.com/rahatsagor/termbelt/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/rahatsagor/termbelt/ci.yml?branch=main&style=flat-square&label=CI&labelColor=1f1f1f" alt="CI status"></a>
  <a href="#requirements"><img src="https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-x64%20%2B%20ARM64-1f1f1f?style=flat-square" alt="macOS, Linux and Windows on x64 and ARM64"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/rahatsagor/termbelt?style=flat-square&logo=go&labelColor=1f1f1f&color=1f1f1f" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-1f1f1f?style=flat-square" alt="MIT License"></a>
</p>

<p align="center">
  <a href="https://termbelt.rsapplication.com">
    <img src="https://raw.githubusercontent.com/rahatsagor/termbelt/main/docs/media/workbench.svg" width="900" alt="The Termbelt workbench: a search box, category tabs, a list of tools with Internet speed selected, and a preview panel with its quick command.">
  </a>
</p>

<table>
  <tr>
    <td width="33%" valign="top">
      <b>1. Try it</b><br>
      <code>bunx termbelt</code><br>
      or <code>npx termbelt</code>. Nothing to install first.
    </td>
    <td width="33%" valign="top">
      <b>2. Keep it</b><br>
      <code>brew install rahatsagor/tap/termbelt</code><br>
      or <code>npm i -g termbelt</code>, or one file.
    </td>
    <td width="33%" valign="top">
      <b>3. Use it your way</b><br>
      <code>termbelt</code> opens the workbench.<br>
      <code>termbelt dns example.com</code> just answers.
    </td>
  </tr>
</table>

## A tool for every "one sec"

<table>
  <tr>
    <td width="33%" valign="top">
      <h4>Network</h4>
      <code>speed</code> Fast.com or Cloudflare speed test<br>
      <code>ip</code> public IP, location, ISP and ASN<br>
      <code>dns</code> any record type, custom resolver, PTR<br>
      <code>tls</code> certificate chain, trust and expiry<br>
      <code>http</code> status, redirects, headers, timings<br>
      <code>ping</code> repeated TCP connection latency<br>
      <code>ports</code> which process holds port 3000<br>
      <code>cidr</code> IPv4 / IPv6 subnet calculator
    </td>
    <td width="33%" valign="top">
      <h4>Domains</h4>
      <code>domains</code> registration status across TLDs, from authoritative RDAP and WHOIS<br>
      <code>whois</code> registrar, dates, status, nameservers, DNSSEC and public contacts<br>
      <code>site</code> DNS, network owner, CDN, hosting and technology evidence for a website
    </td>
    <td width="33%" valign="top">
      <h4>Developer</h4>
      <code>json</code> format, minify, extract paths<br>
      <code>base64</code> standard and URL-safe<br>
      <code>url</code> encode, decode, inspect<br>
      <code>jwt</code> decode claims and expiry<br>
      <code>hash</code> SHA-2, SHA-1, MD5, verify<br>
      <code>uuid</code> secure random v4 IDs<br>
      <code>password</code> secure random passwords<br>
      <code>time</code> Unix s / ms / µs / ns ↔ dates<br>
      <code>cron</code> preview the next runs<br>
      <code>regex</code> RE2 matches and captures
    </td>
  </tr>
</table>

<table>
  <tr>
    <td width="50%" valign="top">
      <h4>Explore with the workbench</h4>
      Run <code>termbelt</code> to search every tool, keep favorites, and fill in forms that expose every
      CLI option. Results scroll, copy to your clipboard (even over SSH), and switch to raw JSON with one key.
    </td>
    <td width="50%" valign="top">
      <h4>Script with direct commands</h4>
      Every tool has <code>--json</code> with a stable envelope and <code>--raw</code> for pipes. Inputs
      come from arguments or stdin, byte-exact, and errors exit non-zero. Completions for bash, zsh, fish
      and PowerShell are built in.
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h4>Honest answers</h4>
      Domain status comes from the authoritative registry, not a reseller. Ambiguous WHOIS replies stay
      <i>unknown</i> instead of guessing. Hosting labels show their evidence. JWTs are marked unverified.
    </td>
    <td width="50%" valign="top">
      <h4>Private and small</h4>
      One self-contained Go binary for macOS, Linux and Windows on x64 and ARM64. No telemetry, no account,
      no background service. Inputs, tokens and results are never saved.
    </td>
  </tr>
</table>

## Contents

- [Requirements](#requirements)
- [Install](#install)
- [The workbench](#the-workbench)
- [Commands](#commands)
- [Scripting](#scripting)
- [Recipes](#recipes)
- [How each tool gets its answers](#how-each-tool-gets-its-answers)
- [Privacy and configuration](#privacy-and-configuration)
- [Troubleshooting](#troubleshooting)
- [Build from source](#build-from-source)
- [License](#license)

## Requirements

- macOS, Linux or Windows, on x64 or ARM64. Linux binaries are statically linked and work on glibc and musl systems.
- The native binary needs nothing else: no Go, Node.js, Bun or browser.
- For `bunx` / `npx` / `npm install`: Bun, or Node.js 18 or later.
- A terminal at least 42 × 16 for the workbench; 100 × 32 or larger is most comfortable.

## Install

### Homebrew

On macOS and Linux:

```sh
brew install rahatsagor/tap/termbelt
```

Shell completions for bash, zsh and fish are installed with it. Update with `brew upgrade termbelt`.

### Bun or npm

Run it without installing:

```sh
bunx termbelt          # or: npx termbelt
bunx termbelt ip       # any command works the same way
```

Or install the `termbelt` command:

```sh
npm install --global termbelt
# or
bun add --global termbelt
```

The `termbelt` package is a small launcher. Your package manager installs only the one native package that matches your operating system and CPU, with no install scripts and no extra downloads. Keep optional dependencies enabled (do not use `--omit=optional`). On Node.js 22.15 and later the launcher replaces itself with the native process; older Node.js versions and Windows run it as a child process with signals forwarded.

A global `bun add` command starts through Node.js, so it needs Node.js on your `PATH`. On a Bun-only machine, use `bunx termbelt`, Homebrew or the standalone binary.

### Standalone binary

1. Download the archive for your system from the [latest release](https://github.com/rahatsagor/termbelt/releases/latest):

   | System               | Archive                                |
   | -------------------- | -------------------------------------- |
   | macOS, Apple silicon | `termbelt_<version>_darwin_arm64.tar.gz` |
   | macOS, Intel         | `termbelt_<version>_darwin_amd64.tar.gz` |
   | Linux, x64           | `termbelt_<version>_linux_amd64.tar.gz`  |
   | Linux, ARM64         | `termbelt_<version>_linux_arm64.tar.gz`  |
   | Windows, x64         | `termbelt_<version>_windows_amd64.zip`   |
   | Windows, ARM64       | `termbelt_<version>_windows_arm64.zip`   |

2. Unpack it and move `termbelt` (`termbelt.exe` on Windows) to a folder on your `PATH`:

   ```sh
   tar -xzf termbelt_*_darwin_arm64.tar.gz
   sudo mv termbelt /usr/local/bin/
   ```

3. On macOS, a file downloaded with a browser is quarantined. Clear the flag once:

   ```sh
   xattr -d com.apple.quarantine /usr/local/bin/termbelt
   ```

<details>
<summary><b>Verify the download</b></summary>

<br>

Every release includes `SHA256SUMS`. Put it next to the archive and run:

```sh
shasum -a 256 --check --ignore-missing SHA256SUMS   # macOS
sha256sum --check --ignore-missing SHA256SUMS       # Linux
```

On Windows, compare `Get-FileHash termbelt_<version>_windows_amd64.zip` with the matching line. Or let Termbelt check it: `termbelt hash --file <archive> --verify <digest>` exits non-zero on a mismatch.

Release archives, npm packages and the Homebrew formula are all built from the tagged source by GitHub Actions. npm packages are published with [provenance](https://docs.npmjs.com/generating-provenance-statements), so npmjs.com shows the exact commit and workflow that built each version.

</details>

### Go

```sh
go install github.com/rahatsagor/termbelt@latest
```

This needs Go 1.26 or later and puts `termbelt` in `$(go env GOPATH)/bin`.

### Shell completions

Homebrew installs completions automatically. Otherwise, generate them for your shell:

```sh
termbelt completion zsh > "${fpath[1]}/_termbelt"          # zsh
termbelt completion bash > ~/.local/share/bash-completion/completions/termbelt
termbelt completion fish > ~/.config/fish/completions/termbelt.fish
termbelt completion powershell | Out-String | Invoke-Expression   # PowerShell
```

## The workbench

```sh
termbelt
```

Type to search, pick a tool, fill in its form, and run it. Every CLI option has a form field, and on/off options are checkboxes.

<p align="center">
  <img src="https://raw.githubusercontent.com/rahatsagor/termbelt/main/docs/media/result.svg" width="780" alt="A Termbelt result screen for the subnet calculator, listing network, netmask, wildcard, usable hosts, host range, broadcast and whether 10.42.7.19 belongs to 10.42.0.0/20.">
</p>

| Where        | Keys                                | Does                                                          |
| ------------ | ----------------------------------- | ------------------------------------------------------------- |
| Launcher     | type                                | Search tools, descriptions and commands                       |
|              | **↑ / ↓**, **Enter**                | Select and open a tool                                        |
|              | **Tab / Shift+Tab**                 | Switch category: all, network, domains, developer, favorites, recent |
|              | **Ctrl+F**                          | Toggle a favorite (saved to your settings)                    |
| Form         | **Tab / Shift+Tab**                 | Next / previous field                                         |
|              | **Ctrl+R**                          | Run                                                           |
|              | **Enter**                           | Run from a single-line field; new line in a multiline editor  |
|              | **Space**                           | Toggle an on/off option                                       |
| Running      | **Esc**                             | Cancel and return to the form                                 |
| Result       | **↑ / ↓ / PageUp / PageDown**       | Scroll                                                        |
|              | **c**                               | Copy the result                                               |
|              | **j**                               | Toggle structured JSON                                        |
|              | **e** / **r**                       | Edit the inputs / run again                                   |
|              | **Esc**                             | Back to the launcher                                          |
| Anywhere     | **Ctrl+C / Ctrl+Q**                 | Quit and restore the terminal                                 |

- **Recent** remembers tool names for the current session only.
- **Copy** uses `pbcopy` on macOS, `wl-copy` (Wayland) or `xclip` / `xsel` (X11) on Linux, and `clip.exe` on Windows. Over SSH, or when no clipboard tool works, it sends the text through the terminal itself (OSC 52), which most modern terminals support.
- **Multiline editors** for JSON, JWT, Base64, hashing and regex accept up to 1,048,576 characters and 10,000 lines; other fields accept 4,096 characters. A paste that would not fit is rejected with a message instead of being cut short, and running stays blocked until you edit it. Use stdin or `hash --file` for anything larger.
- **The layout adapts**: wide terminals show a preview panel; narrow ones show a compact list and keep the focused field in view. Returning to the launcher clears local inputs and results from memory.

## Commands

Run `termbelt COMMAND --help` for every flag.

| Command    | What it does                                                                      | Example                                                    |
| ---------- | --------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `speed`    | Fast.com download, or Cloudflare download + upload, with latency and jitter       | `termbelt speed --provider cloudflare --duration 10`       |
| `ip`       | Public IP, country, city, timezone, ISP, organization, ASN                        | `termbelt ip 1.1.1.1`                                      |
| `domains`  | Authoritative RDAP / WHOIS registration status across TLDs                        | `termbelt domains myidea --tlds com,io,dev,co.uk`          |
| `whois`    | Registrar, dates, status, nameservers, DNSSEC, public contacts                     | `termbelt whois example.com --json`                        |
| `site`     | DNS, network owner, hosting / CDN clues, headers, HTML technology evidence        | `termbelt site https://github.com`                         |
| `dns`      | A, AAAA, MX, NS, TXT, CNAME, SOA, CAA, SRV or any other type; PTR for IPs          | `termbelt dns _dmarc.example.com --type TXT`               |
| `tls`      | Certificate chain, issuer, validity, trust, SANs, key, ALPN, fingerprints, cipher | `termbelt tls https://example.com`                         |
| `http`     | Status, redirects, headers, DNS / TCP / TLS / TTFB / total timings                | `termbelt http https://example.com`                        |
| `ping`     | Repeated TCP connection latency and failure rate (host, host:port or URL)         | `termbelt ping example.com:443 --count 10`                 |
| `ports`    | Local processes listening on TCP ports                                            | `termbelt ports --port 3000`                               |
| `json`     | Validate, format, minify, extract paths; keeps key order and big numbers           | `termbelt json --path 'users[0].name' < data.json`         |
| `base64`   | Standard / URL-safe Base64 encode and decode                                      | `printf hello \| termbelt base64 --raw`                    |
| `url`      | Encode, decode, or inspect URL components                                         | `termbelt url 'https://example.com?a=1' --mode inspect`    |
| `jwt`      | Decode header, claims, timestamps and expiry locally                              | `termbelt jwt < token.txt`                                 |
| `hash`     | SHA-256/512/384/224, SHA-1, MD5 or all; verify; streams large files                | `termbelt hash --file archive.zip --verify <digest>`       |
| `uuid`     | Secure random version 4 UUIDs                                                     | `termbelt uuid --count 5 --raw`                            |
| `password` | Secure random passwords with every required character class                      | `termbelt password --length 32 --no-symbols --raw`         |
| `time`     | Exact Unix s / ms / µs / ns, fractional timestamps, dates and timezones            | `termbelt time 1735689600123456789 --unit ns`              |
| `cron`     | Upcoming runs with timezone and daylight-saving rules                             | `termbelt cron '*/15 * * * *' --zone Asia/Dhaka`           |
| `regex`    | Go / RE2 matches, byte ranges, numbered and named captures                        | `termbelt regex '(?P<word>[a-z]+)' 'hello 123 world'`      |
| `cidr`     | IPv4 / IPv6 ranges, masks, wildcard, host range, counts, membership               | `termbelt cidr 192.168.1.0/24 --contains 192.168.1.42`     |

Also: `termbelt config` for settings, `termbelt completion <shell>`, and `termbelt --version`.

## Scripting

| Flag / variable  | Effect                                                                                                       |
| ---------------- | ------------------------------------------------------------------------------------------------------------ |
| `--json`         | A stable envelope: `tool`, `title`, `summary`, `output`, `table`, `metrics`, `sections`, `notes`, `data`, `duration_ms` |
| `--raw`          | Just the transformed text (extracted JSON strings unquoted, like `jq -r`), or the JSON data when there is no text |
| `--plain`, `NO_COLOR=1` | No colors. Output that is not a terminal is plain automatically                                       |
| `--timeout 3–120` | Network request timeout in seconds (default 20)                                                             |

`--json` and `--raw` can't be combined. Progress goes to stderr and is turned off for JSON and raw output. Failures exit with status 1 and a message on stderr.

```sh
termbelt ip --json > network.json
termbelt uuid --count 10 --raw
termbelt json --minify --raw < data.json
printf '%s' 'hello' | termbelt hash --raw
echo example.com | termbelt dns --type MX
termbelt time -- -1234567890          # negative values go after --
```

<details>
<summary><b>Input rules</b></summary>

<br>

- Local text tools accept up to 16 MiB on stdin. `json`, `base64`, `jwt`, `regex` and `hash` keep stdin byte-exact.
- `domains`, `whois`, `site`, `dns`, `tls`, `http`, `ping`, `url`, `cidr` and `cron` read their target from stdin when no argument is given; one trailing newline is dropped.
- `hash` streams stdin and regular files with bounded memory and can be canceled. Use `hash --file` for a file, or pipe a stream into it.
- An argument is always one value, even with spaces or shell characters in it.
- Timestamp units are detected by size. Use `--unit s`, `ms`, `us` or `ns` for historical or small values. Fractional timestamps keep exact nanoseconds, negative ones included.
- JSON paths use dots and indices (`users.0.name`), brackets for keys with dots (`users[0]["display.name"]`), and negative indices from the end (`items[-1]`). Formatting keeps key order, characters such as `<`, `>` and `&`, and exact numbers.
- Base64 output that isn't UTF-8 text is shown as hexadecimal. `url` encodes a single component; `--mode inspect` takes a whole URL.

</details>

## Recipes

### Is that name still free?

```sh
termbelt domains shipfastly --tlds com,io,dev,app,co.uk
termbelt domains shipfastly --all --only-unregistered     # every IANA root-zone TLD
```

### What is holding port 3000?

```sh
termbelt ports --port 3000
```

### Fail a CI job when a certificate is close to expiring

```sh
termbelt tls example.com --json | jq -e '.data.trusted and .data.days_remaining > 14'
```

### Verify a download before you run it

```sh
termbelt hash --file termbelt_1.2.0_linux_amd64.tar.gz \
  --verify "$(grep linux_amd64 SHA256SUMS | cut -d' ' -f1)"
```

### Check the mail records for a domain

```sh
termbelt dns example.com --type MX
termbelt dns _dmarc.example.com --type TXT
termbelt dns selector1._domainkey.example.com --type TXT
```

### Read a token from the clipboard without pasting it into a website

```sh
pbpaste | termbelt jwt          # macOS; use wl-paste or xclip -o on Linux
```

## How each tool gets its answers

<details>
<summary><b>Speed</b></summary>

<br>

- **Fast.com** mode finds the current public token in Fast.com's script and measures download throughput from Netflix CDN servers. **Cloudflare** mode measures download and upload with Cloudflare's public speed-test endpoints.
- Each direction runs for up to 8 seconds or 128 MiB. Change that with `--duration 2–30`, `--max-mb 1–1024` and `--download-only`. Protocol headers, discovery and latency requests use a little data on top.
- The rate is the average HTTPS throughput across up to three connections. Upload requests adapt to the connection, and only acknowledged uploads count.
- HTTP latency is the median of five requests on an already-open connection, measured before the transfer, so it includes server response time. Jitter is the mean change between consecutive samples.
- JSON includes why each direction stopped and a `partial` flag; server failures also appear in the notes. If a download hits its data limit in under two seconds, Termbelt suggests a larger `--max-mb`.
- These are estimates. Termbelt doesn't reproduce a provider's full browser test or measure packet loss or loaded latency. If a provider changes its API, you get a clear error.

</details>

<details>
<summary><b>Domains</b></summary>

<br>

- Termbelt follows [IANA's RDAP bootstrap](https://www.iana.org/assignments/rdap-dns) to the authoritative registry. A registered record must match the requested domain, and a valid authoritative not-found answer means **unregistered**.
- Many ccTLDs, including `.io`, `.co`, `.me` and `.sh`, have no RDAP service. For those, Termbelt asks the registry's WHOIS server from a bundled IANA snapshot. WHOIS replies are free-form, so they're read conservatively: only an explicit not-found or "free" answer counts as unregistered, only a matching registration record counts as registered, and reserved, refused or ambiguous replies stay **unknown**.
- Empty RDAP 404s, such as Verisign's, count only when they carry the RDAP media type and stay on the authoritative service. Unsupported TLDs, rate limits, redirects to unrelated services, malformed responses and timeouts stay **unknown**.
- Unregistered doesn't mean you can buy it: a name can be reserved, premium, restricted or blocked by policy. Confirm with a registrar.
- `--all` checks [every root-zone TLD](https://data.iana.org/TLD/tlds-alpha-by-domain.txt), including brand and internationalized ones, and can take several minutes. Give multi-label suffixes such as `co.uk` or `com.bd` with `--tlds`. `--all` and `--tlds` can't be combined. A full domain is checked exactly; a subdomain prompts you to use the registrable domain.
- Up to six lookups run at once (`--concurrency 1–12`), interleaved across registries and paced per registry host. The 8-second deadline starts after pacing. A registry's HTTP 429 pauses further requests to it for its `Retry-After` window instead of hammering it.
- `--only-unregistered` filters the table but keeps every result in JSON. `--refresh` updates registry metadata right away, even when the cache can't be written.

</details>

<details>
<summary><b>WHOIS, website and IP intelligence</b></summary>

<br>

- **WHOIS** prefers the registry's RDAP record and adds a bounded lookup at the registrar for public contact data. When RDAP can't answer, it finds the registry's WHOIS server through IANA and queries TCP port 43. That protocol is unencrypted and some networks block it. Redacted contacts can't be recovered.
- **Website intelligence** labels hosting, CDN and technology from the DNS records, HTTP headers and HTML assets it shows you. A CDN or reverse proxy can hide the origin, so labels are evidence-backed inferences, not proof of where a site is hosted.
- **IP intelligence** uses HTTPS `ipwho.is`, falling back to HTTPS `free.freeipapi.com`. These free services can rate-limit. Private, local, CGNAT and documentation addresses are classified on your machine and never sent anywhere. Geolocation is approximate.

</details>

<details>
<summary><b>TLS, DNS and ports</b></summary>

<br>

- **TLS** reads the presented chain even when it's expired or untrusted, then checks system trust and the hostname separately and reports the result. Other HTTP requests keep normal certificate validation.
- **DNS** uses your first system resolver (from `/etc/resolv.conf`, or the active network adapters on Windows) unless you pass `--resolver`. An IP address does a reverse PTR lookup. Service names with underscores, such as SRV, DMARC and DKIM records, work as typed.
- **Ports** uses `lsof` on macOS, `ss` or `/proc/net/tcp` on Linux, and the system TCP table on Windows. It shows the listeners your account can see and never stops a process.

</details>

<details>
<summary><b>Use your own IP lookup API</b></summary>

<br>

A compatible API answers `GET /me` and `GET /ip/:ip` with a JSON object that has an `ip` string. Optional fields: `city`, `region`, `country`, `countryCode`, `continent`, `latitude`, `longitude`, `timezone`, `postal`, `isp`, `org` and `asn`.

```sh
termbelt config set ip-api-url https://ip-api.example.com
termbelt ip
```

Or use `TERMBELT_IP_API_URL`, or `--ip-api-url` for one run. A configured API is used on its own: if it fails, you see the error instead of a silent switch to another provider. Clear it with `termbelt config set ip-api-url ''`.

</details>

Provider references: [Cloudflare speed-test engine](https://github.com/cloudflare/speedtest), [IPWhois HTTPS API](https://ipwhois.io/documentation), [ICANN Lookup / RDAP](https://lookup.icann.org/).

## Privacy and configuration

- **Local tools stay local.** JSON, JWT, Base64, URL, hashes, UUIDs, passwords, time, cron, regex, CIDR and ports never touch the network.
- **No telemetry.** Termbelt never saves JWTs, text inputs, generated passwords, queried domains or results. Network tools contact only the provider or target shown in the result, with the public domain or IP the lookup needs.
- **Untrusted text is escaped.** Values from the network, including control and bidirectional characters, are escaped before they reach your terminal, so a response can't inject terminal commands or reorder what you see.

Settings live in your OS config directory (`~/Library/Application Support/termbelt/config.json` on macOS, `~/.config/termbelt/config.json` on Linux, `%AppData%\termbelt\config.json` on Windows), written atomically with private permissions. Only public IANA metadata is cached.

| Command                                    | Does                                          |
| ------------------------------------------ | --------------------------------------------- |
| `termbelt config`                          | Show the effective settings and their file    |
| `termbelt config path`                     | Print the settings file location              |
| `termbelt config set favorites dns,tls,json` | Set the launcher favorites                  |
| `termbelt config set timeout-seconds 30`   | Save a default network timeout (3–120)        |
| `termbelt config set ip-api-url <url>`     | Use a custom IP lookup API                    |
| `termbelt config reset`                    | Restore the defaults                          |

| Variable              | Overrides                                         |
| --------------------- | ------------------------------------------------- |
| `TERMBELT_CONFIG`     | The settings file path                            |
| `TERMBELT_CACHE_DIR`  | The metadata cache directory                      |
| `TERMBELT_IP_API_URL` | The IP lookup API for this run                    |
| `NO_COLOR`            | Turns off colors in commands and the workbench    |

A damaged settings file doesn't block anything: Termbelt warns, uses the defaults, and `config set`, `config reset` and saving a favorite repair it. Saving reads the file on disk, so temporary environment and command-line overrides are never written back.

## Troubleshooting

<details>
<summary><b><code>missing @rahatsagor/termbelt-…</code> from the npm package</b></summary>

<br>

The package manager skipped the native package for your platform, usually because optional dependencies were turned off. Reinstall without `--omit=optional` (or `--no-optional`). With pnpm or Yarn, make sure optional dependencies for your OS and CPU are allowed.

</details>

<details>
<summary><b><code>termbelt: command not found</code> after <code>bun add -g</code></b></summary>

<br>

Bun's global bin directory (`~/.bun/bin`) must be on your `PATH`, and the global command starts through Node.js. Use `bunx termbelt`, Homebrew, or the standalone binary on a machine without Node.js.

</details>

<details>
<summary><b>A domain shows <i>unknown</i></b></summary>

<br>

The registry didn't give a clear answer: it has no RDAP or WHOIS service, rate-limited the request, timed out, or replied with something Termbelt won't guess about. Try again later, lower `--concurrency`, or check that your network allows outbound TCP port 43 for WHOIS.

</details>

<details>
<summary><b>Copy doesn't reach my clipboard</b></summary>

<br>

Over SSH, Termbelt uses OSC 52, which your terminal must allow (iTerm2, WezTerm, kitty, Windows Terminal and recent tmux with `set -g set-clipboard on` do). Locally on Linux, install `wl-copy` (Wayland) or `xclip` / `xsel` (X11).

</details>

<details>
<summary><b>The workbench looks garbled or too small</b></summary>

<br>

Make the terminal at least 42 × 16. Use a font with box-drawing characters (any common monospace font has them), and set `NO_COLOR=1` or pass `--plain` if your terminal doesn't support color.

</details>

Still stuck? [Open an issue](https://github.com/rahatsagor/termbelt/issues/new) with your OS, the command you ran and what Termbelt printed.

## Build from source

You need [Go](https://go.dev/dl/) 1.26 or later. Release tooling also uses [Bun](https://bun.sh/) 1.4.2+ and Python 3.9+; Node.js 18+ is needed to test the npm launcher. No JavaScript dependencies are installed.

```sh
git clone https://github.com/rahatsagor/termbelt.git
cd termbelt
make build && build/termbelt
```

| Command                  | What it does                                                           |
| ------------------------ | ---------------------------------------------------------------------- |
| `make check`             | Tests with the race detector, then `go vet`                            |
| `make smoke`             | Offline checks against the built executable                            |
| `make edge-smoke`        | Large stdin, cancellation and malformed-input checks                   |
| `make live-smoke`        | Real provider, DNS, TLS and speed checks                               |
| `make fuzz`              | Bounded fuzzing of domain and timestamp parsing                        |
| `make vuln`              | Scan with `govulncheck`                                                |
| `make install`           | Install to `~/.local/bin` (change with `PREFIX=/another/bin`)          |
| `bun run build:release`  | Six native binaries, npm platform packages and release archives        |
| `bun run pack`           | Verify and pack the npm tarballs                                       |
| `bun run test:package`   | Isolated installs and launcher checks with Bun, Node.js and Node.js without `execve` |
| `bun run homebrew`       | Print the Homebrew formula for the built release                       |
| `bun run notices`        | Refresh bundled third-party notices after dependency updates           |

`python3 scripts/tui_smoke.py --binary build/termbelt` runs PTY interaction checks on Unix. `python3 scripts/whois_servers.py` refreshes the bundled WHOIS servers. `python3 scripts/screenshots.py` regenerates the README screenshots from the real workbench (needs `pip install pyte`).

<details>
<summary><b>Project layout</b></summary>

<br>

| Path                    | What's inside                                                         |
| ----------------------- | --------------------------------------------------------------------- |
| `main.go`               | Entry point and version                                               |
| `internal/core/`        | Every tool: network, domains, WHOIS, speed, ports and local utilities |
| `internal/cli/`         | Commands, flags, stdin handling and output modes                      |
| `internal/tui/`         | The Bubble Tea workbench                                              |
| `internal/render/`      | Terminal-safe text, tables and colors                                 |
| `npm/`                  | The npm launcher and platform map                                     |
| `packaging/homebrew/`   | The Homebrew formula template                                         |
| `scripts/`              | Build, packaging, release, smoke-test and screenshot scripts          |
| `docs/media/`           | Images used in this README                                            |

</details>

<details>
<summary><b>Releasing</b></summary>

<br>

1. Set the new `version` in `package.json`, `optionalDependencies` and `main.go`, add it to `CHANGELOG.md`, and commit.
2. Tag the commit and push both:

   ```sh
   git tag vX.Y.Z
   git push origin main vX.Y.Z
   ```

The release workflow tests, vets and scans the code, builds all six binaries from the tag, verifies checksums and that the tag matches `package.json`, publishes the platform packages before the launcher with npm trusted publishing and provenance, and creates the GitHub release. When the `HOMEBREW_TAP_TOKEN` secret (contents write access to `rahatsagor/homebrew-tap`) is set, it also updates `Formula/termbelt.rb` with the new checksums. Every action is pinned to a commit SHA.

All seven npm packages must trust the same GitHub workflow before automated publishing works. A first authenticated publish can use `bun run publish`; retries verify already-published tarballs before continuing.

</details>

Bug reports and ideas are welcome as [issues](https://github.com/rahatsagor/termbelt/issues).

## License

Free and open source under the [MIT License](LICENSE). © 2026 Rahat Sagor.

Native archives and npm packages include dependency licenses in [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES), which builds verify against the dependencies compiled for all six platforms. Bundled registry snapshots come from [IANA's RDAP bootstrap](https://data.iana.org/rdap/dns.json) and [root-zone TLD list](https://data.iana.org/TLD/tlds-alpha-by-domain.txt).

<p align="center">
  <br>
  <a href="https://termbelt.rsapplication.com">
    <img src="https://raw.githubusercontent.com/rahatsagor/termbelt/main/docs/media/icon.svg" width="40" height="40" alt="Termbelt">
  </a>
  <br>
  <a href="https://termbelt.rsapplication.com"><b>termbelt.rsapplication.com</b></a>
</p>
