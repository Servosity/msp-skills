#!/usr/bin/env python3
"""replay_handfixes.py - retire or re-apply the fleet-wide hand-fixes after a reprint.

    python3 tools/maintainer/replay_handfixes.py --slug ninjaone                 # apply and prove
    python3 tools/maintainer/replay_handfixes.py --slug ninjaone --check         # report, change nothing
    python3 tools/maintainer/replay_handfixes.py --slug ninjaone --list-outputs  # paths an apply would write
    python3 tools/maintainer/replay_handfixes.py --self-test

Why this exists
---------------
Sixteen hand-fix ids each sit in 10 or more connector ledgers (678 of 1,485
entries on 2026-10-23), in the same shared template files, and every reprint
re-reconciled them by hand. tools/maintainer/fleet_handfixes.json settles each
one once against a press release:

  retire     the press generates the behavior from `retire_at` on. On a
             connector recorded at or above it, the stale ledger entry is
             removed with the registry's reason (kept as a `retired` record),
             check_handfixes.py demands the registry's native asserts instead,
             and this tool runs the id's gates.
  replay     the press still lacks it; this tool re-applies it with the named
             recipe: hand-written files the entry asserts on are restored from
             the pre-reprint revision (--base, default HEAD), with declarations
             the press now generates dropped; generated files get anchored
             edits.
  delegated  another tool owns it (secret-echo-decoded-funnel:
             apply_secret_funnel.py, which onboard.py runs after this one).

`keep_manual` names a connector whose entry under a shared id carries a part of
its own: the entry is left alone and the ledger check decides.

The registry's `floors` raise a module the press resolves below a patched
version (go get, then go mod tidy) on a connector printed at or above the
floor's `applies_from`, never below what --base already pinned. A floor the
press already meets is a no-op; set `retire_at` once a press resolves it.

Every step is idempotent and fails closed: an anchor that is missing or
ambiguous, a file the recipe cannot restore, or an assert the replay would lose
is named by site, and the run exits 1. Success is measured, never inferred
from the edits: an apply must pass go build, go vet, check_handfixes.py --slug
(the delegated ids excluded), go test of every package it wrote, the id gates
named in the registry, and, when a floor was raised, go test ./... and
check_security_gate.py --slug --require-scanners. --check stays cheap: planned
changes and the ledger check, no build.

Exit codes: 0 applied (or already applied) and proven. 1 a site needs a person,
a proof failed, or --check found work to do. 2 bad usage.
"""

from __future__ import annotations

import argparse
import difflib
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import check_handfixes as chf  # noqa: E402  the ledger's own semantics

ROOT = HERE.parent.parent
REGISTRY = HERE / "fleet_handfixes.json"
CANON = HERE / "handfix_replay"
GENERATED = "DO NOT EDIT"
SLUG = re.compile(r"[a-z0-9][a-z0-9-]*")
EMDASH = chr(0x2014)  # generated code carries it; committed files may not


def tag(hid: str) -> str:
    return f"Hand-fix (handfixes.json {hid})"


# --------------------------------------------------------------------------- #
# Small Go helpers (comment- and string-aware, no parser dependency).
# --------------------------------------------------------------------------- #

def code_only(rel: str, text: str) -> str:
    stripped, _ = chf.strip_comments(rel, text)
    return text if stripped is None else stripped


TOP_FUNC = re.compile(r"^func [^\n]*\{\n.*?^\}\n", re.S | re.M)


def func_spans(text: str, name: str) -> list[tuple[int, int]]:
    out = []
    for m in TOP_FUNC.finditer(text):
        if re.match(r"func (?:\([^)]*\) )?(?:%s)\(" % name, m.group(0)):
            out.append((m.start(), m.end()))
    return out


DECL_START = re.compile(r"^(func|var|const|type)\b")


def go_decls(text: str, rel: str = "x.go") -> list[dict]:
    """Top-level declarations: kind, name (None for a grouped block), and the
    [start, end) span including the doc comment directly above it."""
    masked = code_only(rel, text)
    masked = re.sub(r'"(?:\\.|[^"\\\n])*"|`[^`]*`|\'(?:\\.|[^\'\\\n])*\'',
                    lambda m: re.sub(r"[^\n]", "x", m.group(0)), masked)
    lines = text.splitlines(keepends=True)
    mlines = masked.splitlines(keepends=True)
    offs, pos = [], 0
    for ln in lines:
        offs.append(pos)
        pos += len(ln)
    out, i, depth = [], 0, 0
    while i < len(lines):
        if depth == 0 and DECL_START.match(lines[i]):
            start_line = i
            while start_line > 0 and lines[start_line - 1].startswith("//"):
                start_line -= 1
            head = lines[i]
            kind = DECL_START.match(head).group(1)
            m = re.match(r"func (?:\(\s*\w*\s*\*?(\w+)[^)]*\)\s*)?(\w+)", head) if kind == "func" else \
                re.match(r"(?:var|const|type)\s+(\w+)", head)
            if kind == "func":
                name = f"{m.group(1)}.{m.group(2)}" if m and m.group(1) else (m.group(2) if m else None)
            else:
                name = m.group(1) if m and m.group(1) != "(" else None
                if re.match(r"(?:var|const|type)\s*\(", head):
                    name = None
            d = 0
            j = i
            while True:
                d += mlines[j].count("{") + mlines[j].count("(") - mlines[j].count("}") - mlines[j].count(")")
                if d <= 0:
                    break
                j += 1
                if j >= len(lines):
                    break
            end = offs[j] + len(lines[j]) if j < len(lines) else len(text)
            out.append({"kind": kind, "name": name, "start": offs[start_line], "end": end})
            i = j + 1
            continue
        depth += mlines[i].count("{") - mlines[i].count("}") if depth or not DECL_START.match(lines[i]) else 0
        i += 1
    return out


IMPORT_BLOCK = re.compile(r"^import \(\n(.*?)^\)\n", re.S | re.M)


def prune_unused_imports(text: str, rel: str) -> str:
    m = IMPORT_BLOCK.search(text)
    if not m:
        return text
    rest = code_only(rel, text[:m.start()] + text[m.end():])
    keep = []
    for line in m.group(1).splitlines(keepends=True):
        spec = re.match(r'\s*(?:(\w+|_|\.)\s+)?"([^"]+)"', line)
        if not spec:
            keep.append(line)
            continue
        alias, path = spec.group(1), spec.group(2)
        if alias in ("_", "."):
            keep.append(line)
            continue
        name = alias or re.sub(r"^v\d+$", "", path.rsplit("/", 1)[-1]) or path.rsplit("/", 2)[-2]
        if re.search(r"\b%s\." % re.escape(name), rest):
            keep.append(line)
    body = "".join(keep)
    if not body.strip():
        return text[:m.start()] + text[m.end():]
    return text[:m.start()] + "import (\n" + body + ")\n" + text[m.end():]


def add_import(text: str, path: str) -> str:
    if f'"{path}"' in text:
        return text
    m = IMPORT_BLOCK.search(text)
    if not m:
        return text
    return text[:m.end(1)] + f'\t"{path}"\n' + text[m.end(1):]


def package_decl_names(cli: Path, pkg_dir: str, skip: str, texts: dict) -> dict[str, str]:
    """name -> file (skill-relative) for every top-level declaration in the
    package's other non-test files, as planned."""
    names: dict[str, str] = {}
    d = cli / pkg_dir
    for f in sorted(d.glob("*.go")) if d.is_dir() else []:
        rel = f"cli/{pkg_dir}/{f.name}"
        if f.name.endswith("_test.go") or rel == skip:
            continue
        t = texts.get(rel, f.read_text(encoding="utf-8", errors="replace"))
        for decl in go_decls(t, rel):
            if decl["name"]:
                names.setdefault(decl["name"], rel)
    return names


def semver(v: str) -> tuple:
    """A sort key with SemVer 2.0 precedence (Go module versions follow it): a
    pre-release sorts below its release, and pre-release identifiers compare
    one by one, numeric ones as numbers and below alphanumeric ones."""
    m = re.match(r"v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$", v or "")
    if not m:
        return (0, 0, 0, 0, ())
    pre = m.group(4)
    ids = tuple((0, int(x), "") if x.isdigit() else (1, 0, x) for x in pre.split(".")) if pre else ()
    return (int(m.group(1)), int(m.group(2)), int(m.group(3)), 0 if pre else 1, ids)


# --------------------------------------------------------------------------- #
# The plan for one connector.
# --------------------------------------------------------------------------- #

class Plan:
    def __init__(self, root: Path, slug: str, base: str, registry: dict):
        self.root, self.slug, self.base, self.registry = root, slug, base, registry
        self.skill = root / "skills" / slug
        self.cli = self.skill / "cli"
        self.texts: dict[str, str] = {}
        self.orig: dict[str, str] = {}
        self.made: list[str] = []
        self.notes: list[str] = []
        self.needs_hand: list[tuple[str, str]] = []
        self.gates: list[tuple[str, dict]] = []
        self.tests: dict[str, set[str]] = {}
        self.floor_raises: list[tuple[str, str | None, str]] = []
        self.unsafe: set[str] = set()
        self.module = ""
        m = re.search(r"^module (\S+)", self.read("cli/go.mod") or "", re.M)
        if m:
            self.module = m.group(1)
        self.version, self.version_why = chf.recorded_press_version(self.skill)
        self._base_cache: dict[str, str | None] = {}
        self.relocated: dict[str, str] = {}
        self.retired_files: set[str] = set()
        self.pins: dict[str, list[dict]] = {}
        self.keep_as: dict[str, str] = {}
        self.relocated_files: dict[str, str] = {}
        self.deleted: set[str] = set()

    # files (skill-relative paths)
    def safe(self, rel: str) -> bool:
        """`rel` names a file inside this connector: relative, no `.` or `..`
        part, no symlink on the way. Anything else is never read or written."""
        parts = rel.split("/") if rel else []
        if not parts or rel.startswith("/") or "\\" in rel or any(x in ("", ".", "..") for x in parts):
            ok = False
        else:
            q, ok = self.skill, True
            for x in parts:
                q = q / x
                if q.is_symlink():
                    ok = False
                    break
        if not ok and rel not in self.unsafe:
            self.unsafe.add(rel)
            self.hand("path", f"{rel!r}: not a plain path inside skills/{self.slug}/, so it is never read or written")
        return ok

    def read(self, rel: str) -> str | None:
        if rel in self.texts:
            return self.texts[rel]
        if not self.safe(rel):
            return None
        p = self.skill / rel
        if not p.is_file():
            return None
        t = p.read_text(encoding="utf-8", errors="replace")
        self.orig.setdefault(rel, t)
        self.texts[rel] = t
        return t

    def exists(self, rel: str) -> bool:
        return self.read(rel) is not None

    def write(self, rel: str, text: str, label: str) -> None:
        if not self.safe(rel):
            return
        if rel not in self.orig:
            self.orig[rel] = (self.skill / rel).read_text(encoding="utf-8") if (self.skill / rel).is_file() else ""
        if self.texts.get(rel) != text:
            self.texts[rel] = text
            self.made.append(f"{rel}: {label}")

    def delete(self, rel: str, label: str) -> None:
        if self.safe(rel) and (self.skill / rel).is_file() and rel not in self.deleted:
            self.deleted.add(rel)
            self.made.append(f"{rel}: removed ({label})")

    def at_base(self, rel: str) -> str | None:
        if not self.safe(rel):
            return None
        if rel not in self._base_cache:
            try:
                p = subprocess.run(["git", "-C", str(self.root), "show", f"{self.base}:skills/{self.slug}/{rel}"],
                                   capture_output=True, text=True)
                self._base_cache[rel] = p.stdout if p.returncode == 0 else None
            except OSError:
                self._base_cache[rel] = None
        return self._base_cache[rel]

    def pin(self, hid: str, rel: str, needle: str, absent: bool = False) -> None:
        """Record a ledger assert for an edit this run made, so it is protected
        on the next reprint like the rest of the entry."""
        a = {"file": rel, "not_contains": needle} if absent else \
            {"file": rel, "contains": needle, "min_count": 1, "code_only": True}
        self.pins.setdefault(hid, []).append(a)

    def hand(self, site: str, msg: str) -> None:
        self.needs_hand.append((site, msg))

    def present(self, rel: str, marker: str, scope: str | None = None) -> bool:
        s = self.read(rel) or ""
        spans = func_spans(s, scope) if scope else [(0, len(s))]
        return any(marker in code_only(rel, s[a:b]) for a, b in spans)

    def edit(self, site: str, rel: str, label: str, before: str, after: str, marker: str,
             scope: str | None = None, required: bool = True) -> bool:
        """Replace `before` with `after` once (inside `scope` when given). True
        when the edit is now present; a missing or ambiguous anchor is named."""
        if self.present(rel, marker, scope):
            return True
        s = self.read(rel)
        if s is None:
            if required:
                self.hand(site, f"{rel}: missing, so {label} cannot be wired")
            return False
        spans = func_spans(s, scope) if scope else [(0, len(s))]
        hits = [(a, b) for a, b in spans for _ in range(s[a:b].count(before))]
        if len(hits) == 1:
            a, b = hits[0]
            new = s[:a] + s[a:b].replace(before, after, 1) + s[b:]
            if marker not in code_only(rel, new):
                self.hand(site, f"{rel}: {label}: the anchor sits in a comment, so the edit would not wire it")
                return False
            self.write(rel, new, label)
            return True
        where = f" in {scope}" if scope else ""
        if len(hits) > 1:
            self.hand(site, f"{rel}: {label}: its anchor appears {len(hits)} times{where}, so the site is ambiguous")
        elif required:
            self.hand(site, f"{rel}: {label}: anchor not found{where}")
        return False

    def add_tests_from(self, rel: str) -> None:
        if not rel.endswith("_test.go") or not rel.startswith("cli/"):
            return
        text = self.read(rel) or ""
        names = re.findall(r"^func (Test\w+)\(t \*testing\.T\)", text, re.M)
        if names:
            self.tests.setdefault(os.path.dirname(rel[len("cli/"):]), set()).update(names)

    def restore(self, site: str, rel: str, dedupe: bool = False) -> bool:
        """Bring back a hand-written file the press does not generate, from the
        pre-reprint revision. A file that was generated there is not restored:
        the press dropped or renamed it, and a person must decide."""
        if self.exists(rel):
            self.add_tests_from(rel)
            return True
        old = self.at_base(rel)
        if old is None:
            self.hand(site, f"{rel}: missing here and at {self.base}, so there is nothing to restore")
            return False
        if GENERATED in old[:600]:
            self.hand(site, f"{rel}: a generated file at {self.base} that this print no longer emits; "
                            f"the press renamed or dropped it, so wire the fix by hand")
            return False
        text = old
        if dedupe and rel.endswith(".go") and not rel.endswith("_test.go"):
            text = self.dedupe(rel, text)
        self.write(rel, text, f"restored from {self.base}")
        self.add_tests_from(rel)
        self.restore_dependencies(site, rel, text)
        return True

    def restore_dependencies(self, site: str, rel: str, text: str) -> None:
        """A restored file may call an exported symbol of another internal
        package that only a hand-written file declared before the reprint (the
        shared secret funnel's cliutil.FunnelWithhold, say). Restore that file
        too, one level, so the restored code builds; anything deeper is left to
        the build proof, which names it."""
        if not rel.endswith(".go") or not self.module:
            return
        code = code_only(rel, text)
        imports = dict(re.findall(r'(?m)^\s*(?:(\w+)\s+)?"%s/internal/([\w/]+)"' % re.escape(self.module), code))
        aliases = {}
        for alias, pkg in re.findall(r'(?m)^\s*(?:(\w+)\s+)?"%s/internal/([\w/]+)"' % re.escape(self.module), code):
            aliases[alias or pkg.rsplit("/", 1)[-1]] = pkg
        for name, pkg in aliases.items():
            used = set(re.findall(r"\b%s\.([A-Z]\w*)\b" % re.escape(name), code))
            if not used:
                continue
            have = package_decl_names(self.cli, f"internal/{pkg}", "", self.texts)
            missing = {u for u in used if u not in have}
            if not missing:
                continue
            listing = subprocess.run(["git", "-C", str(self.root), "ls-tree", "--name-only", self.base,
                                      f"skills/{self.slug}/cli/internal/{pkg}/"], capture_output=True, text=True)
            for path in listing.stdout.split():
                dep = path[len(f"skills/{self.slug}/"):]
                if not dep.endswith(".go") or dep.endswith("_test.go") or self.exists(dep):
                    continue
                old = self.at_base(dep) or ""
                if GENERATED in old[:600]:
                    continue
                declared = {d["name"] for d in go_decls(old, dep) if d["name"]}
                if declared & missing:
                    self.write(dep, old, f"restored from {self.base} (declares {', '.join(sorted(declared & missing))} "
                                         f"for {rel})")
                    missing -= declared

    def dedupe(self, rel: str, text: str) -> str:
        pkg = os.path.dirname(rel[len("cli/"):])
        have = package_decl_names(self.cli, pkg, rel, self.texts)
        dropped = []
        for decl in sorted(go_decls(text, rel), key=lambda d: -d["start"]):
            if decl["name"] and decl["name"] in have:
                if decl["name"] in self.keep_as:
                    # the press's copy is weaker: keep this one under its own name
                    new = self.keep_as[decl["name"]]
                    body = text[decl["start"]:decl["end"]]
                    body = re.sub(r"\b%s\b" % re.escape(decl["name"]), new, body)
                    text = text[:decl["start"]] + body + text[decl["end"]:]
                    self.notes.append(f"{rel}: kept the hand-fix {decl['name']} as {new}: the press's "
                                      f"{decl['name']} is weaker")
                    self.relocated[decl["name"]] = have[decl["name"]]
                    continue
                text = text[:decl["start"]] + text[decl["end"]:]
                dropped.append((decl["name"], have[decl["name"]]))
        if dropped:
            text = re.sub(r"\n{3,}", "\n\n", prune_unused_imports(text, rel))
            for name, home in dropped:
                self.relocated[name] = home
            self.notes.append(f"{rel}: dropped {', '.join(n for n, _ in reversed(dropped))}: the press now "
                              f"generates them")
        return text


def render(template: str, **subs: str) -> str:
    text = (CANON / template).read_text(encoding="utf-8").replace("{{EMDASH}}", EMDASH)
    for k, v in subs.items():
        text = text.replace(k, v)
    return text


def failing_asserts(p: Plan, asserts: list[dict]) -> list[dict]:
    """The asserts that do NOT hold on the planned tree (check_handfixes semantics)."""
    bad = []
    for a in asserts:
        text = p.read(a.get("file", ""))
        if text is None:
            bad.append(a)
            continue
        if "contains" in a:
            hay = code_only(a["file"], text) if a.get("code_only") is True else text
            if hay.count(a["contains"]) < int(a.get("min_count", 1)):
                bad.append(a)
                continue
        if "not_contains" in a and a["not_contains"] in text:
            bad.append(a)
    return bad


# --------------------------------------------------------------------------- #
# Recipes. Each re-applies one id on a press 4.34.0 tree and returns the
# needles (substrings) of entry asserts it makes obsolete, with why.
# --------------------------------------------------------------------------- #

def r_derive_lock(p: Plan, hid: str, entry: dict) -> dict:
    rel = "cli/internal/learn/derive_test.go"
    cur = p.read(rel)
    if cur is None:
        p.hand("derive-test", f"{rel}: missing, so the derive-lock tests cannot be made deterministic")
        return {}
    want = render("derive_test.go.tmpl", MODULE=p.module)
    if cur != want:
        digest = hashlib.sha256(cur.replace(p.module, "MODULE").encode()).hexdigest()
        if digest not in DERIVE_GENERATED:
            p.hand("derive-test", f"{rel}: differs from the generated shape this recipe knows (sha256 {digest[:12]}); "
                                  f"merge tools/maintainer/handfix_replay/derive_test.go.tmpl by hand")
            return {}
        p.write(rel, want, "derive-lock tests wait on state, not fixed windows")
    p.add_tests_from(rel)
    return {}


# sha256 of the module-normalized generated derive_test.go (press 4.33.5 = 4.34.0).
DERIVE_GENERATED = {"4dca3f4bee7f728ef5f995cb7a3dba6a57d00b0e651fe66b6b2da94efe43bf7e"}

OAUTH_FOLD = "\tif !strings.EqualFold(target.Hostname(), origin.Hostname()) {\n"
OAUTH_HOST = ("\t// Hand-fix (handfixes.json token-request-stays-on-origin): hosts compare\n"
              "\t// the way the network resolves them, not with Unicode case folding.\n"
              "\tif !oauthTokenSameHost(target.Hostname(), origin.Hostname()) {\n")
OAUTH_HELPER = """
// oauthTokenSameHost compares two URL hostnames the way the network resolves
// them. An IP literal must be the same address, IPv6 zone included (Linux
// interface names are case-sensitive). A DNS name folds ASCII case only;
// strings.EqualFold also folds Unicode (the Kelvin sign equals k), which no
// resolver does. Hand-fix (handfixes.json token-request-stays-on-origin).
func oauthTokenSameHost(a, b string) bool {
	ipA, errA := netip.ParseAddr(a)
	ipB, errB := netip.ParseAddr(b)
	if errA == nil || errB == nil {
		return errA == nil && errB == nil && ipA == ipB
	}
	for _, s := range []string{a, b} {
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				return a == b
			}
		}
	}
	return strings.EqualFold(a, b)
}
"""


def r_token_host(p: Plan, hid: str, entry: dict) -> dict:
    rel = "cli/internal/cliutil/oauth_token.go"
    if not p.edit("token-host", rel, "token-origin hosts compare like the network", OAUTH_FOLD, OAUTH_HOST,
                  "if !oauthTokenSameHost(target.Hostname(), origin.Hostname()) {", "oauthTokenSameOrigin"):
        return {}
    s = p.read(rel)
    if not func_spans(s, "oauthTokenSameHost"):
        span = func_spans(s, "oauthTokenSameOrigin")
        if len(span) != 1:
            p.hand("token-host", f"{rel}: oauthTokenSameOrigin is not one top-level function")
            return {}
        s = s[:span[0][1]] + OAUTH_HELPER + s[span[0][1]:]
        p.write(rel, add_import(s, "net/netip"), "oauthTokenSameHost helper")
    p.pin(hid, rel, "if !oauthTokenSameHost(target.Hostname(), origin.Hostname()) {")
    p.pin(hid, rel, "func oauthTokenSameHost(a, b string) bool {")
    restore_entry_files(p, hid, entry)
    return {}


DRY_LOGIN = ("\t\t\ttok, err := mintClientCredentialsToken(tokenHTTP, tokenURL, clientID, clientSecret)\n",
             "(dry run - no request sent, nothing saved)",
             "\t\t\t// {tag}: a preview never sends the secret or writes credentials.\n"
             "\t\t\tif flags.dryRun {{\n"
             "\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), \"POST %s\\n(dry run - no request sent, nothing saved)\\n\", tokenURL)\n"
             "\t\t\t\treturn nil\n\t\t\t}}\n", "newAuthLoginCmd")
DRY_SET = ("\t\t\tif err := cfg.SaveTokens(\"\", \"\", token, \"\", cfg.TokenExpiry); err != nil {\n",
           "(dry run - nothing saved)",
           "\t\t\t// {tag}: a preview saves nothing.\n"
           "\t\t\tif flags.dryRun {{\n"
           "\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), \"would save the token to %s\\n(dry run - nothing saved)\\n\", cfg.Path)\n"
           "\t\t\t\treturn nil\n\t\t\t}}\n", "newAuthSetTokenCmd")
DRY_LOGOUT = ("\t\t\tif err := cfg.ClearTokens(); err != nil {\n",
              "(dry run - nothing changed)",
              "\t\t\t// {tag}: a preview clears nothing.\n"
              "\t\t\tif flags.dryRun {{\n"
              "\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), \"would clear the stored credentials (%s)\\n(dry run - nothing changed)\\n\", cfg.Path)\n"
              "\t\t\t\treturn nil\n\t\t\t}}\n", "newAuthLogoutCmd")
SET_TOKEN_POSITIONAL = re.compile(r'run\("", "auth", "set-token", "([^"\\]+)"((?:, "[^"]*")*)\)')


def r_auth_dry_run(p: Plan, hid: str, entry: dict) -> dict:
    rel = "cli/internal/cli/auth.go"
    # Every credential-writing command the print has honours --dry-run: one
    # behavior class, proven by the restored tests. A command whose message
    # the ledger pins is required; the others are wired where the print has them.
    wanted = {a.get("contains") for a in entry.get("asserts", []) if a.get("file") == rel}
    gen = p.read(rel) or ""
    for anchor, marker, block, scope in (DRY_LOGIN, DRY_SET, DRY_LOGOUT):
        if marker not in wanted and anchor not in func_text(gen, scope):
            continue
        if p.edit(f"dry-run:{scope}", rel, f"{scope} honours --dry-run", anchor,
                  block.format(tag=tag(hid)) + anchor, marker, scope):
            p.pin(hid, rel, marker)
    restore_entry_files(p, hid, entry)
    # Press 4.34.0 reads `auth set-token`'s token from stdin (cobra.NoArgs); a
    # restored test written for the positional form is moved to stdin.
    gen = p.read(rel) or ""
    if "readSecretFromStdin(cmd.InOrStdin())" in func_text(gen, "newAuthSetTokenCmd"):
        for t in [a["file"] for a in entry.get("asserts", []) if a.get("file", "").endswith("_test.go")]:
            s = p.read(t)
            if s and SET_TOKEN_POSITIONAL.search(s):
                p.write(t, SET_TOKEN_POSITIONAL.sub(r'run("\1\\n", "auth", "set-token"\2)', s),
                        "set-token test reads the token from stdin (press 4.34.0 shape)")
    return {"if !c.DryRun && needsClientCredentialsMint(c.Config) {":
            "press 4.34.0 skips the automatic mint under --dry-run itself; the restored TestDryRunNeverMintsAToken proves it"}


def func_text(s: str, name: str) -> str:
    spans = func_spans(s, name)
    return s[spans[0][0]:spans[0][1]] if spans else ""


DOCTOR_SUGGEST = re.compile(
    r'(?m)^(\t+)suggestion := suggestReadCommand\(cmd\.Root\(\)\)\n'
    r'\1if suggestion != "" \{\n'
    r'\1\treport\["credentials"\] = fmt\.Sprintf\("present, not verified\. Run `%s %s` to confirm the token works end-to-end\.", "([^"]+)", suggestion\)\n'
    r'\1\} else \{\n'
    r'\1\treport\["credentials"\] = "present, not verified\. Run any read command to confirm the token works end-to-end\."\n'
    r'\1\}\n')
DOCTOR_HELPER_SIGS = (
    "func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {",
    "func doctorUnexpectedStatus(status int, path string, report map[string]any) {",
    "const doctorBaseURLEnv = ",
)
DOCTOR_NOAUTH = f"// No auth configured {EMDASH} skip credential validation\n"
DOCTOR_VERIFY_WARN = ('report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) '
                      f'{EMDASH} probe reached a non-auth endpoint; set auth.verify_path to a known-good '
                      'authenticated GET", authAPIErr.StatusCode, verifyPath)\n')
DOCTOR_PLACEHOLDER = 'report["api"] = "not configured (base_url is a placeholder)"\n'


def r_doctor(p: Plan, hid: str, entry: dict) -> dict:
    rel = "cli/internal/cli/doctor.go"
    p.keep_as["doctorBaseURLIsPlaceholder"] = "doctorBaseURLIsShippedPlaceholder"
    if not restore_entry_files(p, hid, entry, dedupe=True):
        return {}
    helper = code_only("x.go", p.read("cli/internal/cli/doctor_verdict.go") or "")
    missing = [sig for sig in DOCTOR_HELPER_SIGS if sig not in helper]
    if missing:
        p.hand("doctor-variant", "cli/internal/cli/doctor_verdict.go is this connector's own variant (lacks "
                                 + "; ".join(m.split("(")[0] for m in missing) + "), so doctor.go is wired by hand")
        return {}
    s = p.read(rel)
    if s is None:
        p.hand("doctor", f"{rel}: missing")
        return {}
    if "doctorProbeCredentials(cmd.Context()" not in code_only(rel, s):
        hits = list(DOCTOR_SUGGEST.finditer(s))
        if len(hits) == 1:
            m = hits[0]
            ind = m.group(1)
            s = (s[:m.start()] + f"{ind}// {tag(hid)}: probe the credential instead of guessing.\n"
                 f'{ind}doctorProbeCredentials(cmd.Context(), c, cmd.Root(), "{m.group(2)}", report)\n' + s[m.end():])
            p.write(rel, s, "doctor probes the credential")
            p.pin(hid, rel, "doctorProbeCredentials(cmd.Context(), c, cmd.Root(),")
        elif len(hits) > 1:
            p.hand("doctor-probe", f"{rel}: the generated credential suggestion appears {len(hits)} times")
        else:
            # The print probes the credential itself (auth.verify_path); its
            # non-auth branch still calls a 404 a credential WARN. Route that
            # status through doctorUnexpectedStatus so a 404 names a wrong base URL.
            n = s.count(DOCTOR_VERIFY_WARN)
            if n == 1:
                ind = re.search(r"(?m)^(\t+)" + re.escape(DOCTOR_VERIFY_WARN), s).group(1)
                s = s.replace(ind + DOCTOR_VERIFY_WARN,
                              f"{ind}// {tag(hid)}: a 404 here means base_url is not the API root.\n"
                              f"{ind}doctorUnexpectedStatus(authAPIErr.StatusCode, verifyPath, report)\n", 1)
                p.write(rel, s, "doctor reads an unexpected probe status")
                p.pin(hid, rel, "doctorUnexpectedStatus(authAPIErr.StatusCode, verifyPath, report)")
            elif "doctorUnexpectedStatus(authAPIErr.StatusCode" not in code_only(rel, s):
                p.hand("doctor-probe", f"{rel}: neither generated credential shape this recipe knows is here "
                                       f"(suggestion block: 0, verify_path WARN line: {n})")
    s = p.read(rel)
    line = next((ln for ln in s.splitlines(keepends=True) if ln.strip() == DOCTOR_NOAUTH.strip()), None)
    marker = 'report["credentials"] = "ERROR not verified: no credential is configured, so nothing was probed."'
    if line and marker not in s:
        ind = line[:len(line) - len(line.lstrip("\t"))]
        p.edit("doctor-noauth", rel, "doctor names an absent credential", line,
               line + f"{ind}{marker}\n", marker)
    if line or marker in (p.read(rel) or ""):
        p.pin(hid, rel, marker)
    s = p.read(rel)
    named = 'report["api"] = "not configured (base_url is a placeholder); set " + doctorBaseURLEnv + " to your own instance URL"'
    if DOCTOR_PLACEHOLDER.strip() in s and named not in s:
        p.edit("doctor-placeholder", rel, "doctor names the base-URL variable",
               DOCTOR_PLACEHOLDER, named + "\n", named)
    if named in (p.read(rel) or ""):
        p.pin(hid, rel, named)
    shipped = "doctorBaseURLIsShippedPlaceholder("
    if shipped in code_only("x.go", p.read("cli/internal/cli/doctor_verdict.go") or ""):
        p.edit("doctor-shipped", rel, "doctor also refuses the shipped placeholder base_url",
               "if cfg != nil && doctorBaseURLIsPlaceholder(cfg.BaseURL) {",
               "if cfg != nil && (doctorBaseURLIsPlaceholder(cfg.BaseURL) || doctorBaseURLIsShippedPlaceholder(cfg.BaseURL)) {",
               "doctorBaseURLIsShippedPlaceholder(cfg.BaseURL)")
        p.pin(hid, rel, "doctorBaseURLIsShippedPlaceholder(cfg.BaseURL)")
        p.pin(hid, "cli/internal/cli/doctor_verdict.go", "func doctorBaseURLIsShippedPlaceholder(base string) bool {")
    return {}


ALIAS_LINE = re.compile(r'(?m)^\t+Aliases:\s+\[\]string\{"create"\},\n')


USE_SHORT = re.compile(r'(?m)^\t+Use:\s+(".*?"),\n(?:.*\n){0,3}?\t+Short:\s+(".*?"),\n')


def find_command_file(p: Plan, rel: str) -> str | None:
    """The file in this print that defines the command `rel` defined before
    the reprint, matched by its Use and Short lines (the press renames files)."""
    old = p.at_base(rel) or ""
    m = USE_SHORT.search(old)
    if not m:
        return None
    hits = []
    for f in sorted((p.cli / "internal" / "cli").glob("*.go")):
        r = f"cli/internal/cli/{f.name}"
        s = p.read(r) or ""
        for n in USE_SHORT.finditer(s):
            if n.group(1) == m.group(1) and n.group(2) == m.group(2):
                hits.append(r)
    return hits[0] if len(hits) == 1 else None


def command_path(p: Plan, rel: str) -> list[str] | None:
    """The Cobra command path of the command a generated file defines: its
    own Use word, then each parent's, following AddCommand(newXCmd(...)) up to
    the root command. None when the chain is not plain."""
    s = p.read(rel) or ""
    m = re.search(r"^func (new\w+Cmd)\(", s, re.M)
    if not m:
        return None
    name, words = m.group(1), []
    files = sorted((p.cli / "internal" / "cli").glob("*.go"))
    for _ in range(12):
        body = None
        for f in files:
            text = p.read(f"cli/internal/cli/{f.name}") or ""
            span = func_spans(text, re.escape(name))
            if span:
                body = text[span[0][0]:span[0][1]]
                break
        use = re.search(r'Use:\s+"([^" ]+)', body or "")
        if not use:
            return None
        words.insert(0, use.group(1))
        parents = []
        for f in files:
            text = p.read(f"cli/internal/cli/{f.name}") or ""
            for hit in re.finditer(r"AddCommand\(%s\(" % re.escape(name), text):
                enclosing = [mm for mm in TOP_FUNC.finditer(text) if mm.start() <= hit.start() < mm.end()]
                if enclosing:
                    fn = re.match(r"func (?:\([^)]*\) )?(\w+)", enclosing[0].group(0))
                    parents.append(fn.group(1) if fn else None)
        if len(parents) != 1 or parents[0] is None:
            return None
        if parents[0] in ("newRootCmd", "RootCmd", "Execute"):
            return words
        name = parents[0]
    return None


def r_create_alias(p: Plan, hid: str, entry: dict) -> dict:
    files = [a["file"] for a in entry.get("asserts", []) if a.get("not_contains") == '[]string{"create"}']
    for rel in files:
        s = p.read(rel)
        if s is None:
            moved = find_command_file(p, rel)
            if moved is None:
                p.hand(f"create-alias:{rel}", f"{rel}: missing, and no file in this print defines the same command "
                                              f"(Use and Short); find it and remove its create alias by hand")
                continue
            p.relocated_files[rel] = moved
            p.notes.append(f"[{hid}] {rel} is {moved} in this print")
            rel, s = moved, p.read(moved)
        hits = ALIAS_LINE.findall(s)
        if len(hits) == 1:
            p.write(rel, ALIAS_LINE.sub("", s, count=1), "destructive command loses the create alias")
        elif '[]string{"create"}' in s:
            p.hand(f"create-alias:{rel}", f"{rel}: the create alias is not on one plain Aliases line")
        p.pin(hid, rel, '[]string{"create"}', absent=True)
    restore_entry_files(p, hid, entry, skip=set(files))
    # A generated command file is named after its command path
    # (alert_item_reset_x.go is `alert item reset x`), so a relocated command's
    # path in the restored test moves with it; running the test proves the move.
    for old, new in p.relocated_files.items():
        path = command_path(p, new)
        if path is None:
            continue  # the restored test then fails, and the proof names it
        before = ", ".join(f'"{w}"' for w in Path(old).stem.split("_"))
        after = ", ".join(f'"{w}"' for w in path)
        for rel in sorted({a["file"] for a in entry.get("asserts", []) if a.get("file", "").endswith("_test.go")}):
            s = p.read(rel) or ""
            if "{" + before + "}" in s:
                p.write(rel, s.replace("{" + before + "}", "{" + after + "}"),
                        f"test follows {Path(old).stem} to {Path(new).stem}")
    return {}


NCW_START = "// NoConfigWriteEnv is the environment switch"
NCW_LOGIN_REFUSE = ("\t\t\t// {tag}: a command whose whole purpose is to write a credential\n"
                    "\t\t\t// must not silently do nothing, so it refuses and names the variable.\n"
                    "\t\t\tif config.NoConfigWrite() {{\n"
                    "\t\t\t\treturn configErr(fmt.Errorf(\"%s is set, so no credential may be written to disk; "
                    "unset it and re-run to save one\", config.NoConfigWriteEnv))\n\t\t\t}}\n")
RUNE = "\t\tRunE: func(cmd *cobra.Command, args []string) error {\n"


def r_no_config_write(p: Plan, hid: str, entry: dict) -> dict:
    cfg = "cli/internal/config/config.go"
    s = p.read(cfg)
    if s is None:
        p.hand("ncw-config", f"{cfg}: missing")
        return {}
    if "func NoConfigWrite() bool {" not in code_only(cfg, s):
        old = p.at_base(cfg) or ""
        i = old.find(NCW_START)
        span = func_spans(old, r"saveCredentialMaterial")
        if i < 0 or len(span) != 1 or span[0][0] < i:
            p.hand("ncw-config", f"{cfg}: the switch is not at {p.base} in the shape this recipe carries "
                                 f"(NoConfigWriteEnv through saveCredentialMaterial)")
            return {}
        block = old[i:span[0][1]].replace("\treturn c.save()\n}", "\treturn c.saveCredentialsThenConfig()\n}")
        if "saveCredentialsThenConfig()" not in block:
            p.hand("ncw-config", f"{cfg}: saveCredentialMaterial at {p.base} does not end in the save this print uses")
            return {}
        anchor = "func (c *Config) SaveTokens("
        if s.count(anchor) != 1:
            p.hand("ncw-config", f"{cfg}: '{anchor}' appears {s.count(anchor)} times")
            return {}
        s = s.replace(anchor, block + "\n" + anchor, 1)
        p.write(cfg, s, "the no-config-write switch")
    if p.edit("ncw-save", cfg, "token saves go through the switch", "\treturn c.saveCredentialsThenConfig()\n}\n",
              "\treturn c.saveCredentialMaterial()\n}\n", "return c.saveCredentialMaterial()", "SaveTokens"):
        p.pin(hid, cfg, "return c.saveCredentialMaterial()")
        p.pin(hid, cfg, "func NoConfigWrite() bool {")
    auth = "cli/internal/cli/auth.go"
    if p.exists(auth):
        for scope in ("newAuthLoginCmd", "newAuthSetTokenCmd"):
            if not func_spans(p.read(auth), scope):
                continue
            p.edit(f"ncw-refuse:{scope}", auth, f"{scope} refuses under the switch", RUNE,
                   RUNE + NCW_LOGIN_REFUSE.format(tag=tag(hid)), "if config.NoConfigWrite() {", scope)
        p.pin(hid, auth, "if config.NoConfigWrite() {")
    # Press 4.34.0 resolves the MCP server's config through config.Load(""),
    # the CLI's own resolver (newMCPConfig), so the hand-fix's MCP half is
    # generated; the entry's tools.go assert keeps pinning it. Its test was
    # written against the old newMCPClient() and this connector's old config
    # format, so it is not restored.
    tools = p.read("cli/internal/mcp/tools.go") or ""
    native_mcp = "func newMCPConfig() (*config.Config, error) {\n\tcfg, err := config.Load(\"\")" in tools
    skip = {"cli/internal/mcp/mcp_config_resolution_test.go"} if native_mcp else set()
    restore_entry_files(p, hid, entry, skip=skip)
    return {"func TestMCPClientHonoursTheConfigEnvVar(":
            "press 4.34.0 resolves the MCP config through config.Load(\"\") itself (newMCPConfig); "
            "the tools.go assert pins it"} if native_mcp else {}


def r_manifest_env(p: Plan, hid: str, entry: dict) -> dict:
    return {}  # the union ran before the entries (plan_connector)


def manifest_env_union(p: Plan) -> None:
    """Every env key (and its user_config prompt) the pre-reprint manifest
    declared comes back: the press manifest under-declares, and mcp-install.md
    and server.json are rendered from this list. Never removes one."""
    rel = "manifest.json"
    cur_raw, old_raw = p.read(rel), p.at_base(rel)
    if cur_raw is None or old_raw is None:
        return
    try:
        cur, old = json.loads(cur_raw), json.loads(old_raw)
    except json.JSONDecodeError:
        p.hand("manifest-env", f"{rel}: not JSON here or at {p.base}")
        return
    ind, ea = detect_format(cur_raw, cur)
    if ind is None:
        p.hand("manifest-env", f"{rel}: does not round-trip as JSON with 1, 2 or 4-space indent")
        return
    old_env = ((old.get("server") or {}).get("mcp_config") or {}).get("env") or {}
    env = cur.setdefault("server", {}).setdefault("mcp_config", {}).setdefault("env", {})
    added = [k for k in old_env if k not in env]
    for k in added:
        env[k] = old_env[k]
    old_uc, uc = old.get("user_config") or {}, cur.get("user_config")
    refs = {m for k in env for m in re.findall(r"\$\{user_config\.([A-Za-z0-9_]+)\}", str(env[k]))}
    uc_added = []
    if isinstance(old_uc, dict):
        if uc is None and refs & set(old_uc):
            uc = cur["user_config"] = {}
        for k in sorted(refs):
            if k in old_uc and k not in (uc or {}):
                uc[k] = old_uc[k]
                uc_added.append(k)
    if added or uc_added:
        p.write(rel, json.dumps(cur, indent=ind, ensure_ascii=ea) + "\n",
                f"declares {', '.join(added) or 'no new env'} again" + (f" (user_config {', '.join(uc_added)})" if uc_added else ""))


def detect_format(raw: str, data) -> tuple[int | None, bool]:
    for ind in (1, 2, 4):
        for ea in (True, False):
            if json.dumps(data, indent=ind, ensure_ascii=ea) + "\n" == raw:
                return ind, ea
    return None, True


def restore_entry_files(p: Plan, hid: str, entry: dict, dedupe: bool = False, skip: set | None = None) -> bool:
    """Restore every hand-written file the entry asserts on. False if one could not be."""
    ok = True
    for rel in sorted({a.get("file", "") for a in entry.get("asserts", [])} - (skip or set())):
        if not rel.startswith("cli/") or p.exists(rel):
            if p.exists(rel):
                p.add_tests_from(rel)
            continue
        ok = p.restore(f"restore:{rel}", rel, dedupe=dedupe) and ok
    return ok


RECIPES = {
    "derive-lock-canonical": r_derive_lock,
    "token-host-compare": r_token_host,
    "auth-dry-run": r_auth_dry_run,
    "doctor-verdict": r_doctor,
    "create-alias-removal": r_create_alias,
    "no-config-write": r_no_config_write,
    "manifest-env-union": r_manifest_env,
}


# --------------------------------------------------------------------------- #
# Floors.
# --------------------------------------------------------------------------- #

def go_env(writable: bool = False) -> dict:
    """Go's environment. Read-only module mode unless `writable` (only a floor
    raise, whose go.mod and go.sum --list-outputs names, may change them)."""
    env = gate_env()
    env["GOFLAGS"] = "-mod=mod" if writable else "-mod=readonly"
    return env


def gate_env() -> dict:
    """The environment of every subprocess that may run Go (the gate scripts
    too): workspace mode off, because in workspace mode even a read-only go
    list may write go.work.sum at the workspace root, outside the connector."""
    env = dict(os.environ)
    env["GOWORK"] = "off"
    return env


class GoListError(Exception):
    pass


def effective_version(listing: str, module: str) -> tuple[str, bool]:
    """(the version whose code the build uses, whether a replace directive set
    it) from `go list -m -json <module>`. A replace by another module path or by
    a local directory cannot be checked against a floor, so it is refused."""
    try:
        info = json.loads(listing)
    except json.JSONDecodeError as e:
        raise GoListError(f"go list -m -json printed no JSON ({e})") from None
    rep = info.get("Replace") if isinstance(info, dict) else None
    if isinstance(rep, dict):
        if rep.get("Path") != module or not str(rep.get("Version", "")).startswith("v"):
            raise GoListError(f"{module} is replaced by {rep.get('Path')} {rep.get('Version') or '(a local directory)'}, "
                              "which a floor cannot verify")
        return rep["Version"], True
    v = str((info or {}).get("Version", ""))
    if not v.startswith("v"):
        raise GoListError(f"go list reports no version for {module}")
    return v, False


def resolved_version(cli: Path, module: str) -> tuple[str | None, bool]:
    """(the effective version, replaced) this module graph selects, or
    (None, False) when the module is not in it."""
    try:
        p = subprocess.run(["go", "list", "-m", "-json", module], cwd=cli,
                           capture_output=True, text=True, env=go_env(), timeout=600)
    except FileNotFoundError:
        raise GoListError("go is not on PATH") from None
    if p.returncode == 0:
        return effective_version(p.stdout, module)
    err = (p.stderr or "").strip()
    if "not a known dependency" in err or "no required module provides" in err:
        return None, False
    raise GoListError(err.splitlines()[-1] if err else f"go list exited {p.returncode}")


def base_required(p: Plan, module: str) -> str | None:
    old = p.at_base("cli/go.mod") or ""
    m = re.search(r"(?m)^\s*(?:require\s+)?%s\s+(v\S+)" % re.escape(module), old)
    return m.group(1) if m else None


# Files a Go command this tool runs with writes allowed (go get, go mod tidy)
# may write. They pass Plan.safe() before any such command runs, like every
# in-process write, so a symlink cannot carry a write out of the connector.
GO_WRITES = ("cli", "cli/go.mod", "cli/go.sum")


def plan_floors(p: Plan) -> None:
    if not all([p.safe(rel) for rel in GO_WRITES]):
        return  # named as [path]; no Go command runs on this module
    for f in p.registry.get("floors") or []:
        module, floor = f["module"], f["min"]
        if p.version is None:
            p.notes.append(f"floor {module} {floor}: skipped, {p.version_why}")
            continue
        vk = chf.press_version_key(p.version)
        if f.get("retire_at") and vk >= chf.press_version_key(f["retire_at"]):
            continue
        try:
            have, replaced = resolved_version(p.cli, module)
        except GoListError as e:
            p.hand(f"floor:{module}", f"cli/go.mod: cannot read the resolved {module} version ({e})")
            continue
        if have is None:
            continue  # not in this module graph
        want = max(floor, base_required(p, module) or floor, key=semver)
        if vk < chf.press_version_key(f["applies_from"]):
            if semver(have) < semver(floor):
                p.notes.append(f"note: {module} {have} is below the {floor} floor; floors apply to prints at "
                               f"{f['applies_from']} and later, so this print is left as it is")
            continue
        if semver(have) < semver(want) and replaced:
            p.hand(f"floor:{module}", f"cli/go.mod: a replace directive pins {module} {have}, below the {want} "
                                      f"floor; go get cannot raise it, so raise the replace by hand")
        elif semver(have) < semver(want):
            p.floor_raises.append((module, have, want))
        else:
            p.notes.append(f"floor {module}: {have} meets {want}")


# --------------------------------------------------------------------------- #
# The run.
# --------------------------------------------------------------------------- #

def load_ledger(p: Plan) -> tuple[dict | None, str | None]:
    raw = p.read("handfixes.json")
    if raw is None:
        return None, None
    try:
        return json.loads(raw), raw
    except json.JSONDecodeError as e:
        p.hand("ledger", f"handfixes.json: not JSON ({e})")
        return None, raw


def plan_connector(root: Path, slug: str, base: str, registry: dict) -> Plan:
    p = Plan(root, slug, base, registry)
    data, raw = load_ledger(p)
    entries_out = []
    retired_records = list((data or {}).get("retired") or [])
    known = {r.get("id") for r in retired_records if isinstance(r, dict)}
    # The manifest's env declarations come back first: two recipes' asserts
    # (and onboard's mcp-install.md and server.json) read them.
    if any(((registry.get("handfixes") or {}).get(e.get("id")) or {}).get("recipe") in ("manifest-env-union", "no-config-write")
           and slug not in (((registry.get("handfixes") or {}).get(e.get("id")) or {}).get("keep_manual") or {})
           and failing_asserts(p, e.get("asserts", []))
           for e in (data or {}).get("handfixes") or []):
        manifest_env_union(p)
    for e in (data or {}).get("handfixes") or []:
        hid = e.get("id")
        r = (registry.get("handfixes") or {}).get(hid)
        if not isinstance(r, dict):
            entries_out.append(e)
            continue
        verdict = r.get("verdict")
        if slug in (r.get("keep_manual") or {}):
            p.notes.append(f"manual [{hid}]: {r['keep_manual'][slug]}")
            entries_out.append(e)
            continue
        if verdict == "delegated":
            p.notes.append(f"delegated [{hid}]: {r.get('tool')}")
            entries_out.append(e)
            continue
        if verdict == "retire":
            if chf.retirement(registry, hid, slug, p.version) is None:
                why = p.version_why or f"press {p.version} < {r['retire_at']}"
                p.notes.append(f"kept [{hid}]: not retired here ({why})")
                entries_out.append(e)
                continue
            p.made.append(f"handfixes.json: retire {hid} (press {p.version} >= {r['retire_at']})")
            p.retired_files.update(a.get("file", "") for a in e.get("asserts", []))
            if hid not in known:
                retired_records.append({"id": hid, "retired_at": r["retire_at"],
                                        "reason": "tools/maintainer/fleet_handfixes.json: " + r["reason"]})
                known.add(hid)
            p.gates += [(hid, g) for g in r.get("gates") or []]
            continue
        if verdict == "replay":
            p.gates += [(hid, g) for g in r.get("gates") or []]
            if not failing_asserts(p, e.get("asserts", [])):
                for a in e.get("asserts", []):
                    if p.exists(a.get("file", "")):
                        p.add_tests_from(a["file"])
                entries_out.append(e)
                continue
            recipe = RECIPES.get(r.get("recipe"))
            if recipe is None:
                p.hand(f"recipe:{hid}", f"fleet_handfixes.json names recipe {r.get('recipe')!r}, which this tool lacks")
                entries_out.append(e)
                continue
            obsolete = recipe(p, hid, e) or {}
            relocated = p.relocated
            new_asserts = []
            for a in e.get("asserts", []):
                if a.get("file") in p.relocated_files:
                    a = dict(a, file=p.relocated_files[a["file"]])
                if not failing_asserts(p, [a]):
                    new_asserts.append(a)
                    continue
                needle = a.get("contains") or a.get("not_contains") or ""
                why = next((w for k, w in obsolete.items() if k in needle), None)
                moved = next((home for name, home in relocated.items()
                              if a.get("file") and needle.startswith(f"func {name.split('.')[-1]}(")), None)
                if moved and not failing_asserts(p, [dict(a, file=moved)]):
                    new_asserts.append(dict(a, file=moved))
                    p.notes.append(f"[{hid}] assert `{needle[:60]}` now pins {moved} (the press generates it)")
                elif why:
                    p.notes.append(f"[{hid}] drops assert `{needle[:60]}`: {why}")
                else:
                    new_asserts.append(a)
                    p.hand(f"assert:{hid}", f"{a.get('file')}: `{needle[:90]}` does not hold after the replay")
            have = {json.dumps(a, sort_keys=True) for a in new_asserts}
            for a in p.pins.get(hid, []):
                if json.dumps(a, sort_keys=True) not in have and not failing_asserts(p, [a]):
                    new_asserts.append(a)
                    have.add(json.dumps(a, sort_keys=True))
            entries_out.append(dict(e, asserts=new_asserts) if new_asserts != e.get("asserts") else e)
            continue
        entries_out.append(e)
    for rec in retired_records:  # a retirement stays proven by execution on every apply
        hid = rec.get("id") if isinstance(rec, dict) else None
        if hid and not any(h == hid for h, _ in p.gates):
            p.gates += [(hid, g) for g in ((registry.get("handfixes") or {}).get(hid) or {}).get("gates") or []]
    # A hand-written file only a retired entry pinned tests the hand-fix's own
    # API, which the press replaced; a reprint that carried it in would not
    # build. Remove it, unless an entry that stays asserts on it.
    still = {a.get("file", "") for e in entries_out for a in e.get("asserts", [])}
    for rel in sorted(p.retired_files - still):
        text = p.read(rel) if rel.startswith("cli/") and rel.endswith(".go") else None
        if text is not None and GENERATED not in text[:600]:
            p.delete(rel, "a retired hand-fix's own file")
    if data is not None:
        new = dict(data)
        new["handfixes"] = entries_out
        if retired_records:
            new["retired"] = retired_records
        ind, ea = detect_format(raw, data)
        if ind is None:
            p.hand("ledger", "handfixes.json does not round-trip as JSON with 1, 2 or 4-space indent; fix its "
                             "formatting first so this tool cannot rewrite unrelated lines")
        else:
            text = json.dumps(new, indent=ind, ensure_ascii=ea) + "\n"
            if text != raw:
                p.write("handfixes.json", text, "ledger updated")
    if (p.cli / "go.mod").is_file():
        plan_floors(p)
    return p


def owned_ids(registry: dict) -> list[str]:
    """Registry ids this tool retires or replays (delegated ids belong to another tool)."""
    return sorted(h for h, r in (registry.get("handfixes") or {}).items() if r.get("verdict") in ("retire", "replay"))


def run_slug(root: Path, slug: str, check: bool, base: str = "HEAD", out=print, prove=None,
             registry: dict | None = None) -> int:
    if not SLUG.fullmatch(slug):
        out(f"{slug}: FAIL: not a connector slug (lowercase letters, digits and hyphens)")
        return 1
    skill = root / "skills" / slug
    if not skill.is_dir() or skill.is_symlink():
        out(f"{slug}: no skills/{slug}")
        return 1
    if registry is None:
        registry, err = chf.load_registry(REGISTRY)
        if err:
            out(f"{slug}: FAIL: {err}")
            return 1
    p = plan_connector(root, slug, base, registry)
    changes = [r for r in p.texts if r not in p.deleted and p.texts[r] != (p.orig.get(r) if (skill / r).exists() else None)]
    for n in p.notes:
        out(f"{slug}: {n}")
    if check:
        for rel in changes:
            n = sum(1 for ln in difflib.unified_diff((p.orig.get(rel) or "").splitlines(), p.texts[rel].splitlines(),
                                                     lineterm="", n=0) if ln[:1] in "+-" and ln[:3] not in ("+++", "---"))
            out(f"{slug}: would change {rel} ({n} changed lines)")
        for rel in sorted(p.deleted):
            out(f"{slug}: would remove {rel}")
        for module, have, want in p.floor_raises:
            out(f"{slug}: would raise {module} {have} -> {want}")
    else:
        for rel in changes:
            dest = skill / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(p.texts[rel], encoding="utf-8")
        for rel in sorted(p.deleted):
            (skill / rel).unlink()
        gofmt([skill / r for r in changes if r.endswith(".go") and p.safe(r)])
        for m in p.made:
            out(f"{slug}: {m}")
        for module, have, want in p.floor_raises:
            if not all([p.safe(rel) for rel in GO_WRITES]):
                break
            got = raise_floor(p.cli, module, want)
            if got is None or semver(got) < semver(want):
                p.hand(f"floor:{module}", f"cli/go.mod: {module} is {got or 'unresolved'} after go get {module}@{want}")
            else:
                out(f"{slug}: raised {module} {have} -> {got}")
    for site, msg in p.needs_hand:
        out(f"{slug}: NEEDS HAND WIRING [{site}]: {msg}")
    if p.needs_hand:
        out(f"{slug}: FAIL: {len(p.needs_hand)} site(s) need a person. Wire each one, keep or update its "
            f"handfixes.json asserts, then re-run with --check")
        return 1
    if check and (changes or p.floor_raises or p.deleted):
        out(f"{slug}: FAIL: not applied; run without --check")
        return 1
    prove = prove or _PROVER[0]
    pkgs = sorted({os.path.dirname(r[len("cli/"):]) for r in changes if r.startswith("cli/") and r.endswith(".go")})
    failed = prove(root, slug, not check, p, pkgs)
    if failed:
        for f in failed:
            out(f"{slug}: PROOF FAILED: {f}")
        out(f"{slug}: FAIL: not proven; sites this run touched: {'; '.join(p.made) or '(no edit this run)'}")
        return 1
    if not changes and not p.floor_raises and not p.deleted:
        out(f"{slug}: already applied, nothing to change" + (" (proven)" if not check else ""))
    else:
        out(f"{slug}: {'applied and proven' if not check else 'would apply'}")
    return 0


def gofmt(paths: list[Path]) -> None:
    if paths and shutil.which("gofmt"):
        subprocess.run(["gofmt", "-w", *map(str, paths)], capture_output=True)


def raise_floor(cli: Path, module: str, want: str) -> str | None:
    for argv in (["go", "get", f"{module}@{want}"], ["go", "mod", "tidy"]):
        try:
            r = subprocess.run(argv, cwd=cli, capture_output=True, text=True, env=go_env(writable=True), timeout=1200)
        except FileNotFoundError:
            return None
        if r.returncode != 0:
            return None
    try:
        got, replaced = resolved_version(cli, module)
    except GoListError:
        return None
    return None if replaced else got


# --------------------------------------------------------------------------- #
# Proofs, by execution.
# --------------------------------------------------------------------------- #

def _tail(p: subprocess.CompletedProcess, n: int = 6) -> str:
    lines = ((p.stdout or "") + (p.stderr or "")).strip().splitlines()
    return " | ".join(lines[-n:]) if lines else f"exit {p.returncode}"


def run_proofs(root: Path, slug: str, full: bool, p: Plan, pkgs: list[str]) -> list[str]:
    failed: list[str] = []
    tools = root / "tools" / "maintainer"
    argv = [sys.executable, "-I", str(tools / "check_handfixes.py"), "--slug", slug]
    owned = owned_ids(p.registry)
    for hid in owned:
        argv += ["--only-id", hid]
    r = subprocess.run(argv, cwd=root, capture_output=True, text=True, timeout=600)
    if r.returncode != 0:
        failed.append(f"check_handfixes.py --slug {slug} (the {len(owned)} ids this tool owns): {_tail(r)}")
    if not full:
        return failed
    cli = p.cli
    if shutil.which("go") is None:
        return failed + ["go is not on PATH, so nothing could be built"]
    for name, cmd in (("go build ./...", ["go", "build", "./..."]), ("go vet ./...", ["go", "vet", "./..."])):
        r = subprocess.run(cmd, cwd=cli, capture_output=True, text=True, timeout=1800, env=go_env())
        if r.returncode != 0:
            return failed + [f"{name}: {_tail(r)}"]
    test_pkgs = sorted(set(pkgs) | set(p.tests))
    if p.floor_raises:
        r = subprocess.run(["go", "test", "./...", "-count=1"], cwd=cli, capture_output=True, text=True,
                           timeout=3600, env=go_env())
        if r.returncode != 0:
            failed.append(f"go test ./... (after raising a floor): {_tail(r)}")
    elif test_pkgs:
        r = subprocess.run(["go", "test", *[f"./{d}/" for d in test_pkgs], "-count=1"], cwd=cli,
                           capture_output=True, text=True, timeout=3600, env=go_env())
        if r.returncode != 0:
            failed.append(f"go test of the packages this run wrote or restored ({', '.join(test_pkgs)}): {_tail(r)}")
    for hid, gate in p.gates:
        bad = run_gate(root, slug, p, gate)
        if bad:
            failed.append(f"gate for {hid}: {bad}")
    if p.floor_raises:
        r = subprocess.run([sys.executable, "-I", str(tools / "check_security_gate.py"), "--slug", slug,
                            "--base", p.base, "--require-scanners"], cwd=root, capture_output=True, text=True,
                           timeout=3600, env=gate_env())
        if r.returncode != 0:
            failed.append(f"check_security_gate.py --slug {slug} --require-scanners (after raising "
                          f"{', '.join(m for m, _, _ in p.floor_raises)}): {_tail(r)}")
    return failed


CLASS_SUMMARY = re.compile(r"== class ([\w-]+): (\d+) failing / (\d+) checked")


def run_gate(root: Path, slug: str, p: Plan, gate: dict) -> str | None:
    kind = gate.get("kind")
    tools = root / "tools" / "maintainer"
    if kind == "check":
        argv = [a.replace("{slug}", slug) for a in gate["argv"]]
        r = subprocess.run([sys.executable, "-I", str(tools / argv[0]), *argv[1:]], cwd=root,
                           capture_output=True, text=True, timeout=3600, env=gate_env())
        return None if r.returncode == 0 else f"{' '.join(argv)}: {_tail(r)}"
    if kind == "class":
        r = subprocess.run([sys.executable, "-I", str(tools / "class_harness.py"), "--slug", slug,
                            "--class", gate["class"]], cwd=root, capture_output=True, text=True, timeout=3600, env=gate_env())
        m = CLASS_SUMMARY.search(r.stdout)
        if r.returncode != 0 or not m or m.group(2) != "0" or m.group(3) != "1":
            return f"class_harness.py --class {gate['class']}: {_tail(r)}"
        return None
    if kind in ("go_test", "proof_test"):
        names = gate["tests"]
        overlay_args: list[str] = []
        tmp = None
        if kind == "proof_test":
            prefix = env_prefix(p)
            if prefix is None:
                return "cannot read this connector's environment prefix from internal/config/config.go"
            tmp = Path(tempfile.mkdtemp(prefix="replay-proof-"))
            src = tmp / "proof_test.go"
            src.write_text(render(gate["template"], PREFIX_=prefix + "_", MODULE=p.module), encoding="utf-8")
            ov = tmp / "overlay.json"
            target = p.cli / gate["pkg"] / "zz_replay_handfixes_proof_test.go"
            ov.write_text(json.dumps({"Replace": {str(target): str(src)}}), encoding="utf-8")
            overlay_args = [f"-overlay={ov}"]
            pkg = "./" + gate["pkg"] + "/"
        else:
            pkg = gate["pkg"]
        try:
            r = subprocess.run(["go", "test", *overlay_args, pkg, "-run", "^(" + "|".join(names) + ")$",
                                "-count=1", "-v"], cwd=p.cli, capture_output=True, text=True, timeout=1800,
                               env=go_env())
        finally:
            if tmp:
                shutil.rmtree(tmp, ignore_errors=True)
        missing = [n for n in names if f"--- PASS: {n} " not in r.stdout]
        if r.returncode != 0 or missing:
            return f"go test {pkg} {', '.join(missing) or ''} did not pass: {_tail(r)}"
        return None
    return f"unknown gate kind {kind!r}"


def env_prefix(p: Plan) -> str | None:
    s = p.read("cli/internal/config/config.go") or ""
    m = re.search(r'os\.Getenv\("([A-Z0-9_]+)_CLIENT_SECRET"\)', s) or re.search(r'"([A-Z0-9_]+)_CONFIG"', s)
    return m.group(1) if m else None


_PROVER = [run_proofs]


def list_outputs(root: Path, slug: str, base: str = "HEAD") -> list[str]:
    if not SLUG.fullmatch(slug):
        raise SystemExit(f"replay_handfixes: {slug!r} is not a connector slug")
    registry, err = chf.load_registry(REGISTRY)
    if err:
        raise SystemExit(f"replay_handfixes: {err}")
    p = plan_connector(root, slug, base, registry)
    skill = root / "skills" / slug
    out = {f"skills/{slug}/{r}" for r in p.texts if p.texts[r] != (p.orig.get(r) if (skill / r).exists() else None)}
    out |= {f"skills/{slug}/{r}" for r in p.deleted}
    if p.floor_raises:
        out |= {f"skills/{slug}/cli/go.mod", f"skills/{slug}/cli/go.sum"}
    return sorted(out)


# --------------------------------------------------------------------------- #
# Self-test (fixtures; no build, no network).
# --------------------------------------------------------------------------- #

def self_test() -> int:
    from replay_handfixes_selftest import run  # noqa: E402  kept apart: it is long
    return run(sys.modules[__name__])


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description="Retire or re-apply the fleet-wide hand-fixes after a reprint.")
    ap.add_argument("--slug", action="append", help="connector to process (repeatable)")
    ap.add_argument("--check", action="store_true", help="report what an apply would change; change nothing")
    ap.add_argument("--list-outputs", action="store_true",
                    help="print every repo path an apply would write for --slug (one per line); change nothing")
    ap.add_argument("--base", default="HEAD",
                    help="revision the pre-reprint hand-written files are restored from (default HEAD)")
    ap.add_argument("--self-test", action="store_true", help="prove the tool both ways on fixtures")
    ap.add_argument("--root", default=str(ROOT), help=argparse.SUPPRESS)
    args = ap.parse_args(argv)
    if args.self_test:
        return self_test()
    if not args.slug:
        ap.error("pass --slug <slug> or --self-test")
    root = Path(args.root)
    rc = 0
    for slug in args.slug:
        if args.list_outputs:
            for path in list_outputs(root, slug, args.base):
                print(path)
            continue
        rc |= run_slug(root, slug, args.check, args.base)
    return rc


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
