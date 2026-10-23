#!/usr/bin/env python3
"""Class: a credential value never reaches CLI output, errors or MCP results.

The defended property
---------------------
A connector holds the operator's credentials. When a server (or a proxy, or the
connector's own diagnostics) reflects a credential back - in an error body, a
`Location` header, an `X-Request-Id`, a truncated title - the connector must
withhold the value before it reaches the terminal or an MCP result. This is the
class the secret-scrub loops chased one encoding at a time (saas-alerts 45
rounds, synology 54): URL and double-URL escaping, HTML entities, JSON `\\u`
escapes, base64 / base64url. A masked tail (`****abcd`) is fine; the whole value
reconstructable from output is the leak.

How it is driven (black box)
----------------------------
The built CLI and MCP server run against a local fake server that answers an
error (HTTP 403) whose body and headers reflect every configured credential in
every encoding above. The test asserts no encoding of any credential survives in
stdout, stderr or an MCP result. `--dry-run` is driven too: its request preview
must mask the Authorization value.

Both directions (self_test): the leak detector fires on each encoding of a
reflected secret and is silent on a masked, credential-free output.
"""

from __future__ import annotations

import class_harness as ch

# How many read leaves to exercise per connector; the error path is identical
# across leaves (one client, one masking funnel), so a few samples prove it.
MAX_LEAVES = 4
# How many MCP read tools to probe before giving up on reaching the API (a
# mirror tool shells to the CLI, so an unbounded loop is slow on mirror-heavy
# connectors). Enough to reach a non-mirror API tool when one exists.
MAX_MCP_PROBES = 8


def _combined(proc_out: str, proc_err: str) -> str:
    return (proc_out or "") + "\n" + (proc_err or "")


def _run_cli(ctx: ch.Ctx, args: list[str], env: dict) -> str:
    import subprocess
    try:
        p = subprocess.run([str(ctx.cli_bin)] + args, capture_output=True,
                           text=True, env=env, timeout=ch.RUN_TIMEOUT)
    except (OSError, subprocess.SubprocessError):
        return ""
    return _combined(p.stdout, p.stderr)


def run(ctx: ch.Ctx) -> list[str]:
    secrets = list(ctx.wiring.sentinels.values())
    if not secrets:
        return []  # no credential to leak (e.g. a browser-cookie connector)
    findings: list[str] = []

    base_env = ch.apply_urls(ctx.wiring, "http://placeholder")  # urls overwritten per server
    import os as _os
    base_env["PATH"] = _os.environ.get("PATH", "")
    base_env[ch.cli_path_env(ctx.slug)] = str(ctx.cli_bin)
    for var in list(base_env):
        if var.endswith("_NO_CONFIG_WRITE"):
            base_env[var] = "1"

    def env_for(server: ch.Server) -> dict:
        e = {**base_env, **ch.apply_urls(ctx.wiring, server.url), "HOME": ch.fresh_home(ctx, "secret-echo")}
        return e

    reached_server = 0
    read_leaves = [lf for lf in ch.leaves(ctx.surface) if ch.runnable_read(lf)][:MAX_LEAVES]

    def scan(where: str, text: str) -> bool:
        for secret in secrets:
            hits = ch.leaks(text, secret)
            if hits:
                findings.append(f"[{ctx.slug}] {where}: a credential value reached output, "
                                f"encoded as {sorted(set(hits))}")
                return True
        return False

    # An authenticated error that an API returns, reflecting the credentials in
    # body AND headers, in every encoding.
    server = ch.start_server(mode="echo", status=403, sentinels=secrets)
    try:
        env = env_for(server)
        # (A) Error-path CLI output in the two agent-facing modes.
        for lf in read_leaves:
            for mode in ("--json", "--agent"):
                out = _run_cli(ctx, list(lf.path) + [mode, "--no-input"], env)
                scan(f"`{' '.join(lf.path)} {mode}`", out)

        # (B) MCP read tools on the error path. Scan the WHOLE raw stdout+stderr
        # of the server (so a secret in structuredContent or on stderr is seen),
        # and keep trying read tools until one actually reaches the API (a
        # local-only tool tests nothing), rather than stopping at the first.
        read_tools = _read_tools(ctx, ch.mcp_list_tools(ctx.mcp_bin, env))
        _probe_mcp(ctx, read_tools, server, env, scan, "error")
        reached_server = len(server.capture.requests)
    finally:
        server.stop()

    # (C) dry-run must mask the Authorization value it previews.
    dry_server = ch.start_server(mode="echo", status=200, sentinels=secrets)
    try:
        env = env_for(dry_server)
        mutate = [lf for lf in ch.leaves(ctx.surface)
                  if lf.method in ("POST", "PUT", "PATCH", "DELETE")
                  and "<" not in lf.use and not _required_flags(lf)]
        if mutate:
            lf = mutate[0]
            out = _run_cli(ctx, list(lf.path) + ["--dry-run", "--no-input"] + _fill_flags(lf), env)
            scan(f"`{' '.join(lf.path)} --dry-run`", out)
    finally:
        dry_server.stop()

    # (D) Success path: a 200 whose CREDENTIAL rides only in response headers
    # (the body is credential-free, so printing legitimate data is not a false
    # positive). A connector that echoes response headers on success leaks here.
    # Driven over BOTH surfaces: the CLI read commands AND the MCP read tools,
    # so an MCP tool that surfaces a 200 response header is checked too.
    ok_server = ch.start_server(mode="echo", status=200, sentinels=secrets, body_echo=False)
    try:
        env = env_for(ok_server)
        for lf in read_leaves[:2]:
            for mode in ("--json", "--agent"):
                out = _run_cli(ctx, list(lf.path) + [mode, "--no-input"], env)
                scan(f"`{' '.join(lf.path)} {mode}` (success headers)", out)
        _probe_mcp(ctx, _read_tools(ctx, ch.mcp_list_tools(ctx.mcp_bin, env)),
                   ok_server, env, scan, "success headers")
        reached_server += len(ok_server.capture.requests)
    finally:
        ok_server.stop()
    if reached_server == 0 and not findings:
        # Honest coverage: no probe reached the client error path, so this
        # connector's redaction funnel was not tested. Report it; do not count
        # it as a clean pass.
        findings.append(
            f"NOTE: [{ctx.slug}] secret-echo not exercised - no argument-free GET read or "
            f"MCP read tool reached the client error path (reads are local/novel commands)")
    return findings


def _read_tools(ctx: ch.Ctx, tools: list[dict]) -> list[dict]:
    """Argument-free read tools, with a mirror of a real API GET ordered before
    a mirror of a local-only command (sql, recall, learnings) that never reaches
    the fake API and would use up the probe cap."""
    api_reads = {" ".join(lf.path) for lf in ch.leaves(ctx.surface)
                 if lf.method == "GET" and lf.api_path}

    def order(t: dict) -> int:
        mirrored = (t.get("_meta") or {}).get("pp:cli-command")
        return 0 if (not mirrored or mirrored in api_reads) else 1

    return sorted((t for t in tools
                   if (t.get("annotations") or {}).get("readOnlyHint")
                   and not (t.get("annotations") or {}).get("destructiveHint")
                   and not (t.get("inputSchema") or {}).get("required")),
                  key=order)


def _probe_mcp(ctx: ch.Ctx, read_tools: list[dict], server: ch.Server,
               env: dict, scan, where: str) -> None:
    """Call read tools over stdio until one reaches the fake server, scanning the
    session's whole raw stdout, stderr and result for a leaked credential."""
    for t in read_tools[:MAX_MCP_PROBES]:
        before = len(server.capture.requests)
        msgs = ch._HANDSHAKE + [{"jsonrpc": "2.0", "id": 10, "method": "tools/call",
                                 "params": {"name": t["name"], "arguments": {}}}]
        resps, out, err = ch.mcp_session(ctx.mcp_bin, msgs, env)
        answered = [m for m in resps if m.get("id") == 10]
        blob = out + "\n" + err + "\n" + "\n".join(ch.mcp_result_text(m) for m in answered)
        scan(f"MCP tool `{t['name']}` ({where})", blob)
        if len(server.capture.requests) > before:
            break  # a tool that reached the API proves this surface


def _required_flags(lf: ch.Leaf) -> list[dict]:
    return [f for f in lf.flags if f.get("required")]


def _fill_flags(lf: ch.Leaf) -> list[str]:
    out = []
    for f in lf.flags:
        if not f.get("required"):
            continue
        name = f["name"]
        kind = f.get("type", "string")
        val = "1" if kind in ("int", "integer", "float") else "cls-test"
        out += [f"--{name}", val]
    return out


def self_test() -> bool:
    secret = ch.sentinel("API_KEY")
    ok = True

    import base64 as _b64
    # Fires on every encoding a reflected secret can take.
    enc = ch.encodings(secret)
    for name, form in enc.items():
        body = f'{{"error":"bad","reflected":"{form}"}}'
        if not ch.leaks(body, secret):
            print(f"    secret-echo self-test: did NOT fire on encoding {name!r}")
            ok = False

    # Decode-funnel completeness (round-2 class C):
    # 1. ordinary JSON string escapes, including `\/` and `\"`, are decoded.
    json_slashed = '{"e":"' + secret.replace("/", "\\/").replace(".", "\\u002e") + '"}'
    if not ch.leaks(json_slashed, secret):
        print("    secret-echo self-test: did NOT reconstruct a secret behind JSON \\/ and \\u escapes")
        ok = False
    # 2. a base64 blob carrying the secret at an offset, after many longer runs,
    #    is still decoded (no sort-and-truncate dropping the short carrier).
    padding = " ".join("A" * 80 + str(i) for i in range(12))
    blob = _b64.b64encode(("x" + secret).encode()).decode()
    if not ch.leaks(padding + " " + blob, secret):
        print("    secret-echo self-test: did NOT decode an offset base64 blob behind longer runs")
        ok = False
    # 3. a nested encoding (base64 of the URL-escaped secret) is undone.
    nested = _b64.b64encode(ch.encodings(secret)["url"].encode()).decode()
    if not ch.leaks(nested, secret):
        print("    secret-echo self-test: did NOT undo a nested base64-of-urlescaped secret")
        ok = False

    # Silent on a correctly masked, credential-free error.
    masked = f'{{"error":"HTTP 403","body":"token ****{secret[-4:]} rejected"}}'
    if ch.leaks(masked, secret):
        print("    secret-echo self-test: fired on a correctly MASKED output (false positive)")
        ok = False

    # Silent on unrelated text.
    if ch.leaks('{"data":[{"id":1,"name":"device-a"}]}', secret):
        print("    secret-echo self-test: fired on credential-free data (false positive)")
        ok = False
    return ok


if __name__ == "__main__":
    import sys
    sys.exit(0 if self_test() else 1)
