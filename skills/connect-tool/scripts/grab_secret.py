# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Lane B: read ONE secret rendered in the bound tab's DOM straight into the
credential store, without the value ever reaching model-visible stdout.

The model authors the selector and the destination. This process is the only one
that touches the value, and it prints ONLY a redacted receipt (len / sha256[:8] /
last4). The `opencli ... eval` result is captured into a variable here and never
echoed; there is no `shell=True` anywhere, no logging of captured output, and
every error message is a fixed string so an exception can never carry the value.

Reads DOM innerText/value, NEVER the clipboard: OpenCLI's page "Copy" button does
not reliably reach the system pasteboard, so a clipboard read returns stale data
(this once captured an unrelated note and nearly stored it as a credential).

Fails loudly unless the selector matches EXACTLY ONE node.

  uv run grab_secret.py --session S --selector CSS --service SVC --account ACCT \\
                        [--attr value|textContent|auto]
  uv run grab_secret.py --selfcheck
"""
from __future__ import annotations

import argparse
import base64
import json
import re
import sys

import credstore
import ctplatform as ct

ATTRS = ("auto", "value", "textContent")
ENVELOPE = re.compile(r"SECRET_ENVELOPE:(\d+):([A-Za-z0-9+/=]*)")


def _js(selector: str, attr: str) -> str:
    """Build the page script. The selector is base64'd so no quoting of it can
    break out of the JS string, and attr is enum-checked by the caller so it can
    never be free text inside the script."""
    sel_b64 = base64.b64encode(selector.encode()).decode()
    return (
        '(() => {'
        f'  const sel = new TextDecoder().decode(Uint8Array.from(atob("{sel_b64}"), c => c.charCodeAt(0)));'
        '   const ns = document.querySelectorAll(sel);'
        '   let out = "SECRET_ENVELOPE:" + ns.length + ":";'
        '   if (ns.length === 1) {'
        '     const el = ns[0];'
        f'    const raw = ("{attr}" === "textContent") ? el.textContent'
        f'              : ("{attr}" === "value")       ? el.value'
        '               : (el.value !== undefined && el.value !== null && el.value !== "") ? el.value'
        '               : (el.textContent || "");'
        '     out += btoa(String.fromCharCode(...new TextEncoder().encode(raw || "")));'
        '   }'
        '   return out;'
        '})()'
    )


def grab(session: str, selector: str, service: str, account: str, attr: str = "auto") -> int:
    if attr not in ATTRS:
        print(f"FAIL: --attr must be one of {'|'.join(ATTRS)}", file=sys.stderr)
        return 2
    oc = ct.opencli_cmd()
    if not oc:
        print("FAIL: opencli not installed", file=sys.stderr)
        return 3
    try:
        # Captured, NEVER printed. shell=False AND a real argv (never a .cmd
        # shim), so the selector cannot be re-parsed as a command.
        raw = ct.run([*oc, "browser", session, "eval", _js(selector, attr)], timeout=90).stdout or ""
    except Exception:
        print("FAIL: browser eval failed", file=sys.stderr)   # fixed message, no detail
        return 3

    m = ENVELOPE.search(raw)
    del raw
    if not m:
        print("FAIL: no envelope (selector/page problem)", file=sys.stderr)
        return 3
    n = int(m.group(1))
    if n != 1:
        print(f"FAIL: selector matched {n} nodes (need exactly 1)", file=sys.stderr)
        return 3
    try:
        value = base64.b64decode(m.group(2)).decode("utf-8")
    except Exception:
        print("FAIL: could not decode the captured value", file=sys.stderr)
        return 3
    # Do NOT strip: leading/trailing whitespace can be significant in a secret.
    if not value:
        print("FAIL: empty value extracted", file=sys.stderr)
        return 3
    try:
        print(credstore.store(service, account, value))
    except credstore.CredError as e:
        print(f"FAIL: {e}", file=sys.stderr)      # CredError messages are fixed strings
        return 5
    finally:
        del value
    return 0


def _selfcheck() -> None:
    import contextlib
    import io
    import os
    import tempfile
    import textwrap

    secret = "sk_test_ABC123"
    with tempfile.TemporaryDirectory() as td:
        stub = os.path.join(td, "opencli.py")
        with open(stub, "w") as fh:
            fh.write(textwrap.dedent(f"""
                import base64, os
                n = os.environ.get("MATCH_N", "1")
                b = base64.b64encode({secret!r}.encode()).decode() if n == "1" else ""
                print(f"SECRET_ENVELOPE:{{n}}:{{b}}")
            """))
        launcher = os.path.join(td, "opencli")
        with open(launcher, "w") as fh:
            fh.write(f'#!/usr/bin/env bash\nexec "{sys.executable}" "{stub}" "$@"\n')
        os.chmod(launcher, 0o755)
        os.environ["PATH"] = td + os.pathsep + os.environ["PATH"]

        svc, acct = "CONNECT_TOOL_SELFCHECK", "grab"
        try:
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                rc = grab("sess", "input#k", svc, acct, "auto")
            out = buf.getvalue()
            assert rc == 0, out
            assert secret not in out, "SECRET LEAKED to stdout"
            assert "len=14" in out and "last4=C123" in out, out
            assert credstore.fetch(svc, acct) == secret, "value did not round-trip to the store"

            os.environ["MATCH_N"] = "0"
            with contextlib.redirect_stderr(io.StringIO()):
                assert grab("sess", "input#k", svc, acct, "auto") != 0, "0-node selector did not fail"
            os.environ["MATCH_N"] = "3"
            with contextlib.redirect_stderr(io.StringIO()):
                assert grab("sess", "input#k", svc, acct, "auto") != 0, "3-node selector did not fail"
            del os.environ["MATCH_N"]

            with contextlib.redirect_stderr(io.StringIO()):
                assert grab("sess", "input#k", svc, acct, 'value";alert(1);//') == 2, \
                    "invalid --attr was accepted"
        finally:
            credstore.delete(svc, acct)
    # the injected JS must carry the selector as data, never interpolated raw
    js = _js('input[data-x="a\'b"]', "auto")
    assert 'data-x' not in js, "selector was interpolated into JS instead of base64"
    assert json.dumps(js)  # serializable, i.e. no stray control characters
    print("grab_secret.py selfcheck OK (round-trip, no leak, 0/3-node + bad --attr fail)")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
        raise SystemExit(0)
    ap = argparse.ArgumentParser()
    ap.add_argument("--session", required=True)
    ap.add_argument("--selector", required=True)
    ap.add_argument("--service", required=True)
    ap.add_argument("--account", required=True)
    ap.add_argument("--attr", default="auto")
    a = ap.parse_args()
    raise SystemExit(grab(a.session, a.selector, a.service, a.account, a.attr))
