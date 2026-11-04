#!/usr/bin/env python3
"""TLS and fixture credentials for a connector that adopted the credential kit.

credkit.Transport refuses to send a held credential over plain http, loopback
included (docs/credential-leak-contract.md, section 2(c)). A harness whose fake
vendor is plain http therefore sees nothing from an adopter: a check that needs
a request reports a false failure, and a check that judges only the requests it
saw passes without having judged anything.

These helpers let a harness serve its fake vendor the way a real vendor is
served. The certificate is made for the run, is its own trust anchor, covers
only 127.0.0.1, and is given to the connector the way an operator's private CA
is given: `<PREFIX>_CA_CERT` where the connector reads one, and SSL_CERT_FILE,
the standard roots override Go reads on Linux. Verification is never turned off
and the kit is never asked for an exception. A connector without the kit gets
none of this: its harness stays plain http with no new variable.

The kit also withholds any output sharing an 8-character piece with a held value
(contract 2(b), case folded), so a harness's fake credential must not read like
words: `clientid` in a fake client id withheld every tools/list that names a
clientId, and `er check` in a fake bearer token withheld doctor's own sentence.
shared_pieces() is what a fixture's self-test measures that with.

Stdlib only, plus the openssl command check_doctor_truth.py already required.
"""

from __future__ import annotations

import codecs
import os
import re
import ssl
import subprocess
import sys
import threading
from dataclasses import dataclass
from pathlib import Path

CA_CERT_VAR = re.compile(r'"([A-Z][A-Z0-9_]*_CA_CERT)"')
PIECE_LEN = 8  # contract 2(b)


@dataclass
class Served:
    """A connector being served over TLS: the certificate, and what makes it trusted."""
    cert: tuple[str, str]
    trust: dict[str, str]


# Which connector this thread is serving, none for plain http. It lives here, not
# in class_harness.py, because that file runs as __main__ and the class modules
# import it again: state kept there would be two separate variables, and every
# class would start a plain server while the driver believed it had TLS. It is per
# thread because `class_harness.py --all` runs connectors on a thread pool: one
# connector's certificate must not reach another connector's classes.
_this_thread = threading.local()


def serving() -> Served | None:
    return getattr(_this_thread, "served", None)


def set_serving(served: Served | None) -> None:
    _this_thread.served = served


def shared_pieces(value: str, text: str) -> list[str]:
    """The 8-character windows of `value` that `text` also contains, read as the
    kit reads them: case folded, and through ROT13, which contract 3 names."""
    folded = value.casefold()
    views = (text.casefold(), codecs.encode(text, "rot13").casefold())
    return sorted({folded[i:i + PIECE_LEN] for i in range(len(folded) - PIECE_LEN + 1)
                   if any(folded[i:i + PIECE_LEN] in v for v in views)})


def windows_hold_digit_and_letter(value: str) -> bool:
    """Is every 8-character window of `value` unlike any word or number?"""
    wins = [value[i:i + PIECE_LEN] for i in range(len(value) - PIECE_LEN + 1)]
    return bool(wins) and all(any(c.isdigit() for c in w) and any(c.isalpha() for c in w) for w in wins)


def make_cert(out_dir: str) -> tuple[str, str]:
    """A self-signed certificate for 127.0.0.1, generated for this run only.
    Returns (certificate path, key path). The certificate is also the trust
    anchor the connector is given, so nothing else is ever trusted."""
    cert, key = os.path.join(out_dir, "stub-cert.pem"), os.path.join(out_dir, "stub-key.pem")
    r = subprocess.run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
                        "-nodes", "-days", "1", "-subj", "/CN=127.0.0.1", "-addext", "subjectAltName=IP:127.0.0.1",
                        "-keyout", key, "-out", cert], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"could not generate the stub's TLS certificate with openssl: {r.stderr.strip()[-400:]}")
    return cert, key


def adopts_kit(skill_dir: Path) -> bool:
    """Did this connector vendor the credential kit?"""
    return (Path(skill_dir) / "cli" / "internal" / "credkit").is_dir()


def ca_cert_var(skill_dir: Path) -> str | None:
    """The environment variable this connector reads a private CA from, taken
    from its config package. None when it declares none."""
    for src in sorted((Path(skill_dir) / "cli" / "internal" / "config").glob("*.go")):
        if src.name.endswith("_test.go"):
            continue
        try:
            m = CA_CERT_VAR.search(src.read_text(encoding="utf-8", errors="replace"))
        except OSError:
            continue
        if m:
            return m.group(1)
    return None


def trust_env(skill_dir: Path, cert: str) -> dict[str, str]:
    """The variables that make this connector trust `cert`."""
    env = {"SSL_CERT_FILE": cert}
    var = ca_cert_var(skill_dir)
    if var:
        env[var] = cert
    return env


def refusal(skill_dir: Path, platform: str = sys.platform) -> str | None:
    """Why this connector cannot be pointed at the certificate on this host, or
    None. Go reads SSL_CERT_FILE on Linux only, so elsewhere a connector with no
    CA variable would be judged on the host's trust store, not on its own code.
    That is a harness limit, never a connector verdict."""
    if platform == "linux" or ca_cert_var(skill_dir):
        return None
    return ("this connector adopted the credential kit and reads no <PREFIX>_CA_CERT, and Go reads SSL_CERT_FILE "
            "on Linux only, so its fake vendor cannot be trusted here; run it on Linux")


def serve_tls(srv, cert: tuple[str, str]) -> None:
    """Put a listening http.server on TLS. The handshake runs on the first read,
    in the connection's own thread, so one stalled client cannot hold the accept
    loop and a client that refuses the certificate fails only its own request."""
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(cert[0], cert[1])
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True, do_handshake_on_connect=False)
