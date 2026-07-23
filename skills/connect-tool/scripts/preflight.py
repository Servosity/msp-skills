# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Pin OpenCLI as THE browser driver and bind the user's already-open Chrome tab.

NEVER falls through to a spawned browser (Playwright, Puppeteer, headless
Chromium): those carry no login. If the bridge is not ready this refuses and
prints the setup steps.

  uv run preflight.py --deps         # one-shot setup check: every dependency
  uv run preflight.py [session]      # bind the focused tab (default: connecttool)
  uv run preflight.py --selfcheck
"""
from __future__ import annotations

import os
import re
import shutil
import sys

import ctplatform as ct

EXTENSION_URL = "https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk"

BOOTSTRAP_HINT = f"""\
--- OpenCLI bridge not ready. To set it up (see references/opencli-bootstrap.md): ---
  1. Install CLI:        npm install -g @jackwener/opencli
  2. Install extension:  {EXTENSION_URL}   (one click, Chrome Web Store)
  3. Open Chrome, focus the tab you want, then re-run.
  Verify any time with:  opencli doctor   (want [OK] Daemon + [OK] Extension)
NOTE: this skill drives your REAL Chrome via OpenCLI bind. It will NOT spawn a browser.
"""


def opencli_argv(session: str, *args: str) -> list[str]:
    exe = ct.which_opencli()
    if not exe:
        raise SystemExit(3)
    return [exe, "browser", session, *args]


def doctor_text() -> str:
    exe = ct.which_opencli()
    if not exe:
        return ""
    try:
        r = ct.run([exe, "doctor"], timeout=60)
        return (r.stdout or "") + (r.stderr or "")
    except Exception:
        return ""


# -- dependency check ------------------------------------------------------

def chrome_path() -> str | None:
    """Locate Chrome. Windows keeps it in the registry App Paths key, not a
    predictable directory, and a per-user install is not under Program Files."""
    if override := os.environ.get("CHROME_APP"):
        return override if os.path.exists(override) else None
    if ct.WINDOWS:
        try:
            import winreg  # type: ignore[import-not-found]
            for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
                try:
                    key = winreg.OpenKey(
                        root, r"SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe")
                    with key:
                        p = winreg.QueryValue(key, None)
                        if p and os.path.exists(p):
                            return p
                except OSError:
                    continue
        except ImportError:
            pass
        for base in ("PROGRAMFILES", "PROGRAMFILES(X86)", "LOCALAPPDATA"):
            root = os.environ.get(base)
            if root:
                p = os.path.join(root, "Google", "Chrome", "Application", "chrome.exe")
                if os.path.exists(p):
                    return p
        return None
    p = "/Applications/Google Chrome.app"
    return p if os.path.isdir(p) else None


def node_major() -> int | None:
    exe = shutil.which("node")
    if not exe:
        return None
    try:
        m = re.search(r"v?(\d+)\.", ct.run([exe, "--version"], timeout=30).stdout)
        return int(m.group(1)) if m else None
    except Exception:
        return None


def deps() -> int:
    """Report EVERY missing dependency, not just the first one."""
    rows: list[tuple[bool, str, str]] = []

    def check(label: str, cmd: str | None, fix: str) -> None:
        found = shutil.which(cmd) if cmd else None
        rows.append((bool(found), label, found or fix))

    print("connect-tool dependency check")
    if ct.WINDOWS or ct.MACOS:
        rows.append((True, "os", "Windows" if ct.WINDOWS else "macOS"))
    else:
        rows.append((False, "os", "connect-tool supports macOS and Windows only"))

    c = chrome_path()
    rows.append((bool(c), "chrome", c or "install Google Chrome from https://www.google.com/chrome/"))

    nm = node_major()
    if nm is None:
        rows.append((False, "node", "install Node 20+ from https://nodejs.org (or: winget install OpenJS.NodeJS.LTS)"))
    elif nm < 20:
        rows.append((False, "node", f"found v{nm}, OpenCLI needs Node 20 or newer"))
    else:
        rows.append((True, "node", f"v{nm}"))

    check("npm", "npm", "ships with Node; reinstall Node if missing")
    oc = ct.which_opencli()
    rows.append((bool(oc), "opencli", oc or "npm install -g @jackwener/opencli"))

    # uv OR a new-enough python3. Only one is needed; uv can supply Python itself.
    if shutil.which("uv"):
        rows.append((True, "python", f"uv: {shutil.which('uv')}"))
    elif sys.version_info >= (3, 12):
        rows.append((True, "python", f"{sys.version.split()[0]} (no uv needed)"))
    else:
        rows.append((False, "python", "install uv (https://astral.sh/uv) or Python 3.12+"))

    if ct.MACOS:
        check("security", "security", "macOS built-in; missing means this is not macOS")

    doc = doctor_text()
    if oc:
        d_ok = "[OK] Daemon" in doc
        e_ok = "[OK] Extension" in doc
        rows.append((d_ok, "daemon", "running" if d_ok else "opencli daemon restart"))
        rows.append((e_ok, "extension", "connected" if e_ok else
                     f"install {EXTENSION_URL} then reload its card in chrome://extensions/"))

    missing = 0
    for ok, label, detail in rows:
        print(f"  {'OK  ' if ok else 'MISS'} {label:<9} {'(' + detail + ')' if ok else '-> ' + detail}")
        missing += 0 if ok else 1

    if missing == 0:
        print("All dependencies present. Focus the Chrome tab you want, then run: preflight.py <target-slug>")
    else:
        print(f"Install the {missing} MISS item(s) above, then re-run: preflight.py --deps", file=sys.stderr)
    return 1 if missing else 0


# -- bind ------------------------------------------------------------------

def preflight(session: str) -> int:
    if not ct.which_opencli():
        print("FAIL: opencli not installed.", file=sys.stderr)
        print(BOOTSTRAP_HINT, file=sys.stderr)
        return 3
    doc = doctor_text()
    if "[OK] Daemon" not in doc:
        print("FAIL: OpenCLI daemon not running.", file=sys.stderr)
        print(BOOTSTRAP_HINT, file=sys.stderr)
        return 4
    if "[OK] Extension" not in doc:
        print("FAIL: OpenCLI Chrome extension not connected.", file=sys.stderr)
        print(BOOTSTRAP_HINT, file=sys.stderr)
        return 4
    if ct.run(opencli_argv(session, "bind"), timeout=60).returncode != 0:
        print("FAIL: bind failed - focus the Chrome tab you want, then retry.", file=sys.stderr)
        return 5
    state = ct.run(opencli_argv(session, "state"), timeout=60).stdout or ""
    url = next(iter(re.findall(r"https?://[^\s\"']+", state)), "")
    if not url or "about:blank" in state.lower():
        print(f"FAIL: bound tab has no real URL (got: {url or 'none'}). Focus a real page.",
              file=sys.stderr)
        return 6
    # Print scheme+host+path only. An OAuth callback URL carries ?code= in its
    # query, and that belongs to the consuming CLI, never to this output.
    print(f"OK bound session={session} url={re.split(r'[?#]', url)[0]}")
    return 0


def _selfcheck() -> None:
    import tempfile
    import textwrap
    with tempfile.TemporaryDirectory() as td:
        # Fake opencli: a healthy bridge on a real page.
        stub = os.path.join(td, "opencli.py")
        with open(stub, "w") as fh:
            fh.write(textwrap.dedent("""
                import sys
                if sys.argv[1] == "doctor":
                    print("[OK] Daemon: running on port 19825")
                    if "MISSING_EXT" not in __import__("os").environ:
                        print("[OK] Extension: connected (v1.0.22)")
                elif sys.argv[3] == "state":
                    print('{"url":"https://example.com/settings/api-keys?code=SHOULD_NOT_PRINT"}')
            """))
        launcher = os.path.join(td, "opencli")
        with open(launcher, "w") as fh:
            fh.write(f'#!/usr/bin/env bash\nexec "{sys.executable}" "{stub}" "$@"\n')
        os.chmod(launcher, 0o755)
        os.environ["PATH"] = td + os.pathsep + os.environ["PATH"]
        os.environ["OPENCLI"] = "opencli"

        import io
        import contextlib
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            rc = preflight("sess")
        out = buf.getvalue()
        assert rc == 0 and out.startswith("OK bound"), (rc, out)
        # the query string, which can carry an OAuth code, must be stripped
        assert "SHOULD_NOT_PRINT" not in out, f"callback query leaked: {out}"
        assert "?" not in out, out

        os.environ["MISSING_EXT"] = "1"
        assert preflight("sess") == 4, "disconnected extension was accepted"
        del os.environ["MISSING_EXT"]

        os.environ["OPENCLI"] = "opencli-does-not-exist"
        assert preflight("sess") == 3, "missing opencli was accepted"

        # --deps must report every miss, not stop at the first
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(io.StringIO()):
            deps()
        d = buf.getvalue()
        assert "MISS opencli" in d, d
        assert "python" in d, "deps stopped before checking python"
        del os.environ["OPENCLI"]
    print("preflight.py selfcheck OK")


if __name__ == "__main__":
    arg = sys.argv[1] if len(sys.argv) > 1 else ""
    if arg == "--selfcheck":
        _selfcheck()
    elif arg == "--deps":
        raise SystemExit(deps())
    else:
        raise SystemExit(preflight(arg or "connecttool"))
