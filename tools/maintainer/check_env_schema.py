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

   The reader spelling is taken from each FILE's import block, so `import env
   "os"` + `env.Getenv(...)` and a dot-import are seen; a function LITERAL bound
   to a name (`recordAdditionalAuthEnv := func(name, ...) { os.Getenv(name) }` in
   autotask/sherweb doctor.go) is treated as the helper it is, so its call sites
   are scanned; and a name carried in a struct field (`os.Getenv(policy.EnvOptOut)`)
   is resolved through the composite literal that set the field.

   A read whose name still cannot be resolved is REPORTED unless one of exactly
   two explanations applies: it is a name-as-parameter helper's own definition
   (its callers are scanned instead), or every value it can take begins with a
   literal prefix the base-owned rules already classify internal (XDG_ ...). See
   explain_unresolved(). Silently dropping these was a hole big enough to walk
   `os.Getenv(strings.Join([]string{"COVE","PASSWORD"}, "_"))` through.

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

Waivers and the rules file
--------------------------
`env_schema_internal.json` carries the internal/operator classification rules
plus per-slug waivers with a MANDATORY reason string, exempting one (slug, var)
pair from BOTH directions. A waiver with an empty reason is itself a failure.

That file is BASE-OWNED, and the code enforces it rather than asserting it:
`--base <sha>` makes the gate read the rules with `git show <base>:<path>`, so a
change-set cannot widen its own rules. Without it the file was self-grantable -
delete every COVE credential declaration, add "COVE_" to `internal_prefixes` in
the same diff, and every COVE read classifies internal, both set differences come
back empty and the gate prints PASS on a connector nobody will ever be prompted
to authenticate. CI passes the PR base SHA. When BASE cannot be read (a shallow
clone, no --base at all) the gate falls back to the working copy and says so
LOUDLY on stdout rather than failing, because a gate that reds a healthy repo
gets ignored; when BASE can be read and the working copy differs, the BASE copy
is the one in force and the difference is printed.

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
  --base <sha>    read env_schema_internal.json from this commit instead of the
                  working copy, so a change-set cannot grant itself new internal
                  classifications. CI passes the PR base SHA.
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
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR
RULES_FILE = Path(__file__).resolve().parent / "env_schema_internal.json"
RULES_REL = str(RULES_FILE.relative_to(ROOT))

# Directories under cli/ that never contain a runtime env read.
SKIP_DIRS = {"testdata", "vendor", "testenv"}

# A plausible environment-variable name. Deliberately strict: resolution walks
# string concatenation and function returns, so this is the filter that keeps a
# path fragment or a header name from being reported as a missing credential.
RE_ENV_NAME = re.compile(r"^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$")

# Reading builtins. os.Setenv is NOT here: writing a variable is not an operator
# input (immybot derives IMMYBOT_OAUTH_SCOPE that way in init()).
#
# The reader NAME is resolved per FILE from that file's import block rather than
# hardcoded as the literal "os.Getenv". Go lets any file rename the package it
# imports, so `import env "os"` + `env.Getenv("COVE_PASSWORD")` is an ordinary,
# compiling, invisible-to-a-literal-scan credential read - and a dot-import
# (`import . "os"`) drops the qualifier entirely. Neither was seen before.
BUILTIN_READ_FUNCS = ("Getenv", "LookupEnv")

RE_IMPORT_GROUP = re.compile(r"(?ms)^import\s*\((.*?)^\)")
RE_IMPORT_SPEC = re.compile(r'(?m)^[\t ]*(?:([A-Za-z_]\w*|\.|_)[\t ]+)?"([^"]+)"')
RE_IMPORT_ONE = re.compile(r'(?m)^import[\t ]+(?:([A-Za-z_]\w*|\.|_)[\t ]+)?"([^"]+)"')


def os_reader_names(src: str) -> set[str]:
    """Every spelling of os.Getenv / os.LookupEnv this FILE can use."""
    quals: set[str] = set()
    for m in RE_IMPORT_ONE.finditer(src):
        if m.group(2) == "os":
            quals.add(m.group(1) or "os")
    for group in RE_IMPORT_GROUP.finditer(src):
        for m in RE_IMPORT_SPEC.finditer(group.group(1)):
            if m.group(2) == "os":
                quals.add(m.group(1) or "os")
    names: set[str] = set()
    for qual in quals:
        if qual == "_":
            continue  # blank import: registered for side effects, never called
        for fn in BUILTIN_READ_FUNCS:
            names.add(fn if qual == "." else f"{qual}.{fn}")
    return names


def builtin_read_regex(names: set[str]) -> re.Pattern:
    """`<reader>(ident)` for any of this file's reader spellings."""
    if not names:
        return re.compile(r"(?!x)x")  # matches nothing
    alt = "|".join(re.escape(n) for n in sorted(names))
    return re.compile(r"\b(?:" + alt + r")\s*\(\s*([A-Za-z_]\w*)\s*\)")

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

RE_RANGE_ALIAS = re.compile(r"for\s+[\w,\s_]*?([A-Za-z_]\w*)\s*:=\s*range\s+([A-Za-z_]\w*)")


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
        # loop variable -> the parameter it ranges over, e.g. the `n` in
        # `for _, n := range names` inside firstNonEmptyEnv(names ...string).
        self.range_aliases: dict[str, str] = {}
        for m in RE_RANGE_ALIAS.finditer(body):
            self.range_aliases[m.group(1)] = m.group(2)

    def param_names(self) -> list[str]:
        return [p for p, _ in self.params]

    def reads_own_param(self, ident: str) -> bool:
        """Is `ident` this function's own parameter (directly or as a range alias)?

        That is the shape of a name-as-parameter helper's DEFINITION - envDir(name),
        journalEnvTruthy(name), firstNonEmptyEnv(names ...string). The read there
        resolves to nothing because the name arrives from the caller, and the
        callers ARE scanned separately (detect_env_param_helpers promotes the
        helper to a reader). So this is an explained unresolved read, not a
        hiding place.
        """
        return self.range_aliases.get(ident, ident) in self.param_names()


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
        # file -> the reader spellings that file's import block permits.
        self.file_readers: dict[Path, set[str]] = {}
        # file -> offsets of function-NAME identifiers in declarations. A
        # declaration is not a call: scanning `func envDir(name string)` as if it
        # were `envDir(...)` manufactured an unresolved read at every helper's
        # own signature, which is noise that a real unresolved read would hide in.
        self.decl_sites: dict[Path, set[int]] = {}
        # Struct-literal field -> [(value expression, its scope, its package)].
        # `os.Getenv(policy.EnvOptOut)` is a real read whose name is carried in a
        # struct field; without this it resolves to nothing.
        self.fields: dict[str, list[tuple[str, Scope, "GoPackage"]]] = {}
        # The same map merged across every package of the connector: the literal
        # is built in internal/cli and read in internal/cliutil.
        self.all_fields: dict[str, list[tuple[str, Scope, "GoPackage"]]] = {}

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
# A function LITERAL. Go's `name := func(args) { ... }` is a callable helper with
# a name, and autotask/sherweb's doctor.go hides two real credential reads behind
# one: `recordAdditionalAuthEnv := func(name, configuredValue string) { os.Getenv(name) ... }`
# called with "AUTOTASK_PSA_SECRET" and "SHERWEB_SUBSCRIPTION_KEY". A scan that
# only knows about top-level declarations sees neither the helper nor its calls.
RE_FUNC_LIT = re.compile(r"\bfunc\s*\(")
RE_LIT_NAME = re.compile(r"([A-Za-z_]\w*)\s*:?=\s*$")
# A field in a composite literal: `EnvOptOut: envOptOut,`. gofmt always leaves
# the trailing comma on the multi-line form, and a quoted map key cannot match
# the exported-identifier field name, so this does not collect map literals.
RE_FIELD = re.compile(r"(?m)^[\t ]*([A-Z]\w*)[\t ]*:[\t ]*(.+?),[\t ]*$")


def _body_span(src: str, open_paren: int) -> tuple[int, int, int] | None:
    """(close-paren, body-open-brace, body-close-brace) for a function header."""
    close_paren = match_close(src, open_paren, "(", ")")
    if close_paren < 0:
        return None
    brace = src.find("{", close_paren)
    if brace < 0:
        return None
    end = match_close(src, brace, "{", "}")
    if end < 0:
        return None
    return close_paren, brace, end


def parse_package(dirpath: Path, files: list[Path]) -> GoPackage:
    return parse_sources(
        dirpath,
        [(path, strip_comments(path.read_text(encoding="utf-8", errors="replace")))
         for path in files],
    )


def parse_sources(dirpath: Path, sources: list[tuple[Path, str]]) -> GoPackage:
    """Build the package model from already-scrubbed sources (testable)."""
    pkg = GoPackage(dirpath)
    for path, src in sources:
        pkg.sources.append((path, src))
        pkg.file_readers[path] = os_reader_names(src)
        spans: list[tuple[int, int, GoFunc]] = []
        decls: set[int] = set()
        headers: list[tuple[int, int]] = []

        for m in RE_FUNC.finditer(src):
            open_paren = src.index("(", m.end() - 1)
            span = _body_span(src, open_paren)
            if span is None:
                continue
            close_paren, brace, end = span
            decls.add(m.start(1))
            headers.append((m.start(), brace))
            params = parse_params(src[open_paren + 1:close_paren])
            fn = GoFunc(m.group(1), params, src[brace + 1:end], path)
            pkg.funcs.setdefault(fn.name, fn)
            spans.append((brace + 1, end, pkg.funcs[fn.name]))

        for m in RE_FUNC_LIT.finditer(src):
            # The `func (` of a METHOD's receiver is inside a declaration header
            # already handled above; everything else is a literal.
            if any(a <= m.start() < b for a, b in headers):
                continue
            open_paren = src.index("(", m.end() - 1)
            span = _body_span(src, open_paren)
            if span is None:
                continue
            close_paren, brace, end = span
            params = parse_params(src[open_paren + 1:close_paren])
            named = RE_LIT_NAME.search(src[max(0, m.start() - 80):m.start()])
            name = named.group(1) if named else ""
            fn = GoFunc(name, params, src[brace + 1:end], path)
            if name:
                pkg.funcs.setdefault(name, fn)
                fn = pkg.funcs[name]
            spans.append((brace + 1, end, fn))

        pkg.spans[path] = spans
        pkg.decl_sites[path] = decls

        for regex, group in ((RE_ASSIGN, 2), (RE_RANGE_LIT, 2)):
            for m in regex.finditer(src):
                scope = pkg.func_at(path, m.start()) or pkg
                scope.bind(m.group(1), m.group(group))

        for m in RE_FIELD.finditer(src):
            scope = pkg.func_at(path, m.start()) or pkg
            pkg.fields.setdefault(m.group(1), []).append((m.group(2), scope, pkg))
    detect_env_param_helpers(pkg)
    return pkg


def link_packages(packages: list[GoPackage]) -> None:
    """Give every package the connector-wide struct-field map.

    A Policy literal built in internal/cli is read in internal/cliutil, so a
    package-local field map resolves the read at one site and not the other.
    """
    merged: dict[str, list[tuple[str, Scope, GoPackage]]] = {}
    for pkg in packages:
        for field, entries in pkg.fields.items():
            merged.setdefault(field, []).extend(entries)
    for pkg in packages:
        pkg.all_fields = merged


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


def detect_env_param_helpers(pkg: GoPackage) -> None:
    """Mark functions that read one of their own string params as an env name.

    Detected rather than hardcoded, so a helper the press renames or a new one it
    introduces is covered without editing this gate. Covers envDir(name),
    journalEnvTruthy(name), globalScopeParamDefault(envName, fallback),
    kindRoot(envName, ...) and the variadic firstNonEmptyEnv(names ...string),
    whose param reaches os.Getenv through `for _, n := range names`.
    """
    for fn in pkg.funcs.values():
        names = fn.param_names()
        if not names:
            continue
        reader_re = builtin_read_regex(pkg.file_readers.get(fn.file, set()))
        for m in reader_re.finditer(fn.body):
            target = fn.range_aliases.get(m.group(1), m.group(1))
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

    sel = RE_SELECTOR.match(expr)
    if sel:
        return resolve_field(sel.group(1), pkg, env_prefix, depth, seen)

    return {UNRESOLVED}


RE_SELECTOR = re.compile(r"^[A-Za-z_]\w*\.([A-Za-z_]\w*)$")


def resolve_field(field: str, pkg: GoPackage, env_prefix: str,
                  depth: int, seen: frozenset) -> set:
    """Resolve `something.Field` through the struct literals that set Field.

    `os.Getenv(policy.EnvOptOut)` in internal/cliutil/freshness.go is a real read
    whose name lives in a field set once, in internal/cli's cachePolicy():
    `EnvOptOut: envOptOut` where `envOptOut := "<PREFIX>_NO_AUTO_REFRESH"`. The
    connector-wide field map is what lets the cliutil-side read see the cli-side
    literal; the field is resolved in the package and scope that BOUND it.
    """
    key = ("field", field)
    if key in seen:
        return {UNRESOLVED}
    entries = pkg.fields.get(field) or pkg.all_fields.get(field) or []
    out: set = set()
    for expr, scope, owner in entries:
        out |= resolve(expr, owner, env_prefix, scope, depth + 1, seen | {key})
    return out or {UNRESOLVED}


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


def resolve_prefixes(expr: str, pkg: GoPackage, env_prefix: str,
                     scope: Scope | None = None, depth: int = 0,
                     seen: frozenset = frozenset()) -> set:
    """The literal string every value of `expr` must START with, where known.

    A weaker question than resolve(), and it answers cases resolve() cannot:
    `envDir(xdgEnvVar(kind))` builds "XDG_" + <runtime suffix> + "_HOME", so the
    NAME is unknowable but the FAMILY is not - every name it can produce begins
    "XDG_", which the base-owned rules classify internal outright. That is enough
    to explain the read without weakening anything: an unknown tail cannot turn
    an internal prefix into an operator-facing credential.

    An empty string means "unknown", and a genuinely empty literal (`return ""`,
    the early-out arm of xdgEnvVar) is indistinguishable from it - which is
    harmless, because an empty variable name is never read: envDir() returns
    early on it and RE_ENV_NAME rejects it.
    """
    expr = expr.strip()
    if not expr or depth > MAX_DEPTH:
        return {""}

    lit = as_literal(expr)
    if lit is not None:
        return {lit}

    parts = split_top(expr, "+")
    if len(parts) > 1:
        return resolve_prefixes(parts[0], pkg, env_prefix, scope, depth + 1, seen)

    if RE_IDENT.match(expr):
        key = (id(scope), expr)
        if key in seen:
            return {""}
        rhs_list = scope.bindings.get(expr, []) if scope is not None else []
        if not rhs_list:
            rhs_list = pkg.bindings.get(expr, [])
            scope = None
        out: set = set()
        for rhs in rhs_list:
            out |= resolve_prefixes(rhs, pkg, env_prefix, scope, depth + 1, seen | {key})
        return out or {""}

    call = RE_CALL.match(expr)
    if call:
        fn = pkg.funcs.get(call.group(1))
        key = (id(pkg), call.group(1))
        if fn is None or key in seen:
            return {""}
        out = set()
        for ret in return_exprs(fn.body):
            out |= resolve_prefixes(ret, pkg, env_prefix, fn, depth + 1, seen | {key})
        return out or {""}

    return {""}


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


class Unresolved:
    """One environment read whose variable NAME the scan could not pin down."""

    def __init__(self, site: str, reader: str, expr: str, why: str | None):
        self.site = site
        self.reader = reader
        self.expr = expr
        self.why = why          # None == unexplained == a finding

    @property
    def key(self) -> tuple:
        return (self.site, self.reader, self.expr)

    def __str__(self) -> str:
        return f"{self.site} {self.reader}({self.expr})" + (f" [{self.why}]" if self.why else "")


def explain_unresolved(expr: str, pkg: GoPackage, env_prefix: str,
                       scope: Scope | None, rules: dict) -> str | None:
    """Why this unresolvable read is nevertheless safe, or None if it is not.

    Two explanations, and only two, because everything else is a place a
    credential can hide:

      helper-definition - the argument is the enclosing function's own parameter.
          That is a name-as-parameter helper's own body (envDir(name),
          firstNonEmptyEnv(names ...string)); the name arrives from callers, and
          detect_env_param_helpers has already promoted the helper to a reader so
          those callers ARE scanned. Nothing hides here.
      internal-family - every value the expression can take begins with a literal
          prefix the BASE-OWNED rules classify internal (XDG_, PP_, ...). An
          unknown tail cannot turn XDG_<something> into a credential.

    Anything else - a strings.Join, a map lookup, a value read off the wire - is
    reported, because `os.Getenv(strings.Join([]string{"COVE","PASSWORD"}, "_"))`
    is a perfectly ordinary Go expression that reads a credential and resolves to
    nothing at all.
    """
    ident = expr.strip()
    if RE_IDENT.match(ident) and isinstance(scope, GoFunc) and scope.reads_own_param(ident):
        return "helper-definition"

    prefixes = {p for p in resolve_prefixes(expr, pkg, env_prefix, scope) if p}
    if prefixes and all(
        any(p.startswith(rule) for rule in rules["internal_prefixes"]) for p in prefixes
    ):
        return "internal-family"
    return None


def scan_slug(slug: str, rules: dict) -> tuple[dict[str, list[str]], list[Unresolved], str]:
    """Return ({ENV_NAME: [read sites]}, [unresolved reads], env prefix).

    The prefix is returned because classification needs it: the internal SUFFIX
    rules are anchored to it (see is_internal).
    """
    cli_dir = SKILLS_DIR / registry.source_dir(slug) / "cli"
    if not cli_dir.is_dir():
        return {}, [], slug.upper().replace("-", "_")

    groups = go_files(cli_dir)
    packages = [parse_package(d, files) for d, files in sorted(groups.items())]
    link_packages(packages)
    prefix = env_prefix_for(slug, packages)
    found, unresolved = scan_packages(packages, prefix, rules)
    return found, unresolved, prefix


def scan_packages(packages: list[GoPackage], prefix: str,
                  rules: dict) -> tuple[dict[str, list[str]], list[Unresolved]]:
    """The scan itself, over an already-parsed package list (filesystem-free)."""
    found: dict[str, list[str]] = {}
    unresolved: dict[tuple, Unresolved] = {}

    for pkg in packages:
        helpers: dict[str, list[int]] = {}
        variadic: set[str] = set()
        for fn in pkg.funcs.values():
            if fn.env_param_idx:
                helpers[fn.name] = sorted(fn.env_param_idx)
                if fn.variadic_env:
                    variadic.add(fn.name)

        for path, src in pkg.sources:
            try:
                rel = str(path.relative_to(ROOT))
            except ValueError:
                rel = str(path)   # self-test fixtures live outside the repo
            # Reader NAMES are per-file: `import env "os"` renames the reader,
            # and a file that does not import os has none.
            readers: dict[str, list[int]] = {
                name: [0] for name in pkg.file_readers.get(path, set())
            }
            readers.update(helpers)
            decls = pkg.decl_sites.get(path, set())
            for name, idxs in sorted(readers.items()):
                for start, args in call_sites(src, name):
                    if start in decls:
                        continue  # a declaration is not a call
                    line = src.count("\n", 0, start) + 1
                    site = f"{rel}:{line}"
                    scope = pkg.func_at(path, start)
                    wanted = list(range(len(args))) if name in variadic else idxs
                    for idx in wanted:
                        if idx >= len(args):
                            continue
                        arg = args[idx].strip()
                        for value in resolve(args[idx], pkg, prefix, scope):
                            if value is UNRESOLVED:
                                rec = Unresolved(
                                    site, name, arg[:80],
                                    explain_unresolved(args[idx], pkg, prefix, scope, rules))
                                unresolved.setdefault(rec.key, rec)
                            elif RE_ENV_NAME.match(value):
                                found.setdefault(value, [])
                                if site not in found[value]:
                                    found[value].append(site)
    return found, sorted(unresolved.values(), key=lambda u: u.key)


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

def git(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", "-C", str(ROOT), *args],
                          capture_output=True, text=True)


def load_rules(base: str | None = None) -> tuple[dict, list[str]]:
    """The classification rules, read from BASE when BASE can be reached.

    This is the whole reason the rules live in their own file. Loading them from
    the PR checkout makes them SELF-GRANTABLE: a change-set can delete every
    COVE credential declaration from the manifests and, in the same diff, add
    "COVE_" to `internal_prefixes`. Every COVE read then classifies internal,
    both directions of the comparison come back empty, and the gate whose entire
    job is "a credential nobody is prompted for must fail" prints PASS. Adding a
    rule here is exactly how a credential prompt disappears, so the rules must
    come from a tree the change-set does not control - the same posture
    security_suppressions.json has always claimed.

    Reading from BASE is therefore the DEFAULT, not an opt-in. Fallback to the
    working copy is deliberate and LOUD (a NOTICE naming why), because a gate
    that hard-fails whenever BASE is unfetchable - a shallow clone, a fresh
    clone with no origin, the very first commit that introduces this file - is a
    false-RED, and a false-RED teaches maintainers to ignore the gate.

    Returns (rules, notices). A rules change that must take effect lands on the
    base branch first; until it does, the gate prints the diff and keeps using
    BASE's copy.
    """
    notices: list[str] = []
    working = RULES_FILE.read_text(encoding="utf-8")
    text = working
    if base:
        show = git("show", f"{base}:{RULES_REL}")
        if show.returncode == 0:
            text = show.stdout
            if text != working:
                notices.append(
                    f"{RULES_REL} DIFFERS from {base}. The BASE copy is the one in "
                    f"force: these rules classify a variable as internal, which is "
                    f"how a credential prompt disappears, so a change-set may not "
                    f"grant itself new ones. Land the rules change on the base "
                    f"branch first."
                )
        else:
            notices.append(
                f"cannot read {RULES_REL} at BASE {base!r} "
                f"({(show.stderr or '').strip().splitlines()[:1] or ['unreachable']}) "
                f"- falling back to the WORKING COPY of the rules. In that mode the "
                f"rules are self-grantable: a diff that adds an internal_prefixes "
                f"entry silences its own findings. Pass --base <merge-base sha> on a "
                f"checkout deep enough to contain it."
            )
    else:
        notices.append(
            f"no --base given - using the WORKING COPY of {RULES_REL}. The rules are "
            f"self-grantable in that mode; CI passes the PR base SHA so a diff cannot "
            f"widen its own internal-variable list."
        )

    try:
        rules = json.loads(text)
    except json.JSONDecodeError as exc:
        if text is working:
            raise
        notices.append(
            f"{RULES_REL} at BASE {base!r} is not valid JSON ({exc}) - falling back "
            f"to the working copy."
        )
        rules = json.loads(working)
    for key in ("internal_exact", "internal_prefixes", "internal_suffixes", "operator_exact"):
        rules.setdefault(key, [])
    rules.setdefault("waivers", {})
    return rules, notices


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
    launch_lines = [
        line for line in doc.split("\n")
        if binary in line and ("--transport http" in line or "supergateway" in line)
    ]
    # EXISTENCE first. Asserting only "every required credential appears ON the
    # launch line" is vacuous when there is no launch line: delete every
    # `<slug>-mcp --transport http` line from the page and the loop iterates zero
    # times, reports nothing missing, and the gate goes green on a page that no
    # longer tells a remote/bridged operator how to start the server at all. A
    # connector that declares credentials ships that line today - all 65 of them -
    # so its absence is a regression, not a shape this fleet has.
    if not launch_lines:
        findings.append(
            f"[{slug}] mcp-install.md has no remote launch line: no line runs "
            f"{binary} with `--transport http` or through supergateway, yet "
            f"manifest.json declares {len(declared)} credential(s). That line is the "
            f"only place the page shows how to start the server outside Claude "
            f"Desktop, and it is where the per-credential assertion below looks; "
            f"with it gone the page documents no remote install and this gate has "
            f"nothing to check."
        )
    for line in launch_lines:
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


# Go fixtures for the scanner half of the self-test. Each is a whole package.
SCAN_FIXTURES: dict[str, tuple[str, set[str], set[str]]] = {}


def _fixture(label: str, src: str, expect_reads: set[str], expect_unexplained: set[str]):
    SCAN_FIXTURES[label] = (src, expect_reads, expect_unexplained)


_fixture(
    "plain os.Getenv literal",
    'package cli\nimport "os"\nfunc a() string { return os.Getenv("COVE_PASSWORD") }\n',
    {"COVE_PASSWORD"}, set(),
)
_fixture(
    "ALIASED os import (import env \"os\")",
    'package cli\nimport env "os"\nfunc a() string { return env.Getenv("COVE_PASSWORD") }\n',
    {"COVE_PASSWORD"}, set(),
)
_fixture(
    "grouped aliased os import",
    'package cli\nimport (\n\t"fmt"\n\tsysenv "os"\n)\nvar _ = fmt.Sprint\n'
    'func a() string { return sysenv.LookupEnvX }\n'
    'func b() string { v, _ := sysenv.LookupEnv("COVE_PASSWORD"); return v }\n',
    {"COVE_PASSWORD"}, set(),
)
_fixture(
    "name computed by strings.Join (the hidden-credential shape)",
    'package cli\nimport (\n\t"os"\n\t"strings"\n)\n'
    'func a() string { return os.Getenv(strings.Join([]string{"COVE", "PASSWORD"}, "_")) }\n',
    set(), {"strings.Join([]string{\"COVE\", \"PASSWORD\"}, \"_\")"},
)
_fixture(
    "name-as-parameter helper: definition explained, call sites resolved",
    'package cli\nimport "os"\n'
    'func envDir(name string) string { return os.Getenv(name) }\n'
    'func a() string { return envDir("COVE_PASSWORD") }\n',
    {"COVE_PASSWORD"}, set(),
)
_fixture(
    "function LITERAL helper (the autotask/sherweb doctor.go shape)",
    'package cli\nimport "os"\n'
    'func a() string {\n'
    '\trecord := func(name, other string) string {\n'
    '\t\treturn os.Getenv(name)\n'
    '\t}\n'
    '\treturn record("COVE_PASSWORD", "")\n'
    '}\n',
    {"COVE_PASSWORD"}, set(),
)
_fixture(
    "internal FAMILY: XDG_ + runtime tail",
    'package cliutil\nimport (\n\t"os"\n\t"strings"\n)\n'
    'func xdgEnvVar(kind int) string {\n'
    '\tsuffix := strings.TrimSuffix(pathKindEnvSuffix(kind), "_DIR")\n'
    '\tif suffix == "" {\n\t\treturn ""\n\t}\n'
    '\treturn "XDG_" + suffix + "_HOME"\n}\n'
    'func envDir(name string) string { return os.Getenv(name) }\n'
    'func a(kind int) string { return envDir(xdgEnvVar(kind)) }\n',
    set(), set(),
)
_fixture(
    "name carried in a struct field",
    'package cli\nimport "os"\n'
    'type Policy struct {\n\tEnvOptOut string\n}\n'
    'func policyFor() Policy {\n'
    '\toptOut := "COVE_PASSWORD"\n'
    '\treturn Policy{\n\t\tEnvOptOut: optOut,\n\t}\n}\n'
    'func a() string {\n\tp := policyFor()\n\treturn os.Getenv(p.EnvOptOut)\n}\n',
    {"COVE_PASSWORD"}, set(),
)


def scanner_self_test(rules: dict) -> int:
    """Prove the SCANNER both directions: it sees the reads it must see, and it
    REPORTS the ones it cannot resolve instead of dropping them on the floor."""
    failed = 0
    for label, (src, expect_reads, expect_unexplained) in SCAN_FIXTURES.items():
        pkg = parse_sources(Path("/fixture"),
                            [(Path("/fixture/x.go"), strip_comments(src))])
        link_packages([pkg])
        found, unresolved = scan_packages([pkg], "COVE", rules)
        got_reads = set(found)
        got_unexplained = {u.expr for u in unresolved if u.why is None}
        ok = got_reads == expect_reads and got_unexplained == expect_unexplained
        print(f"  {'ok  ' if ok else 'BAD '} scan {label}: reads={sorted(got_reads)} "
              f"unexplained={sorted(got_unexplained)}")
        if not ok:
            print(f"        expected reads={sorted(expect_reads)} "
                  f"unexplained={sorted(expect_unexplained)}")
            failed += 1
    return failed


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
        ("mcp-install.md remote launch line DELETED entirely (the vacuous-loop case)",
         HEALTHY_DOC.replace("COVE_USERNAME=<value> cove-mcp --transport http --addr :7777",
                             "# (remote install section removed)"), True),
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

    rules, _ = load_rules()
    failed += scanner_self_test(rules)

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
    reads, unresolved, prefix = scan_slug(slug, rules)
    operator = {name for name in reads if not is_internal(name, rules, prefix)}

    if verbose:
        explained = [u for u in unresolved if u.why]
        print(f"    env prefix {prefix}: {len(reads)} resolved, "
              f"{len(unresolved)} unresolved ({len(explained)} explained)")
        for name in sorted(reads):
            tag = "internal " if is_internal(name, rules, prefix) else "OPERATOR "
            mark = " [waived]" if name in waived else ""
            print(f"      {tag}{name}{mark}  {reads[name][0]}")
        for rec in unresolved:
            print(f"      unresolved {rec}")

    # An unresolved read is a FINDING, not a silent list entry. It used to be
    # collected and dropped: `os.Getenv(strings.Join([]string{"COVE","PASSWORD"},"_"))`
    # reads a credential, resolves to nothing, and the gate printed PASS. Only
    # the two explanations in explain_unresolved() are accepted, so the residue
    # this reports is exactly the residue nothing can vouch for.
    for rec in unresolved:
        if rec.why:
            continue
        findings.append(
            f"[{slug}] {rec.site} reads the environment through an expression this "
            f"gate cannot resolve to a variable name: {rec.reader}({rec.expr}). An "
            f"unresolvable read is indistinguishable from a hidden credential, so "
            f"it cannot be compared against manifest.json at all. Bind the name to "
            f"a string constant (or pass it as a literal) so both this gate and a "
            f"human reading the source can see which variable is being read."
        )

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
    parser.add_argument("--base", default=None,
                        help="commit to read env_schema_internal.json from, so a "
                             "change-set cannot widen its own internal-variable "
                             "rules. PRs: github.event.pull_request.base.sha. "
                             "Pushes: github.event.before.")
    parser.add_argument("--self-test", action="store_true",
                        help="prove the gate fires on broken input and passes healthy input")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    if args.self_test:
        return self_test()

    rules, rule_notices = load_rules(args.base)
    for notice in rule_notices:
        print(f"check_env_schema: NOTE {notice}")
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
