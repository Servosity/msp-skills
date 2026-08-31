#!/usr/bin/env python3
"""Build and validate the .mcpb bundles this repo publishes to the MCP Registry.

The defect this exists to end (issue #287)
------------------------------------------
64 of 65 published bundles cannot launch. `manifest.json` tells the host to run

    "command": "${__dirname}/bin/<slug>-mcp"

while the archive contains only architecture-suffixed binaries
(`bin/<slug>-mcp-darwin-arm64` and friends). `bin/<slug>-mcp` is in no bundle on
any platform. Claude Desktop validates the manifest and the user_config schema
at install time but never test-runs the binary, so the extension installs, the
credential prompt appears, the operator types real credentials, and the failure
surfaces at the first tool call. Verified against the shipped hudu-v0.1.8 asset.

Why the shape is what it is
---------------------------
`mcp_config.platform_overrides` is keyed on OS only - darwin, linux, win32 -
and the manifest template vocabulary has no `${arch}`. So one bundle carrying
one binary per (os, arch) cannot address them all from `command` alone. Two
options survive contact:

  * a launcher shim that detects the arch and execs the right binary. Rejected:
    it depends on the archive's executable bit surviving unpacking AND on a
    shell being present on the host, and neither is guaranteed.
  * a universal (fat) Mach-O for darwin plus per-OS overrides. Chosen.

The decision was argued and settled on issue #287. Upstream
mvanhorn/cli-printing-press#4366 carried the same algorithm and was closed
unmerged on 2026-08-28 ("a multi-platform zip is a new installer product, not a
press operator bug"), so the packaging fix lives downstream, here. This is a
port of that algorithm, not a vendoring of its Go.

Linux and Windows have no fat-binary format, so their overrides name one
architecture (amd64 first: it is the broadest target, and Windows on ARM runs
x64 under emulation while the reverse is not true). The other architectures stay
in the archive - they are what the release actually built - but nothing in the
manifest can address them until the MCPB spec grows an arch axis.

Three defects this deliberately does not reproduce
--------------------------------------------------
1. `zipfile.ZipInfo` defaults to ZIP_STORED. The live hudu bundle is 32 MB
   DEFLATE over 81 MB of binaries; stored it would be ~81 MB. Every member is
   written ZIP_DEFLATED, and `validate` fails a bundle that regresses to stored.
2. Every binary member of every published bundle is mode 0644 - `gh release
   download` writes 0644 and `zip -r` preserves it - so even connectwise-manage,
   the one connector whose manifest paths resolve, ships a non-executable
   server. Verified with `zipinfo -l` on connectwise-manage-v0.1.6. Binaries are
   written 0755 and `validate` asserts the bit.
3. The validator does NOT assert `entry_point == "bin/" + <mcp binary>`. That
   assertion is inverted for this fleet: it would codify the broken bare name
   everywhere and false-RED connectwise-manage, the single manifest that can
   launch. It asserts the weaker, true thing - every path the manifest declares
   names a member the archive actually contains. See the matching note in
   tools/maintainer/check_env_schema.py ("What it deliberately does NOT check").

Reconciled against cli-printing-press 4.31.4 (main @ ba4e6914, 2026-08-31)
-------------------------------------------------------------------------
The press builds a bundle too (`internal/pipeline/mcpb_bundle.go`), and it is a
DIFFERENT product: one operator-local archive holding the ONE binary the press
just cross-compiled, launched by a single `command`. Re-read at 4.31.4:

  * `platform_overrides` still appears nowhere in the press - not in the
    manifest writer, not in a golden. `mcp_config` is `{args, command, env}`,
    `command` is `${__dirname}/<entry_point>`. So the multi-binary shape this
    file writes remains this repo's to own, and nothing upstream has started
    competing for those keys.
  * the press stamps the version into the bundled manifest too
    (`rewriteMCPBManifestVersion`), so `--version` here matches upstream
    behaviour rather than inventing one.
  * 4.31.2-4.31.4 did change WHICH env vars a regenerated manifest declares
    (#4377, #4431: derive `mcp_config.env` / `user_config` from the credentials
    the binary actually reads, instead of PRINTING_PRESS_CLIENT_PROFILE). That
    is inert here: this builder rewrites only `command`,
    `platform_overrides[*].command`, `entry_point` and `version`, and carries
    every other manifest key through byte-for-byte, so a reprinted manifest's
    new env declaration reaches the bundle unaltered.
  * the press preserves the source file's mode (`stat.Mode()&0o777`), which is
    how a 0644 downloaded asset became a non-executable server. This builder
    writes binaries 0755 unconditionally and `validate` asserts the bit.

Scope: this repairs every FUTURE bundle. It does not repair the 65 already
published - that needs a release wave, and re-uploading over an existing tag
would silently invalidate the `packages[0].fileSha256` mcp-publish.yml already
recorded in the MCP Registry for that version. Until that wave lands, 62 skill
READMEs still carry a "one-click `.mcpb`" download that cannot launch
(`grep -rl 'one-click `.mcpb`' skills/*/README.md | wc -l` -> 62); that is
user-facing debt this file does not fix and must not be left implied.

Usage:
    mcpb_bundle.py build --manifest skills/hudu/manifest.json \\
        --bin-dir mcpb-build --binary-prefix hudu-mcp \\
        --out mcpb-build/hudu-mcp.mcpb --version 0.1.9
    mcpb_bundle.py validate mcpb-build/hudu-mcp.mcpb
    mcpb_bundle.py --self-test

Pure stdlib: no `lipo`, no Xcode, no Go toolchain, so a ubuntu-latest runner
produces the macOS-universal slice.

Exit codes: 0 = ok, 1 = a finding, 2 = usage error.
"""

from __future__ import annotations

import argparse
import json
import os
import stat
import struct
import sys
import zipfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/maintainer module)

MANIFEST_MEMBER = "manifest.json"

# Mach-O magics. Go emits 64-bit little-endian binaries, but read both widths
# and both byte orders rather than assuming.
MH_MAGIC = 0xFEEDFACE
MH_MAGIC_64 = 0xFEEDFACF
FAT_MAGIC = 0xCAFEBABE

# Universal-binary layout: a big-endian 8-byte header, one 20-byte record per
# slice, then the slices, each aligned to 2^14 (16 KiB - what arm64 requires and
# amd64 tolerates).
_FAT_HEADER_LEN = 8
_FAT_ARCH_LEN = 20
_FAT_ALIGN_POW = 14

# MCPB platform_overrides keys are OS names, and Windows is "win32".
_OS_KEY = {"windows": "win32"}

# Which architecture an OS override names when the OS has no fat-binary format.
_PREFERRED_ARCH = ("amd64", "arm64", "386", "arm")

# A member above this size stored uncompressed is the ZIP_STORED regression;
# below it, compression choice does not matter.
_STORED_LIMIT = 1 << 20


def os_key(goos: str) -> str:
    return _OS_KEY.get(goos, goos)


# --------------------------------------------------------------------------
# Mach-O
# --------------------------------------------------------------------------


def read_macho_arch(path: Path) -> tuple[int, int]:
    """Return (cputype, cpusubtype) for a thin Mach-O file."""
    head = path.read_bytes()[:12]
    if len(head) < 12:
        raise ValueError(f"{path}: too short to be a Mach-O file")
    for endian in ("<", ">"):
        magic, cputype, cpusubtype = struct.unpack(endian + "III", head)
        if magic in (MH_MAGIC, MH_MAGIC_64):
            return cputype, cpusubtype
    (be_magic,) = struct.unpack(">I", head[:4])
    if be_magic == FAT_MAGIC:
        raise ValueError(f"{path}: already a universal binary; nothing to merge")
    raise ValueError(f"{path}: not a Mach-O file (magic {be_magic:#010x})")


def write_macho_universal(paths: list[Path], out_path: Path) -> list[tuple[int, int]]:
    """Merge thin Mach-O files into one universal binary. Returns the slices'
    (cputype, cpusubtype) pairs, in the order written."""
    if len(paths) < 2:
        raise ValueError(f"a universal binary needs at least two slices, got {len(paths)}")
    slices = []
    seen: set[int] = set()
    for p in paths:
        cpu, sub = read_macho_arch(p)
        if cpu in seen:
            raise ValueError(f"two slices share CPU type {cpu}; a universal binary "
                             f"needs distinct architectures")
        seen.add(cpu)
        slices.append((cpu, sub, p.read_bytes()))
    # Apple orders slices by CPU type; keep the output deterministic.
    slices.sort(key=lambda s: s[0])

    align = 1 << _FAT_ALIGN_POW

    def aligned(v: int) -> int:
        return v if v % align == 0 else v + (align - v % align)

    header = struct.pack(">II", FAT_MAGIC, len(slices))
    offset = _FAT_HEADER_LEN + _FAT_ARCH_LEN * len(slices)
    records = b""
    offsets = []
    for cpu, sub, data in slices:
        offset = aligned(offset)
        offsets.append(offset)
        records += struct.pack(">IIIII", cpu, sub, offset, len(data), _FAT_ALIGN_POW)
        offset += len(data)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    with open(out_path, "wb") as fh:
        fh.write(header + records)
        for (cpu, sub, data), off in zip(slices, offsets):
            fh.write(b"\0" * (off - fh.tell()))
            fh.write(data)
    os.chmod(out_path, 0o755)
    return [(cpu, sub) for cpu, sub, _ in slices]


def read_fat_arches(path: Path) -> list[tuple[int, int, int, int]]:
    """Parse a universal binary: [(cputype, cpusubtype, offset, size), ...]."""
    data = path.read_bytes()
    magic, count = struct.unpack(">II", data[:8])
    if magic != FAT_MAGIC:
        raise ValueError(f"{path}: not a universal binary (magic {magic:#010x})")
    out = []
    for i in range(count):
        base = _FAT_HEADER_LEN + i * _FAT_ARCH_LEN
        cpu, sub, off, size, _align = struct.unpack(">IIIII", data[base:base + _FAT_ARCH_LEN])
        out.append((cpu, sub, off, size))
    return out


# --------------------------------------------------------------------------
# Bundle building
# --------------------------------------------------------------------------


class Target:
    __slots__ = ("goos", "goarch", "path", "member")

    def __init__(self, goos: str, goarch: str, path: Path, member: str):
        self.goos, self.goarch, self.path, self.member = goos, goarch, path, member


def discover_targets(bin_dir: Path, prefix: str) -> list[Target]:
    """Find one binary per registry target in bin_dir.

    Filenames come from registry.asset_name() - THE one place that turns
    (binary, goos, goarch) into a release asset filename - so this can never
    disagree with what release.yml uploaded.
    """
    found = []
    for t in registry.TARGETS:
        name = registry.asset_name(prefix, t["goos"], t["goarch"])
        p = bin_dir / name
        if p.is_file():
            found.append(Target(t["goos"], t["goarch"], p, f"bin/{name}"))
    return found


def _pick(targets: list[Target], goos: str) -> Target | None:
    for want in _PREFERRED_ARCH:
        for t in targets:
            if t.goos == goos and t.goarch == want:
                return t
    for t in targets:
        if t.goos == goos:
            return t
    return None


def _zip_write(zf: zipfile.ZipFile, member: str, data: bytes, mode: int) -> None:
    """Write one member DEFLATED, with an explicit unix mode.

    Both halves matter. ZipInfo defaults to ZIP_STORED, which would take the
    hudu bundle from 32 MB to ~81 MB; and it records no mode unless create_system
    says unix, which is why every published bundle's binaries land 0644 and
    cannot be executed.
    """
    info = zipfile.ZipInfo(member, date_time=(1980, 1, 1, 0, 0, 0))
    info.create_system = 3  # unix, so external_attr's mode bits are honored
    info.external_attr = (stat.S_IFREG | mode) << 16
    info.compress_type = zipfile.ZIP_DEFLATED
    zf.writestr(info, data)


def build_bundle(manifest_path: Path, bin_dir: Path, prefix: str, out_path: Path,
                 version: str | None = None) -> list[str]:
    """Write a multi-platform .mcpb whose every declared command resolves.

    Returns the archive's member names. Raises SystemExit on a bad input, and
    validates the finished archive before returning - a bundle with an
    unresolvable command cannot be produced by this function.
    """
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    targets = discover_targets(bin_dir, prefix)
    if not targets:
        raise SystemExit(
            f"mcpb_bundle: no binaries named '{prefix}-<goos>-<goarch>' found in {bin_dir}. "
            f"Expected the names release.yml uploads. Refusing to build a bundle with no server."
        )
    if version:
        manifest["version"] = version

    members: dict[str, bytes] = {}
    universal_member = ""
    darwin_slices = [t for t in targets if t.goos == "darwin"]
    if len(darwin_slices) > 1:
        # platform_overrides has no architecture axis, so a single darwin entry
        # can only serve both Macs as one universal binary.
        tmp = out_path.parent / f".{prefix}-darwin.universal"
        write_macho_universal([t.path for t in darwin_slices], tmp)
        universal_member = f"bin/{prefix}-darwin"
        members[universal_member] = tmp.read_bytes()
        tmp.unlink()

    for t in targets:
        if universal_member and t.goos == "darwin":
            # The merged binary carries both slices and the manifest cannot
            # address the thin ones, so shipping them too is pure dead weight.
            # (Upstream #4366 kept them; this is a deliberate deviation, and it
            # is why the bundle does not grow.)
            continue
        members[t.member] = t.path.read_bytes()

    server = manifest.setdefault("server", {})
    cfg = server.setdefault("mcp_config", {})
    # REPLACE the override map rather than merging into it. connectwise-manage's
    # checked-in manifest already carries hand-written overrides; a key for an OS
    # this build has no binary for would survive a merge and name a member the
    # archive does not contain. Per-OS args/env are carried forward for the OSes
    # that do get an override, because only `command` is ours to own.
    prior = cfg.get("platform_overrides") or {}
    overrides: dict[str, dict] = {}
    cfg["platform_overrides"] = overrides
    base_command = ""
    for goos in sorted({t.goos for t in targets}):
        if goos == "darwin" and universal_member:
            member = universal_member
        else:
            pick = _pick(targets, goos)
            if pick is None:
                continue
            member = pick.member
        key = os_key(goos)
        carried = {k: v for k, v in (prior.get(key) or {}).items() if k != "command"}
        overrides[key] = {"command": "${__dirname}/" + member, **carried}
        # Prefer darwin for the base command: Claude Desktop ships on macOS and
        # Windows, and win32 always carries its own override anyway.
        if not base_command or goos == "darwin":
            base_command = "${__dirname}/" + member
            server["entry_point"] = member
    cfg["command"] = base_command

    out_path.parent.mkdir(parents=True, exist_ok=True)
    rendered = (json.dumps(manifest, indent=2) + "\n").encode("utf-8")
    with zipfile.ZipFile(out_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        _zip_write(zf, MANIFEST_MEMBER, rendered, 0o644)
        for member in sorted(members):
            _zip_write(zf, member, members[member], 0o755)

    findings = validate_bundle(out_path)
    if findings:
        raise SystemExit("mcpb_bundle: the bundle just built does not validate - this is a bug "
                         "in the builder, not in the release:\n  - " + "\n  - ".join(findings))
    return [MANIFEST_MEMBER] + sorted(members)


# --------------------------------------------------------------------------
# Validation
# --------------------------------------------------------------------------


def _declared_commands(manifest: dict) -> list[tuple[str, str]]:
    """Every (label, command) the manifest declares, base plus per-OS override."""
    cfg = manifest.get("server", {}).get("mcp_config", {})
    out = [("server.mcp_config.command", cfg.get("command", ""))]
    overrides = cfg.get("platform_overrides") or {}
    for key in sorted(overrides):
        ov = overrides[key] or {}
        if ov.get("command"):
            out.append((f"server.mcp_config.platform_overrides.{key}.command", ov["command"]))
    return out


def validate_bundle(path: Path) -> list[str]:
    """Return findings (empty == the bundle can launch on every OS it claims)."""
    findings: list[str] = []
    try:
        zf = zipfile.ZipFile(path)
    except (OSError, zipfile.BadZipFile) as e:
        return [f"{path.name}: cannot be opened as a zip archive ({e})"]
    with zf:
        infos = {i.filename: i for i in zf.infolist()}
        if MANIFEST_MEMBER not in infos:
            return [f"{path.name}: contains no {MANIFEST_MEMBER}"]
        try:
            manifest = json.loads(zf.read(MANIFEST_MEMBER).decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as e:
            return [f"{path.name}: {MANIFEST_MEMBER} is not readable JSON ({e})"]

        referenced: set[str] = set()
        for label, command in _declared_commands(manifest):
            if not command:
                findings.append(f"{path.name}: {label} is empty; the host has nothing to run")
                continue
            prefix = "${__dirname}/"
            if not command.startswith(prefix):
                continue  # e.g. "node": the host resolves it, not the archive
            member = command[len(prefix):]
            referenced.add(member)
            if member not in infos:
                findings.append(
                    f"{path.name}: {label} is {command!r} but the archive contains no {member!r}. "
                    f"The extension would install, prompt for credentials, and fail at the first "
                    f"tool call. Members: {', '.join(sorted(infos))}"
                )

        # entry_point must resolve to a real member. Deliberately NOT
        # `entry_point == "bin/" + <mcp binary>`: that codifies the broken bare
        # name fleet-wide and false-REDs the one manifest that can launch.
        entry = manifest.get("server", {}).get("entry_point")
        if entry:
            referenced.add(entry)
            if entry not in infos:
                findings.append(f"{path.name}: server.entry_point is {entry!r} but the archive "
                                f"contains no such member")

        for name, info in sorted(infos.items()):
            if info.is_dir() or name == MANIFEST_MEMBER:
                continue
            is_binary = name.startswith("bin/") or name in referenced
            if is_binary:
                mode = (info.external_attr >> 16) & 0o7777
                if mode == 0:
                    findings.append(
                        f"{path.name}: {name} records no unix permissions, so it unpacks "
                        f"non-executable and the host cannot start it. The archive must be built "
                        f"with an explicit mode (create_system=3, external_attr)."
                    )
                elif not mode & 0o111:
                    findings.append(
                        f"{path.name}: {name} is mode {mode:04o} - not executable. `gh release "
                        f"download` writes 0644 and `zip -r` preserves it, which is how every "
                        f"published bundle shipped a server nothing can exec."
                    )
            if (info.compress_type == zipfile.ZIP_STORED
                    and info.file_size > _STORED_LIMIT):
                findings.append(
                    f"{path.name}: {name} is {info.file_size} bytes stored UNCOMPRESSED. "
                    f"zipfile.ZipInfo defaults to ZIP_STORED; use ZIP_DEFLATED or the bundle "
                    f"grows about 2.5x (hudu: 32 MB -> 81 MB)."
                )
    return findings


# --------------------------------------------------------------------------
# Self-test
# --------------------------------------------------------------------------


def _fake_macho(cputype: int, cpusubtype: int, payload: bytes) -> bytes:
    """A minimal but structurally real 64-bit little-endian Mach-O header."""
    return struct.pack("<8I", MH_MAGIC_64, cputype, cpusubtype, 2, 0, 0, 0, 0) + payload


CPU_X86_64 = 0x01000007
CPU_ARM64 = 0x0100000C


def self_test() -> int:
    """Both-directions proof: the validator must fire on the shipped defect and
    stay silent on a bundle this builder produced."""
    import tempfile

    failures: list[str] = []

    def expect(cond: bool, msg: str) -> None:
        if not cond:
            failures.append(msg)

    with tempfile.TemporaryDirectory() as td:
        root = Path(td)

        # --- fires: the exact shape of the 64 published bundles -------------
        broken = root / "broken.mcpb"
        with zipfile.ZipFile(broken, "w", zipfile.ZIP_DEFLATED) as zf:
            _zip_write(zf, MANIFEST_MEMBER, json.dumps({
                "name": "demo-mcp",
                "server": {"type": "binary", "entry_point": "bin/demo-mcp",
                           "mcp_config": {"command": "${__dirname}/bin/demo-mcp", "args": []}},
            }).encode(), 0o644)
            for n in ("bin/demo-mcp-darwin-arm64", "bin/demo-mcp-darwin-amd64",
                      "bin/demo-mcp-windows-amd64.exe"):
                _zip_write(zf, n, b"binary", 0o755)
        found = validate_bundle(broken)
        expect(any("contains no 'bin/demo-mcp'" in f for f in found),
               f"the published-bundle defect must be caught, got: {found}")
        expect(any("first tool call" in f for f in found),
               "the finding must say what the operator would experience")

        # --- fires: a per-OS override alone is broken for that OS -----------
        bad_override = root / "override.mcpb"
        with zipfile.ZipFile(bad_override, "w", zipfile.ZIP_DEFLATED) as zf:
            _zip_write(zf, MANIFEST_MEMBER, json.dumps({
                "server": {"entry_point": "bin/demo-mcp-darwin-arm64", "mcp_config": {
                    "command": "${__dirname}/bin/demo-mcp-darwin-arm64",
                    "platform_overrides": {
                        "win32": {"command": "${__dirname}/bin/demo-mcp-windows-amd64.exe"}},
                }},
            }).encode(), 0o644)
            _zip_write(zf, "bin/demo-mcp-darwin-arm64", b"b", 0o755)
        found = validate_bundle(bad_override)
        expect(any("platform_overrides.win32.command" in f for f in found),
               f"a broken per-OS override must be caught, got: {found}")

        # --- fires: the mode every published bundle actually ships ----------
        mode644 = root / "mode644.mcpb"
        with zipfile.ZipFile(mode644, "w", zipfile.ZIP_DEFLATED) as zf:
            _zip_write(zf, MANIFEST_MEMBER, json.dumps({
                "server": {"entry_point": "bin/demo-mcp-darwin-arm64", "mcp_config": {
                    "command": "${__dirname}/bin/demo-mcp-darwin-arm64"}},
            }).encode(), 0o644)
            _zip_write(zf, "bin/demo-mcp-darwin-arm64", b"b", 0o644)
        found = validate_bundle(mode644)
        expect(any("not executable" in f for f in found),
               f"a 0644 binary member must be caught, got: {found}")

        # --- fires: the ZIP_STORED regression -------------------------------
        stored = root / "stored.mcpb"
        with zipfile.ZipFile(stored, "w") as zf:
            _zip_write(zf, MANIFEST_MEMBER, json.dumps({
                "server": {"entry_point": "bin/demo-mcp-darwin-arm64", "mcp_config": {
                    "command": "${__dirname}/bin/demo-mcp-darwin-arm64"}},
            }).encode(), 0o644)
            info = zipfile.ZipInfo("bin/demo-mcp-darwin-arm64")
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | 0o755) << 16
            zf.writestr(info, b"x" * (_STORED_LIMIT + 1))  # ZipInfo defaults to STORED
        found = validate_bundle(stored)
        expect(any("stored UNCOMPRESSED" in f for f in found),
               f"a large ZIP_STORED member must be caught, got: {found}")

        # --- silent: the connectwise-manage shape ---------------------------
        # entry_point is an arch-suffixed member, NOT "bin/" + mcp_binary. It
        # must validate clean; asserting the bare name here is the false-RED
        # issue #287 warns about.
        cwm = root / "cwm.mcpb"
        with zipfile.ZipFile(cwm, "w", zipfile.ZIP_DEFLATED) as zf:
            _zip_write(zf, MANIFEST_MEMBER, json.dumps({
                "server": {"entry_point": "bin/demo-mcp-darwin-arm64", "mcp_config": {
                    "command": "${__dirname}/bin/demo-mcp-darwin-arm64",
                    "platform_overrides": {
                        "darwin": {"command": "${__dirname}/bin/demo-mcp-darwin-arm64"},
                        "linux": {"command": "${__dirname}/bin/demo-mcp-linux-amd64"},
                        "win32": {"command": "${__dirname}/bin/demo-mcp-windows-amd64.exe"}},
                }},
            }).encode(), 0o644)
            for n in ("bin/demo-mcp-darwin-arm64", "bin/demo-mcp-linux-amd64",
                      "bin/demo-mcp-windows-amd64.exe"):
                _zip_write(zf, n, b"b", 0o755)
        expect(validate_bundle(cwm) == [],
               f"an arch-suffixed entry_point whose members all exist must PASS, got "
               f"{validate_bundle(cwm)}")

        # --- silent: a bundle this builder produced -------------------------
        src = root / "src"
        (src / "bin").mkdir(parents=True)
        manifest_path = src / "manifest.json"
        manifest_path.write_text(json.dumps({
            "manifest_version": "0.3", "name": "demo-mcp", "version": "0.0.1",
            "server": {"type": "binary", "entry_point": "bin/demo-mcp",
                       "mcp_config": {"command": "${__dirname}/bin/demo-mcp", "args": [],
                                      "env": {"DEMO_TOKEN": "${user_config.demo_token}"}}},
        }), encoding="utf-8")
        binroot = root / "dl"
        binroot.mkdir()
        (binroot / "demo-mcp-darwin-amd64").write_bytes(_fake_macho(CPU_X86_64, 3, b"A" * 4096))
        (binroot / "demo-mcp-darwin-arm64").write_bytes(_fake_macho(CPU_ARM64, 0, b"B" * 4096))
        (binroot / "demo-mcp-linux-amd64").write_bytes(b"\x7fELF-amd64")
        (binroot / "demo-mcp-linux-arm64").write_bytes(b"\x7fELF-arm64")
        (binroot / "demo-mcp-windows-amd64.exe").write_bytes(b"MZ-win")

        out = root / "built" / "demo-mcp.mcpb"
        members = build_bundle(manifest_path, binroot, "demo-mcp", out, version="1.2.3")
        expect(validate_bundle(out) == [],
               f"the builder's own output must validate, got {validate_bundle(out)}")

        with zipfile.ZipFile(out) as zf:
            built = json.loads(zf.read(MANIFEST_MEMBER).decode())
            infos = {i.filename: i for i in zf.infolist()}
            universal = zf.read("bin/demo-mcp-darwin")
        ov = built["server"]["mcp_config"]["platform_overrides"]
        expect(set(ov) == {"darwin", "linux", "win32"},
               f"every OS present must get an override, got {sorted(ov)}")
        expect(ov["darwin"]["command"] == "${__dirname}/bin/demo-mcp-darwin",
               f"darwin must point at the merged universal binary, got {ov['darwin']}")
        expect(ov["linux"]["command"] == "${__dirname}/bin/demo-mcp-linux-amd64",
               f"linux must name amd64, the broadest target, got {ov['linux']}")
        expect(ov["win32"]["command"] == "${__dirname}/bin/demo-mcp-windows-amd64.exe",
               f"win32 must name the .exe, got {ov['win32']}")
        expect(built["version"] == "1.2.3", "the version must be stamped into the bundled manifest")
        expect("bin/demo-mcp-darwin-arm64" not in members
               and "bin/demo-mcp-darwin-amd64" not in members,
               f"the merged thin slices are unaddressable and must not be shipped, got {members}")
        expect("bin/demo-mcp-linux-arm64" in members,
               "a non-darwin arch the manifest cannot address is still what the release built")
        expect(all(i.compress_type == zipfile.ZIP_DEFLATED
                   for i in infos.values() if not i.is_dir()),
               "every member must be DEFLATED")
        expect(all((i.external_attr >> 16) & 0o111
                   for n, i in infos.items() if n.startswith("bin/")),
               "every binary member must carry the executable bit")

        # the merged file really is a universal Mach-O carrying both slices
        upath = root / "extracted-darwin"
        upath.write_bytes(universal)
        arches = read_fat_arches(upath)
        expect(len(arches) == 2, f"expected 2 slices in the universal binary, got {len(arches)}")
        expect({a[0] for a in arches} == {CPU_X86_64, CPU_ARM64},
               f"expected x86_64 and arm64 slices, got {[hex(a[0]) for a in arches]}")
        for cpu, _sub, off, size in arches:
            expect(off % (1 << _FAT_ALIGN_POW) == 0,
                   f"slice {cpu:#x} is not 2^{_FAT_ALIGN_POW} aligned (offset {off})")
            expect(read_macho_arch_bytes(universal[off:off + size]) == cpu,
                   f"slice {cpu:#x} does not contain that architecture's Mach-O header")

        # --- a stale override for an unbuilt OS must be dropped, not merged --
        # connectwise-manage's checked-in manifest carries hand-written
        # overrides. If a build has no binary for one of those OSes, keeping the
        # key would name a member the archive does not contain.
        stale_manifest = root / "stale-manifest.json"
        stale_manifest.write_text(json.dumps({
            "name": "demo-mcp", "version": "0.0.1",
            "server": {"type": "binary", "entry_point": "bin/demo-mcp", "mcp_config": {
                "command": "${__dirname}/bin/demo-mcp", "args": [],
                "platform_overrides": {
                    "darwin": {"command": "${__dirname}/bin/demo-mcp-darwin-arm64",
                               "env": {"KEEP_ME": "1"}},
                    "linux": {"command": "${__dirname}/bin/demo-mcp-linux-amd64"}},
            }},
        }), encoding="utf-8")
        darwin_only = root / "dl-darwin-only"
        darwin_only.mkdir()
        (darwin_only / "demo-mcp-darwin-arm64").write_bytes(_fake_macho(CPU_ARM64, 0, b"B" * 64))
        stale_out = root / "built" / "stale.mcpb"
        build_bundle(stale_manifest, darwin_only, "demo-mcp", stale_out)
        with zipfile.ZipFile(stale_out) as zf:
            stale_built = json.loads(zf.read(MANIFEST_MEMBER).decode())
        stale_ov = stale_built["server"]["mcp_config"]["platform_overrides"]
        expect(set(stale_ov) == {"darwin"},
               f"an override for an OS this build has no binary for must be dropped, got "
               f"{sorted(stale_ov)}")
        expect(stale_ov["darwin"].get("env") == {"KEEP_ME": "1"},
               "per-OS fields other than command must be carried forward")
        expect(validate_bundle(stale_out) == [], "the rewritten manifest must validate")

        # --- silent: a single darwin arch needs no merge --------------------
        solo = root / "dl-solo"
        solo.mkdir()
        (solo / "demo-mcp-darwin-arm64").write_bytes(_fake_macho(CPU_ARM64, 0, b"B" * 64))
        solo_out = root / "built" / "solo.mcpb"
        solo_members = build_bundle(manifest_path, solo, "demo-mcp", solo_out)
        expect("bin/demo-mcp-darwin" not in solo_members,
               "with one darwin slice there is nothing to merge")
        with zipfile.ZipFile(solo_out) as zf:
            solo_manifest = json.loads(zf.read(MANIFEST_MEMBER).decode())
        expect(solo_manifest["server"]["mcp_config"]["platform_overrides"]["darwin"]["command"]
               == "${__dirname}/bin/demo-mcp-darwin-arm64",
               "a lone darwin slice must be addressed directly")
        expect(validate_bundle(solo_out) == [], "the single-arch bundle must validate")

    if failures:
        print("FAIL: mcpb_bundle self-test\n")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("PASS: mcpb_bundle self-test - the validator fires on the published bare-name defect, a "
          "broken per-OS override, a 0644 binary member and a ZIP_STORED member, and is silent on "
          "the connectwise-manage manifest shape and on a bundle the builder produced (universal "
          "darwin Mach-O with both slices 16 KiB-aligned, per-OS overrides, DEFLATE, mode 0755).")
    return 0


def read_macho_arch_bytes(blob: bytes) -> int:
    magic, cputype = struct.unpack("<II", blob[:8])
    return cputype if magic in (MH_MAGIC, MH_MAGIC_64) else -1


# --------------------------------------------------------------------------


def main(argv: list[str]) -> int:
    # `__doc__` is None under `python -OO`; argparse would crash on the
    # attribute access before it could parse a single argument.
    summary = (__doc__ or "Build and validate .mcpb bundles.").splitlines()[0]
    ap = argparse.ArgumentParser(description=summary)
    ap.add_argument("--self-test", action="store_true",
                    help="run the built-in both-directions proof and exit")
    sub = ap.add_subparsers(dest="cmd")

    b = sub.add_parser("build", help="write a multi-platform .mcpb")
    b.add_argument("--manifest", required=True, help="the skill's manifest.json")
    b.add_argument("--bin-dir", required=True, help="directory holding the downloaded MCP binaries")
    b.add_argument("--binary-prefix", required=True,
                   help="the MCP binary base name, e.g. hudu-mcp. Must match what release.yml "
                        "uploaded; do NOT derive it from manifest.json's `name`, which still "
                        "carries a -pp- leak on 8 connectors.")
    b.add_argument("--out", required=True, help="output .mcpb path")
    b.add_argument("--version", default="", help="stamp this version into the bundled manifest")

    v = sub.add_parser("validate", help="assert a .mcpb can launch on every OS it claims")
    v.add_argument("bundles", nargs="+")

    args = ap.parse_args(argv)

    if args.self_test:
        return self_test()

    if args.cmd == "build":
        members = build_bundle(Path(args.manifest), Path(args.bin_dir), args.binary_prefix,
                               Path(args.out), args.version or None)
        size = Path(args.out).stat().st_size
        print(f"built {args.out} ({size} bytes, {len(members)} members):")
        for m in members:
            print(f"  {m}")
        return 0

    if args.cmd == "validate":
        rc = 0
        for raw in args.bundles:
            p = Path(raw)
            findings = validate_bundle(p)
            if findings:
                rc = 1
                print(f"FAIL: {p} cannot launch as published:\n")
                for f in findings:
                    print(f"  - {f}\n")
            else:
                print(f"PASS: {p} - every declared command resolves to an executable member.")
        if rc:
            print("See issue #287 and tools/maintainer/mcpb_bundle.py.")
        return rc

    ap.print_help()
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
