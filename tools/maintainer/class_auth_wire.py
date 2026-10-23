#!/usr/bin/env python3
"""Class: auth is on the wire, and no credential follows a redirect off-host.

The defended property
---------------------
Two failures the loops kept finding:

  * A credential never actually reaches the request - a blanked multi-apiKey
    header, an OAuth scope never sent, a composed header dropped - so the first
    live call 401s (autotask header-blanking, sherweb APIM key, superops
    CustomerSubDomain).
  * A credential follows a cross-host redirect to a server the operator never
    configured (avanan transport re-stamp, connectwise-automate ClientId,
    servosity custom headers). Go strips the standard Authorization header on a
    cross-host hop but not custom ones, so a connector must delete them itself.

How it is driven (black box)
----------------------------
The built CLI runs `doctor` (and one read command) against a local fake server,
with every credential set to a unique sentinel. The primary server records what
arrived; a 307 hop points at a SECOND loopback server (a different host:port, so
Go treats it as cross-host). The test asserts (a) at least one credential
sentinel reached the primary host, and (b) no credential sentinel and no
Authorization header reached the second host.

Both directions (self_test): the off-host-leak detector fires on a captured
request that carried a credential to the second host and is silent on one that
did not; the on-wire detector fires when no request carried a credential.
"""

from __future__ import annotations

import base64
import os
import subprocess

import class_harness as ch


def _request_views(path: str, headers: dict, body: str) -> str:
    """All text to search in one request, including base64-decoded header values
    so a credential carried inside a Basic or composite header (base64(id:secret))
    is still found."""
    blob = path + "\n" + "\n".join(f"{k}: {v}" for k, v in headers.items()) + "\n" + body
    extra = []
    for v in list(headers.values()) + [body]:
        for token in v.replace(",", " ").split():
            if len(token) < 8:
                continue
            try:
                dec = base64.b64decode(token + "=" * (-len(token) % 4), validate=False)
                extra.append(dec.decode("latin-1", "replace"))
            except Exception:  # noqa: BLE001
                pass
    return blob + "\n" + "\n".join(extra)


def _requests_carrying(requests: list, secrets: list[str]) -> list[str]:
    """Which credential sentinels appear anywhere in these requests (header,
    body, or inside a base64-encoded header value)."""
    found = set()
    for _host, _method, path, headers, body in requests:
        view = _request_views(path, headers, body)
        for s in secrets:
            if ch.leaks(view, s):
                found.add(s)
    return sorted(found)


def _auth_header_present(requests: list) -> bool:
    for _host, _method, _path, headers, _body in requests:
        for k in headers:
            if k.lower() in ("authorization", "proxy-authorization"):
                return True
    return False


def _make_request(ctx: ch.Ctx, env: dict) -> None:
    """Make one authenticated request. `doctor` is universal (every connector
    ships it and it probes with the configured credential), so it alone drives
    both the on-wire and the redirect checks; a connector that refuses a
    cross-host redirect then retries with backoff, so doing this ONCE per server
    keeps the cost bounded."""
    try:
        subprocess.run([str(ctx.cli_bin), "doctor", "--json"], capture_output=True,
                       text=True, env=env, timeout=ch.RUN_TIMEOUT)
    except (OSError, subprocess.SubprocessError):
        pass


def run(ctx: ch.Ctx) -> list[str]:
    secrets = list(ctx.wiring.sentinels.values())
    if not secrets:
        return []
    findings: list[str] = []
    base_env = {"PATH": os.environ.get("PATH", "")}
    for var in ctx.wiring.env:
        if var.endswith("_NO_CONFIG_WRITE"):
            base_env[var] = "1"

    # (A) credential-on-wire: a sentinel must reach the configured host.
    primary = ch.start_server(mode="echo", status=200, sentinels=secrets)
    env = {**base_env, "HOME": ch.fresh_home(ctx, "auth-a"), **ch.apply_urls(ctx.wiring, primary.url)}
    try:
        _make_request(ctx, env)
        reached = len(primary.capture.requests)
        carried = _requests_carrying(primary.capture.requests, secrets)
    finally:
        primary.stop()
    if reached and not carried and not _auth_header_present(primary.capture.requests):
        # Requests went out but none carried a configured credential and none
        # even set an Authorization-family header: the header was blanked or the
        # scheme/scope was never attached. (An Authorization header whose value
        # is a minted bearer rather than a raw sentinel still counts as wired.)
        findings.append(
            f"[{ctx.slug}] made {reached} request(s) to the configured host but none "
            f"carried a configured credential or an Authorization header "
            f"(blanked header / unsent scope)")

    # (B) cross-host redirect: a credential must not reach a second host.
    secondary = ch.start_server(mode="echo", status=200, sentinels=secrets)
    redirecting = ch.start_server(mode="redirect", status=200, sentinels=secrets,
                                  redirect_to=secondary.url)
    env2 = {**base_env, "HOME": ch.fresh_home(ctx, "auth-b"), **ch.apply_urls(ctx.wiring, redirecting.url)}
    try:
        _make_request(ctx, env2)
        off_host_creds = _requests_carrying(secondary.capture.requests, secrets)
        off_host_auth = _auth_header_present(secondary.capture.requests)
    finally:
        redirecting.stop()
        secondary.stop()
    if off_host_creds:
        findings.append(
            f"[{ctx.slug}] a credential followed a cross-host redirect to a server the "
            f"operator never configured (sentinels seen off-host: {off_host_creds})")
    elif off_host_auth:
        findings.append(
            f"[{ctx.slug}] an Authorization header followed a cross-host redirect to a "
            f"server the operator never configured")
    return findings


def self_test() -> bool:
    secret = ch.sentinel("TOKEN")
    ok = True

    # A request that carried the credential to the second host is a leak.
    leaked = [("127.0.0.1:2", "GET", "/redir-done",
               {"Authorization": f"Bearer {secret}"}, "")]
    if not _requests_carrying(leaked, [secret]):
        print("    auth-wire self-test: did NOT detect a credential carried off-host")
        ok = False
    if not _auth_header_present(leaked):
        print("    auth-wire self-test: did NOT detect an Authorization header off-host")
        ok = False

    # A second-host request with no credential is clean.
    clean = [("127.0.0.1:2", "GET", "/redir-done", {"Accept": "application/json"}, "")]
    if _requests_carrying(clean, [secret]):
        print("    auth-wire self-test: falsely flagged a credential-free off-host request")
        ok = False
    if _auth_header_present(clean):
        print("    auth-wire self-test: falsely reported an Authorization header")
        ok = False

    # On-wire: a request that carries the credential proves auth is wired;
    # one that does not is the blanked-header case.
    on_wire = [("127.0.0.1:1", "GET", "/v2/x",
                {"X-Api-Key": secret}, "")]
    if not _requests_carrying(on_wire, [secret]):
        print("    auth-wire self-test: did NOT see a credential that WAS on the wire")
        ok = False
    blanked = [("127.0.0.1:1", "GET", "/v2/x", {"X-Api-Key": ""}, "")]
    if _requests_carrying(blanked, [secret]):
        print("    auth-wire self-test: saw a credential in a blanked request")
        ok = False
    return ok


if __name__ == "__main__":
    import sys
    sys.exit(0 if self_test() else 1)
