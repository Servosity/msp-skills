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

Narrow on purpose: a noisy lint is worse than none, and CI's real Windows test
job stays the authority; this predicts its commonest failures before a PR.
Every guard is LOCAL to the finding it excuses, and read as syntax, not words
(comments masked; statements read with string contents masked; `if` initializers
dropped; conditions split on top-level && and ||):
  * it sits after the conjunct `runtime.GOOS != "windows"` of an `if` whose
    condition has no top-level || (in the rest of that condition, or the block);
  * it follows, in the same enclosing block, an `if` with a top-level disjunct
    exactly `runtime.GOOS == "windows"` whose block skips or returns as a
    top-level statement (a skip nested under another condition is no exit);
  * #! stubs and expected literals: such an `if`, later in the same scope,
    replaces the very variable that holds the literal with `=` (never a
    shadowing `:=`) - watchguard #396's fix;
  * mode bits only: any Windows skip or return in the function, conditions
    included. Deliberate: td-synnex #410's merged fix skips inside a t.Run
    closure on `mode is loose && GOOS == "windows"` and calls a setup closure
    from two places, so a flow-insensitive rule cannot tell it from a skip
    that does not cover the assertion. Requiring more flags correct code on
    main.
A file that does not build on windows (GOOS file suffix, //go:build) is not
read, and a test recorded in tools/maintainer/windows_known_test_failures.json
is reported as known, never blocking - the same split the windows CI job makes.

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


def code_mask(src: str, blank_strings: bool = True) -> str:
    """src with every comment, and (unless blank_strings is False) every
    string/rune literal BODY, replaced by spaces (newlines and the quote
    characters kept), so offsets still line up and a keyword search sees
    only real code."""
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
            if blank_strings:
                blank(i + 1, j)
            i = j + 1
            continue
        if c in "\"'":
            j = i + 1
            while j < n and src[j] != c and src[j] != "\n":
                j += 2 if src[j] == "\\" else 1
            if blank_strings:
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
    """Top-level functions. The body is the first `{` after the name that is
    outside the signature's parentheses and does not open a type literal
    (`interface{}` / `struct{...}`, in parameters or results)."""
    out: list[Func] = []
    brace_idx = {off: k for k, (off, ch) in enumerate(marks)}
    offsets = [off for off, _ in marks]
    for m in _FUNC_RE.finditer(src):
        open_at, paren, k = -1, 0, bisect.bisect_left(offsets, m.end())
        while k < len(marks):
            off, ch = marks[k]
            if ch == "(":
                paren += 1
            elif ch == ")":
                paren -= 1
            elif ch == "{":
                before = src[max(0, off - 16):off].rstrip()
                if paren == 0 and not re.search(r"\b(?:interface|struct)$", before):
                    open_at = off
                    break
                # a type literal: jump over its matching brace
                depth = 0
                while k < len(marks):
                    o2, c2 = marks[k]
                    if c2 == "{":
                        depth += 1
                    elif c2 == "}":
                        depth -= 1
                        if depth == 0:
                            break
                    k += 1
            k += 1
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
                          r"|\.(?:Perm\(\)|Mode\(\))\s*&\s*0[oO]?[0-7]+\s*(?:!=|==)\s*0\b"   # Perm()&0o077 != 0
                          r"|(?:!=|==)\s*\w*\.?(?:Perm\(\)|Mode\(\)\.Perm\(\))")
_OPEN = re.compile(r"\b([A-Za-z_]\w*)\s*,\s*[A-Za-z_]\w*\s*:?=\s*os\.(?:Create|CreateTemp|OpenFile)\(")
_GOOS = re.compile(r"\bruntime\.GOOS\b|\bgoruntime\.GOOS\b")
_SKIP = re.compile(r"\bt\.Skipf?\(")


# --- Exact guard syntax. Conditions are read from code with comments masked
# (string literals kept, so "windows" is visible); statements are read from
# code with strings masked too, so a word inside a message is never code.

_IF = re.compile(r"\bif\b")
_GOOS_NE = re.compile(r"\A\(*\s*(?:go)?runtime\.GOOS\s*!=\s*\"windows\"\s*\)*\Z|"
                      r"\A\(*\s*\"windows\"\s*!=\s*(?:go)?runtime\.GOOS\s*\)*\Z")
_GOOS_EQ = re.compile(r"\A\(*\s*(?:go)?runtime\.GOOS\s*==\s*\"windows\"\s*\)*\Z|"
                      r"\A\(*\s*\"windows\"\s*==\s*(?:go)?runtime\.GOOS\s*\)*\Z")
_EXIT_STMT = re.compile(r"(?m)^[ \t]*(?:t\.Skip(?:f|Now)?\(|return\b)")


def _block_end(text: str, open_brace: int) -> int:
    depth, j = 0, open_brace
    while j < len(text):
        if text[j] == "{":
            depth += 1
        elif text[j] == "}":
            depth -= 1
            if depth == 0:
                return j
        j += 1
    return len(text)


def _split_top(expr: str, sep: str) -> list[str]:
    """Split on a top-level operator (outside parens, brackets, braces)."""
    parts, depth, i, start = [], 0, 0, 0
    while i < len(expr):
        ch = expr[i]
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif depth == 0 and expr.startswith(sep, i):
            parts.append(expr[start:i])
            i += len(sep)
            start = i
            continue
        i += 1
    parts.append(expr[start:])
    return [p.strip() for p in parts]


class IfStmt:
    __slots__ = ("start", "cond", "open", "end")

    def __init__(self, start: int, cond: str, open_: int, end: int) -> None:
        self.start, self.cond, self.open, self.end = start, cond, open_, end


def if_statements(gcode: str) -> list[IfStmt]:
    """Every `if [init;] cond {` in a function, with its condition (the
    initializer dropped) and block span."""
    out = []
    for m in _IF.finditer(gcode):
        depth, j = 0, m.end()
        while j < len(gcode):  # the block opens at the first `{` outside parens
            ch = gcode[j]
            if ch in "([":
                depth += 1
            elif ch in ")]":
                depth -= 1
            elif ch == "{" and depth == 0:
                break
            j += 1
        if j >= len(gcode):
            continue
        cond = _split_top(gcode[m.end():j], ";")[-1]
        out.append(IfStmt(m.start(), cond, j, _block_end(gcode, j)))
    return out


def implies_not_windows(cond: str) -> bool:
    """True only when the condition cannot hold on Windows: no top-level ||,
    and one top-level && conjunct is exactly runtime.GOOS != "windows"."""
    if len(_split_top(cond, "||")) > 1:
        return False
    return any(_GOOS_NE.match(c) for c in _split_top(cond, "&&"))


def always_on_windows(cond: str) -> bool:
    """True only when the condition holds whenever GOOS is windows: one
    top-level || disjunct is exactly runtime.GOOS == "windows"."""
    return any(_GOOS_EQ.match(d) for d in _split_top(cond, "||"))


def _top_level(code: str, block_open: int, pos: int) -> bool:
    seg = code[block_open + 1:pos]
    return seg.count("{") - seg.count("}") == 0


def _enclosing_end(code: str, pos: int) -> int:
    depth = 0
    for j in range(pos - 1, -1, -1):
        if code[j] == "}":
            depth += 1
        elif code[j] == "{":
            if depth == 0:
                return _block_end(code, j)
            depth -= 1
    return len(code)


def guard_model(gcode: str, code: str) -> tuple[list[tuple[int, int]], list[tuple[int, int]]]:
    """(not-windows spans, after-exit spans). A not-windows span is the body
    of an `if` whose condition implies non-Windows. An after-exit span runs
    from the end of an `if` that always runs on Windows and exits (t.Skip or
    return as a TOP-LEVEL statement of its block, read with strings masked)
    to the end of the block that encloses that `if`."""
    nw, after = [], []
    for st in if_statements(gcode):
        if implies_not_windows(st.cond):
            # && short-circuits left to right: everything after the GOOS
            # conjunct, in the condition and the block, runs off Windows only.
            header = gcode[st.start:st.open]
            cm = None
            for cm in re.finditer(r"(?:go)?runtime\.GOOS\s*!=\s*\"windows\"|\"windows\"\s*!=\s*(?:go)?runtime\.GOOS", header):
                pass
            nw.append((st.start + cm.end() if cm else st.open, st.end))
        elif always_on_windows(st.cond):
            if any(_top_level(code, st.open, st.open + 1 + mm.start())
                   for mm in _EXIT_STMT.finditer(code[st.open + 1:st.end])):
                after.append((st.end, _enclosing_end(code, st.start)))
    return nw, after


def windows_reassigns(gcode: str, code: str, var: str, decl: int) -> bool:
    """An `if` that always runs on Windows, in the same block as the
    declaration of `var` and after it, assigns `var` with `=` (never a
    shadowing `:=`) as a top-level statement of its block."""
    scope_end = _enclosing_end(code, decl)
    for st in if_statements(gcode):
        if not (decl < st.start < scope_end) or not always_on_windows(st.cond):
            continue
        if _enclosing_end(code, st.start) != scope_end:
            continue
        block = code[st.open + 1:st.end]
        for am in re.finditer(r"(?m)^[ \t]*([A-Za-z_]\w*(?:\s*,\s*[A-Za-z_]\w*)*)\s*=(?!=)", block):
            if var in [v.strip() for v in am.group(1).split(",")] and _top_level(code, st.open, st.open + 1 + am.start()):
                return True
    return False


def skips_on_windows(gcode: str, code: str, src_body: str) -> bool:
    """Mode-bit assertions only (the documented trade-off): any `if` whose
    condition mentions runtime.GOOS == "windows", conditions included, that
    skips or returns as a top-level statement; or a function that skips
    itself and says why in terms of Windows (the press's read-only-dir
    probe). td-synnex #410's merged fix skips on `loose && windows` inside a
    t.Run closure, which a flow-insensitive rule cannot tell apart from a
    skip that misses the assertion."""
    for st in if_statements(gcode):
        if re.search(r"\b(?:go)?runtime\.GOOS\s*==\s*\"windows\"", st.cond) and any(
                _top_level(code, st.open, st.open + 1 + mm.start())
                for mm in _EXIT_STMT.finditer(code[st.open + 1:st.end])):
            return True
    return bool(_SKIP.search(code)) and "windows" in src_body.lower()


_FUNC_LIT = re.compile(r"\bfunc\s*\([^)]*\)\s*(?:\([^)]*\)|[\w.*\[\]]+)?\s*\{")


def func_literal_spans(code: str) -> list[tuple[int, int, bool]]:
    """(body start, body end, runs inline) for each function literal. One
    invoked on the spot (`func() { ... }()`, not under defer or go) runs
    inline, deferred calls inside it included; any other runs LATER."""
    out = []
    for m in _FUNC_LIT.finditer(code):
        end = _block_end(code, m.end() - 1)
        before = code[max(0, code.rfind("\n", 0, m.start()) + 1):m.start()]
        inline = (re.match(r"\s*\(", code[end + 1:end + 3] or "") is not None
                  and not re.search(r"\b(?:defer|go)\s*$", before))
        out.append((m.end(), end, inline))
    return out


def _top_split(expr: str) -> list[str]:
    """Split on top-level commas (outside parens, brackets and quotes)."""
    parts, depth, cur, q = [], 0, [], ""
    for ch in expr:
        if q:
            cur.append(ch)
            if ch == q:
                q = ""
            continue
        if ch in "\"`'":
            q = ch
        elif ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            parts.append("".join(cur))
            cur = []
            continue
        cur.append(ch)
    parts.append("".join(cur))
    return [p.strip() for p in parts]


def var_holding(line: str, needle_pos: int) -> str | None:
    """For `a, b := x, "lit"` the variable that receives the literal at
    needle_pos (positional), or None when the literal is not assigned."""
    m = re.match(r"^\s*([A-Za-z_]\w*(?:\s*,\s*[A-Za-z_]\w*)*)\s*:?=\s*", line)
    if not m or needle_pos < m.end():
        return None
    lhs = [v.strip() for v in m.group(1).split(",")]
    rhs = line[m.end():]
    acc, idx = 0, None
    for k, part in enumerate(_top_split(rhs)):
        start = rhs.find(part, acc)
        if start <= needle_pos - m.end() < start + len(part):
            idx = k
            break
        acc = start + len(part)
    return lhs[idx] if idx is not None and idx < len(lhs) else None


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
    masked = code_mask(src)                       # comments AND string bodies blanked
    guards = code_mask(src, blank_strings=False)  # comments blanked, "windows" kept
    for f in functions(src, marks):
        body = src[f.start:f.end]
        # Rules read the CODE: a commented-out Close(), chmod or comparison
        # neither counts nor fires. Literal values are read from the lexer.
        code = masked[f.start:f.end]
        gcode = guards[f.start:f.end]
        nw_spans, after_exit = guard_model(gcode, code)
        mode_exit = skips_on_windows(gcode, code, body)  # mode bits only (documented trade-off)
        closures = func_literal_spans(code)
        flits = [lit for lit in lits if f.start <= lit.start < f.end]

        def runs_on_windows(off: int) -> bool:
            """off (relative to the function) can run on Windows: no `if`
            that implies non-Windows encloses it, and no Windows exit earlier
            in its enclosing block comes before it."""
            return not any(a <= off < b for a, b in nw_spans + after_exit)

        def add(off: int, rule: str, why: str) -> None:
            out.append(Finding(slug, rel, _line_of(src, off), rule, f.name, why))

        def holder_replaced_on_windows(lit_start: int) -> bool:
            """The literal at lit_start is assigned to a variable that a
            Windows branch later replaces in the same scope."""
            line_start = src.rfind("\n", 0, lit_start) + 1
            holder = var_holding(_line_text(src, lit_start), lit_start - line_start)
            return bool(holder) and windows_reassigns(gcode, code, holder, line_start - f.start)

        # errno-text: POSIX errno wording inside a comparison.
        for lit in flits:
            low = lit.value.lower()
            hit = next((e for e in ERRNO_TEXT if e in low), None)
            cline = _line_text(masked, lit.start)
            if (hit and _COMPARE.search(cline) and re.search(r"\berr\w*\b|\.Error\(\)", cline)
                    and runs_on_windows(lit.start - f.start)):
                add(lit.start, "errno-text", f"compares an error against the POSIX text {hit!r}")
                break

        # shebang-stub: a helper that writes a #! script for tests to RUN.
        # Only helpers: a Test that writes its own stub as a tripwire (the
        # press's teach-playbook test, which expects a refusal before any
        # exec) still detects its regression on Windows, by another route.
        if not f.name.startswith("Test"):
            for lit in flits:
                if (lit.value.startswith("#!/") and runs_on_windows(lit.start - f.start)
                        and not holder_replaced_on_windows(lit.start)):
                    add(lit.start, "shebang-stub", "writes a #! script for tests to run; Windows cannot execute it")
                    break

        # mode-bits: chmod to a mode Windows cannot honor, or a Perm() assert.
        if not mode_exit:
            flagged = False
            chmods = [cm for cm in _CHMOD.finditer(code) if runs_on_windows(cm.start())]
            for cm in chmods:
                line = _line_text(src, f.start + cm.start())
                mm = _OCTAL.search(line[line.find(cm.group(0)):])
                if mm and loose_mode(mm.group(1)):
                    add(f.start + cm.start(), "mode-bits",
                        f"chmods to 0o{mm.group(1)}; Windows has no group/other or read-only mode bits")
                    flagged = True
                    break
            if not flagged and chmods:
                for om in _OCTAL.finditer(code):
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
                pm = next((m for m in _PERM_ASSERT.finditer(code) if runs_on_windows(m.start())), None)
                if pm:
                    add(f.start + pm.start(), "mode-bits", "asserts Unix permission bits; Windows reports 0666")

        # open-file: created file never closed, or renamed before it is
        # closed. A close in a closure that runs later (defer func, t.Cleanup)
        # does not precede a rename; one invoked on the spot does.
        for om in _OPEN.finditer(code):
            var = om.group(1)
            if var == "_" or not runs_on_windows(om.start()):
                continue
            v = re.escape(var)
            rest = code[om.end():]
            closes = [om.end() + c.start() for c in re.finditer(rf"\b{v}\.Close\(", rest)]
            returned = any(var in _top_split(rm.group(1))
                           for rm in re.finditer(r"\breturn\b([^\n]*)", rest))
            if not closes and not returned:
                add(f.start + om.start(), "open-file",
                    f"{var} is never closed; Windows cannot remove an open file, so TempDir cleanup fails")
                continue

            def closed_at(pos: int) -> int | None:
                """When the close at pos takes effect, in source order, or
                None when it runs after the function body moves on."""
                lit = next(((a, b, inl) for a, b, inl in closures if a <= pos < b), None)
                deferred = re.search(r"\bdefer\s+$", code[code.rfind("\n", 0, pos) + 1:pos]) is not None
                if lit is None:
                    return None if deferred else pos
                return lit[1] if lit[2] else None   # inline closure: at its end (defers included)

            effective = sorted(p for p in (closed_at(c) for c in closes) if p is not None)
            ren = re.search(rf"\bos\.Rename\(\s*{v}\.Name\(\)", rest)
            if ren and not any(p < om.end() + ren.start() for p in effective):
                add(f.start + om.start(), "open-file",
                    f"{var} is renamed before it is closed; Windows refuses to rename an open file")

        # goos-literal: a GOOS-branching function's result compared to one literal.
        if goos_funcs:
            lit_rx = r"(?:`[^`]*`|\"(?:[^\"\\]|\\.)*\")"
            for name in sorted(goos_funcs):
                n = re.escape(name)
                shapes = (
                    rf"(\w+)\s*,\s*(\w+)\s*:?=\s*(?<![\w.]){n}\([^;]*\)\s*,\s*{lit_rx}",   # got, want := F(x), "lit"
                    rf"(?<![\w.]){n}\([^;{{]*\)\s*[!=]=\s*{lit_rx}",                   # F(x) != "lit"
                    rf"\b(\w+)\s*:=\s*(?<![\w.]){n}\([^;]*\)\s*;\s*\1\s*[!=]=\s*{lit_rx}",  # got := F(x); got != "lit"
                )
                hit = None
                for cm in re.finditer(rf"(?<![\w.]){n}\(", code):   # code: a comment never matches
                    if not runs_on_windows(cm.start()):
                        continue
                    line = _line_text(src, f.start + cm.start())
                    m1 = re.search(shapes[0], line)
                    if m1:
                        line_start = src.rfind("\n", 0, f.start + cm.start()) + 1
                        if windows_reassigns(gcode, code, m1.group(2), line_start - f.start):
                            continue  # `got, want := F(), "lit"` with want replaced on Windows
                        hit = cm
                        break
                    if any(re.search(sh, line) for sh in shapes[1:]):
                        hit = cm
                        break
                if hit is not None:
                    add(f.start + hit.start(), "goos-literal",
                        f"compares {name}(), which branches on runtime.GOOS, against one literal")
                    break
    return out


_CLOCK_STAMP = re.compile(r"\b([A-Za-z_]\w*)\s*:?=\s*time\.Now\(\)[\w.()]*\.Format\(time\.RFC3339Nano\)")


def lint_source_file(slug: str, rel: str, src: str) -> list[Finding]:
    out: list[Finding] = []
    if "INSERT OR IGNORE" not in src or "RFC3339Nano" not in src:
        return out
    _, marks = lex(src)
    masked = code_mask(src, blank_strings=False)  # comments blanked: a "previously:" note never fires
    for f in functions(src, marks):
        body = masked[f.start:f.end]
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
    ("a masked Perm() assertion (`Perm()&0o077 != 0`)", "a_test.go",
     "package cli\nfunc TestPerm(t *testing.T) {\n\tif info.Mode().Perm()&0o077 != 0 {\n\t\tt.Fatal(info)\n\t}\n}\n",
     {"mode-bits"}),
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
    ("a GOOS branch that only picks a file name does not excuse a mode-bit assert (Codex r13)", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tname := \"x\"\n\tif runtime.GOOS == \"windows\" {\n\t\tname = \"x.exe\"\n\t}\n"
     "\tif fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(name)\n\t}\n}\n",
     {"mode-bits"}),
    ("`if runtime.GOOS != \"windows\" && <perm assert>` is guarded (the press's own shape)", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif runtime.GOOS != \"windows\" && fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(1)\n\t}\n}\n",
     set()),
    ("a perm assert inside an `if runtime.GOOS != \"windows\" {` block is guarded", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif runtime.GOOS != \"windows\" {\n\t\tif fi.Mode().Perm() != 0o600 {\n\t\t\tt.Fatal(1)\n\t\t}\n\t}\n}\n",
     set()),
    ("...but an assert BEFORE the GOOS conjunct in the same condition runs on Windows", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif fi.Mode().Perm() != 0o600 && runtime.GOOS != \"windows\" {\n\t\tt.Fatal(1)\n\t}\n}\n",
     {"mode-bits"}),
    ("...but one AFTER that block is not", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif runtime.GOOS != \"windows\" {\n\t\tt.Log(1)\n\t}\n\tif fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(1)\n\t}\n}\n",
     {"mode-bits"}),
    ("a guard inside a comment guards nothing", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\t// if runtime.GOOS == \"windows\" { t.Skip() }\n\tif fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(1)\n\t}\n}\n",
     {"mode-bits"}),
    ("a Windows return guard excuses a mode-bit assert", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\treturn\n\t}\n"
     "\tif fi.Mode().Perm() != 0o600 {\n\t\tt.Fatal(1)\n\t}\n}\n",
     set()),
    ("a deferred close does not precede a rename (Codex r13)", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tf, _ := os.CreateTemp(dir, \"x\")\n\tdefer f.Close()\n"
     "\tif err := os.Rename(f.Name(), f.Name()+\".after\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"open-file"}),
    ("a commented-out close is no close", "a_test.go",
     "package client\nfunc helper(t *testing.T) string {\n\tf, _ := os.CreateTemp(t.TempDir(), \"x\")\n\t// f.Close()\n\treturn \"\"\n}\n",
     {"open-file"}),
    ("an open-file finding is not excused by a GOOS branch", "a_test.go",
     "package client\nfunc helper(t *testing.T) string {\n\tf, _ := os.CreateTemp(t.TempDir(), \"x\")\n"
     "\tif runtime.GOOS == \"windows\" {\n\t\tt.Log(\"x\")\n\t}\n\treturn \"\"\n}\n",
     {"open-file"}),
    # Codex round 14: every guard is local to the finding it excuses.
    ("a Windows branch that renames the stub but keeps the #! body (Codex r14)", "a_test.go",
     "package mcp\nfunc callWithArgvStub(t *testing.T) string {\n\tstub, body := filepath.Join(t.TempDir(), \"x\"), \"#!/bin/sh\\necho ARGV\\n\"\n"
     "\tif runtime.GOOS == \"windows\" {\n\t\tstub = stub + \".bat\"\n\t}\n\treturn run(stub, body)\n}\n",
     {"shebang-stub"}),
    ("a perm assert inside `if runtime.GOOS != \"windows\" || strict {` runs on Windows (Codex r14)", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tstrict := true\n\tif runtime.GOOS != \"windows\" || strict {\n"
     "\t\tif fi.Mode().Perm() != 0o600 {\n\t\t\tt.Fatal(1)\n\t\t}\n\t}\n}\n",
     {"mode-bits"}),
    ("a close in a multi-line t.Cleanup runs after the rename (Codex r14)", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tf, _ := os.CreateTemp(dir, \"x\")\n\tt.Cleanup(func() {\n\t\tf.Close()\n\t})\n"
     "\tif err := os.Rename(f.Name(), f.Name()+\".after\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"open-file"}),
    ("a close in a multi-line defer func runs after the rename", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tf, _ := os.CreateTemp(dir, \"x\")\n\tdefer func() {\n\t\t_ = f.Close()\n\t}()\n"
     "\tos.Rename(f.Name(), dst)\n}\n",
     {"open-file"}),
    ("returning f.Name() leaves the handle open (Codex r14)", "a_test.go",
     "package client\nfunc fixture(t *testing.T) string {\n\tf, _ := os.CreateTemp(t.TempDir(), \"fixture\")\n\tf.WriteString(\"x\")\n"
     "\treturn f.Name()\n}\n",
     {"open-file"}),
    ("returning the handle itself hands it to the caller", "a_test.go",
     "package client\nfunc fixture(t *testing.T) *os.File {\n\tf, _ := os.CreateTemp(t.TempDir(), \"fixture\")\n\treturn f\n}\n",
     set()),
    ("a test that skips Windows up front may rename an open file (Codex r14)", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\tt.Skip(\"Unix permits renaming open files\")\n\t}\n"
     "\tf, _ := os.CreateTemp(dir, \"x\")\n\tdefer f.Close()\n\tos.Rename(f.Name(), dst)\n}\n",
     set()),
    ("an errno literal compared AFTER an up-front Windows skip is guarded", "a_test.go",
     "package cli\nfunc TestRefused(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\tt.Skip(\"posix text\")\n\t}\n"
     "\tif err == nil || !strings.Contains(err.Error(), \"connection refused\") {\n\t\tt.Fatal(err)\n\t}\n}\n",
     set()),
    ("an unrelated GOOS branch does not excuse errno text", "a_test.go",
     "package cli\nfunc TestRefused(t *testing.T) {\n\tname := \"x\"\n\tif runtime.GOOS == \"windows\" {\n\t\tname = \"y\"\n\t}\n"
     "\tif err == nil || !strings.Contains(err.Error(), \"connection refused\"+name) {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"errno-text"}),
    # Codex round 15: exact guard syntax.
    ("an exit NESTED under a second condition is not unconditional", "a_test.go",
     "package cli\nfunc TestRefused(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\tif testing.Short() {\n\t\t\tt.Skip(\"slow\")\n\t\t}\n\t}\n"
     "\tif err == nil || !strings.Contains(err.Error(), \"connection refused\") {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"errno-text"}),
    ("a negated condition does not exclude Windows", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif !(runtime.GOOS != \"windows\") {\n\t\tif fi.Mode().Perm() != 0o600 {\n\t\t\tt.Fatal(1)\n\t\t}\n\t}\n}\n",
     {"mode-bits"}),
    ("the word return inside a log message is no exit", "a_test.go",
     "package cli\nfunc TestRefused(t *testing.T) {\n\tif runtime.GOOS == \"windows\" {\n\t\tt.Log(\"return to normal path\")\n\t}\n"
     "\tif err == nil || !strings.Contains(err.Error(), \"connection refused\") {\n\t\tt.Fatal(err)\n\t}\n}\n",
     {"errno-text"}),
    ("an if initializer is not the condition", "a_test.go",
     "package cli\nfunc TestPriv(t *testing.T) {\n\tif ok := runtime.GOOS != \"windows\"; ok || true {\n\t\tif fi.Mode().Perm() != 0o600 {\n\t\t\tt.Fatal(1)\n\t\t}\n\t}\n}\n",
     {"mode-bits"}),
    ("a CONDITIONAL replacement of the #! body does not cover Windows", "a_test.go",
     "package mcp\nfunc callStub(t *testing.T) string {\n\tbody := \"#!/bin/sh\\necho ARGV\\n\"\n"
     "\tif runtime.GOOS == \"windows\" && testing.Short() {\n\t\tbody = \"@echo off\"\n\t}\n\treturn run(body)\n}\n",
     {"shebang-stub"}),
    ("a SHADOWING := is not a replacement", "a_test.go",
     "package mcp\nfunc callStub(t *testing.T) string {\n\tbody := \"#!/bin/sh\\necho ARGV\\n\"\n"
     "\tif runtime.GOOS == \"windows\" {\n\t\tbody := \"@echo off\"\n\t\t_ = body\n\t}\n\treturn run(body)\n}\n",
     {"shebang-stub"}),
    ("an interface{} in the signature does not hide the body", "a_test.go",
     "package mcp\nfunc stub(t *testing.T, args ...interface{}) string {\n\tp := filepath.Join(t.TempDir(), \"x\")\n"
     "\tos.WriteFile(p, []byte(\"#!/bin/sh\\necho\\n\"), 0o700)\n\treturn p\n}\n",
     {"shebang-stub"}),
    ("returning `f != nil` hands nothing over", "a_test.go",
     "package client\nfunc fixture(t *testing.T) bool {\n\tf, _ := os.CreateTemp(t.TempDir(), \"fixture\")\n\treturn f != nil\n}\n",
     {"open-file"}),
    ("a close deferred inside an immediately invoked closure precedes the rename", "a_test.go",
     "package client\nfunc TestSwap(t *testing.T) {\n\tf, _ := os.CreateTemp(dir, \"x\")\n\tfunc() {\n\t\tdefer f.Close()\n\t\tf.WriteString(\"x\")\n\t}()\n"
     "\tos.Rename(f.Name(), dst)\n}\n",
     set()),
    ("a GOOS-blind comparison in a COMMENT is not code", "a_test.go",
     "package cli\nfunc TestShellQuote(t *testing.T) {\n\t// was: if shellQuote(`x`) != `'x'` {\n\twant := expected()\n\tif got := shellQuote(`x`); got != want {\n\t\tt.Fatal(got)\n\t}\n}\n",
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
    commented = _ST_CLOCK_FIXED.replace("now := snapshotClock()",
                                        "// previously: now := time.Now().UTC().Format(time.RFC3339Nano)\n\tnow := snapshotClock()")
    expect(not lint_source_file("demo", "x.go", commented), "the old clock line kept in a COMMENT does not fire")
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
