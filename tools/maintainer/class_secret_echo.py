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

    # An authenticated error that an API returns, reflecting the credentials.
    server = ch.start_server(mode="echo", status=403, sentinels=secrets)
    env = ch.apply_urls(ctx.wiring, server.url)
    env["PATH"] = __import__("os").environ.get("PATH", "")
    env["HOME"] = ch.fresh_home(ctx, "secret-echo")
    for var in list(env):
        if var.endswith("_NO_CONFIG_WRITE"):
            env[var] = "1"
    reached_server = 0
    try:
        # (A) Error-path CLI output in the two agent-facing modes.
        read_leaves = [lf for lf in ch.leaves(ctx.surface) if ch.runnable_read(lf)][:MAX_LEAVES]
        for lf in read_leaves:
            for mode in ("--json", "--agent"):
                out = _run_cli(ctx, list(lf.path) + [mode, "--no-input"], env)
                for secret in secrets:
                    hits = ch.leaks(out, secret)
                    if hits:
                        findings.append(
                            f"[{ctx.slug}] `{' '.join(lf.path)} {mode}`: a credential value "
                            f"reached output, encoded as {sorted(set(hits))}")
                        break  # one finding per leaf is enough to flag the class

        # (B) MCP read-only tool error path.
        tools = ch.mcp_list_tools(ctx.mcp_bin, env)
        for t in tools:
            ann = t.get("annotations") or {}
            if not ann.get("readOnlyHint") or ann.get("destructiveHint"):
                continue
            schema = t.get("inputSchema") or {}
            if schema.get("required"):
                continue  # keep the probe argument-free
            msgs = ch._HANDSHAKE + [{
                "jsonrpc": "2.0", "id": 10, "method": "tools/call",
                "params": {"name": t["name"], "arguments": {}}}]
            answered = [m for m in ch.mcp_drive(ctx.mcp_bin, msgs, env) if m.get("id") == 10]
            if not answered:
                continue
            text = ch.mcp_result_text(answered[0])
            for secret in secrets:
                if ch.leaks(text, secret):
                    findings.append(
                        f"[{ctx.slug}] MCP tool `{t['name']}`: a credential value reached "
                        f"the tool result, encoded as {sorted(set(ch.leaks(text, secret)))}")
                    break
            break  # one read tool proves the funnel

        # (C) dry-run must mask the Authorization value it previews.
        mutate = [lf for lf in ch.leaves(ctx.surface)
                  if lf.method in ("POST", "PUT", "PATCH", "DELETE")
                  and "<" not in lf.use and not _required_flags(lf)]
        if mutate:
            lf = mutate[0]
            out = _run_cli(ctx, list(lf.path) + ["--dry-run", "--no-input"]
                           + _fill_flags(lf), env)
            for secret in secrets:
                if ch.leaks(out, secret):
                    findings.append(
                        f"[{ctx.slug}] `{' '.join(lf.path)} --dry-run`: the request preview "
                        f"shows a credential value instead of a mask")
                    break
        # A probe truly exercised the class only if it reached the fake server,
        # where the reflected credential lives. An MCP tool that answered from
        # local data, or a read that hit no endpoint, tested nothing.
        reached_server = len(server.capture.requests)
    finally:
        server.stop()
    if reached_server == 0 and not findings:
        # Honest coverage: no probe reached the client error path, so this
        # connector's redaction funnel was not tested. Report it; do not count
        # it as a clean pass.
        findings.append(
            f"NOTE: [{ctx.slug}] secret-echo not exercised - no argument-free GET read or "
            f"MCP read tool reached the client error path (reads are local/novel commands)")
    return findings


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

    # Fires on every encoding a reflected secret can take.
    enc = ch.encodings(secret)
    for name, form in enc.items():
        body = f'{{"error":"bad","reflected":"{form}"}}'
        if not ch.leaks(body, secret):
            print(f"    secret-echo self-test: did NOT fire on encoding {name!r}")
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
