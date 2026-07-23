# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Append ONE structured event to the run's events.jsonl (the audit trail).

Structured for three jobs: troubleshooting (phase/operation/status/error_class),
review (an ordered stream of what did and did not happen, holds and their
resolutions), and improvement (patterns.py mines error_class across runs).

REFUSES to write any secret-shaped field, by field name AND by value, so a secret
value cannot land in the log by construction. Appends under a lock, because
Windows gives no atomic-append guarantee for concurrent writers.

  RUN_DIR=<dir> uv run audit_log.py event=bind status=ok target=halopsa scheme=api-key
  uv run audit_log.py --selfcheck
"""
from __future__ import annotations

import datetime as dt
import json
import os
import pathlib
import re
import sys

import ctplatform as ct

FORBIDDEN = {"value", "secret", "token", "access_token", "refresh_token",
             "api_key", "client_secret", "bearer", "password", "credential", "cookie"}
# Catch a secret embedded inside an otherwise innocent field.
EMBEDDED = re.compile(r'(access_token|refresh_token|client_secret|api_?key|password|passwd)["\s]*[:=]',
                      re.I)


def _lock(path: pathlib.Path):
    """Serialize appends. flock on POSIX, msvcrt on Windows."""
    fh = path.open("a", encoding="utf-8")
    try:
        if ct.WINDOWS:
            import msvcrt  # type: ignore[import-not-found]
            msvcrt.locking(fh.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(fh.fileno(), fcntl.LOCK_EX)
    except OSError:
        pass          # a lock we cannot take is not a reason to lose the event
    return fh


def emit(run_dir: str, pairs: list[str]) -> int:
    rec: dict[str, str] = {"ts": dt.datetime.now(dt.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")}
    for kv in pairs:
        if "=" not in kv:
            print(f"REFUSING malformed field '{kv}' (expected key=value)", file=sys.stderr)
            return 9
        k, v = kv.split("=", 1)
        if k.lower() in FORBIDDEN:
            print(f"REFUSING to log forbidden field '{k}'", file=sys.stderr)
            return 9
        if EMBEDDED.search(v):
            print(f"REFUSING to log value with an embedded secret in field '{k}'", file=sys.stderr)
            return 9
        rec[k] = v.replace("\n", " ")
    d = ct.secure_mkdir(pathlib.Path(run_dir))
    line = json.dumps(rec, sort_keys=True)     # json.dumps escapes keys AND values
    with _lock(d / "events.jsonl") as fh:
        fh.write(line + "\n")
    print(f"logged: {line[:90]}")
    return 0


def _selfcheck() -> None:
    import contextlib
    import io
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        with contextlib.redirect_stdout(io.StringIO()):
            assert emit(td, ["event=bind", "status=ok", "target=demo", "scheme=api-key"]) == 0
        ev = pathlib.Path(td) / "events.jsonl"
        rec = json.loads(ev.read_text().strip())
        assert rec["event"] == "bind" and rec["status"] == "ok", rec
        assert len(ev.read_text().strip().splitlines()) == 1

        with contextlib.redirect_stderr(io.StringIO()):
            assert emit(td, ["access_token=leak"]) == 9, "forbidden field accepted"
            assert emit(td, ["ACCESS_TOKEN=leak2"]) == 9, "uppercase forbidden field accepted"
            assert emit(td, ['detail={"access_token":"leak3"}']) == 9, "embedded secret accepted"
            assert emit(td, ["noequals"]) == 9, "malformed field accepted"
        with contextlib.redirect_stdout(io.StringIO()):
            assert emit(td, ["detail=token refresh scheduled"]) == 0, "benign detail rejected"
            # a key containing a quote must not be able to break the JSON line
            assert emit(td, ['we"ird=va"lue']) == 0
        body = ev.read_text()
        assert "leak" not in body, "a secret reached the log"
        for line in body.strip().splitlines():
            json.loads(line)       # every line is still valid JSON
    print("audit_log.py selfcheck OK (refuses secret-shaped fields and values, valid JSON)")


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
        raise SystemExit(0)
    rd = os.environ.get("RUN_DIR")
    if not rd:
        print("usage: RUN_DIR=<dir> audit_log.py key=value [key=value ...]", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(emit(rd, args))
