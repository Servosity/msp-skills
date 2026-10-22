#!/usr/bin/env python3
"""Lint connector tests for assertions that only hold on Unix.

The ci.yml `windows` job runs every changed connector's tests on a real Windows
runner, but only AFTER the pull request exists. Each Unix-only assertion found
there cost a full CI round (backup-radar #408, td-synnex #410, proxmox #398,
watchguard #393/#396). This lint finds the same classes before the PR, from the
source alone, on any OS:

  errno-text      a test compares an error message against POSIX errno text
                  ("connection refused", "no such file or directory"...).
                  Windows words these differently (td-synnex #410).
  mode-bits       a test chmods a path to a group/other-accessible or
                  read-only mode, or asserts Perm() bits. Windows has no Unix
                  mode bits (td-synnex #410, proxmox #398).
  shebang-stub    a test helper writes a "#!/..." script for tests to run.
                  Windows cannot execute it (watchguard #393/#396).
  open-file       a test leaves a file it created open (never closed, or
                  closed only after a rename). Windows refuses to remove or
                  rename an open file, so t.TempDir cleanup fails (td-synnex #410).
  goos-literal    a test compares the result of a same-package function that
                  branches on runtime.GOOS against one literal (backup-radar #408).
  clock-key       non-test code uses a time.Now() RFC3339Nano stamp as the key
                  of an INSERT OR IGNORE. Two fast calls can read the same tick
                  on Windows, so the second row is silently dropped (watchguard #396).

Narrow on purpose: a noisy lint is worse than none. A finding is skipped when
its function mentions runtime.GOOS, or skips itself with a reason that names
Windows (the author already handled the platform),
when the file does not build on windows (a GOOS file suffix or a //go:build line
that excludes windows), and when the test is a recorded known Windows failure in
tools/maintainer/windows_known_test_failures.json (reported as known, never
blocking - the same split the windows CI job makes).

    python3 tools/maintainer/check_unix_test_asserts.py --all
    python3 tools/maintainer/check_unix_test_asserts.py --slug proxmox [--slug ...]
    python3 tools/maintainer/check_unix_test_asserts.py --root DIR --slug S
        scan DIR/skills/S/cli instead of this checkout (proof runs on old commits)
    python3 tools/maintainer/check_unix_test_asserts.py --self-test

Exit 0 = no new finding, 1 = finding(s), 2 = usage or environment error.
Output lines are `skills/<slug>/cli/<file>:<line>: [<slug>] <rule> in <func>: <why>`.
"""

from __future__ import annotations

import argparse
import bisect
import codecs
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
sys.path.insert(0, str(HERE))
import windows_known_failures  # noqa: E402  (local tools/ module)

SLUG_RE = re.compile(r"\A[a-z0-9][a-z0-9-]*\Z")

# --------------------------------------------------------------------------- #
# A small Go lexer: enough to find string literals and top-level function
# bodies without being fooled by braces or quotes inside strings and comments.
# --------------------------------------------------------------------------- #


class Lit:
    __slots__ = ("start", "end", "value")

    def __init__(self, start: int, end: int, value: str) -> None:
        self.start, self.end, self.value = start, end, value


def _decode(body: str) -> str:
    try:
        return codecs.decode(body.encode("utf-8", "backslashreplace"), "unicode_escape")
    except (UnicodeDecodeError, ValueError):
        return body


def lex(src: str) -> tuple[list[Lit], list[tuple[int, str]]]:
    """(string literals, structural chars) where structural chars are the
    offsets of every { } ( ) outside strings, runes and comments."""
    lits: list[Lit] = []
    marks: list[tuple[int, str]] = []
    i, n = 0, len(src)
    while i < n:
        c = src[i]
        if c == "/" and src.startswith("//", i):
            j = src.find("\n", i)
            i = n if j < 0 else j
            continue
        if c == "/" and src.startswith("/*", i):
            j = src.find("*/", i + 2)
            i = n if j < 0 else j + 2
            continue
        if c == "`":
            j = src.find("`", i + 1)
            j = n - 1 if j < 0 else j
            lits.append(Lit(i, j + 1, src[i + 1:j].replace("\r", "")))  # Go drops CR in raw strings
            i = j + 1
            continue
        if c in "\"'":
            j = i + 1
            while j < n and src[j] != c and src[j] != "\n":
                j += 2 if src[j] == "\\" else 1
            if c == '"':
                lits.append(Lit(i, j + 1, _decode(src[i + 1:j])))
            i = j + 1
            continue
        if c in "{}()":
            marks.append((i, c))
        i += 1
    return lits, marks


def code_mask(src: str) -> str:
    """src with every comment and every string/rune literal BODY replaced by
    spaces (newlines and the quote characters kept), so offsets still line up
    and a keyword search sees only real code."""
    out = list(src)
    i, n = 0, len(src)

    def blank(a: int, b: int) -> None:
        for k in range(a, b):
            if out[k] != "\n":
                out[k] = " "

    while i < n:
        c = src[i]
        if c == "/" and src.startswith("//", i):
            j = src.find("\n", i)
            j = n if j < 0 else j
            blank(i, j)
            i = j
            continue
        if c == "/" and src.startswith("/*", i):
            j = src.find("*/", i + 2)
            j = n if j < 0 else j + 2
            blank(i, j)
            i = j
            continue
        if c == "`":
            j = src.find("`", i + 1)
            j = n - 1 if j < 0 else j
            blank(i + 1, j)
            i = j + 1
            continue
        if c in "\"'":
            j = i + 1
            while j < n and src[j] != c and src[j] != "\n":
                j += 2 if src[j] == "\\" else 1
            blank(i + 1, min(j, n))
            i = j + 1
            continue
        i += 1
    return "".join(out)


class Func:
    __slots__ = ("name", "start", "end", "line")

    def __init__(self, name: str, start: int, end: int, line: int) -> None:
        self.name, self.start, self.end, self.line = name, start, end, line


_FUNC_RE = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)", re.M)


def functions(src: str, marks: list[tuple[int, str]]) -> list[Func]:
    """Top-level functions. The body opens at the first `{` after the name
    that ends a line (gofmt style), which skips braces inside the signature's
    types (`interface{}`, `struct{...}`)."""
    out: list[Func] = []
    brace_idx = {off: k for k, (off, ch) in enumerate(marks)}
    offsets = [off for off, _ in marks]
    for m in _FUNC_RE.finditer(src):
        open_at = -1
        for k in range(bisect.bisect_left(offsets, m.end()), len(marks)):
            off, ch = marks[k]
            if ch != "{":
                continue
            eol = src.find("\n", off)
            rest = src[off + 1:eol if eol >= 0 else len(src)].strip()
            if rest == "" or rest.startswith("//"):
                open_at = off
                break
            if src.rfind("\n", 0, off) < m.start():
                # a one-line body on the func line: `func f() { return x }`
                open_at = off
                break
        if open_at < 0:
            continue
        depth, end = 0, -1
        for off, ch in marks[brace_idx[open_at]:]:
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    end = off
                    break
        if end < 0:
            continue
        out.append(Func(m.group(1), open_at, end, src.count("\n", 0, m.start()) + 1))
    return out


# --------------------------------------------------------------------------- #
# Build constraints: does this file compile for GOOS=windows?
# --------------------------------------------------------------------------- #

GOOS = {"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js",
        "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos"}
GOARCH = {"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle",
          "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}


def _eval_constraint(expr: str) -> bool:
    toks = re.findall(r"!|&&|\|\||\(|\)|[A-Za-z0-9_.]+", expr)
    pos = 0

    def tag(t: str) -> bool:
        if t in ("windows", "amd64") or t.startswith("go1"):
            return True
        return False  # unix, cgo, linux, darwin, custom tags: not set by a plain windows run

    def p_or() -> bool:
        nonlocal pos
        v = p_and()
        while pos < len(toks) and toks[pos] == "||":
            pos += 1
            v = p_and() or v
        return v

    def p_and() -> bool:
        nonlocal pos
        v = p_not()
        while pos < len(toks) and toks[pos] == "&&":
            pos += 1
            v = p_not() and v
        return v

    def p_not() -> bool:
        nonlocal pos
        if pos < len(toks) and toks[pos] == "!":
            pos += 1
            return not p_not()
        if pos < len(toks) and toks[pos] == "(":
            pos += 1
            v = p_or()
            pos += 1  # ')'
            return v
        t = toks[pos] if pos < len(toks) else ""
        pos += 1
        return tag(t)

    try:
        return p_or()
    except IndexError:
        return True  # unparseable: assume it builds, so the lint still looks


def builds_on_windows(path: Path, src: str) -> bool:
    stem = path.name[:-3] if path.name.endswith(".go") else path.name
    if stem.endswith("_test"):
        stem = stem[:-5]
    parts = stem.split("_")
    if len(parts) >= 2:
        last, prev = parts[-1], parts[-2]
        if last in GOARCH and prev in GOOS:
            if prev != "windows":
                return False
        elif last in GOOS and last != "windows":
            return False
    for line in src.splitlines():
        s = line.strip()
        if s.startswith("package "):
            break
        if s.startswith("//go:build "):
            return _eval_constraint(s[len("//go:build "):])
    return True


# --------------------------------------------------------------------------- #
# Rules
# --------------------------------------------------------------------------- #

# Only wording the OS produces. "not a directory", "is a directory", "file
# exists" and "permission denied" are left out on purpose: connectors write
# those phrases into their OWN messages (a 403 mapped to "permission denied",
# SetHomeOverride's "is not a directory"), and those pass on Windows.
ERRNO_TEXT = (
    "connection refused", "no such file or directory", "directory not empty",
    "operation not permitted", "broken pipe", "address already in use",
    "read-only file system", "too many open files", "bad file descriptor",
    "text file busy", "connection reset by peer", "no route to host",
    "network is unreachable", "resource temporarily unavailable",
)
_COMPARE = re.compile(r"\b(?:Contains|HasPrefix|HasSuffix|EqualError|ErrorContains|MatchString)\(|[!=]=")
_OCTAL = re.compile(r"(?<![\w.])0[oO]?([0-7]{3,4})\b")
_CREATE_CALL = re.compile(r"\b(?:WriteFile|MkdirAll|Mkdir|OpenFile|MkdirTemp)\(")
_CHMOD = re.compile(r"\b(?:os\.Chmod|os\.Lchmod|\.Chmod)\(")
_PERM_ASSERT = re.compile(r"\.(?:Perm\(\)|Mode\(\))\s*(?:&\s*0[oO]?[0-7]+\s*)?(?:!=|==)\s*0[oO]?[0-7]{3,4}\b"
                          r"|(?:!=|==)\s*\w*\.?(?:Perm\(\)|Mode\(\)\.Perm\(\))")
_OPEN = re.compile(r"\b([A-Za-z_]\w*)\s*,\s*[A-Za-z_]\w*\s*:?=\s*os\.(?:Create|CreateTemp|OpenFile)\(")
_GOOS = re.compile(r"\bruntime\.GOOS\b|\bgoruntime\.GOOS\b")
_SKIP = re.compile(r"\bt\.Skipf?\(")


def platform_guarded(body: str) -> bool:
    """The author already handled the platform: the function branches on
    runtime.GOOS, or it skips itself and says why in terms of Windows (the
    press's own read-only-directory probe does exactly that)."""
    if _GOOS.search(body):
        return True
    return bool(_SKIP.search(body)) and "windows" in body.lower()


def loose_mode(digits: str) -> bool:
    """A mode Windows cannot honor: group/other bits set, or no owner write."""
    v = int(digits, 8) & 0o777
    return bool(v & 0o077) or not (v & 0o200)


class Finding:
    __slots__ = ("slug", "rel", "line", "rule", "func", "why", "known")

    def __init__(self, slug: str, rel: str, line: int, rule: str, func: str, why: str) -> None:
        self.slug, self.rel, self.line, self.rule, self.func, self.why = slug, rel, line, rule, func, why
        self.known = False

    def render(self) -> str:
        tag = " (known Windows failure, ledgered)" if self.known else ""
        return f"{self.rel}:{self.line}: [{self.slug}] {self.rule} in {self.func}: {self.why}{tag}"

    def as_dict(self) -> dict:
        return {"slug": self.slug, "file": self.rel, "line": self.line, "rule": self.rule,
                "func": self.func, "why": self.why, "known": self.known}


def _line_of(src: str, off: int) -> int:
    return src.count("\n", 0, off) + 1


def _line_text(src: str, off: int) -> str:
    s = src.rfind("\n", 0, off) + 1
    e = src.find("\n", off)
    return src[s:e if e >= 0 else len(src)]


def goos_branching_funcs(non_test_srcs: list[str]) -> set[str]:
    """Names of package-level functions whose body branches on runtime.GOOS."""
    names: set[str] = set()
    for src in non_test_srcs:
        _, marks = lex(src)
        for f in functions(src, marks):
            if _GOOS.search(src[f.start:f.end]):
                names.add(f.name)
    return names


def lint_test_file(slug: str, rel: str, src: str, goos_funcs: set[str]) -> list[Finding]:
    out: list[Finding] = []
    lits, marks = lex(src)
    for f in functions(src, marks):
        body = src[f.start:f.end]
        if platform_guarded(body):
            continue
        flits = [lit for lit in lits if f.start <= lit.start < f.end]

        def add(off: int, rule: str, why: str) -> None:
            out.append(Finding(slug, rel, _line_of(src, off), rule, f.name, why))

        # errno-text: POSIX errno wording inside a comparison.
        for lit in flits:
            low = lit.value.lower()
            hit = next((e for e in ERRNO_TEXT if e in low), None)
            line = _line_text(src, lit.start)
            if hit and _COMPARE.search(line) and re.search(r"\berr\w*\b|\.Error\(\)", line):
                add(lit.start, "errno-text", f"compares an error against the POSIX text {hit!r}")
                break

        # shebang-stub: a helper that writes a #! script for tests to RUN.
        # Only helpers: a Test that writes its own stub as a tripwire (the
        # press's teach-playbook test, which expects a refusal before any
        # exec) still detects its regression on Windows, by another route.
        if not f.name.startswith("Test"):
            for lit in flits:
                if lit.value.startswith("#!/"):
                    add(lit.start, "shebang-stub", "writes a #! script for tests to run; Windows cannot execute it")
                    break

        # mode-bits: chmod to a mode Windows cannot honor, or a Perm() assert.
        chmods = list(_CHMOD.finditer(body))
        flagged = False
        for cm in chmods:
            line = _line_text(src, f.start + cm.start())
            mm = _OCTAL.search(line[line.find(cm.group(0)):])
            if mm and loose_mode(mm.group(1)):
                add(f.start + cm.start(), "mode-bits",
                    f"chmods to 0o{mm.group(1)}; Windows has no group/other or read-only mode bits")
                flagged = True
                break
        if not flagged and chmods:
            for om in _OCTAL.finditer(body):
                line = _line_text(src, f.start + om.start())
                if _CREATE_CALL.search(line) or _CHMOD.search(line):
                    continue
                if loose_mode(om.group(1)):
                    add(f.start + chmods[0].start(), "mode-bits",
                        f"chmods to a mode from data that includes 0o{om.group(1)}; "
                        "Windows has no group/other or read-only mode bits")
                    flagged = True
                    break
        if not flagged:
            pm = _PERM_ASSERT.search(body)
            if pm:
                add(f.start + pm.start(), "mode-bits", "asserts Unix permission bits; Windows reports 0666")

        # open-file: created file never closed, or renamed before close.
        for om in _OPEN.finditer(body):
            var = om.group(1)
            if var == "_":
                continue
            rest = body[om.end():]
            close = re.search(rf"\b{re.escape(var)}\.Close\(", rest)
            returned = re.search(rf"\breturn\b[^\n]*\b{re.escape(var)}\b", rest)
            if close is None and returned is None:
                add(f.start + om.start(), "open-file",
                    f"{var} is never closed; Windows cannot remove an open file, so TempDir cleanup fails")
                continue
            ren = re.search(rf"\bos\.Rename\(\s*{re.escape(var)}\.Name\(\)", rest)
            if ren and close is not None and ren.start() < close.start():
                add(f.start + om.start(), "open-file",
                    f"{var} is renamed before it is closed; Windows refuses to rename an open file")

        # goos-literal: a GOOS-branching function's result compared to one literal.
        if goos_funcs:
            for name in goos_funcs:
                n = re.escape(name)
                lit = r"(?:`[^`]*`|\"(?:[^\"\\]|\\.)*\")"
                shapes = (
                    rf"\w+\s*,\s*\w+\s*:?=\s*(?<![\w.]){n}\([^;]*\)\s*,\s*{lit}",      # got, want := F(x), "lit"
                    rf"(?<![\w.]){n}\([^;{{]*\)\s*[!=]=\s*{lit}",                     # F(x) != "lit"
                    rf"\b(\w+)\s*:=\s*(?<![\w.]){n}\([^;]*\)\s*;\s*\1\s*[!=]=\s*{lit}",  # got := F(x); got != "lit"
                )
                for cm in re.finditer(rf"(?<![\w.]){n}\(", body):
                    line = _line_text(src, f.start + cm.start())
                    if any(re.search(sh, line) for sh in shapes):
                        add(f.start + cm.start(), "goos-literal",
                            f"compares {name}(), which branches on runtime.GOOS, against one literal")
                        break
                else:
                    continue
                break
    return out


_CLOCK_STAMP = re.compile(r"\b([A-Za-z_]\w*)\s*:?=\s*time\.Now\(\)[\w.()]*\.Format\(time\.RFC3339Nano\)")


def lint_source_file(slug: str, rel: str, src: str) -> list[Finding]:
    out: list[Finding] = []
    if "INSERT OR IGNORE" not in src or "RFC3339Nano" not in src:
        return out
    _, marks = lex(src)
    for f in functions(src, marks):
        body = src[f.start:f.end]
        if "INSERT OR IGNORE" not in body:
            continue
        for sm in _CLOCK_STAMP.finditer(body):
            var = sm.group(1)
            if re.search(rf"INSERT OR IGNORE[^\n]*\n?[^\n]*\b{re.escape(var)}\b", body):
                out.append(Finding(slug, rel, _line_of(src, f.start + sm.start()), "clock-key", f.name,
                                   f"keys an INSERT OR IGNORE on the time.Now() stamp {var}; two fast "
                                   "calls can share one Windows clock tick and the second row is dropped"))
                break
    return out


# --------------------------------------------------------------------------- #
# Driver
# --------------------------------------------------------------------------- #


def _is_generated(src: str) -> bool:
    head = src[:2048]
    return "Code generated" in head and "DO NOT EDIT" in head


def lint_slug(root: Path, slug: str) -> list[Finding]:
    cli = root / "skills" / slug / "cli"
    if not (cli / "go.mod").is_file():
        return []
    findings: list[Finding] = []
    by_dir_tests: dict[Path, list[Path]] = {}
    by_dir_srcs: dict[Path, list[str]] = {}
    for p in sorted(cli.rglob("*.go")):
        if any(part in ("vendor", "testdata") or part.startswith(".") for part in p.relative_to(cli).parts[:-1]):
            continue
        try:
            src = p.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        if not builds_on_windows(p, src):
            continue
        rel = str(p.relative_to(root))
        if p.name.endswith("_test.go"):
            by_dir_tests.setdefault(p.parent, []).append(p)
        else:
            by_dir_srcs.setdefault(p.parent, []).append(src)
            if not _is_generated(src):
                findings += lint_source_file(slug, rel, src)
    for d, tests in by_dir_tests.items():
        goos_funcs = goos_branching_funcs(by_dir_srcs.get(d, []))
        for p in tests:
            src = p.read_text(encoding="utf-8", errors="replace")
            findings += lint_test_file(slug, str(p.relative_to(root)), src, goos_funcs)
    return findings


def all_connector_slugs(root: Path) -> list[str]:
    return sorted(p.parent.parent.name for p in (root / "skills").glob("*/cli/go.mod"))


def mark_known(findings: list[Finding], ledger_entries: list[dict] | None) -> None:
    if not ledger_entries:
        return
    for f in findings:
        if f.func in windows_known_failures.tests_for(f.slug, ledger_entries):
            f.known = True


def run(root: Path, slugs: list[str], use_ledger: bool, as_json: bool) -> int:
    entries = None
    if use_ledger:
        try:
            entries = windows_known_failures.load()
        except windows_known_failures.LedgerError as e:
            print(f"check_unix_test_asserts: cannot read the Windows known-failures ledger: {e}",
                  file=sys.stderr)
            return 2
    findings: list[Finding] = []
    for s in slugs:
        findings += lint_slug(root, s)
    mark_known(findings, entries)
    new = [f for f in findings if not f.known]
    known = [f for f in findings if f.known]
    if as_json:
        print(json.dumps({"new": [f.as_dict() for f in new], "known": [f.as_dict() for f in known]}, indent=1))
    else:
        for f in new + known:
            print(f.render())
        print(f"check_unix_test_asserts: {len(slugs)} connector(s), {len(new)} new finding(s), "
              f"{len(known)} ledgered known Windows failure(s)")
        if new:
            print("FAIL: guard each with `if runtime.GOOS == \"windows\" { ... }` (or assert the Windows "
                  "behavior), or record a diagnosed failure in tools/maintainer/windows_known_test_failures.json.")
    return 1 if new else 0


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description="Lint connector tests for Unix-only assertions.")
    ap.add_argument("--slug", action="append", default=[])
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--root", default=str(REPO), help="repository root to scan (default: this checkout)")
    ap.add_argument("--no-ledger", action="store_true",
                    help="do not excuse ledgered known Windows failures (proof runs)")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--self-test", action="store_true")
    a = ap.parse_args(argv)
    if a.self_test:
        return _self_test()
    root = Path(a.root).resolve()
    if a.all:
        slugs = all_connector_slugs(root)
    else:
        slugs = a.slug
    if not slugs:
        ap.error("give --all or at least one --slug")
    for s in slugs:
        if not SLUG_RE.match(s):
            ap.error(f"{s!r} is not a skill slug")
    return run(root, slugs, not a.no_ledger, a.json)


# Each case is (label, file name, source, expected rules). The broken sources
# are trimmed copies of the real failures; the fixed ones are their real fixes.
_ST_PKG_SRC = """package cli

import "runtime"

func shellQuote(s string) string {
\tif runtime.GOOS == "windows" {
\t\treturn psQuote(s)
\t}
\treturn "'" + s + "'"
}
"""
_ST_CASES = (
    ("backup-radar #408 TestShellQuote", "a_test.go",
     "package cli\nfunc TestShellQuote(t *testing.T) {\n\tif got, want := shellQuote(`x`), `'x'`; got != want {\n\t\tt.Errorf(\"%s\", got)\n\t}\n}\n",
     {"goos-literal"}),
    ("backup-radar #408 fixed", "a_test.go",
     "package cli\nfunc TestShellQuote(t *testing.T) {\n\twant := `'x'`\n\tif runtime.GOOS == \"windows\" {\n\t\twant = `'x'`\n\t}\n\tif got := shellQuote(`x`); got != want {\n\t\tt.Errorf(\"%s\", got)\n\t}\n}\n",
     set()),
    ("td-synnex #410 errno text", "a_test.go",
     "package cli\nfunc TestRefused(t *testing.T) {\n\t_, err := post()\n\tif err == nil || !strings.Contains(err.Error(), \"connection failed: connection refused\") {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"errno-text"}),
    ("an app's own 'permission denied' message is not errno text", "a_test.go",
     "package cli\nfunc TestHint(t *testing.T) {\n\tif h := hint(err); !strings.Contains(h, \"permission denied\") {\n\t\tt.Fatal(h)\n\t}\n}\n",
     set()),
    ("td-synnex #410 unsafe-credentials mode table", "a_test.go",
     "package cli\nfunc TestUnsafe(t *testing.T) {\n\tsetup := func(m os.FileMode) {\n\t\tif err := os.Chmod(p, m); err != nil {\n\t\t\tt.Fatal(err)\n\t\t}\n\t}\n\tfor _, tc := range []struct{ mode os.FileMode }{{0o600}, {0o644}} {\n\t\tsetup(tc.mode)\n\t}\n}\n",
     {"mode-bits"}),
    ("proxmox #398 group-writable dir", "a_test.go",
     "package cli\nfunc TestWritableDir(t *testing.T) {\n\tif err := os.Chmod(dir, 0o770); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"mode-bits"}),
    ("proxmox #398 fixed (GOOS skip)", "a_test.go",
     "package cli\nfunc TestWritableDir(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\tt.Skip(\"POSIX\")\n\t}\n\tif err := os.Chmod(dir, 0o770); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
     set()),
    ("a private 0600 chmod is portable", "a_test.go",
     "package cli\nfunc TestPrivate(t *testing.T) {\n\tos.WriteFile(p, b, 0o644)\n\tif err := os.Chmod(p, 0o600); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
     set()),
    ("a Perm() assertion", "a_test.go",
     "package cli\nfunc TestPerm(t *testing.T) {\n\tif fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(fi)\n\t}\n}\n",
     {"mode-bits"}),
    ("a probe-and-skip that names Windows (the press's read-only-dir test)", "a_test.go",
     "package cli\nfunc TestRO(t *testing.T) {\n\tos.Chmod(dir, 0o555)\n\tif probe() {\n\t\t// Root and Windows can still create files here.\n\t\tt.Skip(\"stayed writable\")\n\t}\n}\n",
     set()),
    ("watchguard #393 shebang stub helper", "a_test.go",
     "package mcp\nfunc callWithArgvStub(t *testing.T) string {\n\tstub := filepath.Join(t.TempDir(), \"x-cli\")\n\tos.WriteFile(stub, []byte(\"#!/bin/sh\\necho ARGV\\n\"), 0o700)\n\treturn run(stub)\n}\n",
     {"shebang-stub"}),
    ("watchguard #396 fixed (.bat on Windows)", "a_test.go",
     "package mcp\nfunc callWithArgvStub(t *testing.T) string {\n\tstub, body := \"x\", \"#!/bin/sh\\n\"\n\tif runtime.GOOS == \"windows\" {\n\t\tstub, body = stub+\".bat\", \"@echo off\"\n\t}\n\treturn run(stub, body)\n}\n",
     set()),
    ("td-synnex #410 stderr temp file never closed", "a_test.go",
     "package client\nfunc captureStderr(t *testing.T) string {\n\tf, err := os.CreateTemp(t.TempDir(), \"stderr\")\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tb, _ := os.ReadFile(f.Name())\n\treturn string(b)\n}\n",
     {"open-file"}),
    ("td-synnex #410 fixed (defer Close)", "a_test.go",
     "package client\nfunc captureStderr(t *testing.T) string {\n\tf, err := os.CreateTemp(t.TempDir(), \"stderr\")\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tdefer f.Close()\n\tb, _ := os.ReadFile(f.Name())\n\treturn string(b)\n}\n",
     set()),
    ("renamed before closed", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tf, _ := os.CreateTemp(dir, \"x\")\n\tos.Rename(f.Name(), dst)\n\tf.Close()\n}\n",
     {"open-file"}),
    ("a file that does not build on windows is not linted", "a_linux_test.go",
     "package cli\nfunc TestWritableDir(t *testing.T) {\n\tos.Chmod(dir, 0o770)\n}\n",
     set()),
    ("a //go:build !windows file is not linted", "a_test.go",
     "//go:build !windows\n\npackage cli\nfunc TestWritableDir(t *testing.T) {\n\tos.Chmod(dir, 0o770)\n}\n",
     set()),
    ("a //go:build windows file IS linted", "a_test.go",
     "//go:build windows || linux\n\npackage cli\nfunc TestWritableDir(t *testing.T) {\n\tos.Chmod(dir, 0o770)\n}\n",
     {"mode-bits"}),
    ("braces and quotes inside strings and comments do not end a function early", "a_test.go",
     "package cli\n// a } in a comment\nfunc TestX(t *testing.T) {\n\ts := \"}\\\"{\"\n\t_ = `}`\n\tos.Chmod(dir, 0o770)\n}\n",
     {"mode-bits"}),
)
_ST_CLOCK_BROKEN = """package cli

func recordSnapshot(ctx context.Context, db *sql.DB, parent string) (string, bool, error) {
\tnow := time.Now().UTC().Format(time.RFC3339Nano)
\tres, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO runs (snapshot_at, parent) VALUES (?, ?)`, now, parent)
\treturn now, err == nil, err
}
"""
_ST_CLOCK_FIXED = _ST_CLOCK_BROKEN.replace("now := time.Now()", "now := snapshotClock()")


def _self_test() -> int:
    import tempfile

    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    goos = goos_branching_funcs([_ST_PKG_SRC])
    expect(goos == {"shellQuote"}, "a function that branches on runtime.GOOS is recognised")
    print("test-file rules (broken fires, fixed is silent):")
    for label, name, src, want in _ST_CASES:
        p = Path("/x") / name
        got = set()
        if builds_on_windows(p, src):
            got = {f.rule for f in lint_test_file("demo", name, src, goos)}
        expect(got == want, f"{label} -> {sorted(want) or 'silent'} (got {sorted(got) or 'silent'})")
    print("source rule:")
    expect({f.rule for f in lint_source_file("demo", "x.go", _ST_CLOCK_BROKEN)} == {"clock-key"},
           "watchguard #396 time.Now() RFC3339Nano key on INSERT OR IGNORE -> clock-key")
    expect(not lint_source_file("demo", "x.go", _ST_CLOCK_FIXED),
           "watchguard #396 fixed (injectable clock, tie stepped past) -> silent")
    print("ledger:")
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        d = root / "skills" / "demo" / "cli" / "internal" / "cli"
        d.mkdir(parents=True)
        (root / "skills" / "demo" / "cli" / "go.mod").write_text("module demo-pp-cli\n", encoding="utf-8")
        (d / "a_test.go").write_text(_ST_CASES[5][2], encoding="utf-8")
        found = lint_slug(root, "demo")
        expect([f.func for f in found] == ["TestWritableDir"], "lint_slug walks a connector tree")
        mark_known(found, [{"id": "X", "kind": "unix-only-test", "why": "w", "fix": "f",
                            "failures": {"demo": ["TestWritableDir"]}}])
        expect(all(f.known for f in found), "a ledgered known Windows failure is marked known, not new")
        found2 = lint_slug(root, "demo")
        mark_known(found2, [{"id": "X", "kind": "unix-only-test", "why": "w", "fix": "f",
                             "failures": {"other": ["TestWritableDir"]}}])
        expect(not any(f.known for f in found2), "another connector's ledger entry does not excuse it")
    if failures:
        print(f"\nFAIL: check_unix_test_asserts self-test - {len(failures)} case(s)")
        return 1
    print("\nPASS: check_unix_test_asserts - every rule fires on its real failure and is silent on its fix")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
