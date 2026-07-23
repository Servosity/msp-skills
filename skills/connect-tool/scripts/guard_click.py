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
    exe = ct.which_opencli()
    if not exe:
        print("FAIL: opencli not installed", file=sys.stderr)
        return 3
    label = (ct.run([exe, "browser", session, "find", "--selector", target], timeout=90).stdout or "")
    hit = next((w for w in deny_words() if re.search(rf"\b{re.escape(w)}", label, re.I)), None)
    allow = os.environ.get("ALLOW", "").strip().lower()
    if hit and allow != hit:
        shot = hold_dir() / f"HOLD-{dt.datetime.now(dt.UTC).strftime('%Y%m%d-%H%M%S')}.png"
        ct.run([exe, "browser", session, "screenshot", str(shot)], timeout=90)
        print(f"HOLD: '{target}' matches irreversible verb '{hit}'. {shot} saved. "
              "NOT clicking; surfaced for the user.", file=sys.stderr)
        return 10
    r = ct.run([exe, "browser", session, "click", target], timeout=90)
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
                if cmd == "find":
                    print('{"matches_n":1,"entries":[{"label":"Save changes"}]}')
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
        del os.environ["HOLD_EXTRA"], os.environ["RUN_DIR"]
    print("guard_click.py selfcheck OK (hold, ALLOW override, HOLD_EXTRA)")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
        raise SystemExit(0)
    if len(sys.argv) < 3:
        print("usage: guard_click.py <session> <target-selector-or-text>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(guard(sys.argv[1], sys.argv[2]))
