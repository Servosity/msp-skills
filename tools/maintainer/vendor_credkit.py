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
github.com/mark3labs/mcp-go, golang.org/x/sys) to the connector's go.mod when it lacks them,
pinned to the version most connectors already use (`go get`, so go.sum
follows). It then proves the copy builds with `go vet ./internal/credkit/`.

--slug also routes the one child process the press template starts before the
kit could see it: internal/cliutil/darwin_acl_darwin.go runs dsmemberutil, and
the contract requires every child to start through credkit.RunCommand. The
template is rewritten to ONE canonical kit-routed form (darwin_acl_kit_routed),
so every adopter carries the same bytes and the security gate's reviewed,
content-bound entries for that file (security_suppressions.json) hold for all
of them; a copy that is neither the template nor that form is refused.

--slug also writes the connector's constants table (the kit's consttable.go):
credkit_constants.go, in the package that registers the kit's SQL driver (or,
with no store, the one with the Declarations literal). It runs the kit's
cmd/constgen inside the connector, with the connector's Go and
golang.org/x/text, over its public text and every SQL statement its source
carries, so a process reads their verdict instead of decoding them again.
--refresh-hash rewrites the kit's constsource.go after an edit to the kit
(the hash a table is built for); --slug refuses a stale one.

--check fails when any connector's internal/credkit/ differs from the kit by
one byte, lacks a kit file, or carries a file the kit does not have, and names
the file; and when a constants table was built for another kit.
"""

from __future__ import annotations

import argparse
import collections
import contextlib
import hashlib
import io
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
KIT_REL = Path("tools/maintainer/credkit")
VENDOR_REL = Path("internal/credkit")
KIT_MODULES = ("golang.org/x/text", "github.com/mark3labs/mcp-go", "golang.org/x/sys")


def kit_files(kit: Path) -> dict[str, bytes]:
    """The vendored set: every non-test .go file of the kit package."""
    return {p.name: p.read_bytes() for p in sorted(kit.glob("*.go")) if not p.name.endswith("_test.go")}


HASH_FILE = "constsource.go"
CONST_FILE = "credkit_constants.go"
_HASH_RE = re.compile(r'(?m)^const kitSourceHash = "([0-9a-f]*)"$')


def kit_source_hash(kit: Path) -> str:
    """What constsource.go must say: the SHA-256 over every other non-test
    .go file of the kit in name order, each its name, a NUL, its bytes, a NUL."""
    h = hashlib.sha256()
    for p in sorted(kit.glob("*.go")):
        if p.name.endswith("_test.go") or p.name == HASH_FILE:
            continue
        h.update(p.name.encode() + b"\0" + p.read_bytes() + b"\0")
    return h.hexdigest()


def stated_hash(kit: Path) -> str | None:
    f = kit / HASH_FILE
    m = _HASH_RE.search(f.read_text()) if f.is_file() else None
    return m.group(1) if m else None


def refresh_hash(root: Path, out=print) -> int:
    kit = root / KIT_REL
    f = kit / HASH_FILE
    if stated_hash(kit) is None:
        out(f"vendor_credkit: {(KIT_REL / HASH_FILE).as_posix()} has no kitSourceHash line")
        return 1
    f.write_text(_HASH_RE.sub(f'const kitSourceHash = "{kit_source_hash(kit)}"', f.read_text(), count=1))
    out(f"kitSourceHash is {kit_source_hash(kit)}")
    return 0


def constants_dir(cli: Path) -> Path | None:
    """The package a connector's constants table goes in: the one that
    registers the kit's SQL driver, else the one with the Declarations
    literal. None when there is no single such package."""
    for marker in ("credkit.SQLDriver(", "credkit.Declarations{"):
        dirs = {p.parent for p in (cli / "internal").rglob("*.go")
                if not p.name.endswith("_test.go") and VENDOR_REL.as_posix() not in p.relative_to(cli).as_posix()
                and marker in p.read_text(encoding="utf-8", errors="replace")}
        if len(dirs) == 1:
            return dirs.pop()
        if dirs:
            return None
    return None


def go_toolchain(gomod: str) -> str | None:
    """The Go a connector's CI and release build with: go.mod's toolchain
    line, else its go line (setup-go's go-version-file)."""
    m = re.search(r"(?m)^toolchain\s+(go\S+)", gomod) or re.search(r"(?m)^go\s+(\S+)", gomod)
    if not m:
        return None
    return m.group(1) if m.group(1).startswith("go") else "go" + m.group(1)


def write_constants(root: Path, cli: Path, module: str, out=print, run=subprocess.run) -> int:
    """Build the connector's constants table with the kit's cmd/constgen, run
    inside the connector (its own Go and golang.org/x/text)."""
    d = constants_dir(cli)
    if d is None:
        return 0
    pm = re.search(r"(?m)^package\s+(\w+)", next(p for p in sorted(d.glob("*.go")) if not p.name.endswith("_test.go")).read_text())
    gen = cli / "internal" / "credkitconstgen"
    src = (root / KIT_REL / "cmd/constgen/main.go").read_text()
    if src.count('\t"credkit"\n') != 1:
        out("vendor_credkit: cmd/constgen/main.go does not import the kit as \"credkit\"")
        return 1
    env = dict(os.environ)
    tc = go_toolchain((cli / "go.mod").read_text())
    if tc:
        env["GOTOOLCHAIN"] = tc
    try:
        gen.mkdir()
        (gen / "main.go").write_text(src.replace('\t"credkit"\n', f'\t"{module}/{VENDOR_REL.as_posix()}"\n'))
        r = run(["go", "run", "./internal/credkitconstgen", "-root", ".", "-pkg", pm.group(1),
                 "-kit", f"{module}/{VENDOR_REL.as_posix()}"], cwd=cli, capture_output=True, text=True, env=env)
    finally:
        shutil.rmtree(gen, ignore_errors=True)
    if r.returncode != 0:
        out(f"vendor_credkit: building the constants table failed:\n{r.stderr.strip()}")
        return 1
    (d / CONST_FILE).write_text(r.stdout)
    out(f"wrote {(d / CONST_FILE).relative_to(root)}")
    return 0


def constants_drift(root: Path) -> list[str]:
    """Every constants table built for another kit than this one."""
    want = stated_hash(root / KIT_REL)
    return [f"{p.relative_to(root)} was built for another kit: re-run vendor_credkit.py --slug {p.relative_to(root).parts[1]}"
            for p in sorted(root.glob(f"skills/*/cli/internal/**/{CONST_FILE}"))
            if f"credkit-constants 1\\n{want} " not in p.read_text()]


def carriers(root: Path) -> list[Path]:
    return sorted(p for p in root.glob(f"skills/*/cli/{VENDOR_REL.as_posix()}") if p.is_dir())


def carrier_drift(root: Path, d: Path, want: dict[str, bytes] | None = None) -> list[str]:
    """Every way one carrier differs from the kit, one line each. Other gates
    (check_env_schema) treat a carrier as the reviewed kit only when this is empty."""
    if want is None:
        want = kit_files(root / KIT_REL)
    rel = d.relative_to(root).as_posix()
    have = {p.name for p in d.iterdir()}
    problems = []
    for name in sorted(want):
        f = d / name
        if name not in have:
            problems.append(f"{rel}/{name} is missing (the kit has {KIT_REL.as_posix()}/{name})")
        elif not f.is_file() or f.is_symlink() or f.read_bytes() != want[name]:
            problems.append(f"{rel}/{name} differs from {KIT_REL.as_posix()}/{name}")
    for name in sorted(have - set(want)):
        problems.append(f"{rel}/{name} is not part of the kit")
    return problems


def drift(root: Path) -> list[str]:
    """Every way a carrier differs from the kit, one line each."""
    want = kit_files(root / KIT_REL)
    problems = []
    for d in carriers(root):
        problems += carrier_drift(root, d, want)
    return problems


def check(root: Path, out=print) -> int:
    if not (root / KIT_REL).is_dir():
        if carriers(root):
            out(f"{KIT_REL.as_posix()} is missing while connectors carry the kit")
            return 1
        return 0
    problems = drift(root) + constants_drift(root)
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


DARWIN_ACL_REL = Path("internal/cliutil/darwin_acl_darwin.go")
_ACL_EXEC = ('\tout, err := exec.Command("/usr/bin/dsmemberutil", "getuuid", "-u", '
             'strconv.FormatUint(uint64(uid), 10)).CombinedOutput() // #nosec G204 -- fixed system path '
             'plus a decimal uid; PATH must not hide it.\n')
_ACL_ROUTED = ('\tcmd := exec.Command("/usr/bin/dsmemberutil", "getuuid", "-u", '
               'strconv.FormatUint(uint64(uid), 10)) // #nosec G204 -- fixed system path plus a decimal uid; '
               'PATH must not hide it.\n'
               '\tvar buf bytes.Buffer\n'
               '\tcmd.Stdout, cmd.Stderr = &buf, &buf\n'
               '\terr := credkit.RunCommand(cmd) // Hand-fix (handfixes.json credkit-wiring)\n'
               '\tout := buf.Bytes()\n')


def darwin_acl_kit_routed(text: str, module: str) -> str | None:
    """The press template's darwin_acl_darwin.go with dsmemberutil started
    through credkit.RunCommand: the one canonical form every adopter gets.
    None when text is not the template (already routed, or edited)."""
    head = 'import (\n\t"encoding/binary"\n'
    tail = '\t"golang.org/x/sys/unix"\n)\n'
    if text.count(_ACL_EXEC) != 1 or text.count(head) != 1 or text.count(tail) != 1:
        return None
    return (text.replace(head, 'import (\n\t"bytes"\n\t"encoding/binary"\n')
                .replace(tail, f'\t"golang.org/x/sys/unix"\n\n\t"{module}/internal/credkit"\n)\n')
                .replace(_ACL_EXEC, _ACL_ROUTED))


def darwin_acl_is_routed(text: str, module: str) -> bool:
    return _ACL_ROUTED in text and f'\t"{module}/internal/credkit"\n' in text


def route_darwin_acl(cli: Path, out=print) -> int:
    """Rewrite cli's darwin_acl_darwin.go to the kit-routed form (see the
    module docstring). 0 when done or nothing to do, 1 when the file is
    neither the template nor the routed form."""
    f = cli / DARWIN_ACL_REL
    if not f.is_file():
        return 0
    text = f.read_text(encoding="utf-8")
    m = re.search(r"(?m)^module\s+(\S+)", (cli / "go.mod").read_text())
    routed = darwin_acl_kit_routed(text, m.group(1)) if m else None
    if routed is not None:
        f.write_text(routed, encoding="utf-8")
        out(f"routed dsmemberutil through credkit.RunCommand in {DARWIN_ACL_REL.as_posix()}")
        return 0
    if m and darwin_acl_is_routed(text, m.group(1)):
        return 0
    out(f"vendor_credkit: {DARWIN_ACL_REL.as_posix()} is neither the press template nor the kit-routed form; "
        "re-generate it from the template, then run --slug again")
    return 1


def vendor(root: Path, slug: str, out=print, run=subprocess.run) -> int:
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", slug):
        out(f"vendor_credkit: {slug!r} is not a connector slug")
        return 1
    cli = root / "skills" / slug / "cli"
    if not (cli / "go.mod").is_file():
        out(f"vendor_credkit: {cli.relative_to(root)}/go.mod not found")
        return 1
    stated = stated_hash(root / KIT_REL)
    if stated is not None and stated != kit_source_hash(root / KIT_REL):
        out(f"vendor_credkit: {(KIT_REL / HASH_FILE).as_posix()} is stale; run vendor_credkit.py --refresh-hash")
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
    if route_darwin_acl(cli, out) != 0:
        return 1
    r = run(["go", "vet", f"./{VENDOR_REL.as_posix()}/"], cwd=cli, capture_output=True, text=True)
    if r.returncode != 0:
        out(f"vendor_credkit: go vet ./{VENDOR_REL.as_posix()}/ failed:\n{r.stderr.strip()}")
        return 1
    if stated is not None:
        module = re.search(r"(?m)^module\s+(\S+)", (cli / "go.mod").read_text()).group(1)
        return write_constants(root, cli, module, out, run)
    return 0


def bound_to_routed(root: Path = ROOT) -> bool:
    """Every content-bound security_suppressions.json entry for the template
    file also accepts the kit-routed form of each connector's copy, so an
    adopter keeps exactly the reviewed exceptions it had (same file, same rules)."""
    import json
    sys.path.insert(0, str(ROOT / "tools/maintainer"))
    from check_security_gate import bound_digest  # noqa: E402  (local tools/ module)
    entries = [e for e in json.loads((root / "tools/maintainer/security_suppressions.json").read_text())["bound"]
               if e.get("file") == "cli/" + DARWIN_ACL_REL.as_posix()]
    if not entries:
        return False
    ok = True
    for f in sorted(root.glob(f"skills/*/cli/{DARWIN_ACL_REL.as_posix()}")):
        cli = f.parents[2]
        module = re.search(r"(?m)^module\s+(\S+)", (cli / "go.mod").read_text()).group(1)
        text = f.read_text(encoding="utf-8")
        # An adopter's copy is already routed; the digests below still decide.
        routed = text if darwin_acl_is_routed(text, module) else darwin_acl_kit_routed(text, module)
        if routed is None:
            print(f"    {f.relative_to(root)} is neither the press template nor the kit-routed form")
            ok = False
            continue
        slug = cli.parent.name
        edited = routed.replace("cmd.Stdout, cmd.Stderr = &buf, &buf", "cmd.Stdout, cmd.Stderr = &buf, nil")
        for e in entries:
            neutral = e.get("slug_neutral") is True
            d = bound_digest(routed.encode(), slug, neutral)
            if e.get("bind") != "file" or d not in e["sha256"]:
                print(f"    {e['rule']}: the kit-routed {f.relative_to(root)} ({d}) is not bound")
                ok = False
            if edited == routed or bound_digest(edited.encode(), slug, neutral) in e["sha256"]:
                print(f"    {e['rule']}: an edit to the kit-routed {f.relative_to(root)} is still bound")
                ok = False
    return ok


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
        for slug, text in (("one", "require golang.org/x/text v0.41.0\nrequire github.com/mark3labs/mcp-go v0.47.0\n"
                                   "require golang.org/x/sys v0.46.0\n"),
                           ("two", "require (\n\tgolang.org/x/text v0.42.0\n\tgithub.com/mark3labs/mcp-go v0.57.0 // indirect\n"
                                   "\tgolang.org/x/sys v0.46.0 // indirect\n)\n"),
                           ("three", "require (\n\tgolang.org/x/text v0.42.0\n\tgithub.com/mark3labs/mcp-go v0.47.0\n"
                                     "\tgolang.org/x/sys v0.31.0\n)\n"),
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

        (kit / HASH_FILE).write_text('package credkit\n\nconst kitSourceHash = "00"\n')
        expect(vendor(root, "one", out=lambda *_: None, run=fake_run) == 1, "a stale kitSourceHash refuses --slug")
        expect(refresh_hash(root, out=lambda *_: None) == 0 and stated_hash(kit) == kit_source_hash(kit)
               and vendor(root, "one", out=lambda *_: None, run=fake_run) == 0, "--refresh-hash makes it current")
        table = root / "skills/one/cli/internal/credsql" / CONST_FILE
        table.parent.mkdir(parents=True)
        table.write_text('const constantsTable = "credkit-constants 1\\n' + "0" * 64 + ' go1.26.10 v0.42.0\\n"\n')
        expect(check(root, out=lambda *_: None) == 1, "a constants table built for another kit fails --check")
        table.write_text('const constantsTable = "credkit-constants 1\\n' + kit_source_hash(kit) + ' go1.26.10 v0.42.0\\n"\n')
        expect(check(root, out=lambda *_: None) == 0, "a constants table built for this kit passes --check")
        shutil.rmtree(table.parent)
        (kit / HASH_FILE).unlink()

        calls.clear()
        vendor(root, "bare", out=lambda *_: None, run=fake_run)
        gets = [c[0][2] for c in calls if c[0][:2] == ["go", "get"]]
        expect(gets == ["golang.org/x/text@v0.42.0", "github.com/mark3labs/mcp-go@v0.47.0", "golang.org/x/sys@v0.46.0"],
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
        acl = root / "skills/one/cli" / DARWIN_ACL_REL
        acl.parent.mkdir(parents=True)
        # The first copy still in the template's form: adopters' copies are routed.
        template = next((t for t in sorted(ROOT.glob(f"skills/*/cli/{DARWIN_ACL_REL.as_posix()}"))
                         if darwin_acl_kit_routed(t.read_text(encoding="utf-8"), "m") is not None), None)
        if template is not None:
            acl.write_text(template.read_text(encoding="utf-8"), encoding="utf-8")
            expect(vendor(root, "one", out=lambda *_: None, run=fake_run) == 0
                   and _ACL_ROUTED in acl.read_text() and '\t"one/internal/credkit"\n' in acl.read_text(),
                   "--slug routes the press template's dsmemberutil through credkit.RunCommand")
            once = acl.read_text()
            expect(vendor(root, "one", out=lambda *_: None, run=fake_run) == 0 and acl.read_text() == once,
                   "--slug leaves the kit-routed form unchanged")
            acl.write_text(once.replace("cmd.Stdout, cmd.Stderr", "cmd.Stdout"))
            expect(vendor(root, "one", out=lambda *_: None, run=fake_run) == 1,
                   "an edited darwin_acl_darwin.go is refused, not rewritten")
            acl.unlink()
            # A connector that has already adopted the kit, against the real rules.
            adopter = root / "adopter"
            cli = adopter / "skills" / template.parents[3].name / "cli"
            (cli / DARWIN_ACL_REL).parent.mkdir(parents=True)
            (adopter / "tools/maintainer").mkdir(parents=True)
            (adopter / "tools/maintainer/security_suppressions.json").write_text(
                (ROOT / "tools/maintainer/security_suppressions.json").read_text(encoding="utf-8"), encoding="utf-8")
            gomod = (template.parents[2] / "go.mod").read_text(encoding="utf-8")
            (cli / "go.mod").write_text(gomod, encoding="utf-8")
            adopted = darwin_acl_kit_routed(template.read_text(encoding="utf-8"),
                                            re.search(r"(?m)^module\s+(\S+)", gomod).group(1))
            (cli / DARWIN_ACL_REL).write_text(adopted, encoding="utf-8")
            expect(bound_to_routed(adopter), "a copy already kit-routed still counts as bound")
            (cli / DARWIN_ACL_REL).write_text(adopted.replace("&buf, &buf", "&buf, nil"), encoding="utf-8")
            with contextlib.redirect_stdout(io.StringIO()):
                edited_refused = not bound_to_routed(adopter)
            expect(edited_refused, "an edited kit-routed copy does not count as bound")
        elif not any(ROOT.glob(f"skills/*/cli/{DARWIN_ACL_REL.as_posix()}")):
            expect(False, "no connector carries darwin_acl_darwin.go to test against")
        expect(bound_to_routed(), "every rule bound to the template binds the kit-routed form too, and not an edit of it")
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
    g.add_argument("--refresh-hash", action="store_true", help="rewrite the kit's constsource.go after an edit to the kit")
    a = ap.parse_args()
    if a.self_test:
        return self_test()
    if a.refresh_hash:
        return refresh_hash(ROOT)
    if a.check:
        return check(ROOT)
    return vendor(ROOT, a.slug)


if __name__ == "__main__":
    sys.exit(main())
