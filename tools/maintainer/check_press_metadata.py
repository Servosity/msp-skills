#!/usr/bin/env python3
"""check_press_metadata.py - the press's own metadata beside each connector, and
the press-version floor for NEW connectors.

1. skills/<dir>/press/ (publish plan step 4.3)
------------------------------------------------
cli-printing-press writes three files beside every CLI it generates, and the
vendoring step used to drop all three:

  .printing-press.json   provenance: printing_press_version, generated_at,
                         run_id, spec_checksum (sha256 of the spec it read), ...
  tools-manifest.json    every tool's name, HTTP method and path, plus the
                         API's auth and base URL
  spec.json | spec.yaml | spec.yml | spec.graphql
                         the API spec the CLI was generated from

msp-skills-publish's onboard.py now copies them into skills/<dir>/press/ (never
into cli/, so the build and cli_hash are unaffected). This check validates a
press/ folder wherever one exists:

  - it holds only those files (one spec at most), each a regular UTF-8 file,
    and .printing-press.json is present (a folder without its provenance
    cannot say which code it describes);
  - JSON files parse; tools-manifest.json is {"tools": [{name, method, path}]}
    with a real HTTP method;
  - .printing-press.json's printing_press_version equals manifest.json's, and
    its generated_at and spec_checksum equal manifest.json's press_provenance
    when that records them (so the folder belongs to the print that was
    vendored, not an older or newer one);
  - the spec's sha256 equals spec_checksum (as the press computes it: raw, or
    with CRLF normalized to LF). This is the parse check for YAML and GraphQL,
    which the standard library cannot parse: the bytes are the ones the press
    parsed;
  - nothing secret-looking or personal: no private key block, cloud or chat
    token shape, JWT, or /Users/<name> or /home/<name>/ path. A hit is reported
    by file and line, never echoed.

A connector WITHOUT a press/ folder is reported in the coverage line, never a
failure (yet: governance tiers will start requiring it).

2. The press floor (publish plan step 4.5)
------------------------------------------
tools/maintainer/press_floor.json holds `min_press_version`, upstream's own
declared minimum. With --base, a connector registered now but not at the base
is NEW, and it must record a printing_press_version at or above the floor in
its manifest.json. Existing connectors are never affected. The floor is the
higher of the base's and this tree's value (a PR cannot lower it), and only the
BASE's `not_press_generated` list exempts a connector (a PR cannot exempt
itself).

Usage
    python3 tools/maintainer/check_press_metadata.py                 # every press/ folder + coverage
    python3 tools/maintainer/check_press_metadata.py --base <rev>    # ... plus the floor for new connectors
    python3 tools/maintainer/check_press_metadata.py --slug <slug>
    python3 tools/maintainer/check_press_metadata.py --inspect DIR [--manifest FILE] [--gitleaks]
        per-file verdicts as JSON, for onboard.py deciding what it may copy
    python3 tools/maintainer/check_press_metadata.py --self-test

Exit 0 = clean (missing metadata included), 1 = findings, 2 = the check could
not run (an unreadable base, a missing floor file). Pure stdlib.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import html
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_engine_freshness  # noqa: E402  (local tools/ module: the nested-aware version read)
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
FLOOR_REL = "tools/maintainer/press_floor.json"
REGISTRY_REL = "tools/maintainer/skills.json"
PRESS_DIR = "press"

PROVENANCE = ".printing-press.json"
TOOLS = "tools-manifest.json"
SPECS = ("spec.json", "spec.yaml", "spec.yml", "spec.graphql")
# Written by onboard.py's vendor_press, never by the press: why a file was
# withheld, and the proof that a published spec's source is public.
PUBLICATION = "publication.json"
ALLOWED = (PROVENANCE, TOOLS) + SPECS + (PUBLICATION,)
SPEC_BASES = ("public-url", "repo-history")
NOT_PUBLIC = "spec source not public"
METHODS = {"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

# Secret shapes worth refusing outright in vendor data. ci_guards.sh guard 4's
# set plus the token families vendor specs actually carry as "examples"
# (measured on the library copies: JWTs, private key blocks, AWS key ids).
# gitleaks stays the authoritative scan in CI; these are the shapes no reviewer
# should ever have to judge.
SECRET_RES = (
    ("private key block", re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
    ("AWS access key id", re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    ("GitHub token", re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})")),
    ("Slack token", re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}")),
    ("Stripe live key", re.compile(r"\b[sr]k_live_[0-9A-Za-z]{16,}")),
    ("Google API key", re.compile(r"\bAIza[0-9A-Za-z_-]{35}")),
    ("JWT", re.compile(r"\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}")),
)
# A filesystem home directory with a path below it: /Users/<name>/..., /home/<name>/...
# (any name: hyphens, digits, dots) or C:\\Users\\<name> in any slash spelling.
# Anchored so it does not start inside a URL host or a longer path
# (portal.example.com/home/settings) and needs the "/" after the name, so API
# routes such as Users/me or Users/{id} (SCIM, 3CX) are not mistaken for one.
# press/ is excluded from ci_guards.sh's human-authored list, like cli/, so the
# personal-path rule is enforced here instead (measured on the library copies:
# it catches every real home path there, including ones inside vendor specs).
PERSONAL_RE = re.compile(r"(?<![\w.-])/(?:Users|home)/[A-Za-z0-9._-]+/"
                         r"|(?<![\w.-])[A-Za-z]:(?:\\{1,4}|/)Users(?:\\{1,4}|/)[A-Za-z0-9._-]")

_SEMVER = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$")


class EnvError(Exception):
    """The check could not run (exit 2): never reported as a pass."""


def parse_version(value: object) -> tuple[tuple[int, int, int], bool] | None:
    """((major, minor, patch), is_prerelease), or None for anything that is not
    a version string."""
    if not isinstance(value, str):
        return None
    m = _SEMVER.match(value.strip())
    if not m:
        return None
    return (int(m.group(1)), int(m.group(2)), int(m.group(3))), m.group(4) is not None


def meets_floor(value: object, floor: str) -> bool:
    """value >= floor. A prerelease of the floor version itself does not meet it
    (4.28.0-rc1 < 4.28.0), the rule upstream's own library gate applies."""
    got, want = parse_version(value), parse_version(floor)
    if got is None or want is None:
        return False
    if got[0] != want[0]:
        return got[0] > want[0]
    return not got[1]


def spec_checksums(data: bytes) -> set[str]:
    """Every checksum the press accepts for these bytes: raw, CRLF->LF, and the
    all-CRLF form of the LF text (internal/pipeline/spec_checksum.go)."""
    lf = data.replace(b"\r\n", b"\n")
    return {"sha256:" + hashlib.sha256(b).hexdigest() for b in (data, lf, lf.replace(b"\n", b"\r\n"))}


_ESCAPE = re.compile(r"\\(?:u([0-9a-fA-F]{4})|x([0-9a-fA-F]{2})|(.))", re.S)
_SIMPLE_ESCAPES = {"n": "\n", "t": "\t", "r": "\r", "f": "\f", "b": "\b", "0": "\0"}


def _unescape(text: str) -> str:
    """One level of backslash escapes (JSON, YAML double quotes, GraphQL): \\n is
    a line break, \\/ a slash, \\u002f a slash, so a path cannot hide behind one."""
    def sub(m: re.Match) -> str:
        if m.group(1):
            return chr(int(m.group(1), 16))
        if m.group(2):
            return chr(int(m.group(2), 16))
        return _SIMPLE_ESCAPES.get(m.group(3), m.group(3))
    return _ESCAPE.sub(sub, text)


DECODERS = (("backslash escapes", _unescape), ("percent-encoding", urllib.parse.unquote),
            ("HTML entities", html.unescape))


def decoded_views(text: str, depth: int = 3) -> list[tuple[str, str]]:
    """The raw text and every distinct text the decoders reach from it, up to
    `depth` layers (a JSON string holding an escaped YAML string, a
    percent-encoded file:// URL). The scan runs on each: one decoder chain,
    shared by every rule, so a new encoding is one entry, not a new rule."""
    views = [("", text)]
    seen = {text}
    frontier = [("", text)]
    for _ in range(depth):
        nxt = []
        for label, t in frontier:
            for name, fn in DECODERS:
                d = fn(t)
                if d not in seen:
                    seen.add(d)
                    nxt.append(((label + " + " if label else "") + name, d))
        views += nxt
        frontier = nxt
    return views


def scan_text(text: str) -> list[str]:
    """Secret-looking or personal content in the text OR any decoded view of it,
    as 'line N: <kind>' (never the match)."""
    hits: list[str] = []
    for label, view in decoded_views(text):
        where = f" (after decoding {label})" if label else ""
        for n, line in enumerate(view.splitlines(), 1):
            for kind, rx in SECRET_RES:
                if rx.search(line):
                    hits.append(f"line {n}{where}: looks like a {kind}")
            if PERSONAL_RE.search(line):
                hits.append(f"line {n}{where}: a personal filesystem path")
            if len(hits) >= 5:
                return hits + ["(further hits not listed)"]
        if hits:
            return hits  # the raw view's line numbers are the useful ones
    return hits


def _manifest_view(manifest: dict | None) -> tuple[str | None, dict]:
    if not isinstance(manifest, dict):
        return None, {}
    prov = manifest.get("press_provenance")
    return check_engine_freshness._press_version(manifest), prov if isinstance(prov, dict) else {}


def git_blob_sha(data: bytes) -> str:
    return hashlib.sha1(b"blob %d\0" % len(data) + data).hexdigest()


def _git_at(repo: Path, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", "-C", str(repo), *args], capture_output=True, text=True, timeout=600)


def _outside_press(path: str) -> bool:
    parts = path.split("/")
    return not (len(parts) >= 4 and parts[0] == "skills" and parts[2] == PRESS_DIR)


def history_path(repo: Path, commit: str, blob: str) -> str | None:
    """A path at `commit` that holds `blob`, outside every skills/*/press/
    folder (a spec already published elsewhere in the repo), else None."""
    ls = _git_at(repo, "ls-tree", "-r", "--full-tree", commit)
    if ls.returncode != 0:
        return None
    for line in ls.stdout.splitlines():
        meta, _, path = line.partition("\t")
        if meta.split()[-1:] == [blob] and _outside_press(path):
            return path
    return None


def spec_urls(provenance: dict) -> list[str]:
    """Every http(s) URL the provenance records as the spec's source."""
    out = []
    for key in ("spec_url", "spec_source", "spec_path"):
        v = provenance.get(key)
        if isinstance(v, str) and urllib.parse.urlsplit(v).scheme in ("http", "https") \
                and urllib.parse.urlsplit(v).netloc:
            out.append(v)
    return out


def url_status(url: str, timeout: float = 20.0) -> tuple[int | None, str]:
    """(status, note) of an UNAUTHENTICATED request: HEAD, then a one-byte GET
    when HEAD is refused. No credentials, cookies or auth headers are sent."""
    for method in ("HEAD", "GET"):
        req = urllib.request.Request(url, method=method, headers={
            "User-Agent": "msp-skills-check-press-metadata", **({"Range": "bytes=0-0"} if method == "GET" else {})})
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.status, method
        except urllib.error.HTTPError as e:
            if method == "HEAD" and e.code in (400, 403, 405, 501):
                continue
            return e.code, method
        except (urllib.error.URLError, OSError, ValueError) as e:
            return None, f"{type(e).__name__}: {e}"[:200]
    return None, "no answer"


def spec_source_record(spec_name: str, data: bytes, provenance: dict, repo: Path,
                       ref: str) -> tuple[dict | None, str]:
    """(record, reason): a record proving the spec's source is public, or None
    and why it is withheld. Public means the spec's exact bytes are already in
    the repository's history outside press/, or the provenance names an
    http(s) spec URL that answers 2xx without credentials today."""
    blob = git_blob_sha(data)
    found = _git_at(repo, "log", ref, "--format=%H", "-1", f"--find-object={blob}")
    commit = found.stdout.strip() if found.returncode == 0 else ""
    if commit and history_path(repo, commit, blob):
        return {"file": spec_name, "basis": "repo-history", "blob": blob, "commit": commit,
                "path": history_path(repo, commit, blob)}, ""
    urls = spec_urls(provenance)
    if not urls:
        return None, (f"{NOT_PUBLIC}: the provenance records no http(s) spec URL and the spec "
                      "is not in the repository's history")
    notes = []
    for url in urls:
        status, note = url_status(url)
        if status is not None and 200 <= status < 300:
            return {"file": spec_name, "basis": "public-url", "url": url, "status": status,
                    "checked_on": dt.date.today().isoformat()}, ""
        notes.append(f"{url} answered {status} ({note})" if status is not None else f"{url}: {note}")
    if all(": " in n and " answered " not in n for n in notes):
        return None, "spec source unknown: could not reach " + "; ".join(notes)
    return None, f"{NOT_PUBLIC}: " + "; ".join(notes) + " without credentials"


def check_publication(pub: object, entries: list[str], specs_ok: dict[str, bytes],
                      provenance: dict | None, repo: Path) -> tuple[list[str], dict[str, str]]:
    """(folder problems, {spec: reason}) for publication.json against the folder."""
    problems: list[str] = []
    spec_bad: dict[str, str] = {}
    rec = None
    if pub is not None:
        if not isinstance(pub, dict) or set(pub) - {"schema", "spec", "withheld"}:
            return [f"{PUBLICATION}: must be an object with only schema, spec and withheld"], {}
        withheld = pub.get("withheld") or {}
        if not isinstance(withheld, dict):
            problems.append(f"{PUBLICATION}: withheld must map a file name to a reason")
            withheld = {}
        for name, why in withheld.items():
            if name not in ALLOWED or name in (PROVENANCE, PUBLICATION):
                problems.append(f"{PUBLICATION}: withheld names {name!r}, not a withholdable press file")
            elif name in entries:
                problems.append(f"{PUBLICATION}: {name} is listed as withheld but is present")
            if not isinstance(why, str) or not why.strip():
                problems.append(f"{PUBLICATION}: withheld {name!r} has no reason")
        rec = pub.get("spec")
    for name, data in specs_ok.items():
        if not isinstance(rec, dict) or rec.get("file") != name or rec.get("basis") not in SPEC_BASES:
            spec_bad[name] = (f"published without a recorded public source in {PUBLICATION} "
                              f"(a vendor spec ships only when its source is provably public)")
            continue
        if rec["basis"] == "public-url":
            ok = (isinstance(rec.get("url"), str) and rec["url"] in spec_urls(provenance or {})
                  and isinstance(rec.get("status"), int) and 200 <= rec["status"] < 300
                  and isinstance(rec.get("checked_on"), str)
                  and re.fullmatch(r"\d{4}-\d{2}-\d{2}", rec["checked_on"]))
            if not ok:
                spec_bad[name] = (f"{PUBLICATION} public-url record does not name a 2xx check of a "
                                  f"spec URL that {PROVENANCE} records")
        else:
            commit = rec.get("commit") if isinstance(rec.get("commit"), str) else ""
            ok = (rec.get("blob") == git_blob_sha(data) and re.fullmatch(r"[0-9a-f]{40}", commit)
                  and _git_at(repo, "merge-base", "--is-ancestor", commit, "HEAD").returncode == 0
                  and history_path(repo, commit, rec["blob"]) is not None)
            if not ok:
                spec_bad[name] = (f"{PUBLICATION} repo-history record does not prove this spec's bytes "
                                  "are in this repository's history outside press/")
    if isinstance(rec, dict) and rec.get("file") not in specs_ok:
        problems.append(f"{PUBLICATION}: a spec record for {rec.get('file')!r}, which is not here")
    return problems, spec_bad


def inspect_dir(d: Path, manifest: dict | None, check_source: bool = False,
                repo: Path = ROOT, ref: str = "origin/main",
                records: dict | None = None) -> tuple[dict[str, str], list[str]]:
    """({file: "ok" | reason}, folder-level problems) for one press/ folder.

    `manifest` is the connector's manifest.json (None = not known: the version
    agreement checks are skipped, which only --inspect callers may ask for).
    With `check_source` (onboard.py, at vendoring time) a spec is kept only
    when spec_source_record proves its source public, and the proof is put in
    `records["spec"]`; without it (CI) a present spec must carry that proof
    in publication.json."""
    verdicts: dict[str, str] = {}
    problems: list[str] = []
    if d.is_symlink():
        return verdicts, ["the folder is a symlink"]
    try:
        entries = sorted(os.listdir(d))
    except OSError as e:
        return verdicts, [f"cannot list the folder ({e})"]
    for name in entries:
        if name not in ALLOWED:
            problems.append(f"{name}: not a press metadata file (allowed: {', '.join(ALLOWED)})")
    specs = [s for s in SPECS if s in entries]
    if len(specs) > 1:
        problems.append(f"more than one spec ({', '.join(specs)}); keep the one .printing-press.json names")
    want_version, want_prov = _manifest_view(manifest)
    provenance: dict | None = None
    texts: dict[str, tuple[bytes, str]] = {}
    for name in [n for n in ALLOWED if n in entries]:
        p = d / name
        if p.is_symlink() or not p.is_file():
            verdicts[name] = "not a regular file"
            continue
        try:
            raw = p.read_bytes()
            text = raw.decode("utf-8")
        except (OSError, UnicodeDecodeError) as e:
            verdicts[name] = f"unreadable or not UTF-8 ({type(e).__name__})"
            continue
        hits = scan_text(text)
        if hits:
            verdicts[name] = "secret-looking or personal content: " + "; ".join(hits)
            continue
        texts[name] = (raw, text)
        verdicts[name] = "ok"
    for name in (PROVENANCE, TOOLS, "spec.json"):
        if verdicts.get(name) != "ok":
            continue
        try:
            obj = json.loads(texts[name][1])
        except ValueError as e:
            verdicts[name] = f"not valid JSON ({e})"
            continue
        if not isinstance(obj, dict):
            verdicts[name] = "not a JSON object"
            continue
        if name == PROVENANCE:
            provenance = obj
        elif name == TOOLS:
            tools = obj.get("tools")
            if not isinstance(tools, list) or not tools:
                verdicts[name] = "no `tools` list"
                continue
            for i, t in enumerate(tools):
                if not (isinstance(t, dict) and isinstance(t.get("name"), str) and t["name"]
                        and isinstance(t.get("path"), str)
                        and isinstance(t.get("method"), str) and t["method"].upper() in METHODS):
                    verdicts[name] = f"tool #{i} lacks a name, a path or a real HTTP method"
                    break
    if PROVENANCE not in entries:
        problems.append(f"{PROVENANCE} is missing: without it the folder cannot say which print it describes"
                        + ("" if entries else " (the folder is empty)"))
    elif verdicts.get(PROVENANCE) == "ok" and provenance is not None:
        got = provenance.get("printing_press_version")
        if parse_version(got) is None:
            verdicts[PROVENANCE] = f"printing_press_version {got!r} is not a version"
        elif manifest is not None:
            if want_version is None:
                verdicts[PROVENANCE] = ("manifest.json records no printing_press_version, so this "
                                        f"folder ({got}) cannot be tied to the vendored code")
            elif got != want_version:
                verdicts[PROVENANCE] = (f"printing_press_version {got} disagrees with manifest.json "
                                        f"{want_version} (the folder belongs to another print)")
            else:
                for key in ("generated_at", "spec_checksum"):
                    if want_prov.get(key) and provenance.get(key) != want_prov[key]:
                        verdicts[PROVENANCE] = (f"{key} {provenance.get(key)!r} disagrees with manifest.json "
                                                f"press_provenance {want_prov[key]!r} (another print)")
                        break
        checksum = provenance.get("spec_checksum")
        for s in specs:
            if verdicts.get(s) == "ok" and isinstance(checksum, str) and checksum:
                if checksum not in spec_checksums(texts[s][0]):
                    verdicts[s] = f"sha256 does not match {PROVENANCE} spec_checksum (not the spec the press read)"
    elif verdicts.get(PROVENANCE) != "ok":
        problems.append(f"{PROVENANCE} is unusable ({verdicts.get(PROVENANCE)})")
    specs_ok = {n: texts[n][0] for n in specs if verdicts.get(n) == "ok"}
    if check_source:
        for n, data in specs_ok.items():
            rec, why = spec_source_record(n, data, provenance or {}, repo, ref)
            if rec is None:
                verdicts[n] = why
            elif records is not None:
                records["spec"] = rec
    else:
        pub = None
        if verdicts.get(PUBLICATION) == "ok":
            try:
                pub = json.loads(texts[PUBLICATION][1])
            except ValueError as e:
                verdicts[PUBLICATION] = f"not valid JSON ({e})"
        extra, spec_bad = check_publication(pub, entries, specs_ok, provenance, repo)
        problems += extra
        verdicts.update(spec_bad)
    return verdicts, problems


def gitleaks_verdicts(d: Path, verdicts: dict[str, str]) -> str:
    """Run gitleaks (CI's authoritative scan, with this repo's config) over d and
    mark each flagged file. Returns a note for the caller."""
    exe = shutil.which("gitleaks")
    if not exe:
        return "gitleaks not installed: CI's secret scan may still flag a file copied here"
    with tempfile.TemporaryDirectory() as t:
        report = Path(t) / "report.json"
        cfg = ROOT / ".gitleaks.toml"
        p = subprocess.run([exe, "detect", "--no-git", "--source", str(d),
                            *(["--config", str(cfg)] if cfg.is_file() else []),
                            "--redact", "--no-banner",
                            "--report-format", "json", "--report-path", str(report)],
                           capture_output=True, text=True, timeout=300)
        if p.returncode not in (0, 1):
            raise EnvError(f"gitleaks failed ({p.returncode}): {p.stderr.strip()[-300:]}")
        try:
            found = json.loads(report.read_text(encoding="utf-8") or "[]")
        except (OSError, ValueError) as e:
            raise EnvError(f"gitleaks report unreadable ({e})") from e
    for f in found or []:
        name = Path(str(f.get("File", ""))).name
        if name in verdicts:
            verdicts[name] = f"gitleaks flags it ({f.get('RuleID', 'rule')})"
    return f"gitleaks: {len(found or [])} finding(s)"


def _read_json(p: Path) -> dict | None:
    try:
        obj = json.loads(p.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None
    return obj if isinstance(obj, dict) else None


def check_fleet(only: str | None = None) -> tuple[list[str], dict]:
    findings: list[str] = []
    cov = {"connectors": 0, "press": 0, "tools": 0, "spec": 0, "missing": []}
    connector_dirs = {registry.skill_path(slug).name for slug, e in registry.skills().items()
                      if not e.get("markdown_only")}
    if not only:
        # Every press/ folder on disk is checked: one under a directory that is
        # not a registered connector would otherwise escape every rule (and
        # ci_guards.sh skips press/).
        for p in sorted(registry.SKILLS_DIR.glob(f"*/{PRESS_DIR}")):
            if p.parent.name not in connector_dirs:
                findings.append(f"skills/{p.parent.name}/{PRESS_DIR}/: not under a registered connector, "
                                "so nothing vouches for it; remove it or register the connector")
    for slug, entry in registry.skills().items():
        if entry.get("markdown_only") or (only and slug != only):
            continue
        cov["connectors"] += 1
        sdir = registry.skill_path(slug)
        rel = sdir.relative_to(ROOT).as_posix()
        d = sdir / PRESS_DIR
        if not d.exists() and not d.is_symlink():
            cov["missing"].append(slug)
            continue
        manifest = _read_json(sdir / "manifest.json")
        if manifest is None:
            findings.append(f"{rel}/manifest.json: unreadable, so {rel}/{PRESS_DIR}/ cannot be checked")
            continue
        verdicts, problems = inspect_dir(d, manifest)
        findings += [f"{rel}/{PRESS_DIR}/: {p}" for p in problems]
        findings += [f"{rel}/{PRESS_DIR}/{n}: {v}" for n, v in verdicts.items() if v != "ok"]
        if not problems and all(v == "ok" for v in verdicts.values()):
            cov["press"] += 1
            cov["tools"] += TOOLS in verdicts
            cov["spec"] += any(s in verdicts for s in SPECS)
            pub = _read_json(d / PUBLICATION) or {}
            for name, why in (pub.get("withheld") or {}).items():
                if name in SPECS:
                    key = ("spec withheld: not public" if str(why).startswith(NOT_PUBLIC) else
                           "spec withheld: source unknown" if str(why).startswith("spec source unknown")
                           else "spec withheld: other")
                    cov[key] = cov.get(key, 0) + 1
    return findings, cov


def _git(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", "-C", str(ROOT), *args], capture_output=True, text=True, timeout=60)


def load_floor(text: str, where: str) -> tuple[str, dict]:
    try:
        obj = json.loads(text)
    except ValueError as e:
        raise EnvError(f"{where} is not valid JSON ({e})") from e
    floor = obj.get("min_press_version") if isinstance(obj, dict) else None
    if parse_version(floor) is None or parse_version(floor)[1]:
        raise EnvError(f"{where}: min_press_version {floor!r} is not a release version")
    exempt = obj.get("not_press_generated") or {}
    if not isinstance(exempt, dict) or not all(isinstance(v, str) and v.strip() for v in exempt.values()):
        raise EnvError(f"{where}: not_press_generated must map slug -> a non-empty reason")
    return floor, exempt


def check_floor(base: str) -> list[str]:
    """Findings for connectors this tree adds relative to `base`."""
    if _git("rev-parse", "--verify", "--quiet", f"{base}^{{commit}}").returncode != 0:
        raise EnvError(f"--base {base} does not resolve (fetch it); the floor check did not run")
    shown = _git("show", f"{base}:{REGISTRY_REL}")
    if shown.returncode != 0:
        raise EnvError(f"{base}:{REGISTRY_REL} is unreadable; the floor check did not run")
    try:
        base_skills = json.loads(shown.stdout)["skills"]
    except (ValueError, KeyError, TypeError) as e:
        raise EnvError(f"{base}:{REGISTRY_REL} is not a registry ({e})") from e
    floors: list[str] = []
    exempt: dict = {}
    base_floor = _git("show", f"{base}:{FLOOR_REL}")
    if base_floor.returncode == 0:
        f, exempt = load_floor(base_floor.stdout, f"{base[:12]}:{FLOOR_REL}")
        floors.append(f)
    head_path = ROOT / FLOOR_REL
    if head_path.exists():
        floors.append(load_floor(head_path.read_text(encoding="utf-8"), FLOOR_REL)[0])
    if not floors:
        raise EnvError(f"no {FLOOR_REL} here or at {base[:12]}; the floor check did not run")
    floor = max(floors, key=lambda v: parse_version(v)[0])
    findings = []
    if base_floor.returncode == 0:
        if not head_path.exists():
            findings.append(f"{FLOOR_REL}: deleted, but {base[:12]} has one; the floor can be raised, "
                            "never removed")
        elif parse_version(floors[-1])[0] < parse_version(floors[0])[0]:
            findings.append(f"{FLOOR_REL}: min_press_version {floors[-1]} lowers the floor "
                            f"{floors[0]} at {base[:12]}; the floor can be raised, never lowered")
    for slug, entry in registry.skills().items():
        if entry.get("markdown_only") or slug in base_skills:
            continue
        rel = registry.skill_path(slug).relative_to(ROOT).as_posix()
        if slug in exempt:
            print(f"NOTE: new connector {slug} is exempt from the press floor ({exempt[slug]})")
            continue
        manifest = _read_json(registry.skill_path(slug) / "manifest.json")
        version = check_engine_freshness._press_version(manifest) if manifest else None
        if meets_floor(version, floor):
            print(f"NOTE: new connector {slug} was printed with cli-printing-press {version} "
                  f"(floor {floor})")
            continue
        why = ("records no printing_press_version" if version is None
               else f"was printed with cli-printing-press {version}")
        findings.append(
            f"{rel}/manifest.json: new connector {slug} {why}, below the floor {floor} "
            f"({FLOOR_REL}). Reprint it with a current cli-printing-press and onboard it "
            "again (onboard.py records the version from .printing-press.json). A connector "
            "the press never generated needs a reviewed not_press_generated entry, landed "
            "in its own PR first.")
    return findings


# --------------------------------------------------------------------------- #
# Self-test: every rule fires on a broken folder and stays silent on a good one.
# --------------------------------------------------------------------------- #
def _publication_cases(t, build, clean, expect, base, spec, good_prov, manifest) -> None:
    """The spec publication rule, both ways: in CI (a published spec must carry
    its proof) and at vendoring time (--check-spec-source decides)."""
    import http.server
    import socket
    import threading

    no_pub = {k: v for k, v in base.items() if k != PUBLICATION}
    expect(not clean(build(t, no_pub)), "a spec without a publication record fails")
    def pub(**spec_rec):
        return {**base, PUBLICATION: json.dumps({"schema": 1, "spec": {
            "file": "spec.json", "basis": "public-url", "url": good_prov["spec_url"], "status": 200,
            "checked_on": "2026-10-22", **spec_rec}}).encode()}
    expect(not clean(build(t, pub(url="https://elsewhere.invalid/x.json"))),
           "a public-url record naming a URL the provenance does not record fails")
    expect(not clean(build(t, pub(status=401))), "a public-url record with a 401 fails")
    expect(not clean(build(t, pub(basis="trust-me"))), "an unknown basis fails")
    withheld = json.dumps({"schema": 1, "withheld": {"spec.json": NOT_PUBLIC + ": x"}}).encode()
    expect(clean(build(t, {PROVENANCE: base[PROVENANCE], TOOLS: base[TOOLS], PUBLICATION: withheld})),
           "a withheld spec, recorded with its reason, passes")
    expect(not clean(build(t, {**no_pub, PUBLICATION: withheld})),
           "a spec listed as withheld but present fails")
    odd = json.dumps({"schema": 1, "withheld": {"dogfood-results.json": "x"}}).encode()
    expect(not clean(build(t, {PROVENANCE: base[PROVENANCE], PUBLICATION: odd})),
           "withholding a file that is not press metadata fails")
    # repo-history, in a throwaway repository.
    repo = t / "histrepo"
    repo.mkdir()
    env = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1",
           "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
           "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid"}
    def g(*a):
        return subprocess.run(["git", "-C", str(repo), *a], capture_output=True, text=True, env=env)
    g("init", "-q", "-b", "main")
    (repo / "docs").mkdir()
    (repo / "docs" / "openapi.json").write_bytes(spec)
    (repo / "skills" / "x" / PRESS_DIR).mkdir(parents=True)
    (repo / "skills" / "x" / PRESS_DIR / "spec.yaml").write_bytes(b"only: under press\n")
    g("add", "-A")
    g("commit", "-qm", "authored spec")
    commit = g("rev-parse", "HEAD").stdout.strip()
    hist = {"schema": 1, "spec": {"file": "spec.json", "basis": "repo-history", "blob": git_blob_sha(spec),
                                  "commit": commit, "path": "docs/openapi.json"}}
    def clean_in(d: Path) -> bool:
        v, p = inspect_dir(d, manifest, repo=repo)
        return not p and all(x == "ok" for x in v.values())
    expect(clean_in(build(t, {**no_pub, PUBLICATION: json.dumps(hist).encode()})),
           "a repo-history record whose blob is in the repo outside press/ passes")
    other = {**hist, "spec": {**hist["spec"], "blob": git_blob_sha(b"another spec\n")}}
    expect(not clean_in(build(t, {**no_pub, PUBLICATION: json.dumps(other).encode()})),
           "a repo-history record for different bytes fails")
    only_press = b"only: under press\n"
    prov2 = {**good_prov, "spec_checksum": "sha256:" + hashlib.sha256(only_press).hexdigest()}
    man2 = {**manifest, "press_provenance": {**manifest["press_provenance"],
                                             "spec_checksum": prov2["spec_checksum"]}}
    d2 = build(t, {PROVENANCE: json.dumps(prov2).encode(), "spec.yaml": only_press,
                   PUBLICATION: json.dumps({"schema": 1, "spec": {
                       "file": "spec.yaml", "basis": "repo-history", "blob": git_blob_sha(only_press),
                       "commit": commit, "path": "skills/x/press/spec.yaml"}}).encode()})
    v2, p2 = inspect_dir(d2, man2, repo=repo)
    expect(v2.get("spec.yaml") != "ok", "a spec found only inside another press/ folder is not 'in history'")
    # --check-spec-source at vendoring time, against a local server.
    class H(http.server.BaseHTTPRequestHandler):
        def do_HEAD(self):
            self.send_response(200 if self.path == "/public.json" else 401)
            self.end_headers()
        do_GET = do_HEAD
        def log_message(self, *a):
            pass
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        closed_port = sock.getsockname()[1]
    spec2 = b'{"openapi": "3.0.1", "paths": {"/b": {}}}\n'  # not in the throwaway history
    sum2 = "sha256:" + hashlib.sha256(spec2).hexdigest()
    try:
        port = srv.server_address[1]
        for url, want in ((f"http://127.0.0.1:{port}/public.json", "kept"),
                          (f"http://127.0.0.1:{port}/private.json", NOT_PUBLIC),
                          (f"http://127.0.0.1:{closed_port}/x.json", "spec source unknown")):
            prov = {**good_prov, "spec_url": url, "spec_checksum": sum2}
            d = build(t, {PROVENANCE: json.dumps(prov).encode(), "spec.json": spec2})
            rec: dict = {}
            v, _ = inspect_dir(d, None, check_source=True, repo=repo, ref="HEAD", records=rec)
            got = "kept" if v.get("spec.json") == "ok" and rec.get("spec", {}).get("basis") == "public-url" \
                else v.get("spec.json", "")
            expect(got.startswith(want), f"--check-spec-source: {url.rsplit('/', 1)[1]} -> {want}")
        prov = {**{k: v for k, v in good_prov.items() if k != "spec_url"}, "spec_checksum": sum2}
        d = build(t, {PROVENANCE: json.dumps(prov).encode(), "spec.json": spec2})
        v, _ = inspect_dir(d, None, check_source=True, repo=repo, ref="HEAD", records={})
        expect(v.get("spec.json", "").startswith(NOT_PUBLIC),
               "--check-spec-source: no URL and not in history -> withheld, spec source not public")
        prov = {k: v for k, v in good_prov.items() if k != "spec_url"}
        d = build(t, {PROVENANCE: json.dumps(prov).encode(), "spec.json": spec})
        rec = {}
        v, _ = inspect_dir(d, manifest, check_source=True, repo=repo, ref="HEAD", records=rec)
        expect(v.get("spec.json") == "ok" and rec.get("spec", {}).get("basis") == "repo-history",
               "--check-spec-source: the spec's bytes are in the repo outside press/ -> kept (repo-history)")
    finally:
        srv.shutdown()
        srv.server_close()


def _self_test() -> int:
    failures = 0

    def expect(cond: bool, label: str) -> None:
        nonlocal failures
        print(f"  {'ok  ' if cond else 'FAIL'} {label}")
        failures += 0 if cond else 1

    spec = b'{"openapi": "3.0.1", "paths": {"/a": {}}}\n'
    spec_url = "https://example.invalid/openapi.json"
    good_prov = {"schema_version": 1, "printing_press_version": "4.33.5", "spec_url": spec_url,
                 "generated_at": "2026-10-14T12:00:00Z", "spec_checksum": "sha256:" + hashlib.sha256(spec).hexdigest()}
    public = {"schema": 1, "spec": {"file": "spec.json", "basis": "public-url", "url": spec_url,
                                    "status": 200, "checked_on": "2026-10-22"}}
    good_tools = {"api_name": "x", "tools": [{"name": "a_get", "method": "GET", "path": "/a"}]}
    manifest = {"printing_press_version": "4.33.5",
                "press_provenance": {"printing_press_version": "4.33.5",
                                     "generated_at": good_prov["generated_at"],
                                     "spec_checksum": good_prov["spec_checksum"]}}

    def build(t: Path, files: dict[str, bytes]) -> Path:
        d = t / f"case{len(list(t.iterdir()))}"
        d.mkdir()
        for name, data in files.items():
            (d / name).write_bytes(data)
        return d

    def clean(d: Path, m: dict | None = manifest) -> bool:
        v, p = inspect_dir(d, m)
        return not p and all(x == "ok" for x in v.values())

    base = {PROVENANCE: json.dumps(good_prov).encode(), TOOLS: json.dumps(good_tools).encode(), "spec.json": spec,
            PUBLICATION: json.dumps(public).encode()}
    with tempfile.TemporaryDirectory() as tmp:
        t = Path(tmp)
        expect(clean(build(t, base)), "a complete, consistent folder passes")
        expect(clean(build(t, {**base, "spec.json": spec.replace(b"\n", b"\r\n")})),
               "a CRLF checkout of the same spec passes (the press normalizes line endings)")
        expect(clean(build(t, {PROVENANCE: base[PROVENANCE]})), "provenance alone passes")
        _publication_cases(t, build, clean, expect, base, spec, good_prov, manifest)
        expect(not clean(build(t, {**base, "dogfood-results.json": b"{}"})),
               "an extra file (a live dogfood transcript) fails")
        expect(not clean(build(t, {**base, ".printing-press-pii-polish.json": b"{}"})),
               "the PII-polish ledger fails")
        expect(not clean(build(t, {TOOLS: base[TOOLS]})), "a folder without its provenance fails")
        expect(not clean(build(t, {})), "an empty folder fails (it would count as coverage)")
        stale = {**good_prov, "printing_press_version": "4.24.0"}
        expect(not clean(build(t, {**base, PROVENANCE: json.dumps(stale).encode()})),
               "a version that disagrees with manifest.json fails (a stale folder after a reprint)")
        other = {**good_prov, "generated_at": "2026-06-13T05:49:07Z"}
        expect(not clean(build(t, {**base, PROVENANCE: json.dumps(other).encode()})),
               "a generated_at that disagrees with press_provenance fails (another print, same version)")
        expect(not clean(build(t, base), {"printing_press_version": None}),
               "a manifest with no version cannot vouch for a folder")
        expect(not clean(build(t, {**base, "spec.json": spec + b" "})),
               "a spec whose sha256 is not spec_checksum fails")
        expect(not clean(build(t, {**base, "spec.yaml": b"openapi: 3.0.1\n"})), "two specs fail")
        expect(not clean(build(t, {**base, TOOLS: json.dumps({"tools": [{"name": "a", "method": "FETCH",
                                                                         "path": "/a"}]}).encode()})),
               "a tool with no real HTTP method fails")
        expect(not clean(build(t, {**base, TOOLS: b"{not json"})), "unparseable JSON fails")
        expect(not clean(build(t, {PROVENANCE: base[PROVENANCE], "spec.yaml": b"\xff\xfeopenapi"})),
               "a spec that is not UTF-8 fails")
        home = {**good_prov, "spec_path": "/Users/" + "someone/printing-press/.runstate/x/spec.yaml"}
        expect(not clean(build(t, {**base, PROVENANCE: json.dumps(home).encode()})),
               "a personal path in .printing-press.json fails (two library copies carry one)")
        for path_text, bad in (("/home/" + "alice-smith/private/spec.yaml", True),
                               ("C:" + "\\Users\\alice\\x", True),
                               ("D:" + "\\\\Users\\\\bob", True),
                               ("file:///" + "Users/someone/Downloads/x.wav", True),
                               ("/Use" + "rs/me", False), ("/Users/{id}/roles", False),
                               ("https://portal.example.com/h" + "ome/settings/x", False)):
            case = {**good_prov, "note": path_text}
            expect(clean(build(t, {**base, PROVENANCE: json.dumps(case).encode()})) is not bad,
                   f"{path_text!r} {'is' if bad else 'is not'} a personal path")
        key = b"-----BEGIN " + b"PRIVATE KEY-----\nMII\n"
        expect(not clean(build(t, {**base, "spec.json": spec, TOOLS: base[TOOLS] + b"\n" + key})),
               "a private key block fails")
        jwt = ("eyJ" + "hbGciOiJIUzI1NiJ9.eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijk").encode()
        expect(not clean(build(t, {**base, TOOLS: json.dumps({"tools": good_tools["tools"],
                                                              "example": jwt.decode()}).encode()})),
               "a JWT fails")
        link = build(t, base)
        (link / PROVENANCE).unlink()
        (link / PROVENANCE).symlink_to(t / "case0" / PROVENANCE)
        expect(not clean(link), "a symlinked file fails")
    for v, f, ok in (("4.33.5", "4.28.0", True), ("4.28.0", "4.28.0", True), ("4.27.9", "4.28.0", False),
                     ("4.28.0-rc1", "4.28.0", False), ("4.24.0", "4.28.0", False), (None, "4.28.0", False),
                     ("", "4.28.0", False), ("5.0.0", "4.28.0", True), ("4.100.0", "4.28.0", True)):
        expect(meets_floor(v, f) is ok, f"floor {f}: {v!r} {'meets' if ok else 'is below'} it")
    for bad in ('{"min_press_version": "4.28"}', '{"min_press_version": "4.28.0-rc1"}',
                '{"min_press_version": "4.28.0", "not_press_generated": {"x": ""}}', "[]"):
        try:
            load_floor(bad, "case")
            expect(False, f"a malformed floor file is refused: {bad}")
        except EnvError:
            expect(True, f"a malformed floor file is refused: {bad}")
    print("self-test:", "PASS" if not failures else f"FAIL ({failures})")
    return 0 if not failures else 1


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--base", help="enables the floor check for connectors added since this revision")
    ap.add_argument("--slug", help="check one connector's press/ folder")
    ap.add_argument("--inspect", metavar="DIR", help="per-file verdicts for DIR as JSON (onboard.py)")
    ap.add_argument("--manifest", metavar="FILE", help="with --inspect: the manifest.json it must agree with")
    ap.add_argument("--gitleaks", action="store_true", help="with --inspect: also run gitleaks, if installed")
    ap.add_argument("--check-spec-source", action="store_true",
                    help="with --inspect: keep a spec only when its source is provably public "
                         "(in the repo's history, or an http(s) spec URL answering without credentials)")
    ap.add_argument("--ref", default="origin/main", help="with --check-spec-source: the history to search")
    ap.add_argument("--self-test", action="store_true")
    args = ap.parse_args(argv)
    if args.self_test:
        return _self_test()
    try:
        if args.inspect:
            d = Path(args.inspect)
            manifest = _read_json(Path(args.manifest)) if args.manifest else None
            if args.manifest and manifest is None:
                raise EnvError(f"--manifest {args.manifest} is unreadable")
            records: dict = {}
            verdicts, problems = inspect_dir(d, manifest, check_source=args.check_spec_source,
                                             ref=args.ref, records=records)
            note = gitleaks_verdicts(d, verdicts) if args.gitleaks else ""
            spec = records.get("spec")
            if spec and verdicts.get(spec["file"]) != "ok":
                spec = None  # gitleaks withheld it after all
            print(json.dumps({"files": verdicts, "problems": problems, "note": note, "spec_source": spec},
                             indent=1, sort_keys=True))
            return 0
        if args.slug and not registry.SLUG_RE.match(args.slug):
            raise EnvError(f"refusing unsafe slug {args.slug!r}")
        findings, cov = check_fleet(args.slug)
        if args.base:
            findings += check_floor(args.base)
        else:
            print("NOTE: no --base given, so the press floor for new connectors was not checked")
    except EnvError as e:
        print(f"check_press_metadata: ENVIRONMENT ERROR - {e}")
        return 2
    missing = cov["missing"]
    withheld = "".join(f", {k} {v}" for k, v in sorted(cov.items()) if k.startswith("spec withheld"))
    print(f"NOTE: press metadata: {cov['press']} of {cov['connectors']} connectors carry a valid "
          f"skills/<dir>/{PRESS_DIR}/ (tools manifest {cov['tools']}, spec published {cov['spec']}"
          f"{withheld}); missing for {len(missing)}{': ' + ', '.join(missing) if missing else ''}")
    if findings:
        print("check_press_metadata FAILED:")
        for f in findings:
            print(f"  {f}")
        return 1
    print("check_press_metadata: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
