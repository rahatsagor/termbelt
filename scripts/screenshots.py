#!/usr/bin/env python3
"""Regenerate the README screenshots from the real workbench.

Runs the executable in a pseudo-terminal, emulates the screen with pyte and
writes each frame as an SVG with real colors. Unix only; needs `pip install pyte`.

    make build && python3 scripts/screenshots.py --binary build/termbelt

With --landing PATH it also runs every tool with a sample input, at 108, 76 and 58
columns, and writes the real form and result frames as JSON for the
website's interactive preview. Network tools make real requests.
"""
import argparse
import fcntl
import json
import subprocess
import os
import pathlib
import pty
import re
import select
import struct
import tempfile
import termios
import time

import pyte

SHOTS = [
    {"file": "workbench.svg", "title": "termbelt — workbench", "size": (112, 32), "keys": []},
    {"file": "result.svg", "title": "termbelt — cidr", "size": (100, 25),
     "keys": [["cidr", 0.4], ["\r", 0.5], ["10.42.0.0/20", 0.3], ["\t", 0.2], ["10.42.7.19", 0.3], ["\x12", 1.2]]},
]
FG, BG = "e4e6ee", "10121c"
NAMED = {"black": "000000", "red": "f18383", "green": "99c97c", "brown": "ebc474", "yellow": "ebc474",
         "blue": "7aa2f7", "magenta": "b8a1ff", "cyan": "67e8c9", "white": "e4e6ee"}
CELL, LINE, FONT, PAD, BAR = 8.6, 17.5, 14.2, 18, 34


def color(value, fallback):
    if value in NAMED:
        return NAMED[value]
    return value if len(value) == 6 else fallback


class Session:
    """The real executable running in a pseudo-terminal, emulated by pyte."""

    def __init__(self, binary, cols, rows, config):
        self.screen = pyte.Screen(cols, rows)
        self.stream = pyte.ByteStream(self.screen)
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", TERMBELT_CONFIG=config, TERMBELT_IP_API_URL="")
            env.pop("NO_COLOR", None)
            os.execve(binary, [binary], env)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
        self.pump(1.5)

    def pump(self, seconds, until=None):
        end = time.time() + seconds
        while time.time() < end:
            if select.select([self.fd], [], [], 0.05)[0]:
                try:
                    data = os.read(self.fd, 65536)
                except OSError:
                    return
                # pyte prints private CSI sequences it does not know, such as keyboard-mode requests.
                self.stream.feed(re.sub(rb"\x1b\[[<>=?][0-9;]*u|\x1b\[[<>=][0-9;]*[a-zA-Z]", b"", data))
            elif until and until(self.screen):
                return

    def send(self, keys):
        for text, wait in keys:
            os.write(self.fd, text.encode())
            self.pump(wait)

    def close(self):
        os.write(self.fd, b"\x03")
        self.pump(0.3)
        try:
            os.kill(self.pid, 9)
        except ProcessLookupError:
            pass
        os.waitpid(self.pid, 0)


def capture(binary, cols, rows, keys, config):
    session = Session(binary, cols, rows, config)
    session.send(keys)
    session.pump(0.8)
    session.close()
    return session.screen


def svg(screen, cols, rows, title):
    width, height = round(cols * CELL + PAD * 2), round(rows * LINE + PAD * 2 + BAR)
    escape = lambda s: s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    parts = []
    for y in range(rows):
        line, x = screen.buffer[y], 0
        while x < cols:
            def style(c):
                fg, bg = color(c.fg, FG), color(c.bg, BG)
                return (bg, fg, c.bold) if c.reverse else (fg, bg, c.bold)
            current, start, text = style(line[x]), x, ""
            while x < cols and style(line[x]) == current:
                text += line[x].data or " "
                x += 1
            fg, bg, bold = current
            px, py = PAD + start * CELL, BAR + PAD + y * LINE
            if bg != BG:
                parts.append(f'<rect x="{px:.1f}" y="{py:.1f}" width="{len(text) * CELL:.1f}" height="{LINE}" fill="#{bg}"/>')
            if text.strip():
                weight = ' font-weight="700"' if bold else ""
                parts.append(f'<text x="{px:.1f}" y="{py + LINE * 0.76:.1f}" fill="#{fg}"{weight} textLength="{len(text) * CELL:.1f}" lengthAdjust="spacingAndGlyphs" xml:space="preserve">{escape(text)}</text>')
    body = "\n".join(parts)
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}" role="img" aria-label="{escape(title)}">
<title>{escape(title)}</title>
<rect width="{width}" height="{height}" rx="12" fill="#{BG}"/>
<rect x=".5" y=".5" width="{width - 1}" height="{height - 1}" rx="11.5" fill="none" stroke="#2b2f45"/>
<path d="M12 .5h{width - 24}a11.5 11.5 0 0 1 11.5 11.5v{BAR - 12}H.5V12A11.5 11.5 0 0 1 12 .5z" fill="#181a28"/>
<circle cx="20" cy="17" r="5.5" fill="#f18383"/><circle cx="38" cy="17" r="5.5" fill="#ebc474"/><circle cx="56" cy="17" r="5.5" fill="#99c97c"/>
<text x="{width / 2}" y="21.5" text-anchor="middle" font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,monospace" font-size="12" fill="#7c809c">{escape(title)}</text>
<g font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,'Liberation Mono',monospace" font-size="{FONT}">
{body}
</g>
</svg>
'''


DOWN, TAB, CLEAR, RUN = "\x1b[B", "\t", "\x15", "\x12"
# Sample inputs, typed into each tool's form in the order of core.Tools.
DEMOS = {
    "speed": [CLEAR + "cloudflare", TAB, CLEAR + "6", TAB, CLEAR + "200"],
    "ip": ["1.1.1.1"],
    "domains": ["shipfastly", TAB, CLEAR + "com,io,dev,app,co"],
    "whois": ["example.com"],
    "site": ["github.com"],
    "dns": ["example.com", TAB, TAB, "1.1.1.1"],
    "tls": ["github.com"],
    "http": ["https://example.com"],
    "ping": ["example.com:443"],
    "ports": ["7000"],
    "json": ['{"name":"termbelt","tools":21,"platforms":["macOS","Linux","Windows"],"local":true}'],
    "base64": ["hello, termbelt"],
    "url": ["https://example.com/search?q=one terminal&tab=fewer", TAB, CLEAR + "inspect"],
    "jwt": ["eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkFkYSBMb3ZlbGFjZSIsImlhdCI6MTczNTY4OTYwMCwiZXhwIjoxODkzNDU2MDAwfQ.c2lnbmF0dXJlLW5vdC12ZXJpZmllZA"],
    "hash": ["hello, termbelt", TAB, CLEAR + "all"],
    "uuid": [CLEAR + "3"],
    "password": [TAB, CLEAR + "3"],
    "time": ["1735689600", TAB, CLEAR + "Asia/Dhaka"],
    "cron": ["*/15 9-17 * * 1-5", TAB, CLEAR + "Asia/Dhaka", TAB, CLEAR + "5"],
    "regex": ["(?P<word>[a-z]+)", TAB, "hello 123 world"],
    "cidr": ["10.42.0.0/20", TAB, "10.42.7.19"],
}


# The CLI equivalent of each sample, shown under the website preview.
COMMANDS = {
    "speed": "termbelt speed --provider cloudflare --duration 6 --max-mb 200",
    "ip": "termbelt ip 1.1.1.1",
    "domains": "termbelt domains shipfastly --tlds com,io,dev,app,co",
    "whois": "termbelt whois example.com",
    "site": "termbelt site github.com",
    "dns": "termbelt dns example.com --resolver 1.1.1.1",
    "tls": "termbelt tls github.com",
    "http": "termbelt http https://example.com",
    "ping": "termbelt ping example.com:443",
    "ports": "termbelt ports --port 7000",
    "json": "termbelt json '{\"name\":\"termbelt\",\"tools\":21,\"platforms\":[\"macOS\",\"Linux\",\"Windows\"],\"local\":true}'",
    "base64": "termbelt base64 'hello, termbelt'",
    "url": "termbelt url 'https://example.com/search?q=one terminal&tab=fewer' --mode inspect",
    "jwt": "termbelt jwt < token.txt",
    "hash": "termbelt hash 'hello, termbelt' --algorithm all",
    "uuid": "termbelt uuid --count 3",
    "password": "termbelt password --count 3",
    "time": "termbelt time 1735689600 --zone Asia/Dhaka",
    "cron": "termbelt cron '*/15 9-17 * * 1-5' --zone Asia/Dhaka --count 5",
    "regex": "termbelt regex '(?P<word>[a-z]+)' 'hello 123 world'",
    "cidr": "termbelt cidr 10.42.0.0/20 --contains 10.42.7.19"
}


def tools_from_source():
    source = (pathlib.Path(__file__).resolve().parent.parent / "internal" / "core" / "core.go").read_text()
    text = r'"((?:[^"\\]|\\.)*)"'
    pattern = re.compile(r"\{ID: " + text + ", Name: " + text + ", Category: " + text + ", Description: " + text + ", Hint: " + text + ", Example: " + text + r"(, Local: true)?")
    decode = lambda value: json.loads('"' + value + '"')
    return [{"id": decode(m[1]), "name": decode(m[2]), "category": decode(m[3]), "description": decode(m[4]), "hint": decode(m[5]), "example": decode(m[6]), "local": bool(m[7])} for m in pattern.finditer(source)]


def body(screen, cols, styles):
    """Returns the colored lines between the header and footer rules."""
    # Result tables also draw ──── rules, so use the first (header) and last (footer) ones.
    rules = [y for y in range(screen.lines) if screen.display[y].strip().startswith("────")]
    lines = []
    for y in range(rules[0] + 2, rules[-1]):
        row, runs, x = screen.buffer[y], [], 2
        while x < cols - 2:
            def style(c):
                fg, bg = color(c.fg, FG), color(c.bg, BG)
                return (bg, fg, c.bold) if c.reverse else (fg, bg, c.bold)
            current, text = style(row[x]), ""
            while x < cols - 2 and style(row[x]) == current:
                text += row[x].data or " "
                x += 1
            runs.append([text, current])
        # Trailing spaces on the terminal background carry no information.
        while runs and runs[-1][1][1] == BG and not runs[-1][0].strip():
            runs.pop()
        if runs and runs[-1][1][1] == BG:
            runs[-1][0] = runs[-1][0].rstrip()
        for run in runs:
            run[1] = styles.setdefault(run[1], len(styles))
        lines.append(runs)
    while lines and not lines[-1]:
        lines.pop()
    return lines


def landing(binary, output, config):
    tools = tools_from_source()
    version = subprocess.run([binary, "--version"], capture_output=True, text=True).stdout.split()[-1]
    styles, layouts = {}, {}
    for name, cols in (("wide", 108), ("medium", 76), ("narrow", 58)):
        frames = {}
        for index, tool in enumerate(tools):
            session = Session(binary, cols, 64, config)
            session.send([(DOWN, 0.05)] * index + [("\r", 0.6)])
            session.send([(keys, 0.25) for keys in DEMOS[tool["id"]]])
            form = body(session.screen, cols, styles)
            started = time.time()
            session.send([(RUN, 0.2)])
            session.pump(40, until=lambda screen: "scroll" in screen.display[-3] + screen.display[-2])
            seconds = round(time.time() - started, 1)
            session.pump(0.4)
            frames[tool["id"]] = {"form": form, "result": body(session.screen, cols, styles), "seconds": seconds, "command": COMMANDS[tool["id"]]}
            session.close()
            print(f"{name} {tool['id']}: {seconds}s")
        layouts[name] = {"cols": cols, "frames": frames}
    data = {"version": version, "tools": tools, "favorites": ["dns", "tls", "json"], "styles": [[fg, bg, int(bold)] for fg, bg, bold in sorted(styles, key=styles.get)], "layouts": layouts}
    pathlib.Path(output).write_text(json.dumps(data, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    print(f"Wrote {output}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", default="build/termbelt")
    parser.add_argument("--output", default=str(pathlib.Path(__file__).resolve().parent.parent / "docs" / "media"))
    parser.add_argument("--landing", metavar="PATH", help="also write the website preview frames as JSON")
    args = parser.parse_args()
    binary = str(pathlib.Path(args.binary).resolve())
    with tempfile.TemporaryDirectory(prefix="termbelt-shots-") as temporary:
        config = os.path.join(temporary, "config.json")
        pathlib.Path(config).write_text(json.dumps({"favorites": ["dns", "tls", "json"]}))
        for shot in SHOTS:
            cols, rows = shot["size"]
            screen = capture(binary, cols, rows, shot["keys"], config)
            pathlib.Path(args.output, shot["file"]).write_text(svg(screen, cols, rows, shot["title"]), encoding="utf-8")
            print(f"Wrote {shot['file']}")
        if args.landing:
            landing(binary, args.landing, config)


if __name__ == "__main__":
    main()
