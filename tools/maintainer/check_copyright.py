#!/usr/bin/env python3
"""check_copyright.py - one project copyright line, everywhere.

Why this gate exists: before 2026-08-23 the fleet carried TEN different
copyright strings across 19,307 files - `Damien Stevens and contributors`,
`Abhi Saini and contributors`, and bare usernames like `dstevens`,
`damienstevens`, `servosity`, `phoenix-server`. Nine connectors disagreed with
THEMSELVES (servosity carried three). Whatever a header says, it goes stale the
moment someone else edits the file, and it tells an adopter the wrong thing
about who owns the subtree.

The contract, stated exactly:

    Within this gate's scope, ANY copyright line that exists must be the
    canonical one. A file with no copyright line at all is not a finding here.

Scope (deliberately narrow, and stated the same way in CONTRIBUTING.md so the
docs never over-promise what is actually enforced):

  - every `.go` file under `skills/*/cli/` - the generated connector source
  - every `skills/*/cli/NOTICE`
  - every `skills/*/cli/LICENSE`

Nothing else is scanned. Markdown, Python, shell and the root LICENSE/NOTICE
are outside this gate.

Canonical forms:

  Go            `// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.`
                matched byte-for-byte, at column zero, line comment only.
  NOTICE/LICENSE  `Copyright 2026 Servosity Inc. and msp-skills contributors`
                matched after stripping surrounding whitespace, because the
                Apache-2.0 appendix indents its attribution line.

What "any copyright line" means here. Detection is deliberately WIDER than the
canonical form, so a divergent line cannot hide by being spelled differently.
A line is a candidate when, after optional indentation (spaces or tabs) and one
optional comment opener (`//`, `/*`, `*`, `#`, `--`, `;`, `<!--`), it opens with
a copyright notice - the word `Copyright` in any case, or a bare `(C)`, `(c)`
or the U+00A9 sign - followed by a four-digit year. Any year, not just 2026.
Every candidate is then held to the canonical form above.

That width is the point. The first version of this gate matched only
`^// Copyright 2026 ` and therefore FAILED OPEN on: a different year, any
indentation, a tab, a block comment, and every `Copyright (C)` variant. The
NOTICE/LICENSE half only looked at lines beginning `Copyright 2026 `, so
`Copyright 2019 Someone Else` was invisible there too.

Read and decode errors are FAILURES, never skips. A file this gate cannot read
is a file it cannot vouch for, and silently continuing is how a gate reports
green over a hole.

The escape hatch, so a future maintainer does not have to guess: if a connector
ever legitimately vendors third-party source carrying its own copyright, this
gate will flag it. Today none does - all Go files in scope carry one identical
header - so the right response is an explicit, reviewed exclusion added here,
NOT widening the canonical form or the detection pattern.

Speed: the fleet is ~19,600 Go files. Files whose only copyright lines are
byte-exact canonical headers are settled with two C-level string operations and
never reach the regex, so a clean tree costs about a second.

The press emits the header from whoever ran it, so a reprint reintroduces an
individual's name; this gate is what catches that. Upstream fix tracked as
cli-printing-press#4326.

Usage:
    check_copyright.py            # scan the fleet, exit 1 on any finding
"""
import pathlib
import re
import sys

CANON = "Copyright 2026 Servosity Inc. and msp-skills contributors"
GO_HEADER = f"// {CANON}. Licensed under Apache-2.0. See LICENSE."
ROOT = pathlib.Path(__file__).resolve().parents[2]

# DETECTION, not validation. Fires on any line that OPENS with a copyright
# notice; the caller decides whether that line is the canonical one. Kept wide
# on purpose: year, indentation, comment style and the (C)/(c)/U+00A9 variants
# are all bypasses the narrow first version of this gate let through.
COPYRIGHT_LINE = re.compile(
    r"""
    ^[ \t]*                                                  # any indentation
    (?: (?: //+ | /\*+ | \*+ | \#+ | --+ | ;+ | <!--+ ) [ \t]* )?  # one comment opener
    (?:
          copyright [ \t]* (?: \( [Cc] \) | © )?             # the word, then an optional sign
        | \( [Cc] \)                                         # or a bare (C) / (c)
        | ©                                                  # or a bare U+00A9 sign
    )
    [ \t]* \d{4}                                             # a year, ANY year
    [^\n]* $                                                 # the rest of the line
    """,
    re.IGNORECASE | re.VERBOSE | re.MULTILINE,
)

# Anchored so it only removes lines that are ALREADY byte-exact canonical
# headers at column zero. An indented or block-commented copy contains
# GO_HEADER as a substring but is NOT a whole line, so it survives this strip
# and still reaches the regex. That distinction is what keeps the fast path
# from becoming a bypass of its own.
_CANON_GO_LINE = "\n" + GO_HEADER + "\n"


def _read(path: pathlib.Path, failures: list[str]) -> str | None:
    """Read a file as UTF-8. Any failure is recorded as a FINDING and returns
    None; an unreadable file is never silently skipped."""
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError as e:
        failures.append(f"{path.relative_to(ROOT)}: UNREADABLE (not valid UTF-8: {e.reason})")
    except OSError as e:
        failures.append(f"{path.relative_to(ROOT)}: UNREADABLE ({e.strerror or e})")
    return None


def _scan_go(path: pathlib.Path, text: str, failures: list[str]) -> None:
    # Fast path: drop whole lines that are already byte-exact canonical
    # headers, then ask whether any copyright token is left at all. Both
    # operations are C-level, so the common case never touches the regex.
    probe = ("\n" + text + "\n").replace(_CANON_GO_LINE, "\n")
    if "copyright" not in probe.lower() and "©" not in probe:
        return
    rel = path.relative_to(ROOT)
    for m in COPYRIGHT_LINE.finditer(text):
        line = m.group(0).rstrip("\r")
        if line != GO_HEADER:
            failures.append(f"{rel}: {line.strip()}")
            return  # one finding per file is enough to act on


def _scan_notice(path: pathlib.Path, text: str, failures: list[str]) -> None:
    # The Apache-2.0 body says "Grant of Copyright License", "the copyright
    # owner" and so on; none of those open a line with a copyright-plus-year
    # notice, so only the real attribution line is a candidate. Indentation is
    # allowed here because the Apache appendix indents it by three spaces.
    rel = path.relative_to(ROOT)
    for m in COPYRIGHT_LINE.finditer(text):
        line = m.group(0).rstrip("\r").strip()
        if line != CANON:
            failures.append(f"{rel}: {line}")


def main() -> int:
    failures: list[str] = []
    go_files = 0

    for p in sorted(ROOT.glob("skills/*/cli/**/*.go")):
        go_files += 1
        text = _read(p, failures)
        if text is not None:
            _scan_go(p, text, failures)

    attribution_files = 0
    for name in ("NOTICE", "LICENSE"):
        for p in sorted(ROOT.glob(f"skills/*/cli/{name}")):
            attribution_files += 1
            text = _read(p, failures)
            if text is not None:
                _scan_notice(p, text, failures)

    if failures:
        print(f"FAIL: {len(failures)} finding(s) - a copyright line is not the project's.")
        print(f"  Expected in Go:             {GO_HEADER}")
        print(f"  Expected in NOTICE/LICENSE: {CANON}")
        for f in failures[:25]:
            print(f"  {f}")
        if len(failures) > 25:
            print(f"  ... and {len(failures) - 25} more")
        print("\n  Contributor credit belongs in cli/NOTICE ('contributed by ...')")
        print("  and SKILL.md `author`, not in per-file headers. See CONTRIBUTING.md.")
        print("  An UNREADABLE finding is a gate failure, not a skip: fix the file's")
        print("  permissions or encoding so the line can actually be checked.")
        return 1

    print(
        f"PASS: copyright line is uniform across {go_files} Go file(s) "
        f"and {attribution_files} NOTICE/LICENSE file(s)."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
