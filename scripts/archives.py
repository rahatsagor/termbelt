#!/usr/bin/env python3
"""Create reproducible native release archives from the six build outputs."""
import gzip
import hashlib
import io
import json
import pathlib
import sys
import tarfile
import zipfile
root = pathlib.Path(__file__).resolve().parent.parent
if sys.argv[1] == "--verify":
    version = json.loads((root / "package.json").read_text())["version"]
    for native in sorted((root / "dist/native").glob("termbelt_*")):
        windows = native.suffix == ".exe"
        stem = native.name.removesuffix(".exe").replace("termbelt_", f"termbelt_{version}_", 1)
        destination = root / "dist/releases" / (stem + (".zip" if windows else ".tar.gz"))
        expected = {"termbelt.exe" if windows else "termbelt": native.read_bytes(), "LICENSE": (root / "LICENSE").read_bytes(), "README.md": (root / "README.md").read_bytes(), "THIRD_PARTY_NOTICES": (root / "THIRD_PARTY_NOTICES").read_bytes()}
        if windows:
            with zipfile.ZipFile(destination) as archive:
                assert sorted(archive.namelist()) == sorted(expected), "unexpected release ZIP contents"
                for name, data in expected.items():
                    assert archive.read(name) == data, f"archive content differs: {name}"
        else:
            with tarfile.open(destination) as archive:
                assert sorted(archive.getnames()) == sorted(expected), "unexpected release tar contents"
                for name, data in expected.items():
                    assert archive.getmember(name).isfile(), "release member is not a regular file"
                    assert archive.extractfile(name).read() == data, f"archive content differs: {name}"
                assert archive.getmember("termbelt").mode == 0o755, "release binary is not executable"
    print("Verified native release archive contents")
    raise SystemExit(0)
version = sys.argv[1]
output = root / "dist/releases"
output.mkdir(parents=True, exist_ok=True)
checksums = []
for native in sorted((root / "dist/native").glob("termbelt_*")):
    windows = native.suffix == ".exe"
    files = [(native, "termbelt.exe" if windows else "termbelt"), (root / "LICENSE", "LICENSE"), (root / "README.md", "README.md"), (root / "THIRD_PARTY_NOTICES", "THIRD_PARTY_NOTICES")]
    stem = native.name.removesuffix(".exe").replace("termbelt_", f"termbelt_{version}_", 1)
    destination = output / (stem + (".zip" if windows else ".tar.gz"))
    if windows:
        with zipfile.ZipFile(destination, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            for source, name in files:
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = 0o100644 << 16
                archive.writestr(info, source.read_bytes())
    else:
        with destination.open("wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", filename="", mtime=0, compresslevel=9) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for source, name in files:
                    contents = source.read_bytes()
                    info = tarfile.TarInfo(name)
                    info.size = len(contents)
                    info.mode = 0o755 if name == "termbelt" else 0o644
                    archive.addfile(info, io.BytesIO(contents))
    checksums.append(f"{hashlib.sha256(destination.read_bytes()).hexdigest()}  {destination.name}")
(output / "SHA256SUMS").write_text("\n".join(checksums) + "\n", encoding="utf-8", newline="\n")
