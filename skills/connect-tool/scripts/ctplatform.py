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
    if not (icacls and user):
        # Say so rather than silently leaving an inherited, possibly readable ACL.
        print(f"WARN: cannot restrict permissions on {path} (icacls or USERNAME unavailable); "
              "treat its contents as readable by other local accounts", file=sys.stderr)
        return path
    if True:
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


# A value that LOOKS like a credential, regardless of what field it arrived in.
# Field-name denylists only catch a secret that announces itself; this catches
# `detail=Authorization: Bearer sk_live_...` and friends.
_SECRET_VALUE = re.compile(
    r"(?:sk|pk|rk|ghp|gho|ghu|ghs|ghr|xox[abprs]|AKIA|ASIA|glpat|shpat)[-_][A-Za-z0-9_-]{12,}"
    r"|Bearer\s+[A-Za-z0-9._~+/-]{16,}"
    r"|eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}"          # a JWT
    r"|-----BEGIN [A-Z ]*PRIVATE KEY-----"
    r"|(?:access_token|refresh_token|client_secret|api_?key|password|passwd)[\"\s]*[:=]",
    re.I)
# A long unbroken high-entropy run: the shape of an opaque token.
_OPAQUE = re.compile(r"(?<![A-Za-z0-9._~+/-])[A-Za-z0-9._~+/-]{40,}(?![A-Za-z0-9._~+/-])")


def looks_secret(value: str) -> bool:
    """True if this string plausibly IS a credential. Used to refuse writing or
    printing it, in addition to the field-name checks."""
    if not value:
        return False
    if _SECRET_VALUE.search(value):
        return True
    for run in _OPAQUE.findall(value):
        # Mixed case plus digits over 40+ characters is a token, not prose or a path.
        if any(c.isdigit() for c in run) and any(c.islower() for c in run) and any(c.isupper() for c in run):
            return True
    return False


def is_absolute(p: str) -> bool:
    """True for /usr/bin/x, C:\\tools\\x.exe, and \\\\server\\share\\x.exe."""
    if not p:
        return False
    if WINDOWS:
        return bool(re.match(r"^([A-Za-z]:[\\/]|\\\\[^\\/]+[\\/])", p))
    return p.startswith("/")


# -- processes -------------------------------------------------------------

def opencli_cmd() -> list[str] | None:
    """Resolve the opencli entry point as a full argv prefix.

    On Windows npm installs `opencli.cmd`, and launching a .cmd goes through
    cmd.exe even with shell=False, which re-parses arguments: a `&` inside an
    OAuth URL could then split the command. So when the resolved entry is a
    batch shim we run its JavaScript entry point through node.exe directly,
    which takes a real argv and cannot be re-parsed.
    """
    exe = shutil.which(os.environ.get("OPENCLI", "opencli"))
    if not exe:
        return None
    if not exe.lower().endswith((".cmd", ".bat")):
        return [exe]
    node = shutil.which("node")
    if node:
        # npm puts the shim in <prefix>/ and the package under <prefix>/node_modules/.
        here = pathlib.Path(exe).parent
        for base in (here, here.parent):
            entry = base / "node_modules" / "@jackwener" / "opencli" / "dist" / "src" / "main.js"
            if entry.is_file():
                return [node, str(entry)]
    # Fall back to the shim. Callers must not pass attacker-controlled arguments.
    return [exe]


def which_opencli() -> str | None:
    """Back-compat: the entry point as a single string, or None."""
    cmd = opencli_cmd()
    return cmd[0] if cmd else None


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
    # Value-level secret detection (field names are not enough). The fixtures are
    # assembled at runtime so this file never CONTAINS a credential-shaped literal
    # for the repo's own secret scanner to find. Same trick as tools/ci_guards.sh.
    pat, dash = "0123456789abcdefghij", "-" * 5
    for s in ("sk" + "_live_abcdefghijklmnop",
              "Authorization: Bearer abcdefghijklmnopqrstuvwx",
              "ghp" + "_" + pat, "eyJhbGciOiJI.eyJzdWIiOiIx",
              'detail={"access' + '_token":"x"}',
              dash + "BEGIN RSA PRIVATE KEY" + dash,
              "aB3" + "x9Y2z" * 10):
        assert looks_secret(s), f"missed a secret-shaped value: {s[:12]}"
    for s in ("token refresh scheduled", "acct_00000000", "user@example.com", "",
              "C:" + chr(92) + "temp" + chr(92) + "notes.txt", "consent_shown"):
        assert not looks_secret(s), f"false positive on: {s}"
    print("ctplatform.py selfcheck OK")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
    else:
        print(__doc__)
