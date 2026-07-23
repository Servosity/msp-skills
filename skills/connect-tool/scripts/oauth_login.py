# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Lane A: drive a CLI-owned OAuth loopback login while keeping the token out of
the agent's context.

The consuming CLI's own `auth ... login` runs a 127.0.0.1 callback, catches the
code, and exchanges + stores the token ITSELF. The token never renders in the DOM
and never reaches this process's stdout.

This broker never writes the child's raw output to disk. It consumes the stream
continuously, keeps only a sanitized status, and NAVIGATES the bound browser to
the authorization URL rather than printing it: an authorize URL carries an
unguessable `state` (and tenant/client identifiers) that has no business in model
context. You then drive the consent click with guard_click.py.

  RUN_DIR=<dir> uv run oauth_login.py --start -- <cli login cmd...>   # AUTH_NAVIGATED
  RUN_DIR=<dir> uv run oauth_login.py --finish                        # OAUTH_OK | FAIL
  uv run oauth_login.py --selfcheck
"""
from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys
import threading
import time

import ctplatform as ct

# A consent URL. Anything carrying a credential-bearing parameter is NOT one.
AUTH_URL = re.compile(r"https://[^\s\"']*(?:authorize|oauth|consent|/o/)[^\s\"']*", re.I)
SECRET_PARAM = re.compile(r"[?&](?:access_token|id_token|refresh_token|client_secret|code)=", re.I)
# Negations first: "not authenticated" must never read as success.
FAILED = re.compile(r"(?:not|failed|failure|error|denied|unable to)\s*(?:to\s+)?(?:log ?in|authenticat)", re.I)
SUCCEEDED = re.compile(r"login (?:ok|success(?:ful)?|stored)|token stored|^\s*authenticated|^\s*success",
                       re.I | re.M)


def _statefile(run_dir: str) -> pathlib.Path:
    return pathlib.Path(run_dir) / ".oauth_state.json"


def _consume(proc: subprocess.Popen, found: dict, done: threading.Event) -> None:
    """Read the child's output continuously and KEEP NOTHING but a verdict.

    The raw lines are dropped as they are read, so a token printed by the CLI
    never lands in a file, a variable that outlives the loop, or this process's
    output.
    """
    assert proc.stdout is not None
    for line in proc.stdout:
        if not found.get("url"):
            for cand in AUTH_URL.findall(line):
                if not SECRET_PARAM.search(cand):
                    found["url"] = cand
                    break
        if FAILED.search(line):
            found["failed"] = True
        elif SUCCEEDED.search(line):
            found["ok"] = True
    done.set()


def start(run_dir: str, argv: list[str], session: str, wait: float = 20.0) -> int:
    if not argv:
        print("FAIL: no login command given", file=sys.stderr)
        return 2
    ct.secure_mkdir(pathlib.Path(run_dir))
    proc = ct.detached(argv, stdout=subprocess.PIPE)
    found: dict = {}
    done = threading.Event()
    threading.Thread(target=_consume, args=(proc, found, done), daemon=True).start()

    deadline = time.monotonic() + wait
    while time.monotonic() < deadline and not found.get("url") and not done.is_set():
        time.sleep(0.25)

    if not found.get("url"):
        proc.terminate()
        print("FAIL: no safe authorization URL surfaced", file=sys.stderr)
        return 3

    _statefile(run_dir).write_text(json.dumps({
        "pid": proc.pid,
        # Identity, so a timeout kill cannot hit an unrelated process that reused the pid.
        "argv0": argv[0],
        "started": time.time(),
    }), encoding="utf-8")
    _PROCS[run_dir] = (proc, found, done)

    exe = ct.which_opencli()
    if exe:
        # Navigate the bound tab to the consent page. The URL goes browser-ward,
        # not model-ward.
        ct.run([exe, "browser", session, "open", found["url"]], timeout=90)
        print("AUTH_NAVIGATED session=" + session)
    else:
        print("FAIL: opencli not installed; cannot navigate to consent", file=sys.stderr)
        return 3
    return 0


# In-process handles when --start and --finish run in one interpreter (selfcheck).
_PROCS: dict[str, tuple] = {}


def finish(run_dir: str, wait: float = 300.0) -> int:
    entry = _PROCS.get(run_dir)
    sf = _statefile(run_dir)
    if entry is None and not sf.exists():
        print("FAIL: no login in progress for this run dir", file=sys.stderr)
        return 4
    if entry is None:
        # Separate invocation: we cannot read the (already consumed) stream, so
        # the only honest verdict comes from the consuming CLI's own auth state.
        print("FAIL: --finish must run in the same invocation as --start; "
              "verify with verify_use.py instead", file=sys.stderr)
        sf.unlink(missing_ok=True)
        return 4

    proc, found, done = entry
    deadline = time.monotonic() + wait
    while time.monotonic() < deadline and proc.poll() is None:
        time.sleep(0.5)
    if proc.poll() is None:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        _cleanup(run_dir)
        print("FAIL: login timed out", file=sys.stderr)
        return 4
    done.wait(timeout=10)
    ok = bool(found.get("ok")) and not found.get("failed") and proc.returncode == 0
    _cleanup(run_dir)
    if ok:
        print("OAUTH_OK")
        return 0
    print("FAIL: login did not confirm (the CLI's output was consumed, not stored)", file=sys.stderr)
    return 6


def _cleanup(run_dir: str) -> None:
    _statefile(run_dir).unlink(missing_ok=True)
    _PROCS.pop(run_dir, None)


def _selfcheck() -> None:
    import contextlib
    import io
    import tempfile
    import textwrap

    with tempfile.TemporaryDirectory() as td:
        # Fake opencli so `browser <s> open <url>` is a no-op we can observe.
        opened = os.path.join(td, "opened.txt")
        stub = os.path.join(td, "opencli.py")
        with open(stub, "w") as fh:
            fh.write(textwrap.dedent(f"""
                import sys
                if len(sys.argv) > 4 and sys.argv[3] == "open":
                    open({opened!r}, "a").write(sys.argv[4] + "\\n")
            """))
        launcher = os.path.join(td, "opencli")
        with open(launcher, "w") as fh:
            fh.write(f'#!/usr/bin/env bash\nexec "{sys.executable}" "{stub}" "$@"\n')
        os.chmod(launcher, 0o755)
        os.environ["PATH"] = td + os.pathsep + os.environ["PATH"]

        def login(script: str) -> list[str]:
            return [sys.executable, "-c", script]

        # 1. Happy path: consent URL surfaced to the BROWSER, token never printed.
        run1 = os.path.join(td, "r1")
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            rc = start(run1, login(
                'print("open https://accounts.google.com/o/oauth2/auth?client_id=x", flush=True);'
                'import time; time.sleep(0.2);'
                'print("SECRET bearer abc123", flush=True); print("token stored", flush=True)'),
                "sess")
        out = buf.getvalue()
        assert rc == 0 and out.startswith("AUTH_NAVIGATED"), (rc, out)
        assert "abc123" not in out, "token leaked from --start"
        assert "accounts.google.com" not in out, "authorization URL leaked to stdout"
        assert "accounts.google.com" in open(opened).read(), "consent URL was not navigated to"
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            rc = finish(run1, wait=20)
        out = buf.getvalue()
        assert rc == 0 and "OAUTH_OK" in out, (rc, out)
        assert "abc123" not in out, "token leaked from --finish"
        assert not _statefile(run1).exists(), "state file survived --finish"

        # 2. A token-bearing authorize URL must never be chosen.
        os.truncate(opened, 0)
        run2 = os.path.join(td, "r2")
        with contextlib.redirect_stdout(io.StringIO()):
            start(run2, login(
                'print("https://evil.example/oauth/authorize?access_token=LEAK999", flush=True);'
                'print("https://accounts.google.com/o/oauth2/auth?client_id=ok", flush=True);'
                'import time; time.sleep(0.2); print("token stored", flush=True)'), "sess")
            finish(run2, wait=20)
        nav = open(opened).read()
        assert "LEAK999" not in nav, "tokened authorize URL was navigated to"
        assert "accounts.google.com" in nav, "clean URL not chosen alongside a tokened one"

        # 3. "not authenticated" must not read as success.
        run3 = os.path.join(td, "r3")
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            start(run3, login(
                'print("https://accounts.google.com/o/oauth2/auth?client_id=x", flush=True);'
                'import time; time.sleep(0.2); print("user is not authenticated", flush=True)'),
                "sess")
            rc = finish(run3, wait=20)
        assert rc != 0, "'not authenticated' was read as success"

        # 4. No raw CLI output is ever written to the run dir.
        for d in (run1, run2, run3):
            for f in pathlib.Path(d).glob("*"):
                assert "abc123" not in f.read_text(errors="ignore"), f"raw output persisted in {f}"
    print("oauth_login.py selfcheck OK (URL navigated not printed, token never echoed, "
          "negation rejected, no raw log on disk)")


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
        raise SystemExit(0)
    rd = os.environ.get("RUN_DIR")
    if not rd:
        print("usage: RUN_DIR=<dir> oauth_login.py --start [--session S] -- <cli login cmd> | --finish",
              file=sys.stderr)
        raise SystemExit(2)
    sess = "connecttool"
    if "--session" in args:
        i = args.index("--session")
        sess = args[i + 1]
        del args[i:i + 2]
    if args and args[0] == "--start":
        rest = args[1:]
        if rest and rest[0] == "--":
            rest = rest[1:]
        rc = start(rd, rest, sess)
        if rc == 0:
            rc = finish(rd)          # same invocation: the stream stays consumable
        raise SystemExit(rc)
    if args and args[0] == "--finish":
        raise SystemExit(finish(rd))
    print("usage: RUN_DIR=<dir> oauth_login.py --start -- <cli login cmd>", file=sys.stderr)
    raise SystemExit(2)
