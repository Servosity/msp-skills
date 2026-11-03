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

If the native click fails without landing (OpenCLI 2.x refuses native input to a
hidden background window), it falls back to a page-script click on the same
selector, still requiring exactly one match. The receipt line `click_path=` says
which path ran. A timeout is never retried, because the click may have landed.

  uv run guard_click.py <session> "<selector-or-text>"
  ALLOW=authorize uv run guard_click.py <session> "button:has-text('Authorize')"
  uv run guard_click.py --selfcheck
"""
from __future__ import annotations

import base64
import datetime as dt
import os
import pathlib
import re
import subprocess
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
    found = ct.run(ct.browser_argv(oc, session, "find", ct.find_css_flag(oc), target), timeout=90)
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
        ct.run(ct.browser_argv(oc, session, "screenshot", str(shot)), timeout=90)
        print(f"HOLD: '{target}' matches irreversible verb '{hit}'. {shot} saved. "
              "NOT clicking; surfaced for the user.", file=sys.stderr)
        return 10
    try:
        r = ct.run(ct.browser_argv(oc, session, "click", target), timeout=90)
    except subprocess.TimeoutExpired:
        # Dispatch state unknown: a retry could click twice. Surface, never retry.
        print(f"FAIL: native click on '{target}' timed out; it may or may not have landed. "
              "NOT retrying.", file=sys.stderr)
        return 12
    out = r.stdout or ""
    if r.returncode == 0 and not re.search(r'"ok"\s*:\s*false', out):
        sys.stdout.write(out)
        print("click_path=native")
        return 0
    # The native click failed without landing (e.g. OpenCLI 2.x "Native input requires
    # a visible browser page; input was not dispatched" when the background window is
    # hidden). Fall back to a page-script click on the SAME selector, which must still
    # resolve to exactly one element. The HOLD checks above have already passed.
    code = re.search(r'"code"\s*:\s*"([A-Za-z0-9_.]+)"', out + (r.stderr or ""))
    why = code.group(1) if code else f"exit {r.returncode}"
    if "not dispatched" not in out + (r.stderr or ""):
        # Any other failure may have landed (e.g. the page navigated mid-click); a second
        # click could run a command or create a credential twice. Surface, never retry.
        sys.stdout.write(out)
        print(f"FAIL: native click on '{target}' failed ({why}) and OpenCLI did not say it "
              "was undelivered. NOT retrying.", file=sys.stderr)
        return r.returncode or 1
    try:
        p = ct.run(ct.browser_argv(oc, session, "eval", _page_click_js(target)), timeout=90)
    except subprocess.TimeoutExpired:
        print(f"FAIL: page-script click on '{target}' timed out. NOT retrying.", file=sys.stderr)
        return 12
    pm = re.search(r"PAGE_CLICK:(\d+):(\d+)", p.stdout or "")
    if not pm or pm.group(1) != "1":
        n = pm.group(2) if pm else "?"
        print(f"FAIL: native click failed ({why}) and the page-script fallback did not click "
              f"('{target}' matched {n}).", file=sys.stderr)
        return r.returncode or 1
    print(f"click_path=page-script (native failed: {why})")
    return 0


def _page_click_js(selector: str) -> str:
    """Page script that clicks the element `selector` resolves to, only when it
    resolves to exactly one. The selector is base64'd so no quoting can break out."""
    sel_b64 = base64.b64encode(selector.encode()).decode()
    return (
        "(() => {"
        f' const sel = new TextDecoder().decode(Uint8Array.from(atob("{sel_b64}"), c => c.charCodeAt(0)));'
        " const ns = document.querySelectorAll(sel);"
        ' if (ns.length !== 1) return "PAGE_CLICK:0:" + ns.length;'
        " ns[0].click();"
        ' return "PAGE_CLICK:1:1";'
        "})()"
    )


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
                    if os.environ.get("NATIVE_FAIL") == "landed":
                        print('{"ok":false,"error":{"code":"COMMAND_EXEC","message":'
                              '"Execution context was destroyed."}}')
                        sys.exit(1)
                    if os.environ.get("NATIVE_FAIL"):
                        print('{"ok":false,"error":{"code":"COMMAND_EXEC","message":'
                              '"Native input requires a visible browser page; input was not dispatched."}}')
                        sys.exit(1)
                    print("CLICKED " + sys.argv[4])
                elif cmd == "eval":
                    n = os.environ.get("PAGE_N", "1")
                    print("PAGE_CLICK:1:1" if n == "1" else "PAGE_CLICK:0:" + n)
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
        del os.environ["MATCH_N"]
        # native click fails without landing -> page-script fallback, receipt names the path
        os.environ["NATIVE_FAIL"] = "1"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") == 0, "fallback did not click"
        assert "click_path=page-script (native failed: COMMAND_EXEC)" in buf.getvalue(), buf.getvalue()
        # fallback still needs exactly one element
        os.environ["PAGE_N"] = "2"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") != 0, "ambiguous fallback reported success"
        assert "click_path" not in buf.getvalue()
        del os.environ["PAGE_N"]
        # a failure that may have landed is never retried by the fallback
        os.environ["NATIVE_FAIL"] = "landed"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") != 0, "possibly-landed failure reported success"
        assert "click_path" not in buf.getvalue(), "possibly-landed failure fell back"
        del os.environ["NATIVE_FAIL"]
        # the native path names itself too
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            assert guard("sess", "Save changes") == 0
        assert "click_path=native" in buf.getvalue()
        # a HOLD never reaches either click path
        del os.environ["ALLOW"]
        os.environ["NATIVE_FAIL"] = "1"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
            assert guard("sess", "Save changes") == 10
        assert "click_path" not in buf.getvalue() and "CLICKED" not in buf.getvalue()
        del os.environ["NATIVE_FAIL"], os.environ["RUN_DIR"]
    print("guard_click.py selfcheck OK (hold, ALLOW override, HOLD_EXTRA, fails closed, "
          "page-script fallback)")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
        raise SystemExit(0)
    if len(sys.argv) < 3:
        print("usage: guard_click.py <session> <target-selector-or-text>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(guard(sys.argv[1], sys.argv[2]))
