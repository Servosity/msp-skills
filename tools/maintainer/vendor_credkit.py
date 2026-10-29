#!/usr/bin/env python3
"""vendor_credkit.py - copy the shared credential kit into a connector.

    python3 tools/maintainer/vendor_credkit.py --slug zammad   # vendor (or refresh) one connector
    python3 tools/maintainer/vendor_credkit.py --check         # ci_guards guard 11: no copy drifted
    python3 tools/maintainer/vendor_credkit.py --self-test

The kit (tools/maintainer/credkit, the one copy of record) is a single Go
package. A connector carries it byte-identical at
skills/<slug>/cli/internal/credkit/: every non-test .go file of the package,
and nothing else. Its own tests and go.mod stay here, where CI runs them.

--slug copies the package in, removes any file in that directory that is not
part of the kit, and adds the modules the kit imports (golang.org/x/text,
github.com/mark3labs/mcp-go) to the connector's go.mod when it lacks them,
pinned to the version most connectors already use (`go get`, so go.sum
follows). It then proves the copy builds with `go vet ./internal/credkit/`.

--check fails when any connector's internal/credkit/ differs from the kit by
one byte, lacks a kit file, or carries a file the kit does not have, and names
the file.
"""

from __future__ import annotations

import argparse
import collections
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
KIT_REL = Path("tools/maintainer/credkit")
VENDOR_REL = Path("internal/credkit")
KIT_MODULES = ("golang.org/x/text", "github.com/mark3labs/mcp-go")


def kit_files(kit: Path) -> dict[str, bytes]:
    """The vendored set: every non-test .go file of the kit package."""
    return {p.name: p.read_bytes() for p in sorted(kit.glob("*.go")) if not p.name.endswith("_test.go")}


def carriers(root: Path) -> list[Path]:
    return sorted(p for p in root.glob(f"skills/*/cli/{VENDOR_REL.as_posix()}") if p.is_dir())


def drift(root: Path) -> list[str]:
    """Every way a carrier differs from the kit, one line each."""
    kit = root / KIT_REL
    want = kit_files(kit)
    problems = []
    for d in carriers(root):
        rel = d.relative_to(root).as_posix()
        have = {p.name for p in d.iterdir()}
        for name in sorted(want):
            f = d / name
            if name not in have:
                problems.append(f"{rel}/{name} is missing (the kit has {KIT_REL.as_posix()}/{name})")
            elif f.read_bytes() != want[name]:
                problems.append(f"{rel}/{name} differs from {KIT_REL.as_posix()}/{name}")
        for name in sorted(have - set(want)):
            problems.append(f"{rel}/{name} is not part of the kit")
    return problems


def check(root: Path, out=print) -> int:
    if not (root / KIT_REL).is_dir():
        if carriers(root):
            out(f"{KIT_REL.as_posix()} is missing while connectors carry the kit")
            return 1
        return 0
    problems = drift(root)
    for p in problems:
        out(p)
    if problems:
        out("vendor_credkit: change the kit in tools/maintainer/credkit only, then re-run "
            "vendor_credkit.py --slug <slug> for each connector")
        return 1
    out(f"vendor_credkit: {len(carriers(root))} connector copies match the kit")
    return 0


_REQ_RE = re.compile(r"^\s*(?:require\s+)?(\S+)\s+(v\S+)(?:\s*//.*)?$")


def requirements(gomod: str) -> dict[str, str]:
    """module -> version for every require line of a go.mod."""
    out = {}
    in_block = False
    for line in gomod.splitlines():
        s = line.strip()
        if s.startswith("require ("):
            in_block = True
            continue
        if in_block and s == ")":
            in_block = False
            continue
        if in_block or s.startswith("require "):
            m = _REQ_RE.match(s)
            if m:
                out[m.group(1)] = m.group(2)
    return out


def fleet_pin(root: Path, module: str) -> str | None:
    """The version of module most connectors require; the newest on a tie."""
    counts = collections.Counter()
    for gm in root.glob("skills/*/cli/go.mod"):
        v = requirements(gm.read_text()).get(module)
        if v:
            counts[v] += 1
    if not counts:
        return None
    top = max(counts.values())
    return max((v for v, n in counts.items() if n == top), key=_semver_key)


def _semver_key(v: str) -> tuple:
    nums = re.findall(r"\d+", v.split("-")[0])
    return tuple(int(n) for n in nums)


def vendor(root: Path, slug: str, out=print, run=subprocess.run) -> int:
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", slug):
        out(f"vendor_credkit: {slug!r} is not a connector slug")
        return 1
    cli = root / "skills" / slug / "cli"
    if not (cli / "go.mod").is_file():
        out(f"vendor_credkit: {cli.relative_to(root)}/go.mod not found")
        return 1
    want = kit_files(root / KIT_REL)
    dest = cli / VENDOR_REL
    dest.mkdir(parents=True, exist_ok=True)
    for p in dest.iterdir():
        if p.name not in want:
            if p.is_dir():
                shutil.rmtree(p)
            else:
                p.unlink()
            out(f"removed {p.relative_to(root)} (not part of the kit)")
    for name, data in want.items():
        (dest / name).write_bytes(data)
    out(f"vendored {len(want)} files into {dest.relative_to(root)}")
    have = requirements((cli / "go.mod").read_text())
    for mod in KIT_MODULES:
        if mod in have:
            continue
        pin = fleet_pin(root, mod)
        if not pin:
            out(f"vendor_credkit: no connector requires {mod}, so there is no fleet version to pin")
            return 1
        r = run(["go", "get", f"{mod}@{pin}"], cwd=cli, capture_output=True, text=True)
        if r.returncode != 0:
            out(f"vendor_credkit: go get {mod}@{pin} failed:\n{r.stderr.strip()}")
            return 1
        out(f"added {mod} {pin} to {(cli / 'go.mod').relative_to(root)} (the version most connectors use)")
    r = run(["go", "vet", f"./{VENDOR_REL.as_posix()}/"], cwd=cli, capture_output=True, text=True)
    if r.returncode != 0:
        out(f"vendor_credkit: go vet ./{VENDOR_REL.as_posix()}/ failed:\n{r.stderr.strip()}")
        return 1
    return 0


def self_test() -> int:
    failures = []

    def expect(cond: bool, msg: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + msg)
        if not cond:
            failures.append(msg)

    with tempfile.TemporaryDirectory() as t:
        root = Path(t)
        kit = root / KIT_REL
        kit.mkdir(parents=True)
        (kit / "a.go").write_text("package credkit\n")
        (kit / "b.go").write_text("package credkit // b\n")
        (kit / "a_test.go").write_text("package credkit\n")
        (kit / "go.mod").write_text("module credkit\n")
        for slug, text in (("one", "require golang.org/x/text v0.41.0\nrequire github.com/mark3labs/mcp-go v0.47.0\n"),
                           ("two", "require (\n\tgolang.org/x/text v0.42.0\n\tgithub.com/mark3labs/mcp-go v0.57.0 // indirect\n)\n"),
                           ("three", "require (\n\tgolang.org/x/text v0.42.0\n\tgithub.com/mark3labs/mcp-go v0.47.0\n)\n"),
                           ("bare", "module bare\n")):
            (root / "skills" / slug / "cli").mkdir(parents=True)
            (root / "skills" / slug / "cli" / "go.mod").write_text(f"module {slug}\n\ngo 1.26.10\n\n" + text)

        expect(vendor(root, "../one", out=lambda *_: None) == 1, "a slug that is a path is refused")
        expect(fleet_pin(root, "golang.org/x/text") == "v0.42.0", "x/text pins to the most common fleet version")
        expect(fleet_pin(root, "github.com/mark3labs/mcp-go") == "v0.47.0", "mcp-go pins to the most common fleet version")
        expect(check(root, out=lambda *_: None) == 0, "no carriers: --check passes")

        calls = []

        def fake_run(argv, cwd: str = "", **_):
            calls.append((argv, Path(cwd).parent.name))
            return subprocess.CompletedProcess(argv, 0, "", "")

        expect(vendor(root, "one", out=lambda *_: None, run=fake_run) == 0, "--slug vendors a connector")
        vend = root / "skills/one/cli" / VENDOR_REL
        expect(sorted(p.name for p in vend.iterdir()) == ["a.go", "b.go"], "only non-test .go files are vendored")
        expect([c for c in calls if c[0][:2] == ["go", "get"]] == [], "a connector with both modules gets no go get")
        expect(check(root, out=lambda *_: None) == 0, "a fresh copy passes --check")

        calls.clear()
        vendor(root, "bare", out=lambda *_: None, run=fake_run)
        gets = [c[0][2] for c in calls if c[0][:2] == ["go", "get"]]
        expect(gets == ["golang.org/x/text@v0.42.0", "github.com/mark3labs/mcp-go@v0.47.0"],
               f"a connector without the modules gets them at the fleet pin ({gets})")

        lines = []
        (vend / "b.go").write_text("package credkit // b.\n")
        expect(check(root, out=lines.append) == 1 and any("skills/one/cli/internal/credkit/b.go differs" in l for l in lines),
               "a one-byte change fails --check and names the file")
        (vend / "b.go").write_text("package credkit // b\n")
        lines.clear()
        (vend / "a.go").unlink()
        expect(check(root, out=lines.append) == 1 and any("a.go is missing" in l for l in lines), "a missing kit file fails --check")
        vendor(root, "one", out=lambda *_: None, run=fake_run)
        lines.clear()
        (vend / "extra.go").write_text("package credkit\n")
        expect(check(root, out=lines.append) == 1 and any("extra.go is not part of the kit" in l for l in lines),
               "an extra file fails --check")
        vendor(root, "one", out=lambda *_: None, run=fake_run)
        expect(not (vend / "extra.go").exists() and check(root, out=lambda *_: None) == 0, "--slug removes the extra file")
        shutil.rmtree(kit)
        expect(check(root, out=lambda *_: None) == 1, "the kit removed while connectors carry it fails --check")

    if failures:
        print(f"FAIL: vendor_credkit self-test ({len(failures)} failing)")
        return 1
    print("PASS: vendor_credkit self-test")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=(__doc__ or "").split("\n\n")[0])
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--slug", help="vendor the kit into skills/<slug>/cli/internal/credkit")
    g.add_argument("--check", action="store_true", help="fail when any connector copy differs from the kit")
    g.add_argument("--self-test", action="store_true")
    a = ap.parse_args()
    if a.self_test:
        return self_test()
    if a.check:
        return check(ROOT)
    return vendor(ROOT, a.slug)


if __name__ == "__main__":
    sys.exit(main())
