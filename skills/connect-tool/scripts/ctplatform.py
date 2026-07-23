# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Platform seams for connect-tool: paths, permissions, process spawning, validation.

Everything that differs between macOS and Windows and is NOT credential storage
lives here (credential storage is credstore.py). Imported by every helper, so it
stays stdlib-only and never touches a secret value.

Named ctplatform (not platform) so it cannot shadow the stdlib `platform` module
for anything else on sys.path.

  uv run ctplatform.py --selfcheck
"""
from __future__ import annotations

import os
import pathlib
import re
import shutil
import subprocess
import sys

WINDOWS = sys.platform == "win32"
MACOS = sys.platform == "darwin"

# Reserved DOS device names: a file called CON or NUL is not a file.
_WIN_RESERVED = {"CON", "PRN", "AUX", "NUL", *(f"COM{i}" for i in range(1, 10)),
                 *(f"LPT{i}" for i in range(1, 10))}
_SAFE_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
_ENV_VAR = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


# -- paths -----------------------------------------------------------------

def app_dir() -> pathlib.Path:
    """Root for all connect-tool runtime state. Never inside the repo."""
    if env := os.environ.get("CONNECT_TOOL_HOME"):
        return pathlib.Path(env).expanduser()
    if WINDOWS:
        base = os.environ.get("LOCALAPPDATA") or (pathlib.Path.home() / "AppData/Local")
        return pathlib.Path(base) / "Servosity" / "connect-tool"
    return pathlib.Path.home() / ".config" / "connect-tool"


def state_dir() -> pathlib.Path:
    if env := os.environ.get("CONNECT_TOOL_STATE_DIR"):
        return secure_mkdir(pathlib.Path(env).expanduser())
    return secure_mkdir(app_dir() / "state")


def runs_dir() -> pathlib.Path:
    if env := os.environ.get("CONNECT_TOOL_RUNS_DIR"):
        return secure_mkdir(pathlib.Path(env).expanduser())
    return secure_mkdir(app_dir() / "runs")


def wrapper_dir() -> pathlib.Path:
    """Where launcher shims land. ~/.local/bin is not a Windows PATH convention."""
    if env := os.environ.get("CONNECT_TOOL_BIN"):
        return secure_mkdir(pathlib.Path(env).expanduser())
    if WINDOWS:
        return secure_mkdir(app_dir() / "bin")
    d = pathlib.Path.home() / ".local" / "bin"
    d.mkdir(parents=True, exist_ok=True)
    return d


def secure_mkdir(path: pathlib.Path) -> pathlib.Path:
    """Create a directory only this user can read.

    POSIX: mode 0700. Windows: chmod is a no-op, so break inheritance and grant
    the current user + SYSTEM explicitly via icacls. Only paths reach icacls's
    argv, never a secret. Failure to lock down is reported, not swallowed: the
    caller decides, but silence would be a lie.
    """
    path.mkdir(parents=True, exist_ok=True)
    if not WINDOWS:
        try:
            path.chmod(0o700)
        except OSError:
            pass
        return path
    marker = path / ".acl-applied"
    if marker.exists():
        return path
    user = os.environ.get("USERNAME", "")
    icacls = shutil.which("icacls")
    if icacls and user:
        # /inheritance:r drops inherited (often Users-readable) ACEs first.
        rc = subprocess.run([icacls, str(path), "/inheritance:r",
                             "/grant:r", f"{user}:(OI)(CI)F", "/grant:r", "SYSTEM:(OI)(CI)F"],
                            capture_output=True, text=True, shell=False).returncode
        if rc == 0:
            marker.write_text("1", encoding="utf-8")
        else:
            print(f"WARN: could not restrict permissions on {path}", file=sys.stderr)
    return path


# -- validation ------------------------------------------------------------

def valid_name(name: str) -> bool:
    """A filename-safe token that is also legal on Windows."""
    if not name or not _SAFE_NAME.match(name):
        return False
    if name != name.rstrip(". "):        # Windows silently strips these
        return False
    return name.split(".")[0].upper() not in _WIN_RESERVED


def valid_env_var(name: str) -> bool:
    return bool(_ENV_VAR.match(name or ""))


def is_absolute(p: str) -> bool:
    """True for /usr/bin/x, C:\\tools\\x.exe, and \\\\server\\share\\x.exe."""
    if not p:
        return False
    if WINDOWS:
        return bool(re.match(r"^([A-Za-z]:[\\/]|\\\\[^\\/]+[\\/])", p))
    return p.startswith("/")


# -- processes -------------------------------------------------------------

def which_opencli() -> str | None:
    """Resolve the opencli entry point (opencli.cmd on Windows via npm)."""
    return shutil.which(os.environ.get("OPENCLI", "opencli"))


def run(argv: list[str], *, timeout: int = 120, check: bool = False) -> subprocess.CompletedProcess:
    """Run a command with no shell, ever.

    shell=False means selectors and URLs are never parsed by cmd.exe or bash, so
    a metacharacter in a CSS selector cannot become a command. Output is captured
    for the caller to inspect; callers that handle secrets must not print it.
    """
    return subprocess.run(argv, capture_output=True, text=True, shell=False,
                          timeout=timeout, check=check)


def detached(argv: list[str], stdout, cwd: str | None = None) -> subprocess.Popen:
    """Spawn a child that survives this process (an OAuth login holding a
    127.0.0.1 callback). No shell, no inherited stdin, no console window."""
    kwargs: dict = {"stdin": subprocess.DEVNULL, "stdout": stdout,
                    "stderr": subprocess.STDOUT, "shell": False, "cwd": cwd,
                    "text": True, "bufsize": 1, "errors": "replace"}
    if WINDOWS:
        # getattr: these constants only exist on Windows builds of subprocess.
        kwargs["creationflags"] = (getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0x00000200)
                                   | getattr(subprocess, "DETACHED_PROCESS", 0x00000008)
                                   | getattr(subprocess, "CREATE_NO_WINDOW", 0x08000000))
    else:
        kwargs["start_new_session"] = True
    return subprocess.Popen(argv, **kwargs)


def _selfcheck() -> None:
    import tempfile
    assert valid_name("halopsa-cli") and valid_name("a.b_c-1")
    for bad in ("", "../evil", "a b", "CON", "nul.txt", "trailing.", "trailing ", "-lead"):
        assert not valid_name(bad), f"accepted bad name {bad!r}"
    assert valid_env_var("HALOPSA_API_KEY") and not valid_env_var("BAD VAR")
    assert not valid_env_var("1LEADING")
    if WINDOWS:
        assert is_absolute(r"C:\tools\x.exe") and is_absolute(r"\\srv\share\x.exe")
        assert not is_absolute("relative\\x.exe")
    else:
        assert is_absolute("/usr/bin/true") and not is_absolute("relative/x")
    with tempfile.TemporaryDirectory() as td:
        os.environ["CONNECT_TOOL_HOME"] = td
        # Dirs we own must be private. wrapper_dir on POSIX is the user's shared
        # ~/.local/bin, which is not ours to lock down (it holds no secret; the
        # launcher reads the credential at run time).
        for d in (state_dir(), runs_dir()):
            assert d.is_dir()
            if not WINDOWS:
                assert (d.stat().st_mode & 0o077) == 0, f"{d} is group/world accessible"
        assert wrapper_dir().is_dir()
        del os.environ["CONNECT_TOOL_HOME"]
    # run() must never involve a shell: a metacharacter stays literal data.
    out = run([sys.executable, "-c", "import sys; print(sys.argv[1])", "a&b|c;d"])
    assert out.stdout.strip() == "a&b|c;d", out.stdout
    print("ctplatform.py selfcheck OK")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
    else:
        print(__doc__)
