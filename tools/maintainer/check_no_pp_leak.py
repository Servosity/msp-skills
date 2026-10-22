#!/usr/bin/env python3
"""No connector's own `-pp-` name in what a user reads.

cli-printing-press names everything `<name>-pp-cli` / `<name>-pp-mcp`; this repo
ships `<slug>-cli` / `<slug>-mcp`. The strip (msp-skills-publish step 1) renames
the binaries and the Cobra strings, and then a person was expected to sweep
fourteen more surfaces by hand. Nothing checked the sweep, so leaks shipped:
immybot's `immy-bot-pp-cli` (5d7892724), nine manifests that still name
`<slug>-pp-mcp`, and help text that tells an operator to run `avanan-pp-cli`,
a binary that does not exist. The factory's merge script grepped part of this
right before merging; this is that grep, moved to before the pull request.

What counts as a leak: in a connector's own files, any `<name>-pp-cli` or
`<name>-pp-mcp` token (the guides carry the press LIBRARY name, which is in no
registry field), except a tool the connector really calls by that name
(EXTERNAL_PP_TOOLS: aws-billing posts through `slack-pp-cli`). In Go strings
and the shared catalogs, the connector's OWN names: the slug, the go.mod
module base and the registered cli/mcp binary bases, plus a `%s-pp-` format
string in Go (generated tests use placeholder `-pp-` names on purpose).

Where it looks:
  * the connector's user-facing files: every top-level *.md and references/
    page, manifest.json, server.json, page.json, .claude-plugin/plugin.json,
    install.sh, install.ps1, and docs/skills/<slug>.md;
  * the shared catalogs that name every connector: README.md, catalog.json,
    docs/_data/catalog.json, docs/llms.txt, docs/llms-full.txt and
    tools/maintainer/skills.json;
  * string literals in the connector's Go under cli/internal and cli/cmd (an
    import path such as `<module>-pp-cli/internal/...` is not a leak: the
    module keeps its press name on purpose);
  * handfixes.json `contains` markers on non-Go files, which would force a leak
    to survive. A `not_contains` marker is honored: it is the assertion that
    the leak is GONE (cloudally's `not_contains: cloudally-pp-mcp`).

--base REF makes it a ratchet: a finding whose exact line already exists in
the same file at REF is pre-existing - printed, never blocking - and only a new
or changed line fails. The fleet carried 34 pre-existing leaks when this
landed (nine manifests, 24 guide.md config paths, avanan/datagate help text);
they are listed on every run that touches those connectors until a sweep
removes them. Without --base every finding fails (a full audit).

    python3 tools/maintainer/check_no_pp_leak.py --all [--base origin/main]
    python3 tools/maintainer/check_no_pp_leak.py --slug axcient --base origin/main
    python3 tools/maintainer/check_no_pp_leak.py --self-test

Exit 0 = no new leak, 1 = new leak(s), 2 = usage or environment error.
Lines are `<path>:<line>: [<slug>] <name> ...`.
"""

from __future__ import annotations

import argparse
import html
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import registry  # noqa: E402  (local tools/ module)
from check_unix_test_asserts import code_mask, lex  # noqa: E402  (shared Go lexer)

SLUG_RE = re.compile(r"\A[a-z0-9][a-z0-9-]*\Z")
SHARED_FILES = ("README.md", "catalog.json", "docs/_data/catalog.json", "docs/llms.txt",
                "docs/llms-full.txt", "tools/maintainer/skills.json")
# Tools published in printing-press-library under their -pp- names, which a
# connector really calls by that name. Naming one is a dependency, not a leak.
EXTERNAL_PP_TOOLS = frozenset({"slack-pp-cli"})
# In a connector's own files, any press-style binary name is a leak: the guides
# carry the press LIBRARY name (`x360recover-pp-cli` for axcient), which is in
# no registry field. Shared catalogs stay on own-name matching, so each hit
# can be attributed to one connector.
ANY_PP_NAME = re.compile(r"(?<![A-Za-z0-9-])[a-z0-9]+(?:-[a-z0-9]+)*-pp-(?:cli|mcp)(?![A-Za-z0-9-])")
SKILL_FILES = ("manifest.json", "server.json", "page.json", ".claude-plugin/plugin.json",
               "install.sh", "install.ps1")


def own_names(root: Path, slug: str, meta: dict) -> set[str]:
    names = {slug}
    gomod = root / "skills" / slug / "cli" / "go.mod"
    try:
        for line in gomod.read_text(encoding="utf-8").splitlines():
            if line.startswith("module "):
                # The last element of the path: github.com/x/acme-pp-cli names acme.
                mod = line.split()[1].strip().strip('"`').rstrip("/").rsplit("/", 1)[-1]
                names.add(re.sub(r"-pp-(?:cli|mcp)$", "", mod))
                break
    except OSError:
        pass
    for key, suffix in (("cli_binary", "-cli"), ("mcp_binary", "-mcp")):
        v = meta.get(key)
        if isinstance(v, str) and v.endswith(suffix):
            names.add(v[: -len(suffix)])
    return {n for n in names if n}


def leak_re(names: set[str], go: bool) -> re.Pattern:
    """`<own name>-pp-cli|mcp` as a whole token. In Go, `%s-pp-` counts too,
    and a following `/` does not: `<module>-pp-cli/internal/x` is an import
    path, which keeps the press name on purpose. In a document a following `/`
    is a leaked path (`~/.config/<module>-pp-cli/config.toml`)."""
    alts = sorted((re.escape(n) for n in names), key=len, reverse=True)
    if go:
        alts.append("%s")
    after = r"(?![A-Za-z0-9/-])" if go else r"(?![A-Za-z0-9-])"
    return re.compile(rf"(?<![A-Za-z0-9-])(?:{'|'.join(alts)})-pp-(?:cli|mcp){after}")


class Leak:
    """`text` is the raw source line (what the --base ratchet compares);
    `msg` is an optional explanation shown instead of it."""
    __slots__ = ("rel", "line", "slug", "name", "text", "msg", "pre_existing", "rescan")

    def __init__(self, rel: str, line: int, slug: str, name: str, text: str, msg: str = "") -> None:
        self.rel, self.line, self.slug, self.name, self.text, self.msg = rel, line, slug, name, text, msg
        self.pre_existing = False
        self.rescan = None  # the scanner that found it, to re-run on the base text

    def render(self) -> str:
        tag = "  (pre-existing at base, not blocking)" if self.pre_existing else ""
        return f"{self.rel}:{self.line}: [{self.slug}] {self.name} -> {(self.msg or self.text).strip()[:160]}{tag}"


def lines_of(text: str) -> list[str]:
    """Split on \\n only: a carriage return stays part of its line, so the
    ratchet's exact comparison sees a CRLF change."""
    lines = text.split("\n")
    return lines[:-1] if lines and lines[-1] == "" else lines


_JSON_STR = re.compile(r'"(?:[^"\\\n]|\\.)*"')


_MD_ESCAPE = re.compile(r"\\([!-/:-@\[-`{-~])")  # CommonMark: a backslash before ASCII punctuation


def visible(rel: str, line: str) -> str:
    """The text a reader sees: Markdown backslash escapes (`acme\\-pp\\-cli`)
    and Markdown/HTML entities (`acme&#45;pp-cli`) decoded, both render as
    acme-pp-cli. JSON lines are decoded per string token instead (see
    _scan_text)."""
    low = rel.lower()
    if low.endswith(MARKDOWN_SUFFIXES):
        line = _MD_ESCAPE.sub(r"\1", line)
    return html.unescape(line) if low.endswith(MARKDOWN_SUFFIXES + (".html", ".htm", ".xhtml")) else line


def _json_strings(line: str) -> list[str]:
    """Every JSON string token on a line, decoded (`acme\\u002dpp-mcp` ->
    acme-pp-mcp). A token that does not decode is kept raw."""
    out = []
    for m in _JSON_STR.finditer(line):
        try:
            out.append(json.loads(m.group(0)))
        except ValueError:
            out.append(m.group(0))
    return out


def _scan_text(rel: str, text: str, slug: str, rx: re.Pattern) -> list[Leak]:
    out = []
    is_json = rel.endswith(".json")
    for i, line in enumerate(lines_of(text), 1):
        candidates = [visible(rel, line)] + (_json_strings(line) if is_json else [])
        hit = next((m.group(0) for c in candidates for m in rx.finditer(c)
                    if m.group(0) not in EXTERNAL_PP_TOOLS), None)
        if hit:
            out.append(Leak(rel, i, slug, hit, line))
    return out


READ_ERRORS: list[str] = []


def _read(p: Path) -> str | None:
    """A file's text, or None when it does not exist. Any OTHER read error is
    recorded and fails the run: an unreadable file is not a file without a
    leak."""
    try:
        # surrogateescape: lossless, so the ratchet's exact comparison sees
        # a changed invalid byte (errors="replace" made them all U+FFFD).
        with open(p, encoding="utf-8", errors="surrogateescape", newline="") as fh:
            return fh.read()
    except (FileNotFoundError, NotADirectoryError):
        return None
    except OSError as e:
        READ_ERRORS.append(f"{p}: {e}")
        return None


# The pages a skill links to, in every text format GitHub renders or a reader
# opens (compared case-insensitively: README.MD is a README).
# (github/markup's markdown and markup lists, plus plain text and HTML.)
MARKDOWN_SUFFIXES = (".md", ".markdown", ".mdown", ".mkdn", ".mkd", ".mdwn", ".mdtxt", ".mdtext", ".mdx", ".rmd",
                     ".litcoffee")
REFERENCE_SUFFIXES = MARKDOWN_SUFFIXES + (".html", ".htm", ".xhtml", ".txt", ".text", ".rst", ".rest", ".adoc",
                                          ".asciidoc", ".asc", ".textile", ".rdoc", ".org", ".creole", ".mediawiki",
                                          ".wiki", ".pod", ".pod6")


def _files(d: Path, suffix: str | tuple[str, ...], recursive: bool) -> list[Path]:
    """Files under d ending in suffix. A directory that cannot be listed is
    recorded and fails the run, like an unreadable file: what was not read
    was not checked."""
    out: list[Path] = []
    def err(e: OSError) -> None:
        if not isinstance(e, (FileNotFoundError, NotADirectoryError)):
            READ_ERRORS.append(f"{e.filename}: {e}")
    seen: set[str] = set()
    # A linked folder is read too (references/help -> ../assets/help is still
    # a page users open); each real folder once, so a link cycle ends.
    for top, dirs, names in os.walk(d, onerror=err, followlinks=True):
        real = os.path.realpath(top)
        if real in seen:
            dirs.clear()
            continue
        seen.add(real)
        out += [Path(top) / n for n in names if n.lower().endswith(suffix)]
        if not recursive:
            dirs.clear()
    return sorted(out)


_IMPORT_KW = re.compile(r"^import\b", re.M)


def import_spans(src: str) -> list[tuple[int, int]]:
    """Source ranges holding import paths: only there may `<module>-pp-cli/`
    appear, because the module keeps its press name on purpose. Found in the
    CODE only (comments and string bodies masked), so text that merely looks
    like an import block cannot lend a real string the exemption."""
    code = code_mask(src)
    spans: list[tuple[int, int]] = []
    for m in _IMPORT_KW.finditer(code):
        i = m.end()
        while i < len(code) and code[i] in " \t":
            i += 1
        if i < len(code) and code[i] == "(":
            depth, j = 0, i
            while j < len(code):
                if code[j] == "(":
                    depth += 1
                elif code[j] == ")":
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            spans.append((i, j))
        else:
            # `import [name] "path"`: exactly that one literal, nothing after
            # it on the line (`import _ "fmt"; var Help = "..."`).
            q = re.compile(r"(?:[\w.]+[ \t]+)?([\"`])").match(code, i)
            if q:
                spans.append((q.start(1), q.start(1) + 1))
    return spans


def skill_dir(root: Path, slug: str, meta: dict) -> Path:
    d = meta.get("source_dir", slug)
    return root / "skills" / (d if isinstance(d, str) else slug)


# Each scanner reads ONE file's text, so the --base ratchet can run the very
# same scanner over that file's base version and compare findings.


def scan_doc(rel: str, text: str, slug: str) -> list[Leak]:
    return _scan_text(rel, text, slug, ANY_PP_NAME)


def scan_go(rel: str, src: str, slug: str, names: set[str]) -> list[Leak]:
    """String literals only (never comments). `<module>-pp-cli/...` is exempt
    inside import declarations only; in any other string a following `/` is a
    leaked path (`~/.config/acme-pp-cli/config.toml`)."""
    # No raw-source prefilter: an escaped literal ("acme\x2dpp-cli") shows
    # the leak only once decoded.
    rx_import = leak_re(names, go=True)
    rx_other = leak_re(names | {"%s"}, go=False)
    out: list[Leak] = []
    lits, _ = lex(src)
    imports = import_spans(src)
    for lit in lits:
        in_import = any(a <= lit.start < b for a, b in imports)
        seen_lines: set[int] = set()
        # EVERY match, each on its own source line: a grandfathered leak must
        # not hide a new one later in the same string.
        raw = src[lit.start] == "`"
        # Go drops every carriage return from a raw string's value; lex()
        # does the same, so map each value index back to its source offset.
        offsets = [lit.start + 1 + k for k, ch in enumerate(src[lit.start + 1:lit.end - 1]) if ch != "\r"] if raw else []
        for m in (rx_import if in_import else rx_other).finditer(lit.value):
            # A raw string spans lines; an interpreted "..." string is one line.
            at = offsets[m.start()] if raw and m.start() < len(offsets) else lit.start
            ln = src.count("\n", 0, at) + 1
            if ln in seen_lines:
                continue
            seen_lines.add(ln)
            s0 = src.rfind("\n", 0, at) + 1
            e0 = src.find("\n", at)
            out.append(Leak(rel, ln, slug, m.group(0), src[s0:e0 if e0 >= 0 else len(src)]))
    return out


_JSON_TOKEN = re.compile(r'"(?:[^"\\\n]|\\.)*"')


def _json_pairs(text: str) -> list[tuple[str, str, int, int]]:
    """(decoded key, decoded string value, value offset, key offset) for every
    `"key": "value"` pair, keys and values both DECODED (`"cont\\u0061ins"` is
    `contains`), source offsets kept."""
    out = []
    toks = list(_JSON_TOKEN.finditer(text))
    for k, kt in enumerate(toks[:-1]):
        vt = toks[k + 1]
        if not re.fullmatch(r"\s*:\s*", text[kt.end():vt.start()]):
            continue
        try:
            key, val = json.loads(kt.group(0)), json.loads(vt.group(0))
        except ValueError:
            continue
        if isinstance(key, str) and isinstance(val, str):
            out.append((key, val, vt.start(), kt.start()))
    return out


def _json_object_span(text: str, pos: int) -> tuple[int, int]:
    """The innermost JSON object that contains pos (string-aware)."""
    stack: list[int] = []
    i, n, start = 0, len(text), 0
    while i < n:
        ch = text[i]
        if ch == '"':
            i += 1
            while i < n and text[i] != '"':
                i += 2 if text[i] == "\\" else 1
        elif ch == "{":
            stack.append(i)
        elif ch == "}":
            if stack:
                o = stack.pop()
                if o <= pos <= i and o >= start:
                    return o, i
        if i >= pos and stack:
            start = stack[-1]
        i += 1
    return 0, n


def scan_handfixes(rel: str, text: str, slug: str) -> list[Leak]:
    """A `contains` (or `regex`) marker on a non-Go file that requires a leak
    keeps it alive; a `not_contains` marker is the assertion it is gone. Each
    finding sits on the line of ITS OWN marker value, with that exact line,
    so the ratchet sees a changed marker or target. Keys are decoded too."""
    out: list[Leak] = []
    lines = lines_of(text)
    pairs = _json_pairs(text)
    for key, v, voff, koff in pairs:
        if key not in ("contains", "regex"):
            continue
        hit = ANY_PP_NAME.search(v)
        if not hit or hit.group(0) in EXTERNAL_PP_TOOLS:
            continue
        o, e = _json_object_span(text, koff)
        target = next((val for k2, val, vo2, ko2 in pairs if k2 == "file" and o <= ko2 <= e), "")
        if target.endswith((".go", "go.mod", "go.sum")):
            continue
        ln = text.count("\n", 0, voff) + 1
        raw = lines[ln - 1] if ln - 1 < len(lines) else v
        out.append(Leak(rel, ln, slug, hit.group(0), raw,
                        f"{key} marker on {target} requires the leak to stay: {v}"))
    return out


def scan_shared_text(rel: str, text: str, owner: dict[str, str]) -> list[Leak]:
    rx = leak_re(set(owner), go=False)
    out: list[Leak] = []
    is_json = rel.endswith(".json")
    for i, line in enumerate(lines_of(text), 1):
        seen: set[str] = set()
        for c in [visible(rel, line)] + (_json_strings(line) if is_json else []):
            for m in rx.finditer(c):
                if m.group(0) in seen:
                    continue
                seen.add(m.group(0))
                base = re.sub(r"-pp-(?:cli|mcp)$", "", m.group(0))
                out.append(Leak(rel, i, owner.get(base, "?"), m.group(0), line))
    return out


def scan_slug(root: Path, slug: str, meta: dict) -> list[Leak]:
    sd = skill_dir(root, slug, meta)
    names = own_names(root, sd.name, meta) | {slug}
    leaks: list[Leak] = []
    files = _files(sd, REFERENCE_SUFFIXES, False) + _files(sd / "references", REFERENCE_SUFFIXES, True)
    files += [sd / f for f in SKILL_FILES]
    files += [root / "docs" / "skills" / f"{slug}.md"]
    for p in files:
        text = _read(p) if p.is_file() else None
        if text is not None:
            rel = str(p.relative_to(root))
            leaks += _tag(scan_doc(rel, text, slug), lambda r, t, s=slug: scan_doc(r, t, s))
    for sub in ("cli/internal", "cli/cmd"):
        for p in _files(sd / sub, ".go", True):
            src = _read(p)
            if src is not None:
                rel = str(p.relative_to(root))
                leaks += _tag(scan_go(rel, src, slug, names),
                              lambda r, t, s=slug, n=names: scan_go(r, t, s, n))
    hf = sd / "handfixes.json"
    text = _read(hf) if hf.is_file() else None
    if text is not None:
        rel = str(hf.relative_to(root))
        leaks += _tag(scan_handfixes(rel, text, slug), lambda r, t, s=slug: scan_handfixes(r, t, s))
    return leaks


def _tag(leaks: list[Leak], rescan) -> list[Leak]:
    for lk in leaks:
        lk.rescan = rescan
    return leaks


def scan_shared(root: Path, slugs: list[str], skills: dict) -> list[Leak]:
    """The catalogs that name every connector, read once. Each hit is
    attributed to the connector whose own name it carries."""
    owner: dict[str, str] = {}
    for s in slugs:
        meta = skills.get(s, {})
        for n in own_names(root, skill_dir(root, s, meta).name, meta) | {s}:
            owner.setdefault(n, s)
    if not owner:
        return []
    leaks: list[Leak] = []
    for rel in SHARED_FILES:
        text = _read(root / rel)
        if text is not None:
            leaks += _tag(scan_shared_text(rel, text, owner), lambda r, t, o=owner: scan_shared_text(r, t, o))
    return leaks


def _base_path(root: Path, base: str, rel: str) -> str | None:
    """rel as the base tree resolves it, every symlink on the way (a linked
    file, or a linked folder above it) followed inside that tree, the way the
    scan followed them on disk. None when it leaves the tree or loops."""
    import posixpath
    parts, done, hops = rel.split("/"), [], 0
    while parts:
        cand = "/".join(done + [parts[0]])
        p = subprocess.run(["git", "-C", str(root), "ls-tree", "-z", base, "--", cand], capture_output=True)
        ent = p.stdout.split(b"\0")[0] if p.returncode == 0 else b""
        if ent.startswith(b"120000 "):
            hops += 1
            tgt = subprocess.run(["git", "-C", str(root), "cat-file", "blob", ent.split()[2].decode()],
                                 capture_output=True).stdout.decode("utf-8", "surrogateescape")
            new = posixpath.normpath(posixpath.join("/".join(done), tgt))
            if hops > 40 or tgt.startswith("/") or new == ".." or new.startswith("../"):
                return None
            parts, done = new.split("/") + parts[1:], []
        else:
            done.append(parts.pop(0))
    return "/".join(done)


def git_show(root: Path, base: str, rel: str) -> str | None:
    real = _base_path(root, base, rel)
    if real is None:
        return None
    p = subprocess.run(["git", "-C", str(root), "show", f"{base}:{real}"], capture_output=True)
    return p.stdout.decode("utf-8", errors="surrogateescape") if p.returncode == 0 else None


def mark_pre_existing(root: Path, base: str, leaks: list[Leak]) -> None:
    """A finding is pre-existing only when the SAME scanner, run over that
    file's base version, reports the same leak on an identical line, counted:
    a leaking line copied next to an existing import, or a second copy of an
    existing leak line, is new."""
    from collections import Counter

    budget: dict[str, Counter] = {}
    for lk in leaks:
        if lk.rel not in budget:
            text = git_show(root, base, lk.rel)
            found = lk.rescan(lk.rel, text) if (text is not None and lk.rescan) else []
            budget[lk.rel] = Counter((f.name, f.text) for f in found)
        key = (lk.name, lk.text)   # the whole line, exactly: whitespace and \\r included
        if budget[lk.rel][key] > 0:
            budget[lk.rel][key] -= 1
            lk.pre_existing = True


def resolve(root: Path, base: str) -> bool:
    p = subprocess.run(["git", "-C", str(root), "rev-parse", "--verify", "--quiet", f"{base}^{{commit}}"],
                       capture_output=True, text=True)
    return p.returncode == 0


def run(root: Path, slugs: list[str], base: str | None, skills: dict) -> int:
    if base and not resolve(root, base):
        print(f"check_no_pp_leak: base {base!r} does not resolve to a commit; cannot tell new leaks "
              "from pre-existing ones. Fetch it, or run without --base for a full audit.", file=sys.stderr)
        return 2
    leaks: list[Leak] = []
    for s in slugs:
        leaks += scan_slug(root, s, skills.get(s, {}))
    leaks += scan_shared(root, slugs, skills)
    if base:
        mark_pre_existing(root, base, leaks)
    if READ_ERRORS:
        print("check_no_pp_leak: could not read every input (not a pass):")
        for e in READ_ERRORS:
            print(f"  {e}")
        return 2
    new = [lk for lk in leaks if not lk.pre_existing]
    old = [lk for lk in leaks if lk.pre_existing]
    for lk in new + old:
        print(lk.render())
    print(f"check_no_pp_leak: {len(slugs)} skill(s), {len(new)} new leak(s)"
          + (f", {len(old)} pre-existing at {base}" if base else ""))
    if new:
        print("FAIL: a connector's own -pp- name reaches a user-facing surface. Rename it to the "
              "shipped <slug>-cli / <slug>-mcp (go.mod and Go import paths keep -pp- on purpose).")
    return 1 if new else 0


def main(argv: list[str]) -> int:
    try:  # a finding may carry an undecodable byte (kept losslessly); never crash printing it
        sys.stdout.reconfigure(errors="backslashreplace")  # type: ignore[attr-defined]
    except (AttributeError, ValueError):
        pass
    ap = argparse.ArgumentParser(description="No connector's own -pp- name in user-facing files.")
    ap.add_argument("--slug", action="append", default=[])
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--base", help="ratchet: lines already present at this ref are pre-existing")
    ap.add_argument("--root", default=str(registry.ROOT))
    ap.add_argument("--self-test", action="store_true")
    a = ap.parse_args(argv)
    if a.self_test:
        return _self_test()
    root = Path(a.root).resolve()
    skills = json.loads((root / "tools/maintainer/skills.json").read_text(encoding="utf-8"))["skills"]
    slugs = sorted(skills) if a.all else a.slug
    if not slugs:
        ap.error("give --all or at least one --slug")
    for s in slugs:
        d = skills.get(s, {}).get("source_dir", s)
        if not SLUG_RE.match(s) or not (root / "skills" / str(d)).is_dir():
            ap.error(f"{s!r} is not a registered skill with a directory under skills/")
    return run(root, slugs, a.base, skills)


def _self_test() -> int:
    import contextlib
    import io

    failures: list[str] = []

    def quiet_run(*args: object) -> int:
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return run(*args)  # type: ignore[arg-type]

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    skills = {"acme": {"cli_binary": "acme-cli", "mcp_binary": "acme-mcp"},
              "other": {"cli_binary": "other-cli", "mcp_binary": "other-mcp"}}
    files = {
        "tools/maintainer/skills.json": json.dumps({"skills": skills}),
        "skills/acme/cli/go.mod": "module acme-pp-cli\n",
        "skills/acme/README.md": "# Acme\n\nRun `acme-cli doctor`. Posts through `slack-pp-cli`.\n",
        "skills/acme/manifest.json": '{"name": "acme-mcp"}\n',
        "skills/acme/handfixes.json": json.dumps({"handfixes": [{"id": "m", "asserts": [
            {"file": "manifest.json", "not_contains": "acme-pp-mcp"},
            {"file": "cli/internal/cli/root.go", "contains": "acme-pp-cli/internal/store"}]}]}),
        "skills/acme/cli/internal/cli/root.go":
            'package cli\n\nimport "acme-pp-cli/internal/store"\n\n// acme-pp-cli in a comment is not shown to anyone\n'
            'var use = "acme-cli"\nvar _ = store.X\nvar _ = "sample-pp-cli" // a generated test placeholder name\n',
        "skills/acme/cli/internal/cli/blk.go":
            'package cli\n\nimport (\n\t"fmt"\n\n\tx "acme-pp-cli/internal/cliutil"\n)\n\nvar _ = fmt.Sprint\nvar _ = x.Y\n',
        "README.md": "# Fleet\n\n| acme | `acme-cli` |\n",
        "catalog.json": "{}\n",
    }
    broken = {
        "skills/acme/guide.md": ("Config file: `~/.config/acme-press-pp-cli/config.toml`\n",
                                 "a press-library name in a connector's guide (24 live guides)"),
        "skills/acme/manifest.json": ('{"name": "acme-pp-mcp"}\n', "a manifest naming acme-pp-mcp (9 live)"),
        "skills/acme/cli/internal/cli/api.go": ('package cli\n\nvar hint = "Run \'%s-pp-cli api\' to list"\n',
                                                "a %s-pp- help string in Go (avanan, datagate)"),
        "skills/acme/cli/internal/cli/cfg.go": ('package cli\n\nimport "fmt"\n\nfunc p() { fmt.Println("Config: '
                                                '~/.config/acme-pp-cli/config.toml") }\n',
                                                "a config PATH in a Go string (not an import)"),
        "skills/acme/handfixes.json": (json.dumps({"handfixes": [{"id": "m", "asserts": [
            {"file": "README.md", "contains": "acme-pp-cli doctor"}]}]}),
            "a handfixes contains marker that forces a leak to survive"),
        "README.md": ("# Fleet\n\n| acme | `acme-pp-cli` |\n", "the root README naming acme-pp-cli"),
    }
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)

        def write(fs: dict, reset: bool = False) -> None:
            if reset:  # drop every broken-only file so each case starts clean
                for rel in broken:
                    if rel not in files and (root / rel).exists():
                        (root / rel).unlink()
            for rel, text in fs.items():
                (root / rel).parent.mkdir(parents=True, exist_ok=True)
                (root / rel).write_text(text, encoding="utf-8")

        def leaks() -> list[Leak]:
            return scan_slug(root, "acme", skills["acme"]) + scan_shared(root, ["acme"], skills)

        write(files)
        got = leaks()
        expect(not got, "a clean connector (import path, comment, not_contains marker, external "
                        f"slack-pp-cli, placeholder Go name) has no leak (got {[g.render() for g in got]})")
        for rel, (text, label) in broken.items():
            write(files, reset=True)
            write({rel: text})
            got = leaks()
            expect(len(got) == 1 and got[0].rel == rel and got[0].slug == "acme", f"{label} -> one leak in {rel}")
        write(files, reset=True)
        write({"skills/acme/cli/internal/cli/root.go": files["skills/acme/cli/internal/cli/root.go"]
               + 'var bad = "other-pp-cli"\n'})
        expect(not leaks(), "another connector's name in acme's Go is not acme's leak")

        # Ratchet: a line already present at --base is pre-existing, a new one blocks.
        write(files, reset=True)
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n"})

        def git(*args: str) -> None:
            subprocess.run(["git", "-C", str(root), *args], check=True, capture_output=True,
                           env={"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
                                "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
                                "HOME": tmp, "PATH": "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin"})
        git("init", "-q")
        git("add", "-A")
        git("commit", "-q", "-m", "base")
        rc = quiet_run(root, ["acme"], "HEAD", skills)
        expect(rc == 0, "--base: a leak that already existed at base is listed, not blocking")
        write({"skills/acme/SKILL.md": "Run `acme-pp-cli sync`.\n"})
        rc = quiet_run(root, ["acme"], "HEAD", skills)
        expect(rc == 1, "--base: a NEW leak line blocks even while an old one is grandfathered")
        # A new leak inside an UNCHANGED multi-line raw string still blocks.
        write({"skills/acme/cli/internal/cli/long.go": "package cli\n\nvar long = `Usage:\n  acme-cli doctor\n`\n"})
        git("add", "-A")
        git("commit", "-q", "-m", "a multi-line help string")
        write({"skills/acme/cli/internal/cli/long.go": "package cli\n\nvar long = `Usage:\n  acme-pp-cli doctor\n`\n"})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: a new leak on line 2 of an unchanged multi-line Go string blocks")
        write({"skills/acme/cli/internal/cli/long.go": "package cli\n\nvar long = `Usage:\n  acme-pp-cli doctor\n`\n"})
        git("add", "-A")
        git("commit", "-q", "-m", "a grandfathered leak in a multi-line string")
        write({"skills/acme/cli/internal/cli/long.go":
               "package cli\n\nvar long = `Usage:\n  acme-pp-cli doctor\n  acme-pp-cli sync\n`\n"})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: a NEW leak later in a string that already has a grandfathered one blocks")
        two = ('{"handfixes": [{"id": "a", "asserts": [\n'
               '  {"file": "old.md", "contains": "acme-pp-cli"}]},\n'
               ' {"id": "b", "asserts": [\n'
               '  {"file": "new.md",\n   "contains": "acme-pp-cli"}]}]}\n')
        got = scan_handfixes("h.json", two, "acme")
        expect([g.line for g in got] == [2, 5] and "new.md" in got[1].msg,
               "two identical markers each sit on their own line, with their own target")
        uni = '{"handfixes": [{"id": "a", "asserts": [{"file": "a.md", "contains": "acme' + chr(92) + 'u002dpp-cli"}]}]}\n'
        ekey = '{"handfixes": [{"id": "a", "asserts": [{"file": "guide.md", "cont' + chr(92) + 'u0061ins": "acme-pp-cli"}]}]}\n'
        expect(len(scan_handfixes("h.json", ekey, "acme")) == 1, "an escaped KEY (cont\\u0061ins) is still a contains marker")
        nc = '{"handfixes": [{"id": "a", "asserts": [{"file": "guide.md", "not_contains": "acme-pp-cli"}]}]}\n'
        expect(not scan_handfixes("h.json", nc, "acme"), "a not_contains marker is honored")
        got = scan_handfixes("h.json", uni, "acme")
        expect(len(got) == 1 and got[0].line == 1, "an escaped marker is decoded and keeps its real line")
        # An import line copied verbatim into a user-facing string is a NEW leak.
        imp = 'package cli\n\nimport (\n\t"acme-pp-cli/internal/store"\n)\n\nvar _ = store.X\n'
        write({"skills/acme/cli/internal/cli/long.go": imp})
        git("add", "-A")
        git("commit", "-q", "-m", "an import")
        write({"skills/acme/cli/internal/cli/long.go": imp + 'var help = `\n\t"acme-pp-cli/internal/store"\n`\n'})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: an import line copied into a help string is new (the base had no LEAK on it)")
        fake = ('package cli\n\n/*\nimport (\n*/\nvar hint = "~/.config/acme-pp-cli/config.toml"\n/*\n)\n*/\n')
        expect(len(scan_go("x.go", fake, "acme", {"acme"})) == 1,
               "an import block faked inside comments grants no exemption")
        real = 'package cli\n\nimport (\n\t// a comment ) with a paren\n\t"acme-pp-cli/internal/store"\n)\n'
        expect(not scan_go("x.go", real, "acme", {"acme"}), "a real import block (with a paren in a comment) is exempt")
        cr = 'package cli\n\nvar long = `Usage:\n  acme-\rpp-cli doctor\n`\n'
        got = scan_go("x.go", cr, "acme", {"acme"})
        expect(len(got) == 1 and got[0].line == 4,
               "a raw string with a carriage return inside the name (Go drops CR) is a leak, on its own line")
        jesc = '{"name": "acme' + chr(92) + 'u002dpp-mcp"}\n'
        expect(len(scan_doc("skills/acme/manifest.json", jesc, "acme")) == 1,
               "a JSON-escaped leak (\\u002d) in manifest.json is decoded and found")
        expect(len(scan_doc("skills/acme/README.md", "Run `acme&#45;pp-cli doctor`\n", "acme")) == 1,
               "an HTML entity in Markdown (&#45;) is decoded and found")
        expect(len(scan_shared_text("catalog.json", '{"bin": "acme' + chr(92) + 'u002dpp-cli"}\n', {"acme": "acme"})) == 1,
               "a JSON-escaped leak in a shared catalog is found and attributed")
        esc = 'package cli\n\nvar Help = "Run acme' + chr(92) + 'x2dpp-cli doctor"\n'
        expect(len(scan_go("x.go", esc, "acme", {"acme"})) == 1, "an escaped leak (\\x2d for '-') is still a leak")
        one = 'package cli\n\nimport x "acme-pp-cli/internal/store"; var Help = "~/.config/acme-pp-cli/config.toml"\n'
        got = scan_go("x.go", one, "acme", {"acme"})
        expect(len(got) == 1 and "config.toml" in got[0].text,
               "a single-line import exempts only its own path, not a string later on the line")
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n" * 2})
        write({"skills/acme/cli/internal/cli/long.go": imp})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: a SECOND copy of a grandfathered leak line is new (counted, not a set)")
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n"})
        write({"skills/acme/cli/internal/cli/long.go": imp})
        write({"skills/acme/guide.md": "Config file:  `~/.config/acme-press-pp-cli/config.toml`\n"})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: whitespace changed on a grandfathered leak line -> new (exact line)")
        (root / "skills/acme/guide.md").write_bytes(b"Config file: `~/.config/acme-press-pp-cli/config.toml`\r\n")
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: a grandfathered leak line that gained a carriage return -> new")
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n"})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 0, "--base: the untouched leak line is still pre-existing")
        (root / "skills/acme/guide.md").write_bytes(b"Config file: `~/.config/acme-press-pp-cli/config.toml` \xfe\n")
        git("add", "-A")
        git("commit", "-q", "-m", "an invalid byte on a leak line")
        (root / "skills/acme/guide.md").write_bytes(b"Config file: `~/.config/acme-press-pp-cli/config.toml` \xff\n")
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: an invalid byte that CHANGED on a leak line -> new (lossless decoding)")
        write({"skills/acme/cli/internal/cli/long.go": "package cli\n\nvar long = `Usage:\n  acme-cli doctor\n`\n"})
        write({"skills/acme/handfixes.json": broken["skills/acme/handfixes.json"][0]})
        git("add", "-A")
        git("commit", "-q", "-m", "a contains marker that keeps a leak")
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 0,
               "--base: an UNCHANGED handfixes contains marker is pre-existing, not new")
        rc = quiet_run(root, ["acme"], None, skills)
        expect(rc == 1, "no --base: a full audit fails on every leak")
        expect(quiet_run(root, ["acme"], "no-such-ref", skills) == 2, "an unresolvable --base is an error, never a pass")
        locked = root / "skills/acme/SKILL.md"
        locked.write_text("Run acme-pp-cli\n", encoding="utf-8")
        locked.chmod(0)
        unreadable = not os.access(locked, os.R_OK)
        if unreadable:
            expect(quiet_run(root, ["acme"], None, skills) == 2, "an unreadable input is an error, never a pass")
        READ_ERRORS.clear()
        locked.chmod(0o644)
        locked.unlink()
        assets = root / "skills/acme/assets/help"
        assets.mkdir(parents=True, exist_ok=True)
        (assets / "usage.md").write_text("Run acme-pp-cli doctor\n", encoding="utf-8")
        (root / "skills/acme/references").mkdir(parents=True, exist_ok=True)
        os.symlink("../assets/help", root / "skills/acme/references/help")
        os.symlink("..", assets / "loop")  # a cycle must end
        expect(quiet_run(root, ["acme"], None, skills) == 1, "a page under a linked references/ folder is read")
        git("add", "-A")
        git("commit", "-q", "-m", "a linked references folder with a leak")
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 0,
               "--base: an unchanged page under a linked folder is pre-existing (the base tree's link is followed)")
        (root / "skills/acme/references/help").unlink()
        (assets / "loop").unlink()
        (assets / "usage.md").unlink()
        (root / "skills/acme/references/usage.html").write_text("<p>Run acme-pp-cli doctor.</p>\n", encoding="utf-8")
        (root / "skills/acme/references/esc.md").write_text("Run acme\\-pp\\-cli doctor.\n", encoding="utf-8")
        (root / "skills/acme/references/usage.markdown").write_text("Run acme-pp-cli doctor.\n", encoding="utf-8")
        (root / "skills/acme/references/LOUD.MD").write_text("Run acme-pp-cli doctor.\n", encoding="utf-8")
        found = {lk.rel.rsplit("/", 1)[-1] for lk in scan_slug(root, "acme", skills["acme"])}
        expect({"usage.html", "esc.md", "usage.markdown", "LOUD.MD"} <= found,
               "HTML, .markdown and upper-case .MD pages, and a Markdown-escaped name, are read as rendered")
        for f in ("usage.html", "esc.md", "usage.markdown", "LOUD.MD"):
            (root / "skills/acme/references" / f).unlink()
        gm = root / "skills/acme/cli/go.mod"
        old_mod = gm.read_text(encoding="utf-8") if gm.is_file() else None
        gm.parent.mkdir(parents=True, exist_ok=True)
        gm.write_text("module github.com/example/x360recover-pp-cli\n\ngo 1.26\n", encoding="utf-8")
        expect("x360recover" in own_names(root, "acme", {}), "a qualified module path names its last element")
        if old_mod is None:
            gm.unlink()
        else:
            gm.write_text(old_mod, encoding="utf-8")
        for sub in ("references", "cli/internal"):
            d = root / "skills/acme" / sub
            d.mkdir(parents=True, exist_ok=True)
            (d / ("x.md" if sub == "references" else "x.go")).write_text("acme-pp-cli\n", encoding="utf-8")
            d.chmod(0)
            if not os.access(d, os.R_OK):
                expect(quiet_run(root, ["acme"], None, skills) == 2,
                       f"an unlistable {sub}/ directory is an error, never a pass")
            READ_ERRORS.clear()
            d.chmod(0o755)
    if failures:
        print(f"\nFAIL: check_no_pp_leak self-test - {len(failures)} case(s)")
        return 1
    print("\nPASS: check_no_pp_leak - fires on every leak shape, silent on the clean twin, ratchet holds")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
