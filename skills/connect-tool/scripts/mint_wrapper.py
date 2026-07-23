# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Stamp a launcher so the consuming CLI reads its secret from the credential
store at launch: the value never lives in a config file or an env export the
model can see.

macOS   -> a bash launcher in ~/.local/bin
Windows -> a .cmd shim in %LOCALAPPDATA%\\Servosity\\connect-tool\\bin

The Windows shim contains NO credential logic. It calls this module's `launch`
mode, which reads the credential in-process and execs the real binary, so no
PowerShell or cmd.exe ever receives the secret in a command line or in generated
script text.

Every argument is validated before it is written into an executable file.

  uv run mint_wrapper.py <name> <ENV_VAR> <account> <SERVICE> <absolute-real-binary>
  uv run mint_wrapper.py --launch <ENV_VAR> <account> <SERVICE> <real-binary> [args...]
  uv run mint_wrapper.py --selfcheck
"""
from __future__ import annotations

import os
import pathlib
import shlex
import subprocess
import sys

import credstore
import ctplatform as ct

THIS = pathlib.Path(__file__).resolve()


def mint(name: str, envvar: str, account: str, service: str, real: str) -> int:
    if not ct.valid_name(name):
        print(f"FAIL: bad wrapper name '{name}'", file=sys.stderr)
        return 2
    if not ct.valid_env_var(envvar):
        print(f"FAIL: bad env var '{envvar}'", file=sys.stderr)
        return 2
    if not (ct.valid_name(account) and ct.valid_name(service)):
        print("FAIL: bad keychain account/service", file=sys.stderr)
        return 2
    if not ct.is_absolute(real):
        print("FAIL: real binary must be an absolute path", file=sys.stderr)
        return 2

    bindir = ct.wrapper_dir()
    if ct.WINDOWS:
        dest = bindir / f"{name}.cmd"
        launcher = (
            "@echo off\r\n"
            f"rem connect-tool launcher for {name}. The secret stays in Credential Manager.\r\n"
            f'"{sys.executable}" "{THIS}" --launch {envvar} {account} {service} "{real}" %*\r\n'
        )
        dest.write_text(launcher, encoding="utf-8", newline="")
    else:
        dest = bindir / name
        launcher = (
            "#!/usr/bin/env bash\n"
            f"# connect-tool launcher for {name}. The secret stays in the Keychain, never in a file.\n"
            f'exec {shlex.quote(sys.executable)} {shlex.quote(str(THIS))} --launch '
            f'{envvar} {account} {service} {shlex.quote(real)} "$@"\n'
        )
        dest.write_text(launcher, encoding="utf-8")
        dest.chmod(0o755)
    print(f"wrote launcher {dest} ({envvar} <- {credstore.backend()} {account}/{service} -> {real})")
    return 0


def launch(envvar: str, account: str, service: str, real: str, argv: list[str]) -> int:
    """Read the credential in THIS process and hand it to the child as an
    environment variable. It never touches a command line or a file."""
    env = dict(os.environ)
    if not env.get(envvar):
        value = credstore.fetch(service, account)
        if value is None:
            print(f"FAIL: no credential for {account}/{service}. Run connect-tool to set it up.",
                  file=sys.stderr)
            return 4
        env[envvar] = value
    return subprocess.run([real, *argv], env=env, shell=False).returncode


def _selfcheck() -> None:
    import contextlib
    import io
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        os.environ["CONNECT_TOOL_BIN"] = td
        buf = io.StringIO()
        real = sys.executable        # absolute on both platforms
        with contextlib.redirect_stdout(buf):
            assert mint("halopsa-cli", "HALOPSA_API_KEY", "halopsa", "HALOPSA_API_KEY", real) == 0
        w = pathlib.Path(td) / ("halopsa-cli.cmd" if ct.WINDOWS else "halopsa-cli")
        assert w.exists(), "launcher not written"
        body = w.read_text()
        assert "--launch HALOPSA_API_KEY halopsa HALOPSA_API_KEY" in body, body
        # The launcher must contain no credential logic of its own.
        assert "security" not in body and "Cred" not in body, body
        if not ct.WINDOWS:
            assert os.access(w, os.X_OK), "launcher not executable"

        with contextlib.redirect_stderr(io.StringIO()):
            for bad in ("../evil", "a b", "", "CON"):
                assert mint(bad, "X", "a", "s", real) == 2, f"bad name {bad!r} accepted"
            assert mint("ok", "BAD VAR", "a", "s", real) == 2, "bad env var accepted"
            assert mint("ok", "OK", "a", "s", "relative/path") == 2, "relative binary accepted"

        # launch() feeds the real credential to the child through the environment
        svc, acct = "CONNECT_TOOL_SELFCHECK", "mint"
        try:
            credstore.store(svc, acct, "sk_test_ABC123")
            probe = os.path.join(td, "probe.py")
            with open(probe, "w") as fh:
                fh.write("import os,sys; sys.exit(0 if os.environ.get('X_KEY')=='sk_test_ABC123' else 9)")
            rc = launch("X_KEY", acct, svc, sys.executable, [probe])
            assert rc == 0, f"child did not receive the credential (exit {rc})"
            assert "X_KEY" not in os.environ, "launch() leaked the credential into this process"
        finally:
            credstore.delete(svc, acct)
        del os.environ["CONNECT_TOOL_BIN"]
    print("mint_wrapper.py selfcheck OK (launcher, arg validation, credential reaches the child)")


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
        raise SystemExit(0)
    if args and args[0] == "--launch":
        if len(args) < 5:
            print("usage: mint_wrapper.py --launch <ENV_VAR> <account> <SERVICE> <binary> [args...]",
                  file=sys.stderr)
            raise SystemExit(2)
        raise SystemExit(launch(args[1], args[2], args[3], args[4], args[5:]))
    if len(args) != 5:
        print("usage: mint_wrapper.py <name> <ENV_VAR> <account> <SERVICE> <absolute-binary>",
              file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(mint(*args))
