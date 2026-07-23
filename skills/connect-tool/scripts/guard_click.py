# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""HOLD gate: refuse to click anything whose label reads as irreversible.

Encodes "agree the scope up front; hold the irreversible". A click whose label
matches post / publish / save / pay / delete / revoke / authorize / ... is NOT
performed. Instead a screenshot is taken into the private run dir and the action
is surfaced to the user. Only an explicit ALLOW=<verb> for that exact step lets
it through, which is how a consent click gets driven.

Extra deny verbs can be added per run via HOLD_EXTRA (comma-separated), which is
where a recipe's `holds:` list goes.

  uv run guard_click.py <session> "<selector-or-text>"
  ALLOW=authorize uv run guard_click.py <session> "button:has-text('Authorize')"
  uv run guard_click.py --selfcheck
"""
from __future__ import annotations

import datetime as dt
import os
import pathlib
import re
import sys

import ctplatform as ct

DENY = ["post", "publish", "tweet", "send", "save", "pay", "charge", "subscribe",
        "delete", "remove", "revoke", "deploy", "merge", "confirm", "transfer",
        "authorize", "rotate", "regenerate", "disable", "cancel", "terminate"]


def deny_words() -> list[str]:
    extra = [w.strip().lower() for w in os.environ.get("HOLD_EXTRA", "").split(",") if w.strip()]
    return DENY + extra


def hold_dir() -> pathlib.Path:
    rd = os.environ.get("RUN_DIR")
    # Never /tmp: a HOLD screenshot can show a page mid-flow.
    return ct.secure_mkdir(pathlib.Path(rd) if rd else ct.runs_dir() / "_holds")


def guard(session: str, target: str) -> int:
    oc = ct.opencli_cmd()
    if not oc:
        print("FAIL: opencli not installed", file=sys.stderr)
        return 3
    found = ct.run([*oc, "browser", session, "find", "--selector", target], timeout=90)
    label = found.stdout or ""
    # FAIL CLOSED. A gate that cannot see what it is about to click must refuse,
    # not shrug and click: an unreadable or ambiguous target is exactly the case
    # where an irreversible label would go unnoticed.
    if found.returncode != 0 or not label.strip():
        print(f"HOLD: could not inspect '{target}' (find failed). NOT clicking.", file=sys.stderr)
        return 11
    m = re.search(r'"matches_n"\s*:\s*(\d+)', label)
    if m and int(m.group(1)) != 1:
        print(f"HOLD: '{target}' matched {m.group(1)} elements (need exactly 1). NOT clicking.",
              file=sys.stderr)
        return 11
    if not m:
        print(f"HOLD: could not read a match count for '{target}'. NOT clicking.", file=sys.stderr)
        return 11
    hit = next((w for w in deny_words() if re.search(rf"\b{re.escape(w)}", label, re.I)), None)
    allow = os.environ.get("ALLOW", "").strip().lower()
    if hit and allow != hit:
        shot = hold_dir() / f"HOLD-{dt.datetime.now(dt.UTC).strftime('%Y%m%d-%H%M%S')}.png"
        ct.run([*oc, "browser", session, "screenshot", str(shot)], timeout=90)
        print(f"HOLD: '{target}' matches irreversible verb '{hit}'. {shot} saved. "
              "NOT clicking; surfaced for the user.", file=sys.stderr)
        return 10
    r = ct.run([*oc, "browser", session, "click", target], timeout=90)
    sys.stdout.write(r.stdout or "")
    return r.returncode


def _selfcheck() -> None:
    import contextlib
    import io
    import tempfile
    import textwrap
    with tempfile.TemporaryDirectory() as td:
        stub = os.path.join(td, "opencli.py")
        with open(stub, "w") as fh:
            fh.write(textwrap.dedent("""
                import sys
                cmd = sys.argv[3]
                import os
                if cmd == "find":
                    n = os.environ.get("MATCH_N", "1")
                    if n == "error":
                        sys.exit(1)
                    print('{"matches_n":' + n + ',"entries":[{"label":"Save changes"}]}')
                elif cmd == "click":
                    print("CLICKED " + sys.argv[4])
            """))
        launcher = os.path.join(td, "opencli")
        with open(launcher, "w") as fh:
            fh.write(f'#!/usr/bin/env bash\nexec "{sys.executable}" "{stub}" "$@"\n')
        os.chmod(launcher, 0o755)
        os.environ["PATH"] = td + os.pathsep + os.environ["PATH"]
        os.environ["RUN_DIR"] = os.path.join(td, "run")

        with contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") == 10, "irreversible verb was not held"
        os.environ["ALLOW"] = "save"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            assert guard("sess", "Save changes") == 0
        assert "CLICKED" in buf.getvalue(), "whitelisted verb did not click"
        del os.environ["ALLOW"]
        # a recipe-supplied hold applies too
        os.environ["HOLD_EXTRA"] = "changes"
        with contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") == 10
        del os.environ["HOLD_EXTRA"]
        # FAIL CLOSED: an unreadable target or an ambiguous match must not click.
        os.environ["ALLOW"] = "save"
        for n in ("0", "3", "error"):
            os.environ["MATCH_N"] = n
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
                rc = guard("sess", "Save changes")
            assert rc == 11, f"MATCH_N={n} did not fail closed (got {rc})"
            assert "CLICKED" not in buf.getvalue(), f"MATCH_N={n} clicked anyway"
        del os.environ["MATCH_N"], os.environ["ALLOW"], os.environ["RUN_DIR"]
    print("guard_click.py selfcheck OK (hold, ALLOW override, HOLD_EXTRA, fails closed)")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
        raise SystemExit(0)
    if len(sys.argv) < 3:
        print("usage: guard_click.py <session> <target-selector-or-text>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(guard(sys.argv[1], sys.argv[2]))
