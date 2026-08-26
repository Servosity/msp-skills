#!/usr/bin/env python3
"""Gate: every operator-facing env var a connector READS is declared in its manifest.

Why this exists
---------------
Claude Desktop installs a connector from its `.mcpb` bundle and reads
`skills/<slug>/manifest.json`. The ONLY channel it has for handing an operator's
credentials to the server is `server.mcp_config.env` plus the matching
`user_config` prompts: there is no shell profile, no `~/.config/<cli>/config.toml`
and no `export` in that context. So a variable the binary reads but the manifest
never declares is a variable the operator is never asked for - the server boots,
`tools/list` looks healthy, and the first real tool call fails on missing
credentials. Issue #282 reported it as "three live-verified connectors prompt for
the wrong credentials"; the fleet-wide audit found 60 of 65 connectors
under-declaring, including cove (0 of 4 declared) and quickbooks (1 of 8).

Nothing in the repo could catch it. `check_cli_claims.py` reads docs against the
binary's command surface, never the manifest. `check_skill_contract.py` asserts
manifest.json EXISTS. This gate closes that hole.

What it checks, per slug
------------------------
1. Scan every non-test `.go` file under `skills/<slug>/cli/` for environment
   reads and resolve the variable NAME, not just the literal call:

     os.Getenv("LITERAL") / os.LookupEnv("LITERAL")
     os.Getenv(IDENT)                 IDENT is a const/var/:= bound to a literal
     os.Getenv(A + "_" + B)           concatenation, resolved componentwise
     helper("LITERAL", ...)           any package function that reads one of its
                                      own string params through os.Getenv - this
                                      is DETECTED, not hardcoded, so it covers
                                      envDir() in internal/cliutil/paths.go,
                                      journalEnvTruthy() in internal/learn/journal.go,
                                      globalScopeParamDefault() in auvik's
                                      internal/cli/helpers.go (which hides
                                      AUVIK_TENANT, the one operator-facing
                                      variable behind such a helper),
                                      kindRoot() and firstNonEmptyEnv().

   Names computed through a call are resolved one hop into that function's
   `return` expressions (bounded), which is what reaches <PREFIX>_CONFIG_DIR via
   envDir(kindEnvVar(kind)). A concatenation whose leading component is a runtime
   value - `prefix := strings.ToUpper(...)` in internal/cli/platform_client.go -
   is resolved with the connector's own env prefix substituted in, so
   <PREFIX>_DB / _DATA_DIR / _HOME are seen rather than silently dropped.

   `internal/cliutil/testenv/` is skipped explicitly: it exists to set env vars
   for tests and is not a runtime read.

2. Classify each name operator-facing vs internal from the BASE-OWNED deny-list
   in env_schema_internal.json, FAILING SAFE TO OPERATOR-FACING. An unrecognised
   new variable fails the gate until a human classifies it, because the failure
   this gate exists to prevent is a credential no one is prompted for.

3. Compare against `set(manifest["server"]["mcp_config"]["env"])`, BOTH ways:
     * read-but-not-declared  -> the operator is never prompted (the #282 defect)
     * declared-but-not-read  -> a prompt whose answer goes nowhere (action1
       shipped a phantom ACTION1_REGION the binary never reads)
   A variable that IS read but classifies internal gets its own message: saying
   "no source file reads it" there would be factually false and would tell the
   maintainer to delete a prompt that works.

4. Assert the plumbing is intact: every declared value is exactly
   "${user_config.<key>}" and that <key> exists in `user_config`; and every
   `user_config` key is referenced by some env value (an unreferenced prompt asks
   the operator for a value that is then dropped on the floor).

5. Assert `skills/<slug>/mcp-install.md` matches the manifest. That page is the
   MANUAL install path - three copy-paste JSON blocks (Claude Desktop, GitHub
   Copilot, Gemini CLI) each carrying an explicit "env" map, plus the remote
   command line that launches the binary with credentials in the environment. It
   does NOT go through the `.mcpb` bundle, so a manifest fix does not reach it.
   Before this assertion existed, 61 of 65 pages omitted 125 of their manifest's
   variables - threatlocker's page set only PRINTING_PRESS_CLIENT_PROFILE and
   never THREATLOCKER_API_KEY, so an operator following the shipped doc
   reproduced #282 verbatim. Every JSON block must carry EXACTLY the manifest's
   env keys; every REQUIRED variable must appear on the remote command line.

6. Assert `skills/<slug>/server.json` matches the manifest. server.json is the
   MCP Registry publish channel - a third install path with its own copy of the
   credential list. hudu declared hudu_base_url required in its manifest and
   listed only HUDU_API_KEY in server.json, so a registry install was never asked
   for the base URL and fell back to a placeholder host. Names must match exactly
   both ways, and `isRequired` / `isSecret` / `default` must match the matching
   `user_config` entry. Descriptions are NOT compared: server.json has always
   carried the registry's shorter one-liner.

7. Assert the identity half of a machine credential is masked like its secret
   half. A `user_config` entry behind a variable ending in _CLIENT_ID, _APP_ID,
   _ACCESS_KEY_ID or _PUBLIC_KEY must be `"sensitive": true`, the same as the
   _CLIENT_SECRET / _SECRET_ACCESS_KEY / _SECRET_KEY it is paired with. The fleet
   was split 9-to-7 on this before the rule existed, so the same credential pair
   was half-masked on one connector and fully masked on the next.

Waivers
-------
`env_schema_internal.json` carries per-slug waivers with a MANDATORY reason
string, exempting one (slug, var) pair from BOTH directions. Same posture as
security_suppressions.json: base-owned, not self-grantable from a connector diff,
because a waiver is exactly how a credential prompt disappears. A waiver with an
empty reason is itself a failure.

What it deliberately does NOT check
-----------------------------------
`entry_point == "bin/" + <mcp binary>`. That assertion is inverted for this
fleet: the published `.mcpb` bundles contain only os/arch-suffixed binaries
(`bin/<slug>-mcp-darwin-arm64`), so the bare name exists in no bundle, and
connectwise-manage - the one manifest carrying `platform_overrides` - is the only
one whose entry_point is right. Enforcing the bare name would codify a fleet-wide
break and false-RED the single correct manifest.

Modes:
  --slug <slug>   check one skill. An UNRECOGNISED slug is a usage error, not a
                  vacuous pass: CI runs this per-skill from a matrix, so a drifted
                  interpolation would otherwise print PASS and check nothing
                  across every job (check_cli_claims.py sets the same precedent).
  --all           check every registered skill (the default)
  --warn          print findings as WARN: lines and exit 0 (calibration mode,
                  the same wiring precedent as check_cli_claims.py --warn)
  --self-test     run the built-in both-directions proof and exit
  -v/--verbose    per-slug detail, including every resolved read and its site

Pure stdlib. Run locally:
    python3 tools/maintainer/check_env_schema.py
    python3 tools/maintainer/check_env_schema.py --warn
    python3 tools/maintainer/check_env_schema.py --slug cove -v

Exit code: 0 = pass (or --warn), 1 = findings, 2 = usage error.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR
RULES_FILE = Path(__file__).resolve().parent / "env_schema_internal.json"

# Directories under cli/ that never contain a runtime env read.
SKIP_DIRS = {"testdata", "vendor", "testenv"}

# A plausible environment-variable name. Deliberately strict: resolution walks
# string concatenation and function returns, so this is the filter that keeps a
# path fragment or a header name from being reported as a missing credential.
RE_ENV_NAME = re.compile(r"^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$")

# Reading builtins. os.Setenv is NOT here: writing a variable is not an operator
# input (immybot derives IMMYBOT_OAUTH_SCOPE that way in init()).
BUILTIN_READERS = {"os.Getenv": [0], "os.LookupEnv": [0]}

UNRESOLVED = object()
MAX_DEPTH = 4


# ---------------------------------------------------------------------------
# Go source scrubbing + primitive parsing
# ---------------------------------------------------------------------------

def strip_comments(src: str) -> str:
    """Blank out // and /* */ comments without touching string/rune literals."""
    out = []
    i, n = 0, len(src)
    while i < n:
        ch = src[i]
        if ch == '"' or ch == "'" or ch == "`":
            quote = ch
            out.append(ch)
            i += 1
            while i < n:
                c = src[i]
                out.append(c)
                i += 1
                if quote != "`" and c == "\\" and i < n:
                    out.append(src[i])
                    i += 1
                    continue
                if c == quote:
                    break
            continue
        if ch == "/" and i + 1 < n and src[i + 1] == "/":
            while i < n and src[i] != "\n":
                i += 1
            continue
        if ch == "/" and i + 1 < n and src[i + 1] == "*":
            i += 2
            while i + 1 < n and not (src[i] == "*" and src[i + 1] == "/"):
                if src[i] == "\n":
                    out.append("\n")
                i += 1
            i += 2
            continue
        out.append(ch)
        i += 1
    return "".join(out)


def match_close(src: str, open_at: int, opener: str, closer: str) -> int:
    """Index of the delimiter closing src[open_at], skipping literals. -1 if none."""
    depth = 0
    i, n = open_at, len(src)
    while i < n:
        ch = src[i]
        if ch in '"\'`':
            quote = ch
            i += 1
            while i < n:
                c = src[i]
                if quote != "`" and c == "\\":
                    i += 2
                    continue
                i += 1
                if c == quote:
                    break
            continue
        if ch == opener:
            depth += 1
        elif ch == closer:
            depth -= 1
            if depth == 0:
                return i
        i += 1
    return -1


def split_top(text: str, sep: str) -> list[str]:
    """Split on sep at nesting depth 0, outside string literals."""
    parts, buf = [], []
    depth = 0
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch in '"\'`':
            quote = ch
            buf.append(ch)
            i += 1
            while i < n:
                c = text[i]
                buf.append(c)
                if quote != "`" and c == "\\" and i + 1 < n:
                    buf.append(text[i + 1])
                    i += 2
                    continue
                i += 1
                if c == quote:
                    break
            continue
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        if depth == 0 and ch == sep:
            parts.append("".join(buf))
            buf = []
            i += 1
            continue
        buf.append(ch)
        i += 1
    parts.append("".join(buf))
    return parts


RE_STRING_LIT = re.compile(r'^"((?:[^"\\]|\\.)*)"$|^`([^`]*)`$')


def as_literal(expr: str):
    m = RE_STRING_LIT.match(expr.strip())
    if not m:
        return None
    raw = m.group(1) if m.group(1) is not None else m.group(2)
    try:
        return raw.encode().decode("unicode_escape")
    except UnicodeDecodeError:  # pragma: no cover - defensive
        return raw


RE_IDENT = re.compile(r"^[A-Za-z_]\w*$")
RE_SLICE_LIT = re.compile(r"^\[\]string\s*\{(.*)\}$", re.S)
RE_CALL = re.compile(r"^([A-Za-z_][\w.]*)\s*\(")


# ---------------------------------------------------------------------------
# Package model
# ---------------------------------------------------------------------------

class Scope:
    """A name -> candidate-RHS-expression map (a package, or one function body)."""

    def __init__(self) -> None:
        self.bindings: dict[str, list[str]] = {}

    def bind(self, name: str, expr: str) -> None:
        expr = expr.strip()
        if not expr:
            return
        self.bindings.setdefault(name, [])
        if expr not in self.bindings[name]:
            self.bindings[name].append(expr)


class GoFunc(Scope):
    def __init__(self, name: str, params: list[tuple[str, bool]], body: str, file: Path):
        super().__init__()
        self.name = name
        self.params = params          # [(param name, is_variadic)]
        self.body = body
        self.file = file
        self.env_param_idx: set[int] = set()
        self.variadic_env = False


class GoPackage(Scope):
    """One Go package (one directory), flattened to what name resolution needs.

    Bindings are scoped: a function's locals are looked up first, the package's
    declarations second. Flattening both into one map is wrong in a way that
    invents variable names - internal/cliutil/paths.go binds `suffix` in two
    different functions (`pathKindEnvSuffix(kind)` in one, a TrimSuffix of it in
    the other), and a flat map cross-products them into an XDG_CONFIG_DIR_HOME
    no code reads.
    """

    def __init__(self, path: Path):
        super().__init__()
        self.path = path
        self.funcs: dict[str, GoFunc] = {}
        self.sources: list[tuple[Path, str]] = []
        self.spans: dict[Path, list[tuple[int, int, GoFunc]]] = {}

    def func_at(self, path: Path, offset: int) -> "GoFunc | None":
        """The innermost function whose body contains this source offset."""
        best = None
        for start, end, fn in self.spans.get(path, []):
            if start <= offset < end and (best is None or start > best[0]):
                best = (start, fn)
        return best[1] if best else None


RE_ASSIGN = re.compile(r"(?m)^[\t ]*(?:const\s+|var\s+)?([A-Za-z_]\w*)\s*(?::?=)\s*(.+?)[\t ]*$")
RE_RANGE_LIT = re.compile(r"for\s+[\w,\s_]*?([A-Za-z_]\w*)\s*:=\s*range\s+(\[\]string\{[^}]*\}|[A-Za-z_]\w*)")
RE_FUNC = re.compile(r"(?m)^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(")


def parse_package(dirpath: Path, files: list[Path]) -> GoPackage:
    pkg = GoPackage(dirpath)
    for path in files:
        src = strip_comments(path.read_text(encoding="utf-8", errors="replace"))
        pkg.sources.append((path, src))
        spans: list[tuple[int, int, GoFunc]] = []

        for m in RE_FUNC.finditer(src):
            open_paren = src.index("(", m.end() - 1)
            close_paren = match_close(src, open_paren, "(", ")")
            if close_paren < 0:
                continue
            brace = src.find("{", close_paren)
            if brace < 0:
                continue
            end = match_close(src, brace, "{", "}")
            if end < 0:
                continue
            params = parse_params(src[open_paren + 1:close_paren])
            fn = GoFunc(m.group(1), params, src[brace + 1:end], path)
            pkg.funcs.setdefault(fn.name, fn)
            spans.append((brace + 1, end, pkg.funcs[fn.name]))
        pkg.spans[path] = spans

        for regex, group in ((RE_ASSIGN, 2), (RE_RANGE_LIT, 2)):
            for m in regex.finditer(src):
                scope = pkg.func_at(path, m.start()) or pkg
                scope.bind(m.group(1), m.group(group))
    detect_env_param_helpers(pkg)
    return pkg


def parse_params(text: str) -> list[tuple[str, bool]]:
    """Return [(name, variadic)] for a Go parameter list.

    Go lets a group share one type - `func f(envName, fallback string)` - so a
    comma group of a single token is a NAME, not an anonymous type-only
    parameter. Reading it the other way loses envName's index, which is exactly
    the slot auvik's globalScopeParamDefault("AUVIK_TENANT", "") passes the one
    operator-facing variable hidden behind a name-as-parameter helper.
    """
    out: list[tuple[str, bool]] = []
    for group in split_top(text, ","):
        group = group.strip()
        if not group:
            continue
        variadic = "..." in group
        tokens = group.replace("...", " ").split()
        out.append((tokens[0] if tokens else "", variadic))
    return out


RE_BUILTIN_READ = re.compile(r"\bos\.(?:Getenv|LookupEnv)\s*\(\s*([A-Za-z_]\w*)\s*\)")
RE_RANGE_ALIAS = re.compile(r"for\s+[\w,\s_]*?([A-Za-z_]\w*)\s*:=\s*range\s+([A-Za-z_]\w*)")


def detect_env_param_helpers(pkg: GoPackage) -> None:
    """Mark functions that read one of their own string params as an env name.

    Detected rather than hardcoded, so a helper the press renames or a new one it
    introduces is covered without editing this gate. Covers envDir(name),
    journalEnvTruthy(name), globalScopeParamDefault(envName, fallback),
    kindRoot(envName, ...) and the variadic firstNonEmptyEnv(names ...string),
    whose param reaches os.Getenv through `for _, n := range names`.
    """
    for fn in pkg.funcs.values():
        names = [p for p, _ in fn.params]
        aliases: dict[str, str] = {}
        for m in RE_RANGE_ALIAS.finditer(fn.body):
            if m.group(2) in names:
                aliases[m.group(1)] = m.group(2)
        for m in RE_BUILTIN_READ.finditer(fn.body):
            target = aliases.get(m.group(1), m.group(1))
            if target in names:
                idx = names.index(target)
                fn.env_param_idx.add(idx)
                if fn.params[idx][1]:
                    fn.variadic_env = True


# ---------------------------------------------------------------------------
# Name resolution
# ---------------------------------------------------------------------------

def resolve(expr: str, pkg: GoPackage, env_prefix: str, scope: Scope | None = None,
            depth: int = 0, seen: frozenset = frozenset()) -> set:
    """Resolve a Go expression to the set of env-var names it can evaluate to.

    `scope` is the enclosing function, searched before the package. Returns a set
    of strings, possibly containing UNRESOLVED.
    """
    expr = expr.strip()
    if not expr or depth > MAX_DEPTH:
        return {UNRESOLVED}

    lit = as_literal(expr)
    if lit is not None:
        return {lit}

    slice_lit = RE_SLICE_LIT.match(expr)
    if slice_lit:
        out = set()
        for element in split_top(slice_lit.group(1), ","):
            value = as_literal(element)
            if value is not None:
                out.add(value)
        return out or {UNRESOLVED}

    parts = split_top(expr, "+")
    if len(parts) > 1:
        return resolve_concat(parts, pkg, env_prefix, scope, depth, seen)

    if RE_IDENT.match(expr):
        key = (id(scope), expr)
        if key in seen:
            return {UNRESOLVED}
        rhs_list = []
        if scope is not None:
            rhs_list = scope.bindings.get(expr, [])
        if not rhs_list:
            rhs_list = pkg.bindings.get(expr, [])
            if rhs_list:
                scope = None
        out: set = set()
        for rhs in rhs_list:
            out |= resolve(rhs, pkg, env_prefix, scope, depth + 1, seen | {key})
        return out or {UNRESOLVED}

    call = RE_CALL.match(expr)
    if call:
        fname = call.group(1)
        fn = pkg.funcs.get(fname)
        key = (id(pkg), fname)
        if fn is None or key in seen:
            return {UNRESOLVED}
        out = set()
        for ret in return_exprs(fn.body):
            out |= resolve(ret, pkg, env_prefix, fn, depth + 1, seen | {key})
        return out or {UNRESOLVED}

    return {UNRESOLVED}


def resolve_concat(parts: list[str], pkg: GoPackage, env_prefix: str,
                   scope: Scope | None, depth: int, seen: frozenset) -> set:
    """Cross-product a concatenation, substituting the connector's env prefix
    for an unresolved LEADING component.

    internal/cli/platform_client.go builds its names as
        prefix := strings.ToUpper(strings.ReplaceAll(...session.CLI...))
        name := prefix + suffix
    The prefix is a runtime value, so a literal-only scan drops <PREFIX>_DB,
    <PREFIX>_DATA_DIR, <PREFIX>_HOME and friends entirely. Substituting the
    connector's own env prefix reconstructs exactly the names that code reads.
    """
    resolved = [resolve(p, pkg, env_prefix, scope, depth + 1, seen) for p in parts]
    if UNRESOLVED in resolved[0] and len(resolved[0]) == 1:
        tail = [v for v in resolved[1:]]
        if all(UNRESOLVED not in s for s in tail):
            resolved[0] = {env_prefix}
    out = {""}
    for options in resolved:
        nxt = set()
        for prefix in out:
            for opt in options:
                if opt is UNRESOLVED or prefix is UNRESOLVED:
                    nxt.add(UNRESOLVED)
                else:
                    nxt.add(prefix + opt)
        out = nxt
        if out == {UNRESOLVED}:
            break
    return out


RE_RETURN = re.compile(r"(?m)^[\t ]*return\s+(.+?)[\t ]*$")


def return_exprs(body: str) -> list[str]:
    out = []
    for m in RE_RETURN.finditer(body):
        first = split_top(m.group(1), ",")[0].strip()
        if first:
            out.append(first)
    return out


# ---------------------------------------------------------------------------
# Per-slug scan
# ---------------------------------------------------------------------------

def go_files(cli_dir: Path) -> dict[Path, list[Path]]:
    """Non-test .go files under cli/, grouped by package directory."""
    groups: dict[Path, list[Path]] = {}
    for path in sorted(cli_dir.rglob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        if any(part in SKIP_DIRS for part in path.relative_to(cli_dir).parts):
            continue
        groups.setdefault(path.parent, []).append(path)
    return groups


RE_ENV_PREFIX_CONST = re.compile(r'(?m)^[\t ]*(?:const\s+)?envPrefix\s*=\s*"([A-Z0-9_]+)"')


def env_prefix_for(slug: str, packages: list[GoPackage]) -> str:
    """The connector's own env-var prefix.

    Declared as `const envPrefix` in internal/cliutil/paths.go on presses that
    ship it (servosity is SERVOSITY_MSP, not SERVOSITY, so this cannot be
    guessed from the slug); otherwise derived from the slug.
    """
    for pkg in packages:
        for _, src in pkg.sources:
            m = RE_ENV_PREFIX_CONST.search(src)
            if m:
                return m.group(1)
    return slug.upper().replace("-", "_")


def scan_slug(slug: str) -> tuple[dict[str, list[str]], list[str], str]:
    """Return ({ENV_NAME: [read sites]}, [unresolved read sites], env prefix).

    The prefix is returned because classification needs it: the internal SUFFIX
    rules are anchored to it (see is_internal).
    """
    cli_dir = SKILLS_DIR / registry.source_dir(slug) / "cli"
    if not cli_dir.is_dir():
        return {}, [], slug.upper().replace("-", "_")

    groups = go_files(cli_dir)
    packages = [parse_package(d, files) for d, files in sorted(groups.items())]
    prefix = env_prefix_for(slug, packages)

    found: dict[str, list[str]] = {}
    unresolved: list[str] = []

    for pkg in packages:
        readers: dict[str, list[int]] = dict(BUILTIN_READERS)
        variadic: set[str] = set()
        for fn in pkg.funcs.values():
            if fn.env_param_idx:
                readers[fn.name] = sorted(fn.env_param_idx)
                if fn.variadic_env:
                    variadic.add(fn.name)

        for path, src in pkg.sources:
            rel = str(path.relative_to(ROOT))
            for name, idxs in readers.items():
                for start, args in call_sites(src, name):
                    line = src.count("\n", 0, start) + 1
                    site = f"{rel}:{line}"
                    wanted = list(range(len(args))) if name in variadic else idxs
                    for idx in wanted:
                        if idx >= len(args):
                            continue
                        for value in resolve(args[idx], pkg, prefix,
                                             pkg.func_at(path, start)):
                            if value is UNRESOLVED:
                                unresolved.append(f"{site} {name}({args[idx].strip()[:60]})")
                            elif RE_ENV_NAME.match(value):
                                found.setdefault(value, [])
                                if site not in found[value]:
                                    found[value].append(site)
    return found, unresolved, prefix


RE_WORD_BOUNDARY = re.compile(r"[\w.]")


def call_sites(src: str, name: str) -> list[tuple[int, list[str]]]:
    """Every `name(...)` call in src, as (offset, [argument expressions])."""
    out = []
    needle = name
    start = 0
    while True:
        idx = src.find(needle, start)
        if idx < 0:
            return out
        start = idx + len(needle)
        before = src[idx - 1] if idx else " "
        if RE_WORD_BOUNDARY.match(before):
            continue
        rest = src[idx + len(needle):]
        stripped = rest.lstrip()
        if not stripped.startswith("("):
            continue
        open_paren = idx + len(needle) + (len(rest) - len(stripped))
        close_paren = match_close(src, open_paren, "(", ")")
        if close_paren < 0:
            continue
        inner = src[open_paren + 1:close_paren].strip()
        args = [a for a in split_top(inner, ",")] if inner else []
        out.append((idx, args))


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------

def load_rules() -> dict:
    rules = json.loads(RULES_FILE.read_text(encoding="utf-8"))
    for key in ("internal_exact", "internal_prefixes", "internal_suffixes", "operator_exact"):
        rules.setdefault(key, [])
    rules.setdefault("waivers", {})
    return rules


def is_internal(name: str, rules: dict, env_prefix: str = "") -> bool:
    """Fail-safe classification: anything not recognised is OPERATOR-FACING.

    The suffix rules are PREFIX-ANCHORED - a name is internal by suffix only when
    it is exactly `<the connector's own env prefix><suffix>`. That is the contract
    env_schema_internal.json's own comment has always promised ("matched against
    the tail of the variable name, after the connector's own env prefix"), but the
    code used to match the bare suffix against the whole name, so ANY variable
    ending in _DB, _CONFIG, _HOME or _DATA_DIR silently classified internal in
    BOTH directions: a real credential named that way would never be prompted for
    (the #282 defect) and a phantom prompt for one would never be reported.
    Anchoring costs nothing today - all 65 connectors' suffix-classified reads are
    exactly `<prefix><suffix>`, verified before the tightening landed.
    """
    if name in set(rules["operator_exact"]):
        return False
    if name in set(rules["internal_exact"]):
        return True
    if any(name.startswith(p) for p in rules["internal_prefixes"]):
        return True
    if env_prefix and any(name == env_prefix + s for s in rules["internal_suffixes"]):
        return True
    return False


# The identity half of a machine credential. Masked like the secret half it is
# paired with (_CLIENT_SECRET / _SECRET_ACCESS_KEY / _SECRET_KEY): both halves
# together are the credential, and a UI that masks one and prints the other in
# clear is inconsistent about the same secret.
IDENTITY_SUFFIXES = ("_CLIENT_ID", "_APP_ID", "_ACCESS_KEY_ID", "_PUBLIC_KEY")


def waived_vars(slug: str, rules: dict) -> tuple[set[str], list[str]]:
    """Waived variable names for slug, plus findings for malformed waivers."""
    names: set[str] = set()
    problems: list[str] = []
    for entry in rules["waivers"].get(slug, []):
        var = (entry.get("var") or "").strip()
        reason = (entry.get("reason") or "").strip()
        if not var:
            problems.append(f"[{slug}] waiver entry has no `var`")
            continue
        if not reason:
            problems.append(
                f"[{slug}] waiver for {var} has no `reason`. A waiver is how a "
                f"credential prompt disappears; it must say why in "
                f"tools/maintainer/env_schema_internal.json."
            )
            continue
        names.add(var)
    return names, problems


# ---------------------------------------------------------------------------
# Manifest comparison (pure - driven by both the real scan and --self-test)
# ---------------------------------------------------------------------------

RE_USER_CONFIG_REF = re.compile(r"^\$\{user_config\.([a-z0-9_]+)\}$")


def evaluate(slug: str, manifest: dict, read_operator: set[str], waived: set[str],
             read_all: set[str] | None = None) -> list[str]:
    """Return every finding for one slug. Pure: no filesystem, no scanning.

    `read_all` is every resolved read INCLUDING the internally-classified ones.
    It exists so the declared-but-not-read message can tell the truth: a variable
    the binary really does read, that merely classifies internal, must not be
    reported as "no source file reads it" - that sentence tells the maintainer to
    delete a prompt that works.
    """
    findings: list[str] = []
    server = manifest.get("server") or {}
    mcp_config = server.get("mcp_config") or {}
    declared = mcp_config.get("env") or {}
    user_config = manifest.get("user_config") or {}
    if read_all is None:
        read_all = set(read_operator)

    missing = sorted(v for v in read_operator - set(declared) if v not in waived)
    for var in missing:
        findings.append(
            f"[{slug}] {var} is read by the binary but NOT declared in "
            f"manifest.json server.mcp_config.env - Claude Desktop never prompts "
            f"for it, so the MCP server starts without it."
        )

    extra = sorted(v for v in set(declared) - read_operator if v not in waived)
    for var in extra:
        if var in read_all:
            findings.append(
                f"[{slug}] {var} is declared in manifest.json AND the binary does "
                f"read it, but tools/maintainer/env_schema_internal.json classifies "
                f"it INTERNAL - a press-internal knob operators are not meant to be "
                f"prompted for. Do NOT delete the prompt on the assumption the value "
                f"goes nowhere: either add it to `operator_exact` because operators "
                f"really must supply it, or drop the prompt because the binary "
                f"derives the value itself."
            )
            continue
        findings.append(
            f"[{slug}] {var} is declared in manifest.json but NO source file "
            f"reads it - the operator is prompted for a value that goes nowhere."
        )

    for var in sorted(set(declared)):
        if not var.endswith(IDENTITY_SUFFIXES):
            continue
        value = declared[var]
        m = RE_USER_CONFIG_REF.match(value) if isinstance(value, str) else None
        if not m:
            continue
        cfg = user_config.get(m.group(1)) or {}
        if not cfg.get("sensitive"):
            findings.append(
                f"[{slug}] user_config.{m.group(1)} (behind {var}) is not "
                f"\"sensitive\": true. {var} is the identity half of a machine "
                f"credential; its secret half is masked, so this one must be too or "
                f"the same credential is half printed in clear in the install UI."
            )

    referenced: set[str] = set()
    for var, value in sorted(declared.items()):
        if not isinstance(value, str):
            findings.append(f"[{slug}] env {var} must be a '${{user_config.<key>}}' string, got {value!r}")
            continue
        m = RE_USER_CONFIG_REF.match(value)
        if not m:
            findings.append(
                f"[{slug}] env {var} is {value!r}; it must be exactly "
                f"'${{user_config.<key>}}' or the operator's answer never reaches the server."
            )
            continue
        key = m.group(1)
        referenced.add(key)
        if key not in user_config:
            findings.append(
                f"[{slug}] env {var} points at user_config.{key}, which does not "
                f"exist in manifest.json user_config."
            )

    for key in sorted(set(user_config) - referenced):
        findings.append(
            f"[{slug}] user_config.{key} is prompted for but no "
            f"server.mcp_config.env value references it - the answer is discarded."
        )
    return findings


# ---------------------------------------------------------------------------
# The other two install channels: mcp-install.md and server.json
# ---------------------------------------------------------------------------

# `"env": { ... }` inside a fenced JSON block, captured with its indentation so
# the closing brace of the map - not of the enclosing object - ends the match.
RE_ENV_BLOCK = re.compile(r'( *)"env": \{\n(.*?)\n( *)\}', re.S)
RE_ENV_PAIR = re.compile(r'"([A-Za-z_][A-Za-z0-9_]*)"\s*:\s*"(?:[^"\\]|\\.)*"')
RE_SHELL_ASSIGN = re.compile(r'\b([A-Z][A-Z0-9_]*)=')


def declared_env(manifest: dict) -> dict:
    return ((manifest.get("server") or {}).get("mcp_config") or {}).get("env") or {}


def evaluate_mcp_install(slug: str, manifest: dict, doc: str,
                         mcp_binary: str | None = None) -> list[str]:
    """Assert the manual install page carries exactly the manifest's credentials.

    Pure: the caller reads the file. `doc` is the raw mcp-install.md text and
    `mcp_binary` is the registry's `mcp_binary` for the slug (every one is
    `<slug>-mcp` today, but the registry - not this gate - owns that name).
    """
    findings: list[str] = []
    declared = set(declared_env(manifest))
    if not declared:
        return findings

    blocks = RE_ENV_BLOCK.findall(doc)
    if not blocks:
        return [
            f"[{slug}] mcp-install.md has no `\"env\": {{ ... }}` block at all, but "
            f"manifest.json declares {len(declared)} credential(s). Anyone following "
            f"the manual install path starts the server with no credentials."
        ]
    for index, (_, body, _) in enumerate(blocks, start=1):
        keys = set(RE_ENV_PAIR.findall(body))
        for var in sorted(declared - keys):
            findings.append(
                f"[{slug}] mcp-install.md env block #{index} omits {var}, which "
                f"manifest.json declares. That page is a copy-paste install path "
                f"that never touches the .mcpb bundle, so the operator is never "
                f"asked for it - exactly the #282 defect, one channel over."
            )
        for var in sorted(keys - declared):
            findings.append(
                f"[{slug}] mcp-install.md env block #{index} sets {var}, which "
                f"manifest.json does not declare. The page is telling operators to "
                f"configure a variable the connector no longer takes."
            )

    user_config = manifest.get("user_config") or {}
    required: set[str] = set()
    for var, ref in declared_env(manifest).items():
        m = RE_USER_CONFIG_REF.match(ref) if isinstance(ref, str) else None
        if m and (user_config.get(m.group(1)) or {}).get("required"):
            required.add(var)
    binary = mcp_binary or f"{slug}-mcp"
    for line in doc.split("\n"):
        if binary not in line:
            continue
        if "--transport http" not in line and "supergateway" not in line:
            continue
        present = set(RE_SHELL_ASSIGN.findall(line))
        for var in sorted(required - present):
            findings.append(
                f"[{slug}] mcp-install.md's remote launch line runs {binary} without "
                f"{var}, which manifest.json marks required. A remote/bridged install "
                f"following that line boots without the credential."
            )
    return findings


def evaluate_server_json(slug: str, manifest: dict, server: dict) -> list[str]:
    """Assert the MCP Registry channel carries the same credentials as the manifest.

    Pure: the caller reads the file. Descriptions are deliberately NOT compared -
    server.json has always carried the registry's shorter one-liner, and gating
    prose would false-RED all 65 connectors while catching nothing an operator
    can feel.
    """
    findings: list[str] = []
    declared = declared_env(manifest)
    user_config = manifest.get("user_config") or {}
    if not declared:
        return findings

    packages = server.get("packages") or []
    if not packages:
        return [f"[{slug}] server.json declares no packages, so the registry install "
                f"asks for none of manifest.json's {len(declared)} credential(s)."]

    for pkg in packages:
        label = pkg.get("registryType") or pkg.get("identifier") or "package"
        entries = pkg.get("environmentVariables") or []
        by_name = {e.get("name"): e for e in entries if isinstance(e, dict)}

        for var in sorted(set(declared) - set(by_name)):
            findings.append(
                f"[{slug}] server.json ({label}) omits {var}, which manifest.json "
                f"declares. server.json is the MCP Registry publish channel, so a "
                f"registry install is never asked for it."
            )
        for var in sorted(set(by_name) - set(declared)):
            findings.append(
                f"[{slug}] server.json ({label}) lists {var}, which manifest.json "
                f"does not declare - the registry asks for a value the connector "
                f"no longer takes."
            )

        for var, entry in sorted(by_name.items()):
            ref = declared.get(var)
            m = RE_USER_CONFIG_REF.match(ref) if isinstance(ref, str) else None
            if not m:
                continue
            cfg = user_config.get(m.group(1)) or {}
            if bool(entry.get("isRequired")) != bool(cfg.get("required")):
                findings.append(
                    f"[{slug}] server.json ({label}) {var}.isRequired is "
                    f"{bool(entry.get('isRequired'))} but manifest user_config."
                    f"{m.group(1)}.required is {bool(cfg.get('required'))} - the two "
                    f"install paths disagree about whether the operator may skip it."
                )
            if bool(entry.get("isSecret")) != bool(cfg.get("sensitive")):
                findings.append(
                    f"[{slug}] server.json ({label}) {var}.isSecret is "
                    f"{bool(entry.get('isSecret'))} but manifest user_config."
                    f"{m.group(1)}.sensitive is {bool(cfg.get('sensitive'))} - one "
                    f"install path masks the value and the other prints it."
                )
            if (entry.get("default") or None) != (cfg.get("default") or None):
                findings.append(
                    f"[{slug}] server.json ({label}) {var}.default is "
                    f"{entry.get('default')!r} but manifest user_config."
                    f"{m.group(1)}.default is {cfg.get('default')!r} - a registry "
                    f"install starts on a different endpoint than a .mcpb install."
                )
    return findings


# ---------------------------------------------------------------------------
# Self-test: prove the gate fires on broken input AND stays silent on healthy
# ---------------------------------------------------------------------------

HEALTHY = {
    "server": {"mcp_config": {"env": {"COVE_USERNAME": "${user_config.cove_username}"}}},
    "user_config": {"cove_username": {"type": "string", "required": True}},
}

# A manifest whose single credential is the identity half of a machine pair.
IDENTITY_MANIFEST = {
    "server": {"mcp_config": {"env": {"PAX8_CLIENT_ID": "${user_config.pax8_client_id}"}}},
    "user_config": {"pax8_client_id": {"type": "string", "required": True, "sensitive": True}},
}

HEALTHY_DOC = """
```json
{ "mcpServers": { "cove": { "command": "cove-mcp",
      "env": {
        "COVE_USERNAME": "<your-cove_username>"
      }
    } } }
```
```bash
COVE_USERNAME=<value> cove-mcp --transport http --addr :7777
```
"""

HEALTHY_SERVER = {
    "packages": [{
        "registryType": "mcpb",
        "environmentVariables": [
            {"name": "COVE_USERNAME",
             "description": "a shorter registry one-liner, deliberately not compared",
             "isRequired": True, "isSecret": False},
        ],
    }],
}


def self_test() -> int:
    cases: list[tuple[str, dict, set[str], set[str], bool]] = [
        ("healthy manifest", HEALTHY, {"COVE_USERNAME"}, set(), False),
        ("read-but-not-declared",
         {"server": {"mcp_config": {"env": {}}}, "user_config": {}},
         {"COVE_USERNAME"}, set(), True),
        ("declared-but-not-read", HEALTHY, set(), set(), True),
        ("declared-but-not-read, waived", HEALTHY, set(), {"COVE_USERNAME"}, False),
        ("env value is not a user_config reference",
         {"server": {"mcp_config": {"env": {"COVE_USERNAME": "admin@example.com"}}},
          "user_config": {}},
         {"COVE_USERNAME"}, set(), True),
        ("env value points at a missing user_config key",
         {"server": {"mcp_config": {"env": {"COVE_USERNAME": "${user_config.nope}"}}},
          "user_config": {}},
         {"COVE_USERNAME"}, set(), True),
        ("user_config key nothing references",
         {"server": {"mcp_config": {"env": {}}}, "user_config": {"orphan": {}}},
         set(), set(), True),
    ]
    failed = 0
    for label, manifest, read_operator, waived, expect_findings in cases:
        findings = evaluate("selftest", manifest, read_operator, waived, read_operator)
        got = bool(findings)
        ok = got == expect_findings
        print(f"  {'ok  ' if ok else 'BAD '} {label}: "
              f"expected {'findings' if expect_findings else 'silence'}, "
              f"got {len(findings)} finding(s)")
        for f in findings:
            print(f"        {f}")
        if not ok:
            failed += 1

    # The declared-but-not-read message must tell the truth about a variable the
    # binary DOES read that merely classifies internal. Reporting "no source file
    # reads it" there tells a maintainer to delete a working prompt.
    read_but_internal = evaluate("selftest", HEALTHY, set(), set(), {"COVE_USERNAME"})
    truthful = bool(read_but_internal) and not any(
        "NO source file" in f for f in read_but_internal)
    print(f"  {'ok  ' if truthful else 'BAD '} declared + read + classified internal: "
          f"reports a finding that does NOT claim nothing reads it")
    for f in read_but_internal:
        print(f"        {f}")
    if not truthful:
        failed += 1

    identity_cases = [
        ("identity half masked like its secret half", IDENTITY_MANIFEST, False),
        ("identity half left unmasked",
         {"server": {"mcp_config": {"env": {"PAX8_CLIENT_ID": "${user_config.pax8_client_id}"}}},
          "user_config": {"pax8_client_id": {"type": "string", "required": True}}}, True),
    ]
    for label, manifest, expect in identity_cases:
        findings = [f for f in evaluate("selftest", manifest, {"PAX8_CLIENT_ID"}, set(),
                                        {"PAX8_CLIENT_ID"}) if "sensitive" in f]
        ok = bool(findings) == expect
        print(f"  {'ok  ' if ok else 'BAD '} {label}: "
              f"expected {'findings' if expect else 'silence'}, got {len(findings)}")
        for f in findings:
            print(f"        {f}")
        if not ok:
            failed += 1

    doc_cases: list[tuple[str, str, bool]] = [
        ("mcp-install.md carries every declared credential", HEALTHY_DOC, False),
        ("mcp-install.md env block omits a declared credential",
         HEALTHY_DOC.replace('"COVE_USERNAME": "<your-cove_username>"', '"COVE_PARTNER": "x"'), True),
        ("mcp-install.md env block adds an undeclared credential",
         HEALTHY_DOC.replace('"COVE_USERNAME": "<your-cove_username>"',
                             '"COVE_USERNAME": "<your-cove_username>",\n        "COVE_GONE": "x"'), True),
        ("mcp-install.md remote launch line drops a required credential",
         HEALTHY_DOC.replace("COVE_USERNAME=<value> cove-mcp", "cove-mcp"), True),
        ("mcp-install.md has no env block at all",
         "\n".join(l for l in HEALTHY_DOC.split("\n") if "env" not in l and "COVE_USERNAME" not in l), True),
    ]
    for label, doc, expect in doc_cases:
        # slug "cove" so the remote-launch-line scan finds `cove-mcp` in the fixture
        findings = evaluate_mcp_install("cove", HEALTHY, doc)
        ok = bool(findings) == expect
        print(f"  {'ok  ' if ok else 'BAD '} {label}: "
              f"expected {'findings' if expect else 'silence'}, got {len(findings)}")
        for f in findings:
            print(f"        {f}")
        if not ok:
            failed += 1

    def mutate_server(**kw):
        import copy
        out = copy.deepcopy(HEALTHY_SERVER)
        out["packages"][0]["environmentVariables"][0].update(kw)
        return out

    server_cases: list[tuple[str, dict, bool]] = [
        ("server.json matches the manifest", HEALTHY_SERVER, False),
        ("server.json omits a declared credential (the hudu HUDU_BASE_URL defect)",
         {"packages": [{"registryType": "mcpb", "environmentVariables": []}]}, True),
        ("server.json lists an undeclared credential",
         {"packages": [{"registryType": "mcpb", "environmentVariables": [
             {"name": "COVE_USERNAME", "isRequired": True, "isSecret": False},
             {"name": "COVE_GONE", "isRequired": False, "isSecret": False}]}]}, True),
        ("server.json disagrees about isRequired", mutate_server(isRequired=False), True),
        ("server.json disagrees about isSecret", mutate_server(isSecret=True), True),
        ("server.json disagrees about default", mutate_server(default="https://elsewhere"), True),
        ("server.json declares no packages", {"packages": []}, True),
        ("server.json description differs (deliberately NOT gated)",
         mutate_server(description="something else entirely"), False),
    ]
    for label, server, expect in server_cases:
        findings = evaluate_server_json("selftest", HEALTHY, server)
        ok = bool(findings) == expect
        print(f"  {'ok  ' if ok else 'BAD '} {label}: "
              f"expected {'findings' if expect else 'silence'}, got {len(findings)}")
        for f in findings:
            print(f"        {f}")
        if not ok:
            failed += 1

    rules = load_rules()
    classify_cases = [
        ("COVE_PASSWORD", "COVE", False), ("AUVIK_TENANT", "AUVIK", False),
        ("PRINTING_PRESS_CLIENT_PROFILE", "AUVIK", False),
        ("BRAND_NEW_UNKNOWN_VAR", "AUVIK", False),
        ("AUVIK_CONFIG_DIR", "AUVIK", True), ("AUVIK_LEARN_NO_CAPTURE", "AUVIK", True),
        ("XDG_CACHE_HOME", "AUVIK", True), ("NO_COLOR", "AUVIK", True),
        ("AUVIK_DB", "AUVIK", True),
        # Prefix-anchored: the same suffixes on a name that is NOT this
        # connector's own knob stay OPERATOR-FACING. Before the anchoring these
        # five classified internal, so a real credential named that way was never
        # prompted for and a phantom prompt for one was never reported.
        ("VENDOR_DB", "AUVIK", False), ("SNOWFLAKE_CONFIG", "AUVIK", False),
        ("TENANT_HOME", "AUVIK", False), ("CUSTOMER_DATA_DIR", "AUVIK", False),
        ("AUVIK_TENANT_DB", "AUVIK", False),
        # servosity's prefix is SERVOSITY_MSP, not SERVOSITY - anchoring uses the
        # connector's declared envPrefix, so its own knob still classifies internal.
        ("SERVOSITY_MSP_CONFIG_DIR", "SERVOSITY_MSP", True),
    ]
    for name, prefix, expect_internal in classify_cases:
        got = is_internal(name, rules, prefix)
        ok = got == expect_internal
        print(f"  {'ok  ' if ok else 'BAD '} classify {name} (prefix {prefix}) -> "
              f"{'internal' if got else 'operator-facing'}")
        if not ok:
            failed += 1

    if failed:
        print(f"\nSELF-TEST FAIL: {failed} case(s) behaved wrong.")
        return 1
    print("\nSELF-TEST PASS: the gate fires on every broken shape and stays silent on healthy input.")
    return 0


# ---------------------------------------------------------------------------
# Driver
# ---------------------------------------------------------------------------

def check_slug(slug: str, rules: dict, verbose: bool) -> tuple[str, list[str]]:
    """Return (status, findings). status is one of pass / skip / fail."""
    if registry.is_markdown_only(slug):
        return "skip", []
    skill_dir = SKILLS_DIR / registry.source_dir(slug)
    manifest_path = skill_dir / "manifest.json"
    if not (skill_dir / "cli").is_dir() or not manifest_path.is_file():
        return "skip", []

    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return "fail", [f"[{slug}] manifest.json is not valid JSON: {exc}"]

    waived, findings = waived_vars(slug, rules)
    reads, unresolved, prefix = scan_slug(slug)
    operator = {name for name in reads if not is_internal(name, rules, prefix)}

    if verbose:
        print(f"    env prefix {prefix}: {len(reads)} resolved, {len(unresolved)} unresolved")
        for name in sorted(reads):
            tag = "internal " if is_internal(name, rules, prefix) else "OPERATOR "
            mark = " [waived]" if name in waived else ""
            print(f"      {tag}{name}{mark}  {reads[name][0]}")
        for site in unresolved[:5]:
            print(f"      unresolved {site}")

    findings += evaluate(slug, manifest, operator, waived, set(reads))

    doc_path = skill_dir / "mcp-install.md"
    if doc_path.is_file():
        findings += evaluate_mcp_install(
            slug, manifest, doc_path.read_text(encoding="utf-8"),
            (registry.skills().get(slug) or {}).get("mcp_binary"))

    server_path = skill_dir / "server.json"
    if server_path.is_file():
        try:
            server = json.loads(server_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            findings.append(f"[{slug}] server.json is not valid JSON: {exc}")
        else:
            findings += evaluate_server_json(slug, manifest, server)

    return ("fail" if findings else "pass"), findings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--slug", help="check one skill")
    parser.add_argument("--all", action="store_true", help="check every skill (default)")
    parser.add_argument("--warn", action="store_true",
                        help="print findings as WARN: lines and exit 0")
    parser.add_argument("--self-test", action="store_true",
                        help="prove the gate fires on broken input and passes healthy input")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        return self_test()

    rules = load_rules()
    known = registry.skills()
    if args.slug is not None and args.slug not in known:
        # NOT a vacuous PASS. CI runs this per-skill from a build matrix, so a
        # drifted interpolation would otherwise exit 0 across all 65 jobs having
        # checked nothing. check_cli_claims.py sets the same precedent.
        print(f"check_env_schema: unknown slug '{args.slug}'. Known slugs come "
              f"from the registry; run with --all to check every skill.",
              file=sys.stderr)
        return 2
    slugs = [args.slug] if args.slug else sorted(known)

    findings: list[str] = []
    checked = skipped = 0
    for slug in slugs:
        if args.verbose:
            print(f"==> {slug}")
        status, slug_findings = check_slug(slug, rules, args.verbose)
        if status == "skip":
            skipped += 1
            continue
        checked += 1
        findings.extend(slug_findings)

    if findings:
        label = "WARN" if args.warn else "FAIL"
        affected = len({f.split("]")[0].lstrip("[") for f in findings})
        print(f"{label}: manifest env schema does not match what the binaries read "
              f"({len(findings)} finding(s) across {affected} skill(s)):\n")
        for finding in findings:
            print(f"  - {finding}")
        print("\n  Fix: a credential must be declared identically on all three "
              "install channels - skills/<slug>/manifest.json "
              "server.mcp_config.env as \"${user_config.<key>}\" with a matching "
              "user_config prompt (the .mcpb bundle), skills/<slug>/server.json "
              "environmentVariables (the MCP Registry), and every `env` block plus "
              "the remote launch line in skills/<slug>/mcp-install.md (the manual "
              "path). Drop it everywhere if the binary no longer reads it, or - if "
              "it must stay unprompted - add a reasoned waiver to "
              "tools/maintainer/env_schema_internal.json.")
        return 0 if args.warn else 1

    print(f"PASS: manifest env schema matches the scanned reads in {checked} skill(s) "
          f"({skipped} without a CLI).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
