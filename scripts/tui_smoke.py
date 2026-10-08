#!/usr/bin/env python3
"""Drive the real executable inside a PTY, including terminal capability replies."""
import fcntl
import argparse
import os
import pathlib
import pty
import re
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time

root = pathlib.Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument("--binary", default=str(root / "build/termbelt"))
parser.add_argument("--plain", action="store_true")
arguments = parser.parse_args()
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
temporary = tempfile.TemporaryDirectory(prefix="termbelt-tui-")
environment = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", TERMBELT_CONFIG=temporary.name + "/config.json", TERMBELT_CACHE_DIR=temporary.name + "/cache")
command = [str(pathlib.Path(arguments.binary).resolve())]
if arguments.plain:
    command.append("--plain")
process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, env=environment, start_new_session=True)
os.close(slave)
captured = bytearray()

def observe(seconds=0.6):
    end = time.monotonic() + seconds
    data = bytearray()
    while time.monotonic() < end:
        if select.select([master], [], [], min(0.05, max(0, end-time.monotonic())))[0]:
            try:
                chunk = os.read(master, 65536)
            except OSError:
                break
            data += chunk
            if b"\x1b[6n" in chunk:
                os.write(master, b"\x1b[1;1R")
            if b"\x1b]11;?" in chunk:
                os.write(master, b"\x1b]11;rgb:1717/1919/2626\x1b\\")
            if b"\x1b[c" in chunk:
                os.write(master, b"\x1b[?1;2c")
    captured.extend(data)
    return data.decode("utf-8", "replace")

def send(value, wait=0.6):
    os.write(master, value)
    return observe(wait)

try:
    initial = observe(1.2)
    assert "T E R M B E L T" in initial and "Internet speed" in initial, "launcher not rendered"
    print("PASS launcher in real PTY", flush=True)
    search = send(b"uuid")
    assert "UUID generator" in search, "search did not filter"
    opened = send(b"\r")
    assert "How many" in opened, "UUID form not opened"
    completed = send(b"\x12", 1.0)
    assert "UUID v4" in completed and "secure random identifier" in completed, "UUID result missing"
    print("PASS search -> form -> run -> result", flush=True)
    json_view = send(b"j")
    assert '"tool"' in json_view and '"uuid"' in json_view, "JSON toggle failed"
    print("PASS result JSON toggle", flush=True)
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 13, 38, 0, 0))
    process.send_signal(signal.SIGWINCH)
    narrow = observe(0.5)
    assert "resize" in narrow, "small-terminal guidance missing"
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    process.send_signal(signal.SIGWINCH)
    restored = observe(0.5)
    assert '"uuid"' in restored, "resize lost result state"
    print("PASS resize and restore result state", flush=True)
    send(b"e")
    send(b"\x1b")
    send(b"\x1b")
    search = send(b"json")
    assert "JSON workbench" in search, "second utility search failed"
    send(b"\r")
    send(b'\x1b[200~{"id":9007199254740993}\x1b[201~')
    completed = send(b"\x12", 1.0)
    assert "numeric precision preserved" in completed, "JSON result missing"
    print("PASS multiline paste -> JSON workbench", flush=True)
    send(b"\x1b")
    send(b"\x1b")
    send(b"speed")
    send(b"\r")
    send(b"\x12", 0.25)
    cancelled = send(b"\x1b")
    assert "Seconds per direction" in cancelled, "cancel did not return to form"
    print("PASS running task cancellation", flush=True)
    send(b"\x03", 0.5)
    process.wait(timeout=5)
    assert process.returncode == 0, f"TUI exit {process.returncode}"
    assert b"\x1b[?1049l" in captured, "alternate screen was not restored"
    print("PASS clean exit / terminal restoration", flush=True)
    if arguments.plain:
        sgr = re.findall(rb"\x1b\[([0-9;:]*)m", captured)
        color_codes = set(range(30, 38)) | set(range(40, 48)) | set(range(90, 98)) | set(range(100, 108)) | {38, 48}
        assert not any(int(code) in color_codes for sequence in sgr for code in re.split(rb"[;:]", sequence) if code), "--plain emitted color sequences"
        print("PASS plain TUI has no color sequences", flush=True)
finally:
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
        process.wait(timeout=5)
    os.close(master)
    temporary.cleanup()
