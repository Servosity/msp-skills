#!/usr/bin/env python3
"""Turn the Windows known-failure ledger into a `go test` name pattern for one skill.

The ci.yml `windows` job runs every connector's tests on windows-latest. Some of
those tests fail today for reasons already diagnosed (a real Windows bug in
generated code, or a test that assumes Unix), and the fixes live in other
changes. Listing them in tools/maintainer/windows_known_test_failures.json lets
the job keep GATING on everything else, so a new Windows regression still fails
the row, while the listed tests keep RUNNING in a separate advisory step that
reports them and never fails the job. That is the split between "broken" and
"already known and tracked": gate on the first, report the second.

    python3 tools/maintainer/windows_known_failures.py --slug fortinet
        prints ^(TestA|TestB|...)$ on stdout (no newline), or nothing when the
        ledger records no known failure for that skill.
    python3 tools/maintainer/windows_known_failures.py --self-test
        validates the ledger and proves the pattern both ways.

Pure stdlib, so it runs on the Windows runner's bundled Python.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

LEDGER = Path(__file__).resolve().parent / "windows_known_test_failures.json"
KINDS = {"real-bug", "unix-only-test", "flaky"}
TEST_NAME = re.compile(r"\ATest[A-Za-z0-9_]*\Z")
SLUG = re.compile(r"\A[a-z0-9][a-z0-9-]*\Z")


class LedgerError(Exception):
    """The ledger is malformed. Refuse, never read it as "nothing is known"."""


def load(path: Path = LEDGER) -> list[dict]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        raise LedgerError(f"cannot read {path.name}: {exc}") from exc
    entries = data.get("entries") if isinstance(data, dict) else None
    if not isinstance(entries, list):
        raise LedgerError(f"{path.name} has no 'entries' list")
    seen: set[str] = set()
    for e in entries:
        if not isinstance(e, dict):
            raise LedgerError("an entry is not an object")
        eid = e.get("id")
        if not isinstance(eid, str) or not eid or eid in seen:
            raise LedgerError(f"entry id {eid!r} is missing or repeated")
        seen.add(eid)
        if e.get("kind") not in KINDS:
            raise LedgerError(f"{eid}: kind must be one of {sorted(KINDS)}")
        for field in ("why", "fix"):
            if not isinstance(e.get(field), str) or not e[field].strip():
                raise LedgerError(f"{eid}: '{field}' must say something")
        slugs = e.get("slugs")
        if not isinstance(slugs, list) or not slugs or not all(
                isinstance(s, str) and (s == "*" or SLUG.match(s)) for s in slugs):
            raise LedgerError(f"{eid}: 'slugs' must be a non-empty list of slugs or '*'")
        tests = e.get("tests")
        if not isinstance(tests, list) or not tests or not all(
                isinstance(t, str) and TEST_NAME.match(t) for t in tests):
            raise LedgerError(f"{eid}: 'tests' must be a non-empty list of Go test names")
    return entries


def tests_for(slug: str, entries: list[dict]) -> list[str]:
    names: set[str] = set()
    for e in entries:
        if "*" in e["slugs"] or slug in e["slugs"]:
            names.update(e["tests"])
    return sorted(names)


def pattern_for(slug: str, entries: list[dict]) -> str:
    """An anchored top-level pattern for `go test -run/-skip`, or '' for none.
    Names are validated as identifiers, so they need no regex escaping."""
    names = tests_for(slug, entries)
    return f"^({'|'.join(names)})$" if names else ""


def _self_test() -> int:
    import tempfile

    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    entries = load()
    expect(bool(entries), "the shipped ledger loads and is not empty")

    sample = [
        {"id": "A", "kind": "real-bug", "slugs": ["*"], "tests": ["TestEverywhere"], "why": "w", "fix": "f"},
        {"id": "B", "kind": "unix-only-test", "slugs": ["alpha"], "tests": ["TestOnlyAlpha", "TestEverywhere"],
         "why": "w", "fix": "f"},
    ]
    pat_alpha = pattern_for("alpha", sample)
    pat_beta = pattern_for("beta", sample)
    expect(pat_alpha == "^(TestEverywhere|TestOnlyAlpha)$", "a listed slug gets its own and the '*' tests")
    expect(pat_beta == "^(TestEverywhere)$", "an unlisted slug gets only the '*' tests")
    expect(pattern_for("gamma", sample[1:]) == "", "a slug with nothing recorded gets no pattern (nothing skipped)")
    rx = re.compile(pat_alpha)
    expect(bool(rx.match("TestOnlyAlpha")), "the pattern matches a listed test")
    expect(not rx.match("TestOnlyAlphaPlus") and not rx.match("XTestOnlyAlpha"),
           "the pattern is anchored: a longer or prefixed name is NOT skipped")
    expect(not rx.match("TestSomethingNew"), "an unlisted test is not matched, so it still gates")

    bad_ledgers = {
        "malformed JSON": "{",
        "no entries list": '{"x": 1}',
        "repeated id": json.dumps({"entries": [sample[0], sample[0]]}),
        "unknown kind": json.dumps({"entries": [dict(sample[0], kind="whatever")]}),
        "empty why": json.dumps({"entries": [dict(sample[0], why=" ")]}),
        "a test name that is a regex": json.dumps({"entries": [dict(sample[0], tests=["Test.*"])]}),
        "a bad slug": json.dumps({"entries": [dict(sample[0], slugs=["Bad Slug"])]}),
    }
    with tempfile.TemporaryDirectory() as tmp:
        for label, text in bad_ledgers.items():
            p = Path(tmp) / "ledger.json"
            p.write_text(text, encoding="utf-8")
            try:
                load(p)
                expect(False, f"a ledger with {label} is refused")
            except LedgerError:
                expect(True, f"a ledger with {label} is refused")

    if failures:
        print(f"FAIL: windows_known_failures self-test - {len(failures)} case(s)")
        return 1
    print("PASS: windows_known_failures - the ledger loads, the pattern skips exactly what is listed")
    return 0


def main(argv: list[str]) -> int:
    if argv[1:2] == ["--self-test"]:
        return _self_test()
    if len(argv) == 3 and argv[1] == "--slug" and SLUG.match(argv[2]):
        try:
            sys.stdout.write(pattern_for(argv[2], load()))
        except LedgerError as exc:
            print(f"windows_known_failures: {exc}", file=sys.stderr)
            return 1
        return 0
    print("usage: windows_known_failures.py --slug <slug> | --self-test", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
