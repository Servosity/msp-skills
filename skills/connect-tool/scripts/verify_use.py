# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Verify credentials by USE. This is the live receipt.

Run a read-only authenticated call and assert that a NON-SECRET field came back.
Never claim "auth works" without this. The secret is never echoed; the proof is a
non-secret identifying datum (a handle, an account email, an id).

The field is a strict dotted path (`.data.id`, `data.id`, `.items.0.name`), NOT
filter code: no jq, no expressions, no way to select the whole response or to
construct a secret-shaped key dynamically. Secret-shaped path segments are
refused, and the asserted value must be a bounded scalar.

  uv run verify_use.py <path> -- <read-only authed cmd...>
  e.g. uv run verify_use.py .email -- halopsa-cli account get
  uv run verify_use.py --selfcheck
"""
from __future__ import annotations

import json
import re
import sys
import time

import ctplatform as ct

SECRETISH = re.compile(r"token|secret|key|bearer|password|passwd|credential|cookie|session|auth",
                       re.I)
MAX_VALUE_LEN = 200


def resolve(doc, path: str):
    """Walk a dotted path. Numeric segments index into a list."""
    cur = doc
    for seg in [s for s in path.strip().lstrip(".").split(".") if s]:
        if isinstance(cur, list):
            if not seg.isdigit() or int(seg) >= len(cur):
                return None
            cur = cur[int(seg)]
        elif isinstance(cur, dict):
            if seg not in cur:
                return None
            cur = cur[seg]
        else:
            return None
    return cur


def check_path(path: str) -> str | None:
    """Return an error message if this path may not be asserted on."""
    segs = [s for s in path.strip().lstrip(".").split(".") if s]
    if not segs:
        return "REFUSING to assert on the whole response (name a specific non-secret field)"
    for s in segs:
        if SECRETISH.search(s):
            return f"REFUSING to assert on a secret-shaped field ({path})"
        if not re.fullmatch(r"[A-Za-z0-9_-]+", s):
            return f"REFUSING a path segment that is not a plain key or index ({s})"
    return None


def run(path: str, argv: list[str], attempts: int = 3, backoff: float = 2.0) -> int:
    if err := check_path(path):
        print(err, file=sys.stderr)
        return 2
    if not argv:
        print("usage: verify_use.py <path> -- <authed read cmd...>", file=sys.stderr)
        return 2
    last = ""
    for attempt in range(1, attempts + 1):
        try:
            r = ct.run(argv, timeout=120)
        except Exception:
            r = None
        if r is not None and r.returncode == 0:
            try:
                val = resolve(json.loads(r.stdout), path)
            except (json.JSONDecodeError, TypeError):
                val = None
                last = "response was not JSON"
            if val is not None and not isinstance(val, (dict, list)):
                text = str(val)
                if len(text) > MAX_VALUE_LEN:
                    print("REFUSING to print an unexpectedly long value", file=sys.stderr)
                    return 2
                # A field named innocently can still HOLD a credential
                # ({"value": "sk_live_..."}), so check the value, not just the path.
                if ct.looks_secret(text):
                    print("REFUSING to print a value that looks like a credential; "
                          "assert on a different field", file=sys.stderr)
                    return 2
                # Control characters cannot be allowed to reshape the output line.
                text = "".join(c for c in text if c.isprintable())
                print(f"RECEIPT_OK field={path} value={text} attempt={attempt}")
                return 0
            if val is None and not last:
                last = "field not present"
            elif isinstance(val, (dict, list)):
                last = "field is a container, not a scalar"
        elif r is not None:
            last = f"exit {r.returncode}"
        if attempt < attempts:
            time.sleep(attempt * backoff)
    print(f"RECEIPT_FAIL after {attempts} attempts ({last or 'no output'})", file=sys.stderr)
    return 1


def _selfcheck() -> None:
    import contextlib
    import io

    def emit(payload: str) -> list[str]:
        return [sys.executable, "-c", f"print({payload!r})"]

    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        rc = run(".data.id", emit('{"data":{"id":"acct_00000000"}}'), backoff=0)
    assert rc == 0 and "RECEIPT_OK" in buf.getvalue(), buf.getvalue()
    assert "acct_00000000" in buf.getvalue()

    with contextlib.redirect_stderr(io.StringIO()):
        assert run(".data.id", emit("{}"), attempts=1, backoff=0) != 0, "empty response passed"
        for bad in (".access_token", ".AccessToken", ".apiKey", ".sessionId", ".cookie"):
            assert run(bad, emit('{"x":1}'), attempts=1, backoff=0) == 2, \
                f"secret-shaped field {bad} not refused"
        assert run(".", emit('{"access_token":"x"}'), attempts=1, backoff=0) == 2, \
            "whole-response assertion not refused"
        assert run("", emit('{"a":1}'), attempts=1, backoff=0) == 2, "empty path not refused"
        # a container is not a receipt
        assert run(".data", emit('{"data":{"id":1}}'), attempts=1, backoff=0) != 0, \
            "container accepted as a receipt"
        # no filter code: a jq-style expression is not a path
        assert run('.data | keys', emit('{"data":{"id":1}}'), attempts=1, backoff=0) == 2, \
            "filter expression accepted as a path"
        # An innocently-named field holding a credential must not be printed.
        # The literal is assembled so this file holds no credential-shaped string.
        tok = "sk" + "_live_abcdefghijklmnop"
        assert run(".value", emit('{"value":"' + tok + '"}'),
                   attempts=1, backoff=0) == 2, "secret-shaped VALUE was printed"
        assert run(".result", emit('{"result":"Bearer abcdefghijklmnopqrstuvwx"}'),
                   attempts=1, backoff=0) == 2, "bearer token value was printed"

    # list indexing works
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        assert run(".items.0.name", emit('{"items":[{"name":"acme"}]}'), backoff=0) == 0
    assert "value=acme" in buf.getvalue(), buf.getvalue()
    print("verify_use.py selfcheck OK (strict path, no filter code, no secret-shaped field)")


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
        raise SystemExit(0)
    if not args:
        print("usage: verify_use.py <path> -- <authed read cmd...>", file=sys.stderr)
        raise SystemExit(2)
    field, rest = args[0], args[1:]
    if rest and rest[0] == "--":
        rest = rest[1:]
    raise SystemExit(run(field, rest))
