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
import json
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
                names.add(re.sub(r"-pp-(?:cli|mcp)$", "", line.split()[1].strip()))
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


def _scan_text(rel: str, text: str, slug: str, rx: re.Pattern) -> list[Leak]:
    out = []
    for i, line in enumerate(text.splitlines(), 1):
        for m in rx.finditer(line):
            if m.group(0) not in EXTERNAL_PP_TOOLS:
                out.append(Leak(rel, i, slug, m.group(0), line))
                break
    return out


def _read(p: Path) -> str | None:
    try:
        return p.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return None


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
            e = code.find("\n", i)
            spans.append((i, e if e >= 0 else len(code)))
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
    if "-pp-" not in src:
        return []
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
        for m in (rx_import if in_import else rx_other).finditer(lit.value):
            # A raw `...` string spans lines and maps 1:1 onto the source; an
            # interpreted "..." string is one line.
            at = lit.start + 1 + m.start() if src[lit.start] == "`" else lit.start
            ln = src.count("\n", 0, at) + 1
            if ln in seen_lines:
                continue
            seen_lines.add(ln)
            s0 = src.rfind("\n", 0, at) + 1
            e0 = src.find("\n", at)
            out.append(Leak(rel, ln, slug, m.group(0), src[s0:e0 if e0 >= 0 else len(src)]))
    return out


def scan_handfixes(rel: str, text: str, slug: str) -> list[Leak]:
    """A `contains` (or `regex`) marker on a non-Go file that requires a leak
    keeps it alive; a `not_contains` marker is the assertion it is gone."""
    try:
        data = json.loads(text)
    except ValueError:
        return []
    out: list[Leak] = []
    lines = text.splitlines()
    for entry in data.get("handfixes", []) if isinstance(data, dict) else []:
        for a in entry.get("asserts", []) if isinstance(entry, dict) else []:
            if not isinstance(a, dict):
                continue
            target = str(a.get("file", ""))
            if target.endswith((".go", "go.mod", "go.sum")):
                continue
            for key in ("contains", "regex"):
                v = a.get(key)
                m = ANY_PP_NAME.search(v) if isinstance(v, str) else None
                if m and m.group(0) not in EXTERNAL_PP_TOOLS:
                    needle = json.dumps(v)[1:-1]
                    ln = next((i for i, ln_ in enumerate(lines, 1) if needle in ln_), 0)
                    raw = lines[ln - 1] if ln else needle
                    out.append(Leak(rel, ln, slug, m.group(0), raw,
                                    f"{key} marker on {target} requires the leak to stay: {v}"))
    return out


def scan_shared_text(rel: str, text: str, owner: dict[str, str]) -> list[Leak]:
    rx = leak_re(set(owner), go=False)
    out: list[Leak] = []
    for i, line in enumerate(text.splitlines(), 1):
        for m in rx.finditer(line):
            base = re.sub(r"-pp-(?:cli|mcp)$", "", m.group(0))
            out.append(Leak(rel, i, owner.get(base, "?"), m.group(0), line))
    return out


def scan_slug(root: Path, slug: str, meta: dict) -> list[Leak]:
    sd = skill_dir(root, slug, meta)
    names = own_names(root, sd.name, meta) | {slug}
    leaks: list[Leak] = []
    files = sorted(p for p in sd.glob("*.md")) + sorted((sd / "references").rglob("*.md"))
    files += [sd / f for f in SKILL_FILES]
    files += [root / "docs" / "skills" / f"{slug}.md"]
    for p in files:
        text = _read(p) if p.is_file() else None
        if text is not None:
            rel = str(p.relative_to(root))
            leaks += _tag(scan_doc(rel, text, slug), lambda r, t, s=slug: scan_doc(r, t, s))
    for sub in ("cli/internal", "cli/cmd"):
        for p in sorted((sd / sub).rglob("*.go")) if (sd / sub).is_dir() else []:
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


def git_show(root: Path, base: str, rel: str) -> str | None:
    p = subprocess.run(["git", "-C", str(root), "show", f"{base}:{rel}"],
                       capture_output=True, text=True, errors="replace")
    return p.stdout if p.returncode == 0 else None


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
            budget[lk.rel] = Counter((f.name, f.text.strip()) for f in found)
        key = (lk.name, lk.text.strip())
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
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n" * 2})
        write({"skills/acme/cli/internal/cli/long.go": imp})
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 1,
               "--base: a SECOND copy of a grandfathered leak line is new (counted, not a set)")
        write({"skills/acme/guide.md": "Config file: `~/.config/acme-press-pp-cli/config.toml`\n"})
        write({"skills/acme/cli/internal/cli/long.go": "package cli\n\nvar long = `Usage:\n  acme-cli doctor\n`\n"})
        write({"skills/acme/handfixes.json": broken["skills/acme/handfixes.json"][0]})
        git("add", "-A")
        git("commit", "-q", "-m", "a contains marker that keeps a leak")
        expect(quiet_run(root, ["acme"], "HEAD", skills) == 0,
               "--base: an UNCHANGED handfixes contains marker is pre-existing, not new")
        rc = quiet_run(root, ["acme"], None, skills)
        expect(rc == 1, "no --base: a full audit fails on every leak")
        expect(quiet_run(root, ["acme"], "no-such-ref", skills) == 2, "an unresolvable --base is an error, never a pass")
    if failures:
        print(f"\nFAIL: check_no_pp_leak self-test - {len(failures)} case(s)")
        return 1
    print("\nPASS: check_no_pp_leak - fires on every leak shape, silent on the clean twin, ratchet holds")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
