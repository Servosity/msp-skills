#!/usr/bin/env python3
"""Gate: every CLI invocation a skill's docs claim must exist in the real binary.

This is the load-bearing doc-vs-reality check. Docs drift: a command gets
renamed, a flag is dropped, an example is copied from an older version - and the
README now tells an MSP to run something the shipped binary rejects. This gate
builds the actual CLI, enumerates its real command + flag surface from `--help`,
then walks every invocation in the docs and asserts it resolves.

How it works, per slug in the registry:

  1. Build the CLI binary to a tempdir:
       go build -o <tmp>/<cli_binary> ./cmd/<cli-cmd-dir>   (run in skills/<slug>/cli)
     A build failure fails the slug.

  2. Enumerate the surface. The printing-press binaries hide most of their
     hundreds of commands from the top-level `--help` (Cobra `Hidden`), so a
     `--help` BFS sees only a fraction of the real surface and would flag every
     hidden-but-real command in the SKILL.md Command Reference as "unknown".
     Instead we read the binary's own `agent-context` JSON (schema_version 3),
     which emits the COMPLETE command tree plus each command's flags. Global /
     persistent flags (which agent-context does not list separately) are read
     from the root `--help` Flags:/Global Flags: sections. If agent-context is
     unavailable, we fall back to a depth-3 `--help` BFS (cached, 10s timeout).
     The result is a map {command-path-tuple: set-of-flags} + a global-flag set.

  3. Extract claims from README.md, SKILL.md, guide.md, page.json:
       - inside fenced code blocks tagged bash/sh/shell/console/powershell
       - inside inline backticks
     A claim is any token equal to the cli_binary or mcp_binary name (at a word
     boundary, not part of a URL/path). The words after it, up to a pipe /
     redirect / && / newline, are parsed: leading non-flag words form the
     subcommand path (we match the longest known prefix; the rest are positional
     args, which are fine); `--flag` tokens are validated against that command's
     flags + the global flags.

  4. Findings: an unknown command path, or an unknown flag for a known command.
     A difflib "did you mean" hint is added when a close match exists.

False-positive mitigations: skip text/json/yaml/untagged/other-language fences;
honor `<!-- cli-claims:ignore -->` (skips the next fenced block, or the rest of
the current line for inline use); treat `<...>` placeholder tokens and quoted
strings as positional args (never subcommands); ignore a binary name that is
part of a longer word, a URL, or a filesystem path.

Modes:
  --warn          print findings as WARN: lines, exit 0 (calibration mode)
  --slug <slug>   check only this one skill (the CI per-skill build matrix uses
                  this so each matrix job verifies only its own skill, where Go
                  is already set up). Omit to check every skill in the registry.
  (default)       findings are failures, exit 1

Pure stdlib. Run locally:
    python3 tools/maintainer/check_cli_claims.py --warn
    python3 tools/maintainer/check_cli_claims.py --warn --slug halopsa
    python3 tools/maintainer/check_cli_claims.py
"""

from __future__ import annotations

import difflib
import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR

DOC_FILES = ["README.md", "SKILL.md", "guide.md", "page.json"]
SHELL_FENCES = {"bash", "sh", "shell", "console", "powershell"}
HELP_TIMEOUT = 10
DEPTH_LIMIT = 3
IGNORE_MARKER = "cli-claims:ignore"

# A Cobra "Available Commands:" entry: two-space indent, a command name (no
# spaces), then >=2 spaces and the description. We only need the name.
RE_AVAIL_CMD = re.compile(r"^  ([A-Za-z][\w-]*)(?:\s{2,}.*)?$")
# A flag line in a Flags:/Global Flags: section: optional short flag, then the
# long flag. e.g. "  -h, --help", "      --apply", "      --db string".
RE_FLAG = re.compile(r"^\s+(?:-\w,\s+)?--([a-z0-9][a-z0-9-]*)")
# Section headers.
SEC_AVAILABLE = "Available Commands:"
SEC_FLAGS = "Flags:"
SEC_GLOBAL_FLAGS = "Global Flags:"


# ---------------------------------------------------------------------------
# Surface enumeration
# ---------------------------------------------------------------------------

class Surface:
    """The real command + flag surface of one built binary."""

    def __init__(self) -> None:
        # command-path tuple -> set of long-flag names valid at that command
        self.commands: dict[tuple[str, ...], set[str]] = {}
        self.global_flags: set[str] = set()
        self._help_cache: dict[tuple[str, ...], str] = {}

    def has_command(self, path: tuple[str, ...]) -> bool:
        return path in self.commands

    def all_command_strings(self) -> list[str]:
        return [" ".join(p) for p in self.commands if p]


def _run_help(binary: Path, sub: tuple[str, ...], cache: dict) -> str:
    if sub in cache:
        return cache[sub]
    try:
        out = subprocess.run(
            [str(binary), *sub, "--help"],
            capture_output=True,
            text=True,
            timeout=HELP_TIMEOUT,
        )
        text = (out.stdout or "") + "\n" + (out.stderr or "")
    except (OSError, subprocess.SubprocessError):
        text = ""
    cache[sub] = text
    return text


def _parse_help(text: str) -> tuple[list[str], set[str], set[str]]:
    """Return (child command names, local flags, global flags) from a help text."""
    children: list[str] = []
    local_flags: set[str] = set()
    global_flags: set[str] = set()

    section = None
    for line in text.splitlines():
        stripped = line.strip()
        if stripped == SEC_AVAILABLE:
            section = "avail"
            continue
        if stripped == SEC_FLAGS:
            section = "flags"
            continue
        if stripped == SEC_GLOBAL_FLAGS:
            section = "gflags"
            continue
        # A blank line or a new "Xxx:" header (other than the flag sections)
        # ends the current section.
        if not stripped:
            section = None
            continue
        if stripped.endswith(":") and stripped not in (
            SEC_AVAILABLE, SEC_FLAGS, SEC_GLOBAL_FLAGS
        ):
            section = None
            continue

        if section == "avail":
            m = RE_AVAIL_CMD.match(line)
            if m:
                name = m.group(1)
                if name not in ("help", "completion"):
                    children.append(name)
        elif section == "flags":
            m = RE_FLAG.match(line)
            if m:
                local_flags.add(m.group(1))
        elif section == "gflags":
            m = RE_FLAG.match(line)
            if m:
                global_flags.add(m.group(1))

    return children, local_flags, global_flags


def _global_flags_from_root(binary: Path, cache: dict) -> set[str]:
    """Read the root --help and return its persistent (Global Flags + Flags) set."""
    text = _run_help(binary, (), cache)
    _children, local_flags, global_flags = _parse_help(text)
    return local_flags | global_flags


def _surface_from_agent_context(binary: Path, s: Surface) -> bool:
    """Populate s from the binary's agent-context JSON. Return True on success."""
    try:
        out = subprocess.run(
            [str(binary), "agent-context"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (OSError, subprocess.SubprocessError):
        return False
    if out.returncode != 0 or not out.stdout.strip():
        return False
    try:
        data = json.loads(out.stdout)
    except (json.JSONDecodeError, ValueError):
        return False
    cmds = data.get("commands")
    if not isinstance(cmds, list):
        return False

    def walk(nodes: list, prefix: tuple[str, ...]) -> None:
        for node in nodes:
            name = node.get("name")
            if not name:
                continue
            path = prefix + (name,)
            flags = {f.get("name") for f in (node.get("flags") or []) if f.get("name")}
            s.commands[path] = flags
            subs = node.get("subcommands")
            if isinstance(subs, list) and subs:
                walk(subs, path)

    walk(cmds, ())
    s.commands[()] = set()  # the root itself is a valid (empty) path
    return True


def enumerate_surface(binary: Path) -> Surface:
    s = Surface()
    # Global/persistent flags always come from root --help (agent-context does
    # not list them separately).
    s.global_flags |= _global_flags_from_root(binary, s._help_cache)

    if _surface_from_agent_context(binary, s):
        return s

    # Fallback: depth-limited --help BFS (best-effort when agent-context absent).
    def walk(path: tuple[str, ...], depth: int) -> None:
        text = _run_help(binary, path, s._help_cache)
        children, local_flags, global_flags = _parse_help(text)
        s.commands[path] = local_flags
        if global_flags:
            s.global_flags |= global_flags
        if depth >= DEPTH_LIMIT:
            return
        for child in children:
            child_path = path + (child,)
            if child_path not in s.commands:
                walk(child_path, depth + 1)

    walk((), 0)
    return s


def build_binary(slug: str, cli_binary: str, tmp: Path) -> Path | None:
    """go build the CLI to tmp/<cli_binary>; return the path, or None on failure."""
    cli_root = SKILLS_DIR / slug / "cli"
    cli_cmd, _mcp_cmd = registry.cmd_dirs(slug)
    out = tmp / cli_binary
    try:
        proc = subprocess.run(
            ["go", "build", "-o", str(out), cli_cmd],
            cwd=str(cli_root),
            capture_output=True,
            text=True,
            timeout=300,
        )
    except (OSError, subprocess.SubprocessError) as e:
        print(f"  {slug}: go build error: {e}", file=sys.stderr)
        return None
    if proc.returncode != 0 or not out.exists():
        print(f"  {slug}: go build FAILED:\n{proc.stderr}", file=sys.stderr)
        return None
    return out


# ---------------------------------------------------------------------------
# Claim extraction from docs
# ---------------------------------------------------------------------------

# A single shell stop boundary inside one line. A trailing backslash is a
# line-continuation marker, not an argument, so it ends the invocation too.
STOP_TOKENS = {"|", "||", "&&", ">", ">>", "<", "2>", ";", "&", "\\"}


def _tokenize(segment: str) -> list[str]:
    """Split a command segment into shell-ish tokens, honoring quotes."""
    return _shlex_lite(segment)


def _shlex_lite(s: str) -> list[str]:
    """A tiny quote-aware splitter (avoids shlex choking on unbalanced docs)."""
    tokens: list[str] = []
    cur = ""
    quote = None
    for ch in s:
        if quote:
            if ch == quote:
                quote = None
            else:
                cur += ch
            continue
        if ch in ("'", '"'):
            quote = ch
            cur += "\x00"  # mark this token as a quoted (positional) value
            continue
        if ch.isspace():
            if cur:
                tokens.append(cur)
                cur = ""
            continue
        cur += ch
    if cur:
        tokens.append(cur)
    return tokens


# Cobra subcommand names are lowercase words of [a-z0-9] joined by hyphens.
# Anything that does not match this shape cannot be a subcommand, so it must be
# a positional argument (a time like "9:00", a number, a quoted string, a
# placeholder, a path, an env-var assignment, etc.).
RE_COMMAND_NAME = re.compile(r"^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$")


def _is_positional(tok: str) -> bool:
    """A token that should be treated as an argument value, not a subcommand."""
    if not tok:
        return True
    if "\x00" in tok:  # was quoted
        return True
    if tok.startswith("-"):
        return False  # flags handled separately
    if tok.startswith("<") or ">" in tok:
        return True
    if "=" in tok:
        return True
    if tok.startswith("$") or tok.startswith("{"):
        return True
    if "/" in tok or "." in tok:
        return True
    if tok.isdigit():
        return True
    # Final shape gate: only a lowercase hyphenated word can be a subcommand.
    # (Capital letters, colons, slashes, quotes -> positional value.)
    if not RE_COMMAND_NAME.match(tok):
        return True
    return False


def _looks_like_value(tok: str) -> bool:
    """Heuristic: a bare token that is plainly an argument value, not a command."""
    if "\x00" in tok:
        return True
    if tok.isdigit():
        return True
    return False


def parse_invocation(tokens: list[str], binary: str, surface: Surface):
    """Given tokens beginning at the binary name, resolve the command path and
    collect the flags used. Returns (command_path_tuple, [flag_names],
    remaining_positionals_consumed_ok). Stops at the first shell boundary."""
    # tokens[0] is the binary (or binary= something); start after it.
    rest = []
    for t in tokens[1:]:
        if t in STOP_TOKENS:
            break
        rest.append(t)

    # Resolve the subcommand path: greedily consume leading non-flag,
    # non-positional words while they extend a known command path.
    path: tuple[str, ...] = ()
    flags: list[str] = []
    i = 0
    # First, find the longest command path from the leading words.
    while i < len(rest):
        tok = rest[i]
        if tok.startswith("-"):
            break
        if _is_positional(tok):
            break
        candidate = path + (tok,)
        if surface.has_command(candidate):
            path = candidate
            i += 1
            continue
        # Token is a non-flag word but does not extend a known command.
        # If the current path is already a known command, this word is a
        # positional arg; stop extending.
        break

    # Everything after is flags + positional args. Collect flags.
    for tok in rest[i:]:
        if tok in STOP_TOKENS:
            break
        if tok.startswith("--"):
            name = tok[2:].split("=", 1)[0]
            if name:
                flags.append(name)
        # short flags (-x) and positionals are ignored for validation
    return path, flags


def _strip_inline_ignored(line: str) -> str:
    """Honor an inline `<!-- cli-claims:ignore -->` by dropping the rest of the
    line from the marker onward."""
    idx = line.find(IGNORE_MARKER)
    if idx != -1:
        return line[:idx]
    return line


def extract_claims(text: str, binaries: list[str]):
    """Yield (line_number, raw_snippet, tokens, binary) for every CLI invocation
    in fenced shell blocks and inline backticks of `text`."""
    lines = text.splitlines()
    in_fence = False
    fence_lang = None
    skip_next_fence = False
    claims = []

    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        # Fence open/close detection.
        fence_match = re.match(r"^```(\S*)", stripped) or re.match(r"^~~~(\S*)", stripped)
        if fence_match and not in_fence:
            fence_lang = (fence_match.group(1) or "").lower()
            in_fence = True
            if skip_next_fence:
                # consume until close, then reset
                i += 1
                while i < len(lines) and not (
                    lines[i].strip().startswith("```") or lines[i].strip().startswith("~~~")
                ):
                    i += 1
                in_fence = False
                skip_next_fence = False
                i += 1
                continue
            i += 1
            continue
        if (stripped.startswith("```") or stripped.startswith("~~~")) and in_fence:
            in_fence = False
            fence_lang = None
            i += 1
            continue

        # A standalone ignore marker on its own line arms skip-next-fence.
        if not in_fence and IGNORE_MARKER in line:
            skip_next_fence = True

        if in_fence:
            if fence_lang in SHELL_FENCES:
                _collect_line(line, i + 1, binaries, claims, inline=False)
        else:
            # Inline backticks outside fences.
            for seg in re.findall(r"`([^`]+)`", _strip_inline_ignored(line)):
                _collect_segment(seg, i + 1, binaries, claims)

        i += 1

    return claims


def _collect_line(line: str, lineno: int, binaries: list[str], claims: list, inline: bool):
    work = _strip_inline_ignored(line)
    _collect_segment(work, lineno, binaries, claims)


# Match a binary name at a word boundary, not preceded by a path/URL char.
def _binary_positions(segment: str, binary: str):
    positions = []
    start = 0
    blen = len(binary)
    while True:
        idx = segment.find(binary, start)
        if idx == -1:
            break
        start = idx + 1
        # not preceded by an alnum, '-', '/', '.' (URL/path/longer-word)
        if idx > 0:
            prev = segment[idx - 1]
            if prev.isalnum() or prev in "-/._":
                continue
        # not followed by an alnum, '-', '/', '.' (longer word / URL / path)
        after = idx + blen
        if after < len(segment):
            nxt = segment[after]
            if nxt.isalnum() or nxt in "-/._":
                continue
        positions.append(idx)
    return positions


def _collect_segment(segment: str, lineno: int, binaries: list[str], claims: list):
    for binary in binaries:
        for pos in _binary_positions(segment, binary):
            tail = segment[pos:]
            tokens = _tokenize(tail)
            if not tokens:
                continue
            claims.append((lineno, tail.strip(), tokens, binary))


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def check_slug(slug: str, entry: dict, surface: Surface, findings: list[str]) -> None:
    cli_binary = entry["cli_binary"]
    mcp_binary = entry["mcp_binary"]
    binaries = [cli_binary, mcp_binary]
    known_cmds = surface.all_command_strings()

    for fname in DOC_FILES:
        fpath = SKILLS_DIR / slug / fname
        if not fpath.exists():
            continue
        try:
            text = fpath.read_text(encoding="utf-8")
        except OSError:
            continue
        rel = fpath.relative_to(ROOT)

        for lineno, snippet, tokens, binary in extract_claims(text, binaries):
            # The mcp binary is a server: it takes no subcommands. Only flag
            # validation would apply; we accept any usage of it (it is normally
            # referenced bare, e.g. `halopsa-mcp`).
            if binary == mcp_binary:
                continue

            path, flags = parse_invocation(tokens, binary, surface)

            rest = []
            for t in tokens[1:]:
                if t in STOP_TOKENS:
                    break
                rest.append(t)
            consumed = len(path)

            # Unknown-command detection. `path` is the longest prefix of leading
            # non-flag words that matches a real command (possibly the empty
            # root path). The next un-consumed word is an unknown command IFF it
            # is a bare word (not a flag, not a placeholder/value) that fails to
            # extend the resolved path. Once consumed, any further bare word is a
            # positional argument - so we only inspect the FIRST un-consumed
            # word. A bare-binary reference (no following words) is always fine.
            nxt = rest[consumed] if consumed < len(rest) else None
            if (
                nxt is not None
                and not nxt.startswith("-")
                and not _is_positional(nxt)
                and not surface.has_command(path + (nxt,))
            ):
                attempted = rest[: consumed + 1]
                bad = " ".join([binary] + attempted)
                hint = _hint(" ".join(attempted), known_cmds)
                msg = f"{slug} {rel}:{lineno}: unknown command '{bad}'"
                if hint:
                    msg += f" (did you mean '{binary} {hint}'?)"
                findings.append(msg)
                continue

            # Validate flags against the resolved command + global flags.
            valid = surface.commands.get(path, set()) | surface.global_flags
            # Also fold in flags from ancestor commands (Cobra persistent flags
            # propagate downward); be lenient and accept any ancestor's flags.
            for d in range(len(path)):
                valid |= surface.commands.get(path[: d + 1], set())
            valid |= surface.commands.get((), set())

            for fl in flags:
                if fl not in valid:
                    cmd_str = (binary + " " + " ".join(path)).strip()
                    hint = _hint(fl, sorted(valid))
                    msg = f"{slug} {rel}:{lineno}: unknown flag '--{fl}' for '{cmd_str}'"
                    if hint:
                        msg += f" (did you mean '--{hint}'?)"
                    findings.append(msg)


def _hint(word: str, candidates: list[str]) -> str | None:
    matches = difflib.get_close_matches(word, candidates, n=1, cutoff=0.7)
    return matches[0] if matches else None


def _parse_slug(argv: list[str]) -> str | None:
    """Return the value following --slug, or None if absent."""
    for i, a in enumerate(argv):
        if a == "--slug" and i + 1 < len(argv):
            return argv[i + 1]
    return None


def main(argv: list[str]) -> int:
    warn_mode = "--warn" in argv
    only_slug = _parse_slug(argv)
    findings: list[str] = []
    build_errors = False

    all_skills = registry.skills()
    if only_slug is not None:
        if only_slug not in all_skills:
            print(f"check_cli_claims: unknown slug '{only_slug}'", file=sys.stderr)
            return 1
        targets = {only_slug: all_skills[only_slug]}
    else:
        targets = all_skills

    with tempfile.TemporaryDirectory() as td:
        tmp = Path(td)
        for slug, entry in targets.items():
            # markdown-only skills have no binary to build or introspect.
            if registry.is_markdown_only(slug):
                continue
            binary = build_binary(slug, entry["cli_binary"], tmp)
            if binary is None:
                findings.append(f"{slug}: CLI build failed (cannot verify claims)")
                build_errors = True
                continue
            surface = enumerate_surface(binary)
            check_slug(slug, entry, surface, findings)

    if findings:
        prefix = "WARN" if warn_mode else "FAIL"
        for f in findings:
            print(f"{prefix}: {f}")
        if warn_mode:
            print(f"\n{len(findings)} finding(s) (warn mode: exit 0).")
            return 0
        print(f"\ncheck_cli_claims FAILED: {len(findings)} finding(s).")
        return 1

    print("PASS: all CLI claims resolve against the built binary surface")
    return 0 if not build_errors or warn_mode else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
