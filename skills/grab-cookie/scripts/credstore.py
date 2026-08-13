# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
#
# Vendored into credgrab from Servosity's msp-skills `connect-tool` skill
# (https://github.com/Servosity/msp-skills), Apache-2.0, author Damien Stevens.
# Only change from upstream: the credential-store namespace in target_name() is
# `credgrab/<service>/<account>` instead of `Servosity/connect-tool/...`.
# See tools/credgrab/README.md (Attribution) and tools/credgrab/NOTICE.
#
"""The credential backend: macOS Keychain or Windows Credential Manager.

One interface, two implementations. This is the ONLY module that ever holds a
plaintext secret, and it never prints one: `store()` returns a redacted receipt
(length, sha256[:8], last4) and nothing else.

  macOS   -> `security add-generic-password` / `find-generic-password`
  Windows -> CredWriteW / CredReadW / CredDeleteW through ctypes

Why ctypes and not PowerShell Add-Type P/Invoke: Constrained Language Mode (what
AppLocker and WDAC induce on a managed endpoint, which is a normal state for an
MSP's own machines) blocks arbitrary C# and Win32 P/Invoke from PowerShell.
ctypes inside a normal Python process is unaffected. It is also the stronger
option: the secret is passed as a memory buffer, so unlike the macOS
`security -w <value>` call it never appears in any process command line, and so
never in a Sysmon Event 1 or a 4688 audit record.

Lane C (the user pastes it themselves) is a mode of this module, so it works the
same on both platforms:

  uv run credstore.py --store --service SVC --account ACCT   # hidden prompt
  uv run credstore.py --selfcheck
"""
from __future__ import annotations

import hashlib
import re
import subprocess
import sys

import ctplatform

# CRED_MAX_CREDENTIAL_BLOB_SIZE. A service-account JSON file will not fit; that
# is a Lane C case anyway (see references/security-model.md).
WIN_MAX_BLOB = 2560
CRED_TYPE_GENERIC = 1
# LOCAL_MACHINE, deliberately not ENTERPRISE: ENTERPRISE roams with a roaming
# profile, which would carry the secret to every machine the user signs into.
CRED_PERSIST_LOCAL_MACHINE = 2


def target_name(service: str, account: str) -> str:
    """Credential Manager target. Namespaced so our entries are identifiable and
    revocable in the Windows UI. Never put tenant data in a target name."""
    return f"credgrab/{service}/{account}"


# Below this length, the last 4 characters are a meaningful FRACTION of the
# secret (at 4 they are the whole thing), so the receipt omits them.
MIN_LEN_FOR_LAST4 = 12


def receipt(value: str, service: str, account: str) -> str:
    """The ONLY thing a caller may print. Not the value.

    last4 plus length is what lets an operator match a stored key against the
    masked value the vendor portal displays; sha256[:8] lets two captures be
    compared without either being shown. A short secret gets no last4 at all.
    """
    tail = f" last4={value[-4:]}" if len(value) >= MIN_LEN_FOR_LAST4 else " last4=(withheld, short secret)"
    return (f"STORED service={service} account={account} len={len(value)} "
            f"sha256_8={hashlib.sha256(value.encode()).hexdigest()[:8]}{tail}")


# -- macOS -----------------------------------------------------------------

def _mac_store(service: str, account: str, value: str) -> None:
    # -U updates in place. The value is in argv here; that is the documented,
    # accepted local-only residual (references/security-model.md).
    r = subprocess.run(["/usr/bin/security", "add-generic-password", "-U",
                        "-a", account, "-s", service, "-w", value],
                       capture_output=True, text=True, shell=False)
    if r.returncode != 0:
        raise CredError("keychain write failed")


_MAC_HEX = re.compile(rb"password: 0x([0-9A-Fa-f]+)")


def _mac_fetch(service: str, account: str) -> str | None:
    """Read a Keychain item, correctly, including non-ASCII values.

    `security ... -w` prints the password as HEX (with no 0x prefix and no
    warning) whenever the data is not printable ASCII, so a UTF-8 secret read
    back through -w alone is silently the wrong string. `-g` disambiguates: it
    emits `password: 0x<HEX>  "..."` exactly in that case, and a plain quoted
    value otherwise. So: ask -g whether this is the hex case, and only then
    decode. Both outputs are captured and neither is ever printed.
    """
    g = subprocess.run(["/usr/bin/security", "find-generic-password",
                        "-a", account, "-s", service, "-g"],
                       capture_output=True, shell=False)
    if g.returncode != 0:
        return None
    if m := _MAC_HEX.search(g.stderr):
        try:
            return bytes.fromhex(m.group(1).decode()).decode("utf-8")
        except (ValueError, UnicodeDecodeError):
            raise CredError("stored credential is not valid UTF-8")
    r = subprocess.run(["/usr/bin/security", "find-generic-password",
                        "-a", account, "-s", service, "-w"],
                       capture_output=True, text=True, shell=False)
    if r.returncode != 0:
        return None
    # `security` appends exactly one newline of its own. Strip that one only:
    # trailing whitespace inside the secret is significant.
    out = r.stdout
    return out[:-1] if out.endswith("\n") else out


def _mac_delete(service: str, account: str) -> bool:
    r = subprocess.run(["/usr/bin/security", "delete-generic-password",
                        "-a", account, "-s", service],
                       capture_output=True, text=True, shell=False)
    return r.returncode == 0


# -- Windows ---------------------------------------------------------------

def encode_blob(value: str) -> bytes:
    """Credential Manager stores UTF-16LE, which is what its UI expects to show."""
    blob = value.encode("utf-16-le")
    if len(blob) > WIN_MAX_BLOB:
        raise CredError(f"secret is {len(blob)} bytes; Credential Manager allows {WIN_MAX_BLOB}")
    return blob


def decode_blob(blob: bytes) -> str:
    return blob.decode("utf-16-le")


def _win_structs():
    import ctypes
    import ctypes.wintypes as wt

    class FILETIME(ctypes.Structure):
        _fields_ = [("dwLowDateTime", wt.DWORD), ("dwHighDateTime", wt.DWORD)]

    class CREDENTIAL(ctypes.Structure):
        _fields_ = [
            ("Flags", wt.DWORD), ("Type", wt.DWORD), ("TargetName", wt.LPWSTR),
            ("Comment", wt.LPWSTR), ("LastWritten", FILETIME),
            ("CredentialBlobSize", wt.DWORD), ("CredentialBlob", ctypes.POINTER(ctypes.c_byte)),
            ("Persist", wt.DWORD), ("AttributeCount", wt.DWORD),
            ("Attributes", ctypes.c_void_p), ("TargetAlias", wt.LPWSTR), ("UserName", wt.LPWSTR),
        ]

    advapi = ctypes.WinDLL("advapi32", use_last_error=True)  # type: ignore[attr-defined]
    advapi.CredWriteW.argtypes = [ctypes.POINTER(CREDENTIAL), wt.DWORD]
    advapi.CredWriteW.restype = wt.BOOL
    advapi.CredReadW.argtypes = [wt.LPCWSTR, wt.DWORD, wt.DWORD,
                                 ctypes.POINTER(ctypes.POINTER(CREDENTIAL))]
    advapi.CredReadW.restype = wt.BOOL
    advapi.CredDeleteW.argtypes = [wt.LPCWSTR, wt.DWORD, wt.DWORD]
    advapi.CredDeleteW.restype = wt.BOOL
    advapi.CredFree.argtypes = [ctypes.c_void_p]
    advapi.CredFree.restype = None
    return ctypes, CREDENTIAL, advapi


def _win_store(service: str, account: str, value: str) -> None:
    ctypes, CREDENTIAL, advapi = _win_structs()
    blob = encode_blob(value)
    buf = ctypes.create_string_buffer(blob, len(blob))
    cred = CREDENTIAL()
    cred.Flags = 0
    cred.Type = CRED_TYPE_GENERIC
    cred.TargetName = target_name(service, account)
    cred.Comment = "connect-tool"
    cred.CredentialBlobSize = len(blob)
    cred.CredentialBlob = ctypes.cast(buf, ctypes.POINTER(ctypes.c_byte))
    cred.Persist = CRED_PERSIST_LOCAL_MACHINE
    cred.AttributeCount = 0
    cred.Attributes = None
    cred.UserName = account
    ok = advapi.CredWriteW(ctypes.byref(cred), 0)
    ctypes.memset(buf, 0, len(blob))          # best effort; see the caveat in the docstring
    if not ok:
        raise CredError(f"CredWriteW failed (error {ctypes.get_last_error()})")


def _win_fetch(service: str, account: str) -> str | None:
    ctypes, CREDENTIAL, advapi = _win_structs()
    ptr = ctypes.POINTER(CREDENTIAL)()
    if not advapi.CredReadW(target_name(service, account), CRED_TYPE_GENERIC, 0, ctypes.byref(ptr)):
        return None
    try:
        c = ptr.contents
        # string_at, NOT slicing a c_byte array: c_byte is SIGNED, so any byte
        # >= 0x80 comes back negative and bytearray() raises. That would make
        # every non-ASCII secret unreadable after a successful write.
        return decode_blob(ctypes.string_at(c.CredentialBlob, c.CredentialBlobSize))
    finally:
        advapi.CredFree(ptr)


def _win_delete(service: str, account: str) -> bool:
    _, _, advapi = _win_structs()
    return bool(advapi.CredDeleteW(target_name(service, account), CRED_TYPE_GENERIC, 0))


# -- public interface ------------------------------------------------------

class CredError(RuntimeError):
    """Raised with a FIXED message. Never interpolate a captured value into it."""


def store(service: str, account: str, value: str) -> str:
    """Write a secret and return only its redacted receipt."""
    if not value:
        raise CredError("refusing to store an empty value")
    (_win_store if ctplatform.WINDOWS else _mac_store)(service, account, value)
    return receipt(value, service, account)


def fetch(service: str, account: str) -> str | None:
    return (_win_fetch if ctplatform.WINDOWS else _mac_fetch)(service, account)


def delete(service: str, account: str) -> bool:
    return (_win_delete if ctplatform.WINDOWS else _mac_delete)(service, account)


def backend() -> str:
    return "windows-credential-manager" if ctplatform.WINDOWS else "macos-keychain"


def _selfcheck() -> None:
    # Platform-independent: the receipt is redacted and the blob round-trips.
    secret = "sk_test_ABC123"
    r = receipt(secret, "SVC", "acct")
    assert secret not in r, "SECRET LEAKED into the receipt"
    assert "len=14" in r and "last4=C123" in r, r
    # a short secret must not have its whole value published as "last4"
    short = receipt("abcd", "SVC", "acct")
    assert "abcd" not in short and "withheld" in short, short
    assert decode_blob(encode_blob("hello é 世")) == "hello é 世"
    try:
        encode_blob("x" * (WIN_MAX_BLOB // 2 + 1))
        raise AssertionError("oversized blob was not rejected")
    except CredError:
        pass
    assert target_name("HALOPSA_API_KEY", "halopsa") == \
        "credgrab/HALOPSA_API_KEY/halopsa"

    # Live round-trip against the real backend for this platform.
    svc, acct = "CONNECT_TOOL_SELFCHECK", "selfcheck"
    try:
        out = store(svc, acct, secret)
        assert secret not in out, "SECRET LEAKED from store()"
        assert fetch(svc, acct) == secret, "round-trip mismatch"
        # non-ASCII and significant trailing whitespace must survive the store
        for tricky in ("clé-de-passe-é世", "trailing-space ", "trailing-newline\n"):
            store(svc, acct, tricky)
            assert fetch(svc, acct) == tricky, f"round-trip mangled {tricky!r}"
        store(svc, acct, secret)
        assert fetch(svc, "no-such-account") is None, "fetch invented a credential"
        assert delete(svc, acct) and fetch(svc, acct) is None, "delete did not remove it"
    finally:
        delete(svc, acct)
    print(f"credstore.py selfcheck OK (backend={backend()}, round-trip + no leak)")


def _interactive_store(service: str, account: str) -> int:
    """Lane C. The user types the secret at a hidden prompt in THEIR OWN terminal:
    the value never enters argv, a file, or the agent's context. The agent's only
    confirmation is the redacted receipt, then the Lane-5 verification call."""
    import getpass
    value = getpass.getpass(f"Paste the secret for {account}/{service} (input hidden): ")
    if not value:
        print("FAIL: nothing entered", file=sys.stderr)
        return 2
    try:
        print(store(service, account, value))
    except CredError as e:
        print(f"FAIL: {e}", file=sys.stderr)
        return 5
    finally:
        del value
    return 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if "--selfcheck" in args:
        _selfcheck()
    elif "--store" in args:
        import argparse
        ap = argparse.ArgumentParser()
        ap.add_argument("--store", action="store_true")
        ap.add_argument("--service", required=True)
        ap.add_argument("--account", required=True)
        a = ap.parse_args(args)
        raise SystemExit(_interactive_store(a.service, a.account))
    else:
        print(__doc__)
