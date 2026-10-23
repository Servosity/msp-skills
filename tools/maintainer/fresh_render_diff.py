#!/usr/bin/env python3
"""fresh_render_diff.py - re-render a connector with its own press version and
spec, and report only what differs from template output.

Why (publish plan step 5.4)
---------------------------
A connector PR is mostly press output: PR #400 (CloudAlly) carried 346 files
under cli/, 282 of them marked DO NOT EDIT. A reviewer cannot tell from the
header alone whether a generated file was edited by hand, so the whole tree
gets read every round. This tool regenerates the connector with the SAME press
version from the SAME spec and compares byte for byte. What is left is the
review set:

  hand-written       a file under cli/ the render does not produce
  generated-edited   a file the render produces, with different bytes
  template-deleted   a file the render produces that the connector lacks
  dependency         go.mod or go.sum when they differ from the render (the
                     render writes no go.sum, so go.sum is always listed)
  spec               the connector's own press/spec.* (read in full)
  symlink            any symlink under cli/ (never followed)

Everything byte-identical to the render is template output: it is reviewed
once per press version, not once per connector.

Inputs, and how each fails closed
---------------------------------
* Press version: manifest.json `printing_press_version`, its
  `press_provenance.printing_press_version`, and press/.printing-press.json.
  Every value present must agree. Missing or disagreeing -> exit 2.
* Press binary: `--press-bin`, `$PRINTING_PRESS_BIN`, then `cli-printing-press`
  and `printing-press` on PATH, ~/go/bin, ~/printing-press/bin and
  ~/.cache/fresh-render/press/<version>/. A candidate counts only when its own
  `--version` reports exactly the recorded version. None -> exit 2, naming the
  version and the one command that installs it. A different version is never
  used.
* Spec: `--spec FILE`, then skills/<slug>/press/spec.*, then the local press
  folders (~/printing-press/library and ~/printing-press/manuscripts, or
  $PRESS_HOME). A candidate counts only when its sha256 (raw, or with CRLF
  normalized to LF, as the press computes it) equals the recorded
  `spec_checksum`. None -> exit 2, unless --spec-invariant (below).
* A spec with an external `$ref` (any value not starting with `#`) is refused:
  the press would read whatever file or URL it names.

How the render runs
-------------------
`<press> generate --spec S --output D --validate=false --name <api>` plus the
recorded `--spec-source`, `--auth-preference` and `--category`, the same
command the press itself uses to synthesize a regen base. It runs in a private
temp directory with a scrubbed environment: a throwaway HOME, PATH=/usr/bin:/bin,
no git config, and every proxy variable pointed at a closed local port. A
contributed spec is untrusted data, so for a contributor PR run this inside
msp-sandbox (no network, no credentials): the recipe is in msp-skills-publish
references/contribution-lane.md.

The render is then put in the form a vendored connector takes, with the exact
transforms the pipeline applies and nothing else:
  1. onboard.py's vendor subset: go.mod, go.sum, LICENSE, NOTICE, Makefile,
     .golangci.yml, cmd/ and internal/ (and any other top-level directory
     holding Go), minus bin, build, dist, .manuscripts, .git and vendor;
  2. post-regen-rename.sh (strip-pp-cli): `<api>-pp-cli`/`<api>-pp-mcp` become
     `<api>-cli`/`<api>-mcp` in .go files and the Makefile, module-path imports
     kept, cmd/<api>-pp-{cli,mcp} renamed;
  3. check_copyright.py's canonical line for every copyright line in .go
     files, NOTICE and LICENSE.
Every wrong guess here (a flag not recorded, a transform not replayed) can only
ADD files to the review set: a file leaves it only when its bytes equal the
render.

--spec-invariant (opt-in fallback, weaker evidence)
---------------------------------------------------
When the spec is not available (a contributor's spec is not in the PR and
decision 7 says not to ask for state from their machine), the tool can still
take out the files that do not depend on the spec at all. It renders a set of
built-in reference specs (plus any --reference-spec), all at the recorded press
version and under the connector's own name, and treats a file as template
output only when every reference render produces it with identical bytes AND
the connector's file equals those bytes. Spec-dependent generated files stay in
the review set, labelled as such. The guarantee is empirical over the
reference set; `--calibrate` checks it against every local real spec at a
press version and fails if any file the references call invariant varies
across real specs.

Usage
-----
    python3 tools/maintainer/fresh_render_diff.py --pr 400
    python3 tools/maintainer/fresh_render_diff.py --tree skills/sonicwall
    python3 tools/maintainer/fresh_render_diff.py --tree DIR --slug S --spec FILE --press-bin FILE
    python3 tools/maintainer/fresh_render_diff.py --pr 400 --spec-invariant [--reference-spec FILE ...]
    python3 tools/maintainer/fresh_render_diff.py --calibrate 4.33.5
    python3 tools/maintainer/fresh_render_diff.py --self-test
Add --json for the machine-readable report, --list for one review path per line,
--diff for the render -> connector hunks of every edited generated file.

Exit 0 = report written (the review set may be any size), 1 = --calibrate or
--self-test found a failure, 2 = could not render (fail closed; the reason is
named). Pure stdlib.
"""

from __future__ import annotations

import argparse
import difflib
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
GH_REPO = "Servosity/msp-skills"
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,62}$")
SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+$")
# The whole line must be the version: "4.33.5-rc.1" is a different version.
VERSION_OUT_RE = re.compile(r"cli-printing-press(?: version)? v?(\d+\.\d+\.\d+)")

# onboard.py (msp-skills-publish) vendors exactly this subset of a press tree.
VENDOR_FILES = ("go.mod", "go.sum", "LICENSE", "NOTICE", "Makefile", ".golangci.yml")
VENDOR_DIRS = ("cmd", "internal")
EXCLUDE_DIRS = {"bin", "build", "dist", ".manuscripts", ".git", "vendor"}
SPEC_NAMES = ("spec.json", "spec.yaml", "spec.yml", "spec.graphql")

CANON_GO = ("// Copyright 2026 Servosity Inc. and msp-skills contributors. "
            "Licensed under Apache-2.0. See LICENSE.")
CANON_TXT = "Copyright 2026 Servosity Inc. and msp-skills contributors"
GO_COPYRIGHT_RE = re.compile(r"(?m)^// Copyright \d{4}\b.*$")
TXT_COPYRIGHT_RE = re.compile(r"(?m)^([ \t]*)Copyright \d{4}\b.*$")
DO_NOT_EDIT = b"DO NOT EDIT"
# `$ref` whose value (quoted or bare) does not start with `#`.
EXTERNAL_REF_RE = re.compile(r"""\$ref["']?\s*:\s*(?:["'](?!#)|(?!["'#\s]))""")

RENDER_TIMEOUT = 900
RENDERS = 3
REVIEW_CLASSES = ("hand-written", "generated-edited", "template-deleted",
                  "dependency", "spec", "symlink",
                  "generated-spec-dependent")


class Refuse(Exception):
    """A fail-closed stop: the tool cannot vouch for a render."""


# --------------------------------------------------------------------------
# provenance

@dataclass
class Provenance:
    slug: str
    version: str
    checksum: str | None
    api: str
    flags: list[str] = field(default_factory=list)
    sources: dict = field(default_factory=dict)


def _read_json(p: Path) -> dict:
    try:
        d = json.loads(p.read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:
        raise Refuse(f"{p.name}: unreadable ({e.__class__.__name__})")
    if not isinstance(d, dict):
        raise Refuse(f"{p.name}: not a JSON object")
    return d


def read_provenance(skill_dir: Path, slug: str) -> Provenance:
    mf = skill_dir / "manifest.json"
    if not mf.is_file() or mf.is_symlink():
        raise Refuse(f"skills/{slug}/manifest.json is missing")
    m = _read_json(mf)
    pp = skill_dir / "press" / ".printing-press.json"
    meta = _read_json(pp) if pp.is_file() and not pp.is_symlink() else {}
    prov = m.get("press_provenance") if isinstance(m.get("press_provenance"), dict) else {}
    versions = {
        "manifest.json printing_press_version": m.get("printing_press_version"),
        "manifest.json press_provenance": prov.get("printing_press_version"),
        "press/.printing-press.json": meta.get("printing_press_version"),
    }
    present = {k: v for k, v in versions.items() if v not in (None, "")}
    if not present:
        raise Refuse(f"skills/{slug}: no printing_press_version is recorded anywhere")
    vals = {str(v).lstrip("v") for v in present.values()}
    if len(vals) != 1:
        raise Refuse(f"skills/{slug}: recorded press versions disagree: "
                     + ", ".join(f"{k}={v}" for k, v in present.items()))
    version = vals.pop()
    if not SEMVER_RE.match(version):
        raise Refuse(f"skills/{slug}: press version {version!r} is not X.Y.Z")
    sums = {k: v for k, v in (("manifest.json press_provenance", prov.get("spec_checksum")),
                              ("press/.printing-press.json", meta.get("spec_checksum")))
            if v}
    if len(set(sums.values())) > 1:
        raise Refuse(f"skills/{slug}: recorded spec checksums disagree: "
                     + ", ".join(f"{k}={v}" for k, v in sums.items()))
    checksum = next(iter(sums.values()), None)
    if checksum is not None and not re.fullmatch(r"sha256:[0-9a-f]{64}", str(checksum)):
        raise Refuse(f"skills/{slug}: spec_checksum {checksum!r} is not sha256:<64 hex>")
    gomod = skill_dir / "cli" / "go.mod"
    if not gomod.is_file() or gomod.is_symlink():
        raise Refuse(f"skills/{slug}/cli/go.mod is missing")
    mm = re.search(r"(?m)^module\s+([a-z0-9][a-z0-9-]*)-pp-cli\s*$", gomod.read_text(errors="replace"))
    if not mm:
        raise Refuse(f"skills/{slug}/cli/go.mod: module line is not `<api>-pp-cli`, "
                     "so the render's names cannot be mapped")
    api = mm.group(1)
    flags = ["--name", api]
    for key, flag in (("spec_source", "--spec-source"), ("auth_preference", "--auth-preference"),
                      ("category", "--category")):
        v = meta.get(key)
        if isinstance(v, str) and v and re.fullmatch(r"[A-Za-z0-9_.:/ -]{1,80}", v):
            flags += [flag, v]
    return Provenance(slug=slug, version=version, checksum=checksum, api=api, flags=flags,
                      sources={"versions": present, "checksums": sums,
                               "press_meta": bool(meta)})


# --------------------------------------------------------------------------
# press binary

def _scrubbed_env(tmp: Path) -> dict:
    home = tmp / "home"
    home.mkdir(parents=True, exist_ok=True)
    sink = "http://127.0.0.1:9"
    return {
        "HOME": str(home), "PATH": "/usr/bin:/bin", "TMPDIR": str(tmp),
        "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8",
        "XDG_CONFIG_HOME": str(home / ".config"), "XDG_CACHE_HOME": str(home / ".cache"),
        "XDG_STATE_HOME": str(home / ".local" / "state"), "XDG_DATA_HOME": str(home / ".local" / "share"),
        "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_TERMINAL_PROMPT": "0",
        "HTTP_PROXY": sink, "HTTPS_PROXY": sink, "ALL_PROXY": sink,
        "http_proxy": sink, "https_proxy": sink, "all_proxy": sink, "NO_PROXY": "", "no_proxy": "",
        "GOPROXY": "off", "GOFLAGS": "-mod=mod", "GOTOOLCHAIN": "local",
    }


SANDBOX_EXEC = "/usr/bin/sandbox-exec"
# What a process needs to start and run on macOS, and nothing that holds a
# user's or another process's data: system libraries and binaries, the dyld
# cache, devices, the time zone database.
JAIL_READ_SUBPATHS = ("/usr/lib", "/usr/share", "/usr/bin", "/bin", "/System", "/dev",
                      "/private/var/db/timezone", "/private/var/db/dyld")
JAIL_READ_LITERALS = ("/", "/private/etc/localtime")
# Self-test only: the stand-in press is a Python script and needs its
# interpreter; off macOS (no jail) the self-test runs it, our own script, bare.
_JAIL_EXTRA_READ: list[str] = []
_SELFTEST_UNJAILED = False


def jailed(argv: list[str], binary: Path, work: Path) -> list[str]:
    """The press never runs with this user's files or the network in reach.
    Inside msp-sandbox (SANDBOX_MODE set) the container is the jail. On macOS,
    sandbox-exec denies the network, every write outside `work`, and every file
    content read except `work`, the press binary itself and the system paths in
    JAIL_READ_*, so a spec that names a local file (however its $ref is
    spelled, wherever the file is) reads nothing. Anywhere else the render is
    refused: run it inside msp-sandbox."""
    if os.environ.get("SANDBOX_MODE") or _SELFTEST_UNJAILED:
        return argv
    if sys.platform != "darwin" or not os.access(SANDBOX_EXEC, os.X_OK):
        raise Refuse("no process jail here (not macOS, not inside msp-sandbox): run the render "
                     "inside msp-sandbox (sandbox-run --offline)")

    def q(path: str) -> str:
        return '"' + path.replace("\\", "\\\\").replace('"', '\\"') + '"'

    w = os.path.realpath(work)
    b = os.path.realpath(binary)
    subpaths = [w, *JAIL_READ_SUBPATHS, *(os.path.realpath(x) for x in _JAIL_EXTRA_READ)]
    profile = "".join([
        "(version 1)(allow default)(deny network*)",
        "(deny file-write*)",
        f"(allow file-write* (subpath {q(w)}) (literal \"/dev/null\") (literal \"/dev/tty\"))",
        "(deny file-read-data)",
        "(allow file-read-data " + " ".join(f"(subpath {q(s)})" for s in subpaths)
        + " " + " ".join(f"(literal {q(x)})" for x in (b, *JAIL_READ_LITERALS)) + ")",
    ])
    return [SANDBOX_EXEC, "-p", profile, *argv]


def press_version_of(binary: Path, tmp: Path) -> str | None:
    try:
        p = subprocess.run(jailed([str(binary), "--version"], binary, tmp), capture_output=True,
                           text=True, timeout=30, env=_scrubbed_env(tmp), cwd=str(tmp))
    except (OSError, subprocess.SubprocessError):
        return None
    except Refuse:
        raise
    if p.returncode != 0:
        return None
    for line in ((p.stdout or "") + "\n" + (p.stderr or "")).splitlines():
        m = VERSION_OUT_RE.fullmatch(line.strip())
        if m:
            return m.group(1)
    return None


def press_candidates(version: str, explicit: str | None) -> list[Path]:
    if explicit:
        return [Path(explicit).expanduser()]
    out: list[Path] = []
    if os.environ.get("PRINTING_PRESS_BIN"):
        out.append(Path(os.environ["PRINTING_PRESS_BIN"]).expanduser())
    names = ("cli-printing-press", "printing-press")
    for d in os.environ.get("PATH", "").split(os.pathsep):
        out += [Path(d) / n for n in names if d]
    home = Path.home()
    for d in (home / "go" / "bin", home / "printing-press" / "bin",
              home / ".cache" / "fresh-render" / "press" / version):
        out += [d / n for n in names]
        out += [d / f"{n}-{version}" for n in names]
    seen, uniq = set(), []
    for p in out:
        key = str(p)
        if key not in seen:
            seen.add(key)
            uniq.append(p)
    return uniq


def find_press(version: str, explicit: str | None, tmp: Path) -> Path:
    tried = []
    for cand in press_candidates(version, explicit):
        try:
            real = cand.resolve()
            ok = real.is_file() and os.access(real, os.X_OK)
        except OSError:
            ok = False
        if not ok:
            continue
        got = press_version_of(real, tmp)
        tried.append(f"{cand} ({got or 'no version'})")
        if got == version:
            return real
    install = (f"GOBIN=\"$HOME/.cache/fresh-render/press/{version}\" GOTOOLCHAIN=auto "
               f"go install github.com/mvanhorn/cli-printing-press/v{version.split('.')[0]}"
               f"/cmd/cli-printing-press@v{version}")
    raise Refuse(f"no press binary reports version {version} (never rendered with another). "
                 f"Tried: {', '.join(tried) or 'none found'}. Install the exact version with: "
                 f"{install}  (or pass --press-bin)")


# --------------------------------------------------------------------------
# spec

def spec_digests(data: bytes) -> set[str]:
    return {"sha256:" + hashlib.sha256(data).hexdigest(),
            "sha256:" + hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()}


def _spec_search_roots() -> list[Path]:
    base = Path(os.environ.get("PRESS_HOME", str(Path.home() / "printing-press"))).expanduser()
    return [base / "library", base / "manuscripts"]


def _spec_like(name: str) -> bool:
    low = name.lower()
    return low.endswith((".json", ".yaml", ".yml", ".graphql")) and (
        "spec" in low or "openapi" in low or "swagger" in low)


def find_spec(prov: Provenance, skill_dir: Path, explicit: str | None) -> tuple[Path, str]:
    if not prov.checksum:
        raise Refuse(f"skills/{prov.slug}: no spec_checksum is recorded, so no spec can be "
                     "proven to be the one it was generated from")
    cands: list[tuple[Path, str]] = []
    if explicit:
        cands.append((Path(explicit).expanduser(), "--spec"))
    for n in SPEC_NAMES:
        p = skill_dir / "press" / n
        if p.is_file() and not p.is_symlink():
            cands.append((p, f"skills/{prov.slug}/press/{n}"))
    for p, origin in cands:
        try:
            data = p.read_bytes()
        except OSError:
            continue
        if prov.checksum in spec_digests(data):
            return p, origin
    if explicit:
        raise Refuse(f"--spec {explicit} does not match the recorded {prov.checksum}")
    for root in _spec_search_roots():
        if not root.is_dir():
            continue
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "internal",
                                                             "cmd", "vendor", ".runstate")]
            for fn in filenames:
                if not _spec_like(fn):
                    continue
                p = Path(dirpath) / fn
                try:
                    if p.is_symlink() or p.stat().st_size > 80_000_000:
                        continue
                    data = p.read_bytes()
                except OSError:
                    continue
                if prov.checksum in spec_digests(data):
                    return p, "local press folder"
    raise Refuse(f"skills/{prov.slug}: no spec with {prov.checksum} was found (looked at "
                 f"--spec, skills/{prov.slug}/press/ and {', '.join(map(str, _spec_search_roots()))}). "
                 "Pass --spec FILE with the exact spec it was generated from, or "
                 "--spec-invariant for the weaker fallback")


def guard_untrusted_spec(source_kind: str, origin: str) -> None:
    """In --pr mode the contributor's manifest names the spec checksum, so any
    spec that matches it is bytes the contributor chose, wherever the file came
    from (the PR's press/ folder or an operator's --spec). The press parses it,
    so it renders only inside msp-sandbox (SANDBOX_MODE set: no network, no
    credentials, throwaway HOME). The one exception is a spec found in our own
    press folders: those bytes are our prints, not the PR's."""
    if source_kind == "pr" and origin != "local press folder" and not os.environ.get("SANDBOX_MODE"):
        raise Refuse("the spec matching this PR's checksum is contributor-chosen input, and the "
                     "press would parse it on this machine. Copy the PR tree into msp-sandbox and "
                     "run --tree there (sandbox-run --offline; see the contribution lane)")


YAML_DOLLAR_ESCAPE_RE = re.compile(r"\\(?:x24|u0024|U00000024)", re.IGNORECASE)


def refuse_external_refs(spec: Path) -> None:
    """An early, readable refusal for a spec whose $ref points outside it. A
    lint, not the security boundary: YAML can spell a key many ways (aliases,
    explicit keys, escapes), so the boundary is jailed() and the PR-spec
    guard. JSON is decoded and every key checked, duplicates included."""
    text = spec.read_bytes().decode("utf-8", errors="replace")
    bad: list[str] = []

    def pairs(items):
        for k, v in items:
            if k == "$ref" and not (isinstance(v, str) and v.startswith("#")):
                bad.append(str(v)[:80])
        return dict(items)

    try:
        json.loads(text, object_pairs_hook=pairs)
        is_json = True
    except ValueError:
        is_json = False
    if bad:
        raise Refuse(f"{spec.name}: an external $ref ({bad[0]!r}); the press would read the "
                     "file or URL it names, so it is not rendered")
    if is_json:
        return
    for rx, what in ((EXTERNAL_REF_RE, "an external $ref (a value not starting with '#')"),
                     (YAML_DOLLAR_ESCAPE_RE, "an escaped '$' (it can spell a $ref key)")):
        m = rx.search(text)
        if m:
            line = text.count("\n", 0, m.start()) + 1
            raise Refuse(f"{spec.name}:{line}: {what}; the press would read the file or URL "
                         "it names, so it is not rendered")


# --------------------------------------------------------------------------
# render + normalize

def render(press: Path, spec: Path, flags: list[str], tmp: Path, label: str) -> Path:
    work = tmp / f"render-{label}"
    work.mkdir(parents=True)
    spec_copy = work / ("spec" + "".join(spec.suffixes[-1:]) if spec.suffix else "spec.yaml")
    shutil.copyfile(spec, spec_copy)
    out = work / "out"
    cmd = [str(press), "generate", "--spec", str(spec_copy), "--output", str(out),
           "--validate=false", *flags]
    try:
        p = subprocess.run(jailed(cmd, press, work), capture_output=True, text=True,
                           timeout=RENDER_TIMEOUT, env=_scrubbed_env(work), cwd=str(work))
    except subprocess.TimeoutExpired:
        raise Refuse(f"render {label}: timed out after {RENDER_TIMEOUT}s")
    if p.returncode != 0 or not (out / "go.mod").is_file():
        tail = " | ".join(((p.stdout or "") + (p.stderr or "")).strip().splitlines()[-3:])
        raise Refuse(f"render {label}: press exited {p.returncode}: {tail[:400]}")
    return out


def strip_pp(text: str, api: str) -> str:
    """post-regen-rename.sh's Go-source rules, in its order."""
    t = text.replace(f"{api}-pp-cli/internal", "\0PPMOD\0/internal")
    t = t.replace(f"{api}-pp-cli/cmd", "\0PPMOD\0/cmd")
    t = t.replace(f"{api}-pp-mcp", f"{api}-mcp").replace(f"{api}-pp-cli", f"{api}-cli")
    t = t.replace("%s-pp-mcp", "%s-mcp").replace("%s-pp-cli", "%s-cli")
    return t.replace("\0PPMOD\0", f"{api}-pp-cli")


def _vendor_rel(rel: str) -> bool:
    parts = rel.split("/")
    if len(parts) == 1:
        return rel in VENDOR_FILES
    if any(p in EXCLUDE_DIRS for p in parts[:-1]):
        return False
    return True


def normalize_render(out: Path, api: str) -> dict[str, bytes]:
    """The render in the form a vendored connector takes (see module doc)."""
    top_dirs = set(VENDOR_DIRS)
    for d in out.iterdir():
        if d.is_dir() and not d.is_symlink() and d.name not in EXCLUDE_DIRS and any(d.rglob("*.go")):
            top_dirs.add(d.name)
    res: dict[str, bytes] = {}
    for dirpath, dirnames, filenames in os.walk(out):
        dirnames[:] = [d for d in dirnames if d not in EXCLUDE_DIRS]
        for fn in filenames:
            p = Path(dirpath) / fn
            rel = p.relative_to(out).as_posix()
            if "/" in rel and rel.split("/")[0] not in top_dirs:
                continue
            if not _vendor_rel(rel) or p.is_symlink():
                continue
            data = p.read_bytes()
            if rel.endswith(".go") or rel == "Makefile" or rel in ("NOTICE", "LICENSE"):
                text = data.decode("utf-8")
                if rel.endswith(".go"):
                    text = GO_COPYRIGHT_RE.sub(CANON_GO, strip_pp(text, api))
                elif rel == "Makefile":
                    text = text.replace(f"{api}-pp-mcp", f"{api}-mcp").replace(f"{api}-pp-cli", f"{api}-cli")
                else:
                    text = TXT_COPYRIGHT_RE.sub(lambda m: m.group(1) + CANON_TXT, text)
                data = text.encode("utf-8")
            for variant in ("cli", "mcp"):
                pre = f"cmd/{api}-pp-{variant}/"
                if rel.startswith(pre):
                    rel = f"cmd/{api}-{variant}/" + rel[len(pre):]
            res[rel] = data
    return res


def read_vendored(cli: Path) -> tuple[dict[str, bytes], list[str]]:
    files: dict[str, bytes] = {}
    links: list[str] = []
    for dirpath, dirnames, filenames in os.walk(cli, followlinks=False):
        for d in list(dirnames):
            if (Path(dirpath) / d).is_symlink():
                links.append((Path(dirpath) / d).relative_to(cli).as_posix())
                dirnames.remove(d)
        for fn in filenames:
            p = Path(dirpath) / fn
            rel = p.relative_to(cli).as_posix()
            if p.is_symlink():
                links.append(rel)
            elif p.is_file():
                files[rel] = p.read_bytes()
    return files, sorted(links)


# --------------------------------------------------------------------------
# compare + report

def variants_of(renders: list[dict[str, bytes]]) -> dict[str, list[bytes]]:
    """Every distinct content each path had across renders of the same spec."""
    out: dict[str, list[bytes]] = {}
    for tree in renders:
        for rel, data in tree.items():
            vs = out.setdefault(rel, [])
            if data not in vs:
                vs.append(data)
    return out


def classify(expected: dict[str, bytes] | None, vendored: dict[str, bytes], links: list[str],
             invariant: dict[str, bytes] | None = None,
             variants: dict[str, list[bytes]] | None = None) -> list[dict]:
    """One row per path. expected=None means spec-invariant mode (use invariant).
    In exact mode `expected` holds the files every render emitted (only those
    can be reported deleted) and `variants` every content any render emitted:
    a file equal to any of them is press output for this spec and version."""
    rows = []
    ref = expected if expected is not None else invariant or {}
    if variants is None:
        variants = {k: [v] for k, v in ref.items()}
    for rel in sorted(vendored):
        data = vendored[rel]
        if data in variants.get(rel, ()):
            rows.append({"path": rel, "class": "template"})
            continue
        ref_has = rel in variants
        if rel in ("go.mod", "go.sum"):
            rows.append({"path": rel, "class": "dependency",
                         "note": "no go.sum in a render" if rel == "go.sum" and not ref_has
                         else "differs from the render"})
        elif ref_has:
            rows.append({"path": rel, "class": "generated-edited"})
        elif expected is None and DO_NOT_EDIT in data[:600]:
            rows.append({"path": rel, "class": "generated-spec-dependent",
                         "note": "carries DO NOT EDIT; its render needs the real spec"})
        else:
            row = {"path": rel, "class": "hand-written"}
            if DO_NOT_EDIT in data[:600]:
                row["note"] = "carries DO NOT EDIT but the render does not produce it"
            rows.append(row)
    if expected is not None:
        for rel in sorted(set(expected) - set(vendored) - set(links)):
            rows.append({"path": rel, "class": "template-deleted"})
        # (a file only some renders emit is not in `expected`, so its absence
        # from the connector is not reported as a deletion)
    for rel in links:
        rows.append({"path": rel, "class": "symlink", "note": "not followed"})
    return rows


def edit_diffs(variants: dict[str, list[bytes]], vendored: dict[str, bytes],
               rows: list[dict]) -> dict[str, str]:
    """Unified diffs, render -> connector, for every file the render also
    produces: the hand edits themselves, which is what a reviewer reads.
    Against the closest render when renders differ."""
    out = {}
    for r in rows:
        p = r["path"]
        if r["class"] in ("generated-edited", "dependency") and p in variants and p in vendored:
            b = vendored[p].decode("utf-8", errors="replace").splitlines(keepends=True)
            best = None
            for v in variants[p]:
                a = v.decode("utf-8", errors="replace").splitlines(keepends=True)
                d = "".join(difflib.unified_diff(a, b, f"render/{p}", f"connector/{p}", n=3))
                if best is None or len(d) < len(best):
                    best = d
            out[p] = best or ""
    return out


def build_report(slug: str, source: dict, prov: Provenance, press: Path, mode: str,
                 spec_info: dict | None, rows: list[dict], cli_count: int,
                 outside: list[str], extra: dict | None = None) -> dict:
    counts = {c: 0 for c in ("template", *REVIEW_CLASSES)}
    for r in rows:
        counts[r["class"]] += 1
    review = [r for r in rows if r["class"] != "template"]
    review_files = [r for r in review if r["class"] != "template-deleted"]
    rep = {
        "slug": slug, "source": source, "mode": mode,
        "press": {"version": prov.version, "binary": str(press)},
        "spec": spec_info, "render_flags": prov.flags,
        "provenance": prov.sources,
        "cli_files": cli_count,
        "review_files": len(review_files),
        "review_entries": len(review),
        "counts": counts,
        "review": review,
        "outside_cli": sorted(outside),
    }
    if extra:
        rep.update(extra)
    return rep


def human(rep: dict) -> str:
    c = rep["counts"]
    src = rep["source"]
    where = (f"PR #{src['pr']} @ {src['head'][:12]}" if src.get("kind") == "pr"
             else src.get("path", "?"))
    spec = rep.get("spec") or {}
    lines = [
        f"fresh-render-diff {rep['slug']} ({where})",
        f"  press {rep['press']['version']} ({rep['press']['binary']}), mode {rep['mode']}",
        (f"  spec {spec.get('checksum')} from {spec.get('origin')}" if spec.get("origin")
         else f"  spec {spec.get('checksum')}: {spec.get('note', 'not used')}"),
        f"  cli/ files: {rep['cli_files']} -> review set {rep['review_files']} files"
        + (f" (+{c['template-deleted']} deleted template files)" if c["template-deleted"] else ""),
        f"    identical to the render (template, not reviewed again): {c['template']}",
    ]
    if rep.get("unstable_render_files"):
        lines.append(f"  the press emitted {len(rep['unstable_render_files'])} files differently across "
                     f"{RENDERS} renders (not reproducible upstream; any emitted bytes count as template)")
    for cls in REVIEW_CLASSES:
        if c.get(cls):
            lines.append(f"    {cls}: {c[cls]}")
    if rep.get("invariant"):
        inv = rep["invariant"]
        lines.append(f"  spec-invariant evidence: {inv['references']} reference renders "
                     f"({inv['local_real_specs']} local real specs), "
                     f"{inv['invariant_files']} files identical across all of them")
        for n in inv.get("notes", []):
            lines.append(f"    {n}")
    lines.append(f"  outside cli/ (docs, registry; always reviewed): {len(rep['outside_cli'])}")
    lines.append("  review set:")
    for r in rep["review"]:
        note = f"  ({r['note']})" if r.get("note") else ""
        lines.append(f"    {r['class']:<24} {r['path']}{note}")
    for p, d in sorted((rep.get("diffs") or {}).items()):
        lines.append("")
        lines.append(d.rstrip("\n"))
    return "\n".join(lines)


# --------------------------------------------------------------------------
# sources: --tree and --pr

# macOS's own top-level aliases (/var -> /private/var and so on): the only
# symlinks a tree path may pass through.
SYSTEM_ALIASES = {"/var", "/tmp", "/etc"}


def refuse_symlinked_path(p: Path, what: str) -> None:
    """No component of `p`, from the filesystem root down, is a symlink other
    than the OS's own aliases. A tree checked out from a contributor can carry
    a link at any level (skills/, skills/<slug>/, press/), so each is checked,
    not only the last."""
    p = Path(os.path.abspath(p))
    cur = Path(p.anchor)
    for part in p.parts[1:]:
        cur = cur / part
        if cur.is_symlink() and str(cur) not in SYSTEM_ALIASES:
            raise Refuse(f"{what}: {cur} is a symlink; not read through")


def resolve_tree(path: str, slug: str | None) -> tuple[Path, str]:
    """The skill directory, never reached through a symlink: the given path
    (as typed, not resolved), skills/ and skills/<slug> must all be real
    directories."""
    root = Path(os.path.abspath(Path(path).expanduser()))
    if root.is_symlink():
        raise Refuse(f"--tree {path} is a symlink; pass the real directory")
    if (root / "manifest.json").is_file() and (root / "cli").is_dir():
        return root, slug or root.name
    skills = root / "skills"
    if skills.is_dir():
        if not slug:
            raise Refuse("--tree points at a repository root: pass --slug")
        if skills.is_symlink() or (skills / slug).is_symlink():
            raise Refuse(f"--tree {path}: skills/ or skills/{slug} is a symlink; not read through")
        return skills / slug, slug
    raise Refuse(f"--tree {path}: not a skills/<slug> directory or a repository root")


def _run(cmd: list[str], cwd: Path | None = None, timeout: int = 300) -> str:
    p = subprocess.run(cmd, cwd=str(cwd) if cwd else None, capture_output=True,
                       text=True, timeout=timeout)
    if p.returncode != 0:
        raise Refuse(f"{' '.join(cmd[:4])}...: exited {p.returncode}: "
                     f"{(p.stderr or p.stdout).strip()[:300]}")
    return p.stdout


def pr_tree(pr: int, slug: str | None, repo_dir: Path, tmp: Path) -> tuple[Path, str, dict]:
    head = _run(["gh", "api", f"repos/{GH_REPO}/pulls/{pr}", "-q", ".head.sha"]).strip()
    if not re.fullmatch(r"[0-9a-f]{40}", head):
        raise Refuse(f"PR #{pr}: no head SHA")
    names = _run(["gh", "api", "--paginate", f"repos/{GH_REPO}/pulls/{pr}/files",
                  "-q", ".[].filename"]).splitlines()
    slugs = sorted({n.split("/")[1] for n in names if n.startswith("skills/") and n.count("/") >= 2})
    if slug is None:
        if len(slugs) != 1:
            raise Refuse(f"PR #{pr} touches {len(slugs)} skills ({', '.join(slugs[:8])}): pass --slug")
        slug = slugs[0]
    if not SLUG_RE.match(slug):
        raise Refuse(f"slug {slug!r} is not a slug")
    gitdir = fetch_commit(repo_dir, head, tmp)
    dest = tmp / "pr"
    dest.mkdir()
    links = materialize(gitdir, head, slug, dest)
    skill_dir = dest / "skills" / slug
    if not skill_dir.is_dir():
        raise Refuse(f"PR #{pr} head has no skills/{slug}/")
    return skill_dir, slug, {"kind": "pr", "pr": pr, "head": head, "files_in_pr": len(names)}


def fetch_commit(repo_dir: Path, sha: str, tmp: Path, remote: str = "origin") -> Path:
    """Fetch `sha` into a private bare repository under tmp and return it. Its
    alternates file borrows the existing objects of `repo_dir` read-only, so
    only missing objects download, and nothing (no object, ref or FETCH_HEAD)
    is written into `repo_dir`."""
    common = Path(_run(["git", "-C", str(repo_dir), "rev-parse", "--path-format=absolute",
                        "--git-common-dir"]).strip())
    url = _run(["git", "-C", str(repo_dir), "remote", "get-url", remote]).strip()
    gd = tmp / "git"
    _run(["git", "init", "-q", "--bare", str(gd)])
    (gd / "objects" / "info").mkdir(parents=True, exist_ok=True)
    (gd / "objects" / "info" / "alternates").write_text(str(common / "objects") + "\n")
    _run(["git", "-C", str(gd), "fetch", "--no-tags", "--quiet", url, sha], timeout=900)
    _run(["git", "-C", str(gd), "cat-file", "-e", f"{sha}^{{commit}}"])
    return gd


def materialize(gitdir: Path, sha: str, slug: str, dest: Path) -> list[str]:
    """Write every blob under skills/<slug> at `sha` into dest, byte for byte as
    committed. Read with ls-tree and cat-file, not `git archive`, so no
    .gitattributes in the tree can drop a file (export-ignore) or rewrite one
    (export-subst, eol, filters). Symlinks and submodules are recorded as inert
    links to a path that does not exist (listed, never followed)."""
    env = {**os.environ, "GIT_LITERAL_PATHSPECS": "1"}
    ls = subprocess.run(["git", "-C", str(gitdir), "ls-tree", "-r", "-z", "--full-tree", sha,
                         "--", f"skills/{slug}/"], capture_output=True, timeout=300, env=env)
    if ls.returncode != 0:
        raise Refuse(f"git ls-tree {sha[:12]} skills/{slug}: {ls.stderr.decode(errors='replace')[:200]}")
    blobs: list[tuple[str, str]] = []
    links: list[str] = []
    for rec in ls.stdout.split(b"\0"):
        if not rec:
            continue
        meta, _, raw = rec.partition(b"\t")
        mode, kind, oid = meta.decode().split(" ")
        rel = raw.decode("utf-8", errors="surrogateescape")
        parts = rel.split("/")
        if rel.startswith("/") or ".." in parts or "" in parts or parts[:2] != ["skills", slug]:
            raise Refuse(f"tree entry {rel!r} escapes skills/{slug}/")
        if mode in ("100644", "100755") and kind == "blob":
            blobs.append((oid, rel))
        else:
            links.append(rel)
    cat = subprocess.run(["git", "-C", str(gitdir), "cat-file", "--batch"],
                         input=b"".join(o.encode() + b"\n" for o, _ in blobs),
                         capture_output=True, timeout=600)
    if cat.returncode != 0:
        raise Refuse(f"git cat-file: {cat.stderr.decode(errors='replace')[:200]}")
    buf, pos = cat.stdout, 0
    for oid, rel in blobs:
        nl = buf.index(b"\n", pos)
        head = buf[pos:nl].decode().split(" ")
        if head[0] != oid or head[1] != "blob":
            raise Refuse(f"git cat-file returned {head!r} for {rel}")
        size = int(head[2])
        data = buf[nl + 1:nl + 1 + size]
        pos = nl + 1 + size + 1
        target = dest.joinpath(*rel.split("/"))
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
    for rel in links:
        target = dest.joinpath(*rel.split("/"))
        target.parent.mkdir(parents=True, exist_ok=True)
        os.symlink("/nonexistent-link-target", target)
    return links


def outside_cli(skill_dir: Path) -> list[str]:
    out = []
    for dirpath, dirnames, filenames in os.walk(skill_dir, followlinks=False):
        rel_dir = Path(dirpath).relative_to(skill_dir).as_posix()
        if rel_dir == "cli" or rel_dir.startswith("cli/"):
            dirnames[:] = []
            continue
        for fn in filenames:
            out.append((Path(dirpath) / fn).relative_to(skill_dir).as_posix())
    return out


# --------------------------------------------------------------------------
# reference specs for --spec-invariant and --calibrate

def _ref_spec(title: str, server: str, scheme: dict, resources: list[dict]) -> dict:
    paths: dict = {}
    schemas: dict = {}
    for r in resources:
        name, idp, base = r["name"], r["id"], r["path"]
        props = {"id": {"type": "string"}, **{f: {"type": t} for f, t in r["fields"].items()}}
        schemas[name] = {"type": "object", "properties": props}
        item = {"$ref": f"#/components/schemas/{name}"}
        listing = ({"type": "array", "items": item} if r.get("bare")
                   else {"type": "object", "properties": {r.get("wrap", "data"): {"type": "array", "items": item},
                                                          "next": {"type": "string"}}})
        ops: dict = {"get": {"operationId": f"list{name}", "summary": f"List {name.lower()}",
                             "parameters": [{"name": q, "in": "query", "schema": {"type": "string"}}
                                            for q in r.get("query", [])],
                             "responses": {"200": {"description": "ok", "content": {
                                 "application/json": {"schema": listing}}}}}}
        if "post" in r.get("verbs", ()):
            ops["post"] = {"operationId": f"create{name}", "summary": f"Create a {name.lower()}",
                           "requestBody": {"content": {"application/json": {"schema": item}}},
                           "responses": {"201": {"description": "created"}}}
        paths[base] = ops
        one: dict = {"get": {"operationId": f"get{name}", "summary": f"Get a {name.lower()}",
                             "parameters": [{"name": idp, "in": "path", "required": True,
                                             "schema": {"type": "string"}}],
                             "responses": {"200": {"description": "ok", "content": {
                                 "application/json": {"schema": item}}}}}}
        for verb in ("put", "patch", "delete"):
            if verb in r.get("verbs", ()):
                one[verb] = {"operationId": f"{verb}{name}", "summary": f"{verb.title()} a {name.lower()}",
                             "parameters": [{"name": idp, "in": "path", "required": True,
                                             "schema": {"type": "string"}}],
                             "responses": {"200": {"description": "ok"}}}
        paths[f"{base}/{{{idp}}}"] = one
    spec = {"openapi": "3.0.3", "info": {"title": title, "version": "1.0"},
            "servers": [{"url": server}], "paths": paths,
            "components": {"schemas": schemas}}
    if scheme:
        spec["components"]["securitySchemes"] = {"main": scheme}
        spec["security"] = [{"main": []}]
    return spec


REFERENCE_SPECS = {
    "ref-bearer": lambda: _ref_spec(
        "Reference Bearer API", "https://api.ref-bearer.invalid/v1",
        {"type": "http", "scheme": "bearer"},
        [{"name": "Widget", "id": "widgetId", "path": "/widgets", "bare": True,
          "fields": {"label": "string"}}]),
    "ref-header": lambda: _ref_spec(
        "Reference Header API", "https://ref-header.invalid/api/v3",
        {"type": "apiKey", "in": "header", "name": "X-Api-Key"},
        [{"name": "Account", "id": "accountId", "path": "/accounts", "verbs": ("post", "put", "delete"),
          "fields": {"displayName": "string", "seats": "integer"}, "query": ["status"]},
         {"name": "User", "id": "userId", "path": "/accounts/{accountId}/users", "wrap": "items",
          "fields": {"email": "string", "active": "boolean"}},
         {"name": "Invoice", "id": "invoiceId", "path": "/invoices", "fields": {"total": "number"},
          "query": ["from", "to"]}]),
    "ref-basic": lambda: _ref_spec(
        "Reference Basic API", "https://ref-basic.invalid",
        {"type": "http", "scheme": "basic"},
        [{"name": "Device", "id": "deviceId", "path": "/v2/devices", "verbs": ("patch",),
          "fields": {"hostname": "string", "lastSeen": "string"}, "query": ["site"]},
         {"name": "Alert", "id": "alertId", "path": "/v2/alerts", "verbs": ("delete",),
          "fields": {"severity": "integer", "message": "string"}}]),
    "ref-oauth": lambda: _ref_spec(
        "Reference OAuth API", "https://api.ref-oauth.invalid/rest",
        {"type": "oauth2", "flows": {"clientCredentials": {
            "tokenUrl": "https://api.ref-oauth.invalid/oauth/token", "scopes": {}}}},
        [{"name": "Ticket", "id": "ticketId", "path": "/tickets", "verbs": ("post", "patch"),
          "fields": {"subject": "string", "priority": "integer"}, "wrap": "results"},
         {"name": "Note", "id": "noteId", "path": "/tickets/{ticketId}/notes",
          "fields": {"body": "string"}},
         {"name": "Agent", "id": "agentId", "path": "/agents", "bare": True,
          "fields": {"name": "string"}},
         {"name": "Queue", "id": "queueId", "path": "/queues", "fields": {"depth": "integer"}}]),
    "ref-query": lambda: _ref_spec(
        "Reference Query API", "https://ref-query.invalid/v1",
        {"type": "apiKey", "in": "query", "name": "api_key"},
        [{"name": "Report", "id": "reportId", "path": "/reports", "fields": {"title": "string"},
          "query": ["since"]},
         {"name": "Backup", "id": "backupId", "path": "/backups", "verbs": ("post",),
          "fields": {"sizeBytes": "integer", "state": "string"}}]),
    "ref-open": lambda: _ref_spec(
        "Reference Open API", "https://ref-open.invalid/api",
        {},
        [{"name": "Site", "id": "siteId", "path": "/sites", "bare": True, "fields": {"city": "string"}},
         {"name": "Contact", "id": "contactId", "path": "/sites/{siteId}/contacts",
          "fields": {"phone": "string"}},
         {"name": "Asset", "id": "assetId", "path": "/assets", "verbs": ("put",),
          "fields": {"serial": "string", "warranty": "string"}},
         {"name": "License", "id": "licenseId", "path": "/licenses", "fields": {"seats": "integer"}},
         {"name": "Policy", "id": "policyId", "path": "/policies", "verbs": ("delete",),
          "fields": {"enabled": "boolean"}}]),
}


# Generate flags that change template output without a spec change; one
# reference spec is rendered under each, so a file that varies with them is
# never called invariant.
FLAG_VARIANTS = (["--spec-source", "official"], ["--spec-source", "community"])


def local_reference_specs(version: str, exclude: str | None = None) -> list[tuple[str, Path]]:
    """(slug, spec) for every connector at `version` whose recorded spec_checksum
    matches a spec in the local press folders: our own prints, so trusted
    inputs. Specs are indexed by digest once."""
    index: dict[str, Path] = {}
    for root in _spec_search_roots():
        if not root.is_dir():
            continue
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "internal",
                                                             "cmd", "vendor", ".runstate")]
            for fn in filenames:
                if not _spec_like(fn):
                    continue
                p = Path(dirpath) / fn
                try:
                    if not p.is_symlink() and p.stat().st_size < 80_000_000:
                        for d in spec_digests(p.read_bytes()):
                            index.setdefault(d, p)
                except OSError:
                    pass
    out = []
    for mf in sorted((REPO / "skills").glob("*/manifest.json")):
        slug = mf.parent.name
        if slug == exclude:
            continue
        try:
            prov = read_provenance(mf.parent, slug)
        except Refuse:
            continue
        if prov.version == version and prov.checksum in index:
            out.append((slug, index[prov.checksum]))
    return out


def reference_trees(press: Path, api: str, tmp: Path, extra_specs: list[str],
                    local_refs: list[tuple[str, Path]], label: str = "ref",
                    notes: list[str] | None = None) -> list[tuple[str, dict[str, bytes]]]:
    """Normalized renders of every reference: the built-in specs, one built-in
    spec under each FLAG_VARIANTS entry, --reference-spec files, and local real
    specs at the same version. A reference that will not render is skipped and
    noted (fewer references can only shrink the invariant set)."""
    jobs: list[tuple[str, Path, list[str]]] = []
    for name, build in REFERENCE_SPECS.items():
        p = tmp / f"{label}-{name}.json"
        p.write_text(json.dumps(build(), indent=1), encoding="utf-8")
        jobs.append((f"builtin:{name}", p, []))
    first = tmp / f"{label}-{next(iter(REFERENCE_SPECS))}.json"
    for i, flags in enumerate(FLAG_VARIANTS):
        jobs.append((f"builtin:flags{i}", first, list(flags)))
    for i, s in enumerate(extra_specs):
        jobs.append((f"extra:{i}", Path(s).expanduser(), []))
    for slug, sp in local_refs:
        jobs.append((f"local:{slug}", sp, []))
    trees = []
    for i, (name, spec, flags) in enumerate(jobs):
        try:
            refuse_external_refs(spec)
            out = render(press, spec, ["--name", api, *flags], tmp, f"{label}{i}")
        except Refuse as e:
            if name.startswith("builtin:"):
                raise
            if notes is not None:
                notes.append(f"reference {name} skipped: {str(e)[:160]}")
            continue
        trees.append((name, normalize_render(out, api)))
    return trees


def invariant_of(trees: list[dict[str, bytes]]) -> dict[str, bytes]:
    if not trees:
        return {}
    common = set.intersection(*(set(t) for t in trees))
    return {p: trees[0][p] for p in common if all(t[p] == trees[0][p] for t in trees)}


def calibrate(version: str, press_bin: str | None, keep: bool) -> int:
    """Leave-one-out over the local real specs at `version`: for each real spec,
    compute the invariant set from every OTHER reference (built-in and real)
    and count the files that set calls invariant but this spec renders
    differently. Each such file is a template variant the fallback could not
    have seen. PASS = zero. Also reports the built-in specs alone."""
    tmp = Path(tempfile.mkdtemp(prefix="fresh-render-cal-"))
    try:
        press = find_press(version, press_bin, tmp)
        reals = local_reference_specs(version)
        notes: list[str] = []
        trees = reference_trees(press, "calibrate", tmp, [], reals, notes=notes)
        for n in notes:
            print("  " + n)
        real = [(n, t) for n, t in trees if n.startswith("local:")]
        builtin = [t for n, t in trees if not n.startswith("local:")]
        if len(real) < 2:
            print(f"calibrate {version}: only {len(real)} local real specs render at this version; need 2+")
            return 1
        b_inv = invariant_of(builtin)
        b_over = sorted({p for _, t in real for p, d in b_inv.items() if p in t and t[p] != d})
        print(f"calibrate {version}: {len(builtin)} built-in renders, {len(real)} real specs")
        print(f"  built-in specs alone: {len(b_inv)} invariant files, {len(b_over)} vary across "
              f"real specs: {', '.join(b_over) or 'none'}")
        over: dict[str, list[str]] = {}
        for i, (name, tree) in enumerate(real):
            others = builtin + [t for j, (_, t) in enumerate(real) if j != i]
            for p, d in invariant_of(others).items():
                if p in tree and tree[p] != d:
                    over.setdefault(p, []).append(name.split(":", 1)[1])
        full = invariant_of([t for _, t in trees])
        print(f"  all references: {len(full)} invariant files")
        print(f"  leave-one-out: {sum(len(v) for v in over.values())} unseen variants in "
              f"{len({s for v in over.values() for s in v})} of {len(real)} real specs")
        for p, slugs in sorted(over.items()):
            print(f"    UNSEEN {p}: only {', '.join(slugs)} renders it differently")
        print("calibrate:", "FAIL" if over else "PASS")
        return 1 if over else 0
    except (Refuse, OSError, subprocess.SubprocessError, UnicodeDecodeError, ValueError) as e:
        print(f"calibrate: cannot run: {e}", file=sys.stderr)
        return 2
    finally:
        if keep:
            print(f"kept: {tmp}", file=sys.stderr)
        else:
            shutil.rmtree(tmp, ignore_errors=True)


# --------------------------------------------------------------------------
# main flow

def run(args) -> dict:
    tmp = Path(tempfile.mkdtemp(prefix="fresh-render-"))
    try:
        if args.pr is not None:
            skill_dir, slug, source = pr_tree(args.pr, args.slug, REPO, tmp)
        else:
            skill_dir, slug = resolve_tree(args.tree, args.slug)
            source = {"kind": "tree", "path": str(skill_dir)}
        if not SLUG_RE.match(slug):
            raise Refuse(f"slug {slug!r} is not a slug")
        refuse_symlinked_path(skill_dir, "--tree")
        for sub in ("cli", "press", "manifest.json"):
            if (skill_dir / sub).is_symlink():
                raise Refuse(f"skills/{slug}/{sub} is a symlink; not read through")
        cli = skill_dir / "cli"
        if not cli.is_dir() or cli.is_symlink():
            raise Refuse(f"skills/{slug}/cli is missing (a markdown-only skill has nothing to render)")
        prov = read_provenance(skill_dir, slug)
        press = find_press(prov.version, args.press_bin, tmp)
        vendored, links = read_vendored(cli)
        outside = outside_cli(skill_dir)
        if args.spec_invariant:
            local = [] if args.no_local_references else local_reference_specs(prov.version, exclude=slug)
            notes: list[str] = []
            trees = reference_trees(press, prov.api, tmp, args.reference_spec or [], local, notes=notes)
            inv = invariant_of([t for _, t in trees])
            rows = classify(None, vendored, links, invariant=inv)
            for n in SPEC_NAMES:
                if (skill_dir / "press" / n).is_file():
                    rows.append({"path": f"../press/{n}", "class": "spec"})
            diffs = edit_diffs({k: [v] for k, v in inv.items()}, vendored, rows) if getattr(args, "diff", False) else None
            return build_report(slug, source, prov, press, "spec-invariant",
                                {"checksum": prov.checksum, "note": "the real spec was not used (--spec-invariant)"},
                                rows, len(vendored) + len(links), outside,
                                {"invariant": {"references": len(trees),
                                               "local_real_specs": sum(1 for n, _ in trees if n.startswith("local:")),
                                               "invariant_files": len(inv), "notes": notes},
                                 **({"diffs": diffs} if diffs is not None else {})})
        spec, origin = find_spec(prov, skill_dir, args.spec)
        guard_untrusted_spec(source["kind"], origin)
        refuse_external_refs(spec)
        # Some prints are not reproducible run to run (measured: n-central at
        # 4.24.0 drops three pm_*.go files, and their root.go and tools.go
        # registrations, in about 1 render of 12). Render RENDERS times. A file
        # equal to what ANY render emitted is press output for this spec and
        # version; only a file EVERY render emitted can be reported deleted.
        # That keeps the review set the same from one run to the next.
        renders = [normalize_render(render(press, spec, prov.flags, tmp, f"spec{i}"), prov.api)
                   for i in range(RENDERS)]
        variants = variants_of(renders)
        expected = {p: v[0] for p, v in variants.items() if all(p in r for r in renders)}
        unstable = sorted(p for p, v in variants.items() if len(v) > 1 or p not in expected)
        rows = classify(expected, vendored, links, variants=variants)
        for r in rows:
            if r["path"] in unstable and r["class"] != "template":
                r["note"] = "the press renders this file differently from run to run"
        for n in SPEC_NAMES:
            if (skill_dir / "press" / n).is_file():
                rows.append({"path": f"../press/{n}", "class": "spec"})
        diffs = edit_diffs(variants, vendored, rows) if getattr(args, "diff", False) else None
        return build_report(slug, source, prov, press, "exact",
                            {"checksum": prov.checksum, "origin": origin, "path": str(spec)},
                            rows, len(vendored) + len(links), outside,
                            {"render_files": len(expected), "unstable_render_files": unstable,
                             **({"diffs": diffs} if diffs is not None else {})})
    finally:
        if args.keep:
            print(f"kept: {tmp}", file=sys.stderr)
        else:
            shutil.rmtree(tmp, ignore_errors=True)


# --------------------------------------------------------------------------
# self-test: a fake press binary, so every branch runs anywhere

FAKE_PRESS = r'''#!/usr/bin/env python3
import os, sys, json
V = "__FAKE_VERSION__"
a = sys.argv[1:]
if a == ["--version"]:
    print("cli-printing-press " + V); sys.exit(0)
if a and a[0] == "generate":
    out = a[a.index("--output") + 1]; name = a[a.index("--name") + 1]
    spec = open(a[a.index("--spec") + 1]).read()
    files = {
        "go.mod": "module %s-pp-cli\n\ngo 1.26.9\n" % name,
        "NOTICE": "Copyright 2026 Someone Else\n",
        "Makefile": "build:\n\tgo build -o %s-pp-cli ./cmd/%s-pp-cli\n" % (name, name),
        "cmd/%s-pp-cli/main.go" % name: "// Copyright 2026 x and contributors.\n// Code generated by CLI Printing Press. DO NOT EDIT.\npackage main\n\nimport \"%s-pp-cli/internal/cli\"\n\nfunc main() { cli.Run(\"%s-pp-cli\") }\n" % (name, name),
        "internal/cli/root.go": "// Copyright 2026 x and contributors.\n// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n\nvar use = \"%s-pp-cli\"\n" % name,
        "internal/cli/helpers.go": "// Copyright 2026 x and contributors.\n// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n\nfunc helper() int { return 1 }\n",
        "internal/cli/gone.go": "// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n",
        "internal/cli/spec_%d.go" % (len(spec) % 7): "// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n// %d\n" % len(spec),
        "internal/cli/variant.go": "// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n// %s\n" % ("rare" if "RARE" in spec else "common"),
        # A print that is not reproducible: render 2 of 3 emits another variant,
        # render 3 emits one more file.
        "internal/cli/flaky.go": "// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n// %s\n" % ("b" if "render-spec1" in out else "a"),
        "README.md": "not vendored\n",
        "build/bin.txt": "excluded\n",
    }
    if "render-spec2" in out:
        files["internal/cli/sometimes.go"] = "// Code generated by CLI Printing Press. DO NOT EDIT.\npackage cli\n"
    for rel, body in files.items():
        p = os.path.join(out, rel); os.makedirs(os.path.dirname(p), exist_ok=True)
        open(p, "w").write(body)
    sys.exit(0)
sys.exit(64)
'''


def _self_test() -> int:
    failures: list[str] = []

    def check(name: str, ok: bool, detail: str = "") -> None:
        print(("ok   " if ok else "FAIL ") + name + (f": {detail}" if detail and not ok else ""))
        if not ok:
            failures.append(name)

    # pure functions
    check("strip keeps module imports",
          strip_pp('import "acme-pp-cli/internal/x"\nUse: "acme-pp-cli"\nfmt.Sprintf("%s-pp-mcp")', "acme")
          == 'import "acme-pp-cli/internal/x"\nUse: "acme-cli"\nfmt.Sprintf("%s-mcp")')
    check("external $ref regex hits a file ref", bool(EXTERNAL_REF_RE.search("x:\n  $ref: ./other.yaml\n")))
    check("external $ref regex hits a JSON URL ref", bool(EXTERNAL_REF_RE.search('{"$ref": "https://e/x"}')))
    check("internal $ref is allowed", not EXTERNAL_REF_RE.search('{"$ref": "#/components/schemas/A"}\n$ref: \'#/x\''))
    check("CRLF digest", spec_digests(b"a\r\nb") & spec_digests(b"a\nb") != set())
    saved_mode = os.environ.pop("SANDBOX_MODE", None)
    try:
        try:
            guard_untrusted_spec("pr", "skills/acme/press/spec.yaml")
            check("PR-supplied spec refused outside the sandbox", False)
        except Refuse:
            check("PR-supplied spec refused outside the sandbox", True)
        try:
            guard_untrusted_spec("pr", "local press folder")
            guard_untrusted_spec("tree", "skills/acme/press/spec.yaml")
            os.environ["SANDBOX_MODE"] = "offline"
            guard_untrusted_spec("pr", "skills/acme/press/spec.yaml")
            check("our press folders, --tree and in-sandbox renders allowed", True)
        except Refuse as e:
            check("our press folders, --tree and in-sandbox renders allowed", False, str(e))
    finally:
        os.environ.pop("SANDBOX_MODE", None)
        if saved_mode is not None:
            os.environ["SANDBOX_MODE"] = saved_mode

    tmp = Path(tempfile.mkdtemp(prefix="fresh-render-selftest-"))
    try:
        bindir = tmp / "bin"
        bindir.mkdir()
        fake = bindir / "cli-printing-press"

        def set_fake_version(v: str) -> None:
            # The version lives in the script itself: the jail lets the press
            # read its own binary and nothing else of ours.
            fake.write_text(FAKE_PRESS.replace("#!/usr/bin/env python3", f"#!{sys.executable}")
                            .replace("__FAKE_VERSION__", v))
            fake.chmod(0o755)

        set_fake_version("9.9.1")
        _JAIL_EXTRA_READ[:] = [sys.prefix, sys.base_prefix, os.path.dirname(os.path.realpath(sys.executable))]
        global _SELFTEST_UNJAILED
        _SELFTEST_UNJAILED = sys.platform != "darwin"
        if _SELFTEST_UNJAILED:
            try:
                _SELFTEST_UNJAILED = False
                jailed(["true"], Path("/bin/true"), tmp)
                check("no jail off macOS outside msp-sandbox", bool(os.environ.get("SANDBOX_MODE")))
            except Refuse:
                check("no jail off macOS outside msp-sandbox: render refused", True)
            _SELFTEST_UNJAILED = not os.environ.get("SANDBOX_MODE")
        spec = tmp / "spec.yaml"
        spec.write_text("openapi: 3.0.3\npaths: {}\n")
        checksum = sorted(spec_digests(spec.read_bytes()))[0]
        # Build the "vendored" connector exactly as the pipeline would from the fake render.
        work = tmp / "seed"
        work.mkdir()
        out = render(fake, spec, ["--name", "acme"], work, "seed")
        expected = normalize_render(out, "acme")
        skill = tmp / "repo" / "skills" / "acme"
        for rel, data in expected.items():
            p = skill / "cli" / rel
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(data)
        (skill / "manifest.json").write_text(json.dumps({
            "printing_press_version": "9.9.1",
            "press_provenance": {"printing_press_version": "9.9.1", "spec_checksum": checksum}}))
        (skill / "SKILL.md").write_text("docs\n")
        check("normalize maps cmd/acme-pp-cli", "cmd/acme-cli/main.go" in expected
              and b'"acme-pp-cli/internal/cli"' in expected["cmd/acme-cli/main.go"]
              and b'cli.Run("acme-cli")' in expected["cmd/acme-cli/main.go"])
        check("normalize canonical copyright", expected["internal/cli/root.go"].startswith(CANON_GO.encode())
              and expected["NOTICE"] == (CANON_TXT + "\n").encode())
        check("normalize drops README and build/", "README.md" not in expected and "build/bin.txt" not in expected)

        ns = argparse.Namespace(pr=None, tree=str(skill), slug=None, spec=str(spec),
                                press_bin=str(fake), spec_invariant=False, reference_spec=[],
                                no_local_references=True, keep=False)
        rep = run(ns)
        check("clean tree: every file is template", rep["review_files"] == 0 and rep["counts"]["template"] == len(expected),
              json.dumps(rep["counts"]))

        # Direction 1: a planted hand-edit inside a generated file must appear.
        helpers = skill / "cli" / "internal" / "cli" / "helpers.go"
        helpers.write_bytes(helpers.read_bytes().replace(b"return 1", b"return 2"))
        (skill / "cli" / "internal" / "cli" / "restore.go").write_text("package cli\n// hand-written\n")
        (skill / "cli" / "internal" / "cli" / "gone.go").unlink()
        os.symlink("/etc/passwd", skill / "cli" / "internal" / "cli" / "link.go")
        rep = run(ns)
        cls = {r["path"]: r["class"] for r in rep["review"]}
        check("planted edit appears", cls.get("internal/cli/helpers.go") == "generated-edited", json.dumps(cls))
        drep = run(argparse.Namespace(**{**vars(ns), "diff": True}))
        hd = (drep.get("diffs") or {}).get("internal/cli/helpers.go", "")
        check("--diff shows the planted hunk", "-func helper() int { return 1 }" in hd
              and "+func helper() int { return 2 }" in hd and "root.go" not in "".join(drep["diffs"]), hd)
        check("hand-written file appears", cls.get("internal/cli/restore.go") == "hand-written")
        check("deleted template file appears", cls.get("internal/cli/gone.go") == "template-deleted")
        check("symlink appears, not followed", cls.get("internal/cli/link.go") == "symlink")
        # Direction 2: a pure template file must not.
        check("pure template file stays out", "internal/cli/root.go" not in cls
              and "cmd/acme-cli/main.go" not in cls, json.dumps(cls))
        # Press nondeterminism: a file equal to ANY render's output is template;
        # a file only some renders emit is never reported deleted.
        flaky = skill / "cli" / "internal" / "cli" / "flaky.go"
        flaky.write_bytes(flaky.read_bytes().replace(b"// a\n", b"// b\n"))
        rep = run(ns)
        cls = {r["path"]: r["class"] for r in rep["review"]}
        check("a variant only one render emitted is still template", "internal/cli/flaky.go" not in cls,
              json.dumps(cls))
        check("a file only some renders emit is not a deletion", "internal/cli/sometimes.go" not in cls)
        check("unstable files are reported", set(rep["unstable_render_files"])
              >= {"internal/cli/flaky.go", "internal/cli/sometimes.go"}, json.dumps(rep["unstable_render_files"]))
        flaky.write_bytes(flaky.read_bytes().replace(b"// b\n", b"// c\n"))
        rep = run(ns)
        cls = {r["path"]: r["class"] for r in rep["review"]}
        check("a flaky file matching no render is an edit", cls.get("internal/cli/flaky.go") == "generated-edited")
        gomod = skill / "cli" / "go.mod"
        gomod.write_bytes(gomod.read_bytes() + b"\nreplace example.invalid/x => ../x\n")
        (skill / "cli" / "go.sum").write_text("example.invalid/x v1.0.0 h1:abc=\n")
        rep = run(ns)
        cls = {r["path"]: r["class"] for r in rep["review"]}
        check("go.mod edit and go.sum are dependency rows",
              cls.get("go.mod") == "dependency" and cls.get("go.sum") == "dependency", json.dumps(cls))

        # Fail closed: wrong press version.
        set_fake_version("9.9.2")
        try:
            run(ns)
            check("wrong press version refuses", False)
        except Refuse as e:
            check("wrong press version refuses", "9.9.1" in str(e) and "go install" in str(e), str(e))
        set_fake_version("9.9.1")
        # Fail closed: spec checksum mismatch.
        other = tmp / "other.yaml"
        other.write_text("openapi: 3.0.3\npaths: {x: 1}\n")
        try:
            run(argparse.Namespace(**{**vars(ns), "spec": str(other)}))
            check("spec checksum mismatch refuses", False)
        except Refuse as e:
            check("spec checksum mismatch refuses", "does not match" in str(e), str(e))
        # Fail closed: no spec anywhere.
        os.environ["PRESS_HOME"] = str(tmp / "nowhere")
        try:
            run(argparse.Namespace(**{**vars(ns), "spec": None}))
            check("missing spec refuses", False)
        except Refuse as e:
            check("missing spec refuses", "no spec with" in str(e), str(e))
        # Found through skills/<slug>/press/ instead.
        (skill / "press").mkdir()
        shutil.copyfile(spec, skill / "press" / "spec.yaml")
        rep = run(argparse.Namespace(**{**vars(ns), "spec": None}))
        check("spec found in press/ by checksum", rep["spec"]["origin"].endswith("press/spec.yaml")
              and any(r["class"] == "spec" for r in rep["review"]))
        # Fail closed: external $ref.
        (skill / "press" / "spec.yaml").unlink()
        ext = tmp / "ext.yaml"
        ext.write_text("openapi: 3.0.3\npaths:\n  /a:\n    $ref: /etc/passwd\n")
        m = json.loads((skill / "manifest.json").read_text())
        m["press_provenance"]["spec_checksum"] = sorted(spec_digests(ext.read_bytes()))[0]
        (skill / "manifest.json").write_text(json.dumps(m))
        try:
            run(argparse.Namespace(**{**vars(ns), "spec": str(ext)}))
            check("external $ref refuses", False)
        except Refuse as e:
            check("external $ref refuses", "external $ref" in str(e), str(e))
        m["press_provenance"]["spec_checksum"] = checksum
        # Fail closed: disagreeing versions.
        m["printing_press_version"] = "9.9.0"
        (skill / "manifest.json").write_text(json.dumps(m))
        try:
            run(ns)
            check("disagreeing versions refuse", False)
        except Refuse as e:
            check("disagreeing versions refuse", "disagree" in str(e), str(e))
        m["printing_press_version"] = "9.9.1"
        (skill / "manifest.json").write_text(json.dumps(m))
        # --spec-invariant: the fake press emits one spec-dependent file per
        # reference spec, so it must not count as template; spec-free files do.
        rep = run(argparse.Namespace(**{**vars(ns), "spec": None, "spec_invariant": True}))
        cls = {r["path"]: r["class"] for r in rep["review"]}
        check("spec-invariant keeps the planted edit", cls.get("internal/cli/helpers.go") == "generated-edited",
              json.dumps(cls))
        check("spec-invariant clears spec-free template", "internal/cli/root.go" not in cls)
        dep = [p for p, c in cls.items() if p.startswith("internal/cli/spec_")]
        check("spec-invariant leaves spec-dependent files in review",
              bool(dep) and all(cls[p] == "generated-spec-dependent" for p in dep), json.dumps(cls))

        # --- Codex round 1 classes, each with its failing input ---
        # (a) a file hidden from `git archive` by export-ignore must still be read.
        src = tmp / "srcrepo"
        src.mkdir()
        g = ["git", "-C", str(src), "-c", "user.name=t", "-c", "user.email=t@example.invalid"]
        subprocess.run(["git", "init", "-q", str(src)], check=True)
        (src / "skills" / "acme" / "cli" / "internal").mkdir(parents=True)
        (src / "skills" / "acme" / ".gitattributes").write_text("cli/internal/hidden.go export-ignore\n"
                                                                "cli/internal/subst.go export-subst\n")
        (src / "skills" / "acme" / "cli" / "internal" / "hidden.go").write_text("package internal\n// hidden\n")
        (src / "skills" / "acme" / "cli" / "internal" / "subst.go").write_text("package internal\n// $Format:%H$\n")
        subprocess.run(g + ["add", "-A"], check=True)
        subprocess.run(g + ["commit", "-q", "-m", "x"], check=True)
        sha = subprocess.run(["git", "-C", str(src), "rev-parse", "HEAD"], capture_output=True,
                             text=True, check=True).stdout.strip()
        out_dir = tmp / "mat"
        out_dir.mkdir()
        materialize(src, sha, "acme", out_dir)
        hid = out_dir / "skills" / "acme" / "cli" / "internal" / "hidden.go"
        sub = out_dir / "skills" / "acme" / "cli" / "internal" / "subst.go"
        check("export-ignore cannot hide a file", hid.is_file())
        check("export-subst cannot rewrite a file", sub.is_file() and "$Format:%H$" in sub.read_text())
        # (b) fetching a PR head writes nothing into the repository it reads from.
        clone = tmp / "clone"
        subprocess.run(["git", "clone", "-q", "--no-local", str(src), str(clone)], check=True)
        before = sorted(p.relative_to(clone).as_posix() for p in (clone / ".git").rglob("*"))
        fetch_commit(clone, sha, tmp)
        after = sorted(p.relative_to(clone).as_posix() for p in (clone / ".git").rglob("*"))
        check("fetch leaves the repository untouched", before == after)
        # (c) escaped $ref keys are external refs too.
        for body, name in ((r'{"paths": {"/a": {"\u0024ref": "/etc/passwd"}}}', "esc.json"),
                           ('paths:\n  /a:\n    "\\x24ref": /etc/passwd\n', "esc.yaml")):
            ep = tmp / name
            ep.write_text(body)
            try:
                refuse_external_refs(ep)
                check(f"escaped $ref refused ({name})", False)
            except Refuse:
                check(f"escaped $ref refused ({name})", True)
        # (c2) the jail itself: whatever a spec manages to name, the press process
        # cannot read our files or reach the network (macOS; msp-sandbox elsewhere).
        if sys.platform == "darwin" and not os.environ.get("SANDBOX_MODE"):
            jw = tmp / "jailwork"
            jw.mkdir()
            (jw / "inside.txt").write_text("ok\n")
            mine = Path(__file__).resolve()
            ok_in = subprocess.run(jailed(["/bin/cat", str(jw / "inside.txt")], Path("/bin/cat"), jw),
                                   capture_output=True).returncode == 0
            home_read = subprocess.run(jailed(["/bin/cat", str(mine)], Path("/bin/cat"), jw),
                                       capture_output=True).returncode
            out_write = subprocess.run(jailed(["/usr/bin/touch", str(tmp / "outside.txt")],
                                              Path("/usr/bin/touch"), jw), capture_output=True).returncode
            check("jail: reads its work dir", ok_in)
            check("jail: cannot write outside its work dir", out_write != 0 and not (tmp / "outside.txt").exists())
            if str(mine).startswith(os.path.realpath(Path.home())):
                check("jail: cannot read a file under HOME", home_read != 0)
                via_data = subprocess.run(jailed(["/bin/cat", "/System/Volumes/Data" + str(mine)],
                                                 Path("/bin/cat"), jw), capture_output=True).returncode
                check("jail: cannot read HOME through /System/Volumes/Data", via_data != 0)

        # --- round-2 classes (failing inputs first) ---
        # (g) the jail allows reads only where it must, not "everything but a list".
        if sys.platform == "darwin" and not os.environ.get("SANDBOX_MODE"):
            fd, stray = tempfile.mkstemp(dir="/private/var/tmp", prefix="frd-jail-")
            os.write(fd, b"secret\n")
            os.close(fd)
            try:
                rc = subprocess.run(jailed(["/bin/cat", stray], Path("/bin/cat"), jw),
                                    capture_output=True).returncode
                check("jail: cannot read a file outside its work dir (any location)", rc != 0)
            finally:
                os.unlink(stray)
        # (h) no symlink anywhere on the tree path or inside the skill directory.
        real_skills = tmp / "outside2" / "skills"
        shutil.copytree(skill, real_skills / "acme", symlinks=True)
        (tmp / "review2").mkdir()
        os.symlink(real_skills, tmp / "review2" / "skills")
        try:
            run(argparse.Namespace(**{**vars(ns), "tree": str(tmp / "review2" / "skills" / "acme")}))
            check("symlinked ancestor of the skill dir refused", False)
        except Refuse:
            check("symlinked ancestor of the skill dir refused", True)
        press_real = tmp / "outside3"
        press_real.mkdir()
        (press_real / ".printing-press.json").write_text(json.dumps({"printing_press_version": "9.9.1"}))
        if (skill / "press").exists():
            shutil.rmtree(skill / "press")
        os.symlink(press_real, skill / "press")
        try:
            run(ns)
            check("symlinked press/ inside the skill dir refused", False)
        except Refuse:
            check("symlinked press/ inside the skill dir refused", True)
        finally:
            (skill / "press").unlink()

        # (d) in --pr mode an operator --spec is still contributor-chosen bytes.
        saved = os.environ.pop("SANDBOX_MODE", None)
        try:
            guard_untrusted_spec("pr", "--spec")
            check("--pr --spec refused outside the sandbox", False)
        except Refuse:
            check("--pr --spec refused outside the sandbox", True)
        finally:
            if saved is not None:
                os.environ["SANDBOX_MODE"] = saved
        # (e) a prerelease of the recorded version is a different version.
        set_fake_version("9.9.1-rc.1")
        try:
            run(ns)
            check("prerelease press version refused", False)
        except Refuse:
            check("prerelease press version refused", True)
        set_fake_version("9.9.1")
        # (f) a symlinked skills/<slug> under --tree is refused, not read through.
        outside = tmp / "outside"
        shutil.copytree(skill, outside / "acme", symlinks=True)
        review = tmp / "review" / "skills"
        review.mkdir(parents=True)
        os.symlink(outside / "acme", review / "acme")
        try:
            run(argparse.Namespace(**{**vars(ns), "tree": str(review.parent), "slug": "acme"}))
            check("symlinked skill dir refused", False)
        except Refuse:
            check("symlinked skill dir refused", True)

        # An unexpected failure (git or gh missing, a timeout, an undecodable
        # file) is "could not render", exit 2, never a traceback.
        g = globals()
        real_run = g["run"]
        try:
            def boom(_args):
                raise subprocess.TimeoutExpired(["git"], 1)
            g["run"] = boom
            check("unexpected failure exits 2", main(["--tree", str(skill)]) == 2)
        finally:
            g["run"] = real_run

        # --calibrate, both directions, with stand-in "local real specs".
        global local_reference_specs
        real_fn = local_reference_specs
        reals = []
        for i, body in enumerate(("openapi: 3.0.3\n# a\n", "openapi: 3.0.3\n# bb\n", "openapi: 3.0.3\n# RARE\n")):
            p = tmp / f"real{i}.yaml"
            p.write_text(body)
            reals.append((f"real{i}", p))
        try:
            local_reference_specs = lambda version, exclude=None: reals[:2]  # noqa: E731
            check("calibrate passes when every variant is seen", calibrate("9.9.1", str(fake), False) == 0)
            local_reference_specs = lambda version, exclude=None: reals  # noqa: E731
            check("calibrate fails on a variant only one real spec renders",
                  calibrate("9.9.1", str(fake), False) == 1)
        finally:
            local_reference_specs = real_fn
    finally:
        _JAIL_EXTRA_READ.clear()
        _SELFTEST_UNJAILED = False
        os.environ.pop("PRESS_HOME", None)
        shutil.rmtree(tmp, ignore_errors=True)
    print("self-test:", "FAIL" if failures else "PASS")
    return 1 if failures else 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    src = ap.add_mutually_exclusive_group()
    src.add_argument("--pr", type=int, help="a Servosity/msp-skills pull request number")
    src.add_argument("--tree", help="a skills/<slug> directory, or a repository root with --slug")
    src.add_argument("--calibrate", metavar="VERSION",
                     help="check the reference specs against local real specs at a press version")
    src.add_argument("--self-test", action="store_true")
    ap.add_argument("--slug")
    ap.add_argument("--spec", help="the exact spec the connector was generated from")
    ap.add_argument("--press-bin", help="a press binary; it must report the recorded version")
    ap.add_argument("--spec-invariant", action="store_true",
                    help="fallback when the spec is unavailable: clear only spec-independent files")
    ap.add_argument("--reference-spec", action="append",
                    help="an extra trusted reference spec for --spec-invariant (repeatable)")
    ap.add_argument("--no-local-references", action="store_true",
                    help="--spec-invariant without the local real specs at the same version")
    ap.add_argument("--diff", action="store_true",
                    help="include render -> connector diffs of every edited generated file")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--list", action="store_true", help="print the review set paths only")
    ap.add_argument("--keep", action="store_true", help="keep the render directory")
    args = ap.parse_args(argv)
    if args.self_test:
        return _self_test()
    if args.calibrate:
        if not SEMVER_RE.match(args.calibrate):
            ap.error("--calibrate needs X.Y.Z")
        return calibrate(args.calibrate, args.press_bin, args.keep)
    if args.pr is None and not args.tree:
        ap.error("pass --pr N or --tree DIR")
    if args.slug and not SLUG_RE.match(args.slug):
        ap.error(f"--slug {args.slug!r} is not a slug")
    try:
        rep = run(args)
    except Refuse as e:
        print(f"fresh-render-diff: cannot render: {e}", file=sys.stderr)
        return 2
    except (OSError, subprocess.SubprocessError, UnicodeDecodeError, ValueError) as e:
        # git or gh missing, a timeout, an undecodable file: still "could not
        # render" (exit 2), never a traceback that reads as a found failure.
        print(f"fresh-render-diff: cannot render: {e.__class__.__name__}: {str(e)[:300]}",
              file=sys.stderr)
        return 2
    if args.json:
        print(json.dumps(rep, indent=1))
    elif args.list:
        for r in rep["review"]:
            print(r["path"])
    else:
        print(human(rep))
    return 0


if __name__ == "__main__":
    sys.exit(main())
