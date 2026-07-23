# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Lane A: drive a CLI-owned OAuth loopback login while keeping the token out of
the agent's context.

The consuming CLI's own `auth ... login` runs a 127.0.0.1 callback, catches the
code, and exchanges + stores the token ITSELF. The token never renders in the DOM
and never reaches this process's stdout.

Three processes, on purpose:

  --start   spawns a detached BROKER, waits only until a safe consent URL is
            known, navigates the bound tab to it, and RETURNS. You then drive
            the consent click while the broker keeps running.
  broker    holds the login child, consumes its output continuously, keeps NO
            raw output anywhere, and records only a sanitized verdict.
  --finish  reads that verdict.

The authorization URL is navigated, never printed: it carries an unguessable
`state` plus tenant and client identifiers that have no business in model
context.

  RUN_DIR=<dir> uv run oauth_login.py --start --session <slug> -- <cli login cmd...>
  RUN_DIR=<dir> uv run oauth_login.py --finish
  uv run oauth_login.py --selfcheck
"""
from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys
import time
import urllib.parse

import ctplatform as ct

AUTH_URL = re.compile(r"https://[^\s\"'<>]*(?:authorize|oauth|consent|/o/)[^\s\"'<>]*", re.I)
# Any parameter whose NAME looks credential-bearing, in the query or the fragment.
SECRET_PARAM = re.compile(r"^(?:access_?token|id_?token|refresh_?token|client_?secret|code|"
                          r"token|api_?key|assertion|session|password|credential)$", re.I)
# Negations first: "not authenticated" must never read as success.
FAILED = re.compile(r"(?:not|failed|failure|error|denied|unable to)\s*(?:to\s+)?(?:log ?in|authenticat)",
                    re.I)
SUCCEEDED = re.compile(r"login (?:ok|success(?:ful)?|stored)|token stored|^\s*authenticated|^\s*success",
                       re.I | re.M)

STATUS = ".oauth_status.json"


def url_is_safe(url: str) -> bool:
    """A consent URL carries no credential. One that does is a leak, not a URL.

    Parameter names are compared after percent-decoding and case-folding, and the
    fragment is checked as well as the query: an implicit-flow response puts the
    token after the `#`, where a query-only check never looks.
    """
    try:
        u = urllib.parse.urlsplit(url)
    except ValueError:
        return False
    for blob in (u.query, u.fragment):
        if not blob:
            continue
        for name, _ in urllib.parse.parse_qsl(blob, keep_blank_values=True):
            if SECRET_PARAM.match(name.strip()):
                return False
    return True


def _status_path(run_dir: str) -> pathlib.Path:
    return pathlib.Path(run_dir) / STATUS


def _write_status(run_dir: str, **fields) -> None:
    p = _status_path(run_dir)
    cur = {}
    if p.exists():
        try:
            cur = json.loads(p.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            cur = {}
    cur.update(fields)
    tmp = p.with_suffix(".tmp")
    tmp.write_text(json.dumps(cur), encoding="utf-8")
    tmp.replace(p)


def _read_status(run_dir: str) -> dict:
    p = _status_path(run_dir)
    if not p.exists():
        return {}
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}


# -- the broker ------------------------------------------------------------

def broker(run_dir: str, argv: list[str]) -> int:
    """Hold the login child and consume its output, keeping only a verdict.

    Raw lines are dropped as they are read: a token the CLI prints never lands in
    a file, never survives the loop, and never reaches any stdout the model sees.
    """
    ct.secure_mkdir(pathlib.Path(run_dir))
    ok = failed = False
    try:
        proc = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, shell=False, text=True,
                                bufsize=1, errors="replace")
    except OSError:
        _write_status(run_dir, done=True, ok=False, reason="login command could not be started")
        return 4
    assert proc.stdout is not None
    for line in proc.stdout:
        if not _read_status(run_dir).get("url_seen"):
            for cand in AUTH_URL.findall(line):
                if url_is_safe(cand):
                    _write_status(run_dir, url=cand, url_seen=True)
                    break
        if FAILED.search(line):
            failed = True
        elif SUCCEEDED.search(line):
            ok = True
    code = proc.wait()
    _write_status(run_dir, done=True, ok=bool(ok and not failed and code == 0),
                  reason="" if ok and not failed and code == 0 else "the CLI did not confirm a login")
    return 0


# -- start / finish --------------------------------------------------------

def start(run_dir: str, argv: list[str], session: str, wait: float = 30.0) -> int:
    if not argv:
        print("FAIL: no login command given", file=sys.stderr)
        return 2
    ct.secure_mkdir(pathlib.Path(run_dir))
    _status_path(run_dir).unlink(missing_ok=True)

    # The broker outlives this invocation, so --finish can be a separate call and
    # you can drive the consent click in between.
    ct.detached([sys.executable, str(pathlib.Path(__file__).resolve()),
                 "--broker", run_dir, "--", *argv], stdout=subprocess.DEVNULL)

    deadline = time.monotonic() + wait
    st: dict = {}
    while time.monotonic() < deadline:
        st = _read_status(run_dir)
        if st.get("url") or st.get("done"):
            break
        time.sleep(0.25)

    url = st.get("url")
    if not url:
        print("FAIL: no safe authorization URL surfaced", file=sys.stderr)
        return 3

    oc = ct.opencli_cmd()
    if not oc:
        print("FAIL: opencli not installed; cannot navigate to consent", file=sys.stderr)
        return 3
    try:
        nav = ct.run([*oc, "browser", session, "open", url], timeout=90)
    except Exception:
        # A raised subprocess error carries argv, and argv carries the URL.
        print("FAIL: could not navigate to the consent page", file=sys.stderr)
        return 3
    finally:
        # The URL has served its purpose; do not leave it sitting in the run dir.
        _write_status(run_dir, url="")
    if nav.returncode != 0:
        print("FAIL: could not navigate to the consent page", file=sys.stderr)
        return 3
    print(f"AUTH_NAVIGATED session={session}", flush=True)
    return 0


def finish(run_dir: str, wait: float = 300.0) -> int:
    if not _status_path(run_dir).exists():
        print("FAIL: no login in progress for this run dir", file=sys.stderr)
        return 4
    deadline = time.monotonic() + wait
    st: dict = {}
    while time.monotonic() < deadline:
        st = _read_status(run_dir)
        if st.get("done"):
            break
        time.sleep(0.5)
    _status_path(run_dir).unlink(missing_ok=True)
    if not st.get("done"):
        print("FAIL: login timed out", file=sys.stderr)
        return 4
    if st.get("ok"):
        print("OAUTH_OK", flush=True)
        return 0
    reason = st.get("reason") or "the CLI did not confirm a login"
    print(f"FAIL: {reason} (its output was consumed, not stored)", file=sys.stderr)
    return 6


def _selfcheck() -> None:
    import contextlib
    import io
    import tempfile
    import textwrap

    # URL safety, including the fragment and encoded parameter names.
    assert url_is_safe("https://accounts.google.com/o/oauth2/auth?client_id=x&scope=openid&state=abc")
    for bad in ("https://e.example/oauth/authorize?access_token=LEAK",
                "https://e.example/oauth/authorize#access_token=LEAK",
                "https://e.example/oauth/authorize?ACCESS_TOKEN=LEAK",
                "https://e.example/oauth/authorize?access%5Ftoken=LEAK",
                "https://e.example/oauth/authorize?code=LEAK",
                "https://e.example/oauth/authorize?api_key=LEAK"):
        assert not url_is_safe(bad), f"accepted a credential-bearing URL: {bad}"

    with tempfile.TemporaryDirectory() as td:
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

        # 1. start RETURNS while the broker keeps running, so consent can be driven.
        run1 = os.path.join(td, "r1")
        buf = io.StringIO()
        t0 = time.monotonic()
        with contextlib.redirect_stdout(buf):
            rc = start(run1, login(
                'import time,sys;'
                'print("open https://accounts.google.com/o/oauth2/auth?client_id=x", flush=True);'
                'time.sleep(1.5);'
                'print("SECRET bearer abc123", flush=True); print("token stored", flush=True)'),
                "sess")
        elapsed = time.monotonic() - t0
        out = buf.getvalue()
        assert rc == 0 and out.startswith("AUTH_NAVIGATED"), (rc, out)
        assert elapsed < 1.4, f"--start blocked for {elapsed:.1f}s; consent could not be driven"
        assert "abc123" not in out and "accounts.google.com" not in out, out
        assert "accounts.google.com" in open(opened).read(), "consent URL was not navigated to"

        # 2. finish is a SEPARATE call and reads the broker's verdict.
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            rc = finish(run1, wait=30)
        out = buf.getvalue()
        assert rc == 0 and "OAUTH_OK" in out, (rc, out)
        assert "abc123" not in out, "token leaked from --finish"
        assert not _status_path(run1).exists(), "status file survived --finish"

        # 3. A token-bearing authorize URL is never navigated to.
        os.truncate(opened, 0)
        run2 = os.path.join(td, "r2")
        with contextlib.redirect_stdout(io.StringIO()):
            start(run2, login(
                'import time;'
                'print("https://evil.example/oauth/authorize?access_token=LEAK999", flush=True);'
                'print("https://accounts.google.com/o/oauth2/auth?client_id=ok", flush=True);'
                'time.sleep(0.3); print("token stored", flush=True)'), "sess")
            finish(run2, wait=30)
        nav = open(opened).read()
        assert "LEAK999" not in nav, "tokened authorize URL was navigated to"
        assert "accounts.google.com" in nav, "clean URL not chosen alongside a tokened one"

        # 4. "not authenticated" must not read as success.
        run3 = os.path.join(td, "r3")
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            start(run3, login(
                'import time;'
                'print("https://accounts.google.com/o/oauth2/auth?client_id=x", flush=True);'
                'time.sleep(0.3); print("user is not authenticated", flush=True)'), "sess")
            rc = finish(run3, wait=30)
        assert rc != 0, "'not authenticated' was read as success"

        # 5. Nothing raw, and no URL, is left behind in any run dir.
        for d in (run1, run2, run3):
            for f in pathlib.Path(d).glob("*"):
                body = f.read_text(errors="ignore")
                assert "abc123" not in body, f"raw output persisted in {f}"
                assert "LEAK999" not in body, f"a tokened URL persisted in {f}"
    print("oauth_login.py selfcheck OK (start returns for consent, finish is separate, "
          "URL navigated not printed, fragment+encoded params rejected, nothing raw on disk)")


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
        raise SystemExit(0)
    if args and args[0] == "--broker":
        rest = args[2:]
        if rest and rest[0] == "--":
            rest = rest[1:]
        raise SystemExit(broker(args[1], rest))

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
        raise SystemExit(start(rd, rest, sess))
    if args and args[0] == "--finish":
        raise SystemExit(finish(rd))
    print("usage: RUN_DIR=<dir> oauth_login.py --start -- <cli login cmd>", file=sys.stderr)
    raise SystemExit(2)
