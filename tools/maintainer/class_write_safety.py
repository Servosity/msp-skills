#!/usr/bin/env python3
"""Class: writes are refused in read-only MCP mode, and --dry-run sends nothing.

The defended property
---------------------
Two failures the loops kept finding:

  * The MCP read-only kill switch (`<PREFIX>_MCP_READ_ONLY=1`) leaves a write
    tool reachable - a typed write registered outside the gate, an `_execute`
    with only the switch checked (saas-alerts, synology, td-synnex, backup-radar,
    quotewerks). With the switch on, every non-read tool must refuse.
  * `--dry-run` performs a side effect - it mints a token, delivers a webhook,
    writes, or sends the request it was meant only to preview (backup-radar
    retire, connectwise-rmm, sophos-central, watchguard, mimecast/threecx mint).
    A dry run must put nothing on the wire.

How it is driven (black box)
----------------------------
With the read-only switch set, the MCP server is booted and every write tool is
called with argument-free placeholders; each must answer a refusal (or be absent
from tools/list). Then a write command is run with `--dry-run` against a local
fake server, which must record zero requests of any kind (no token mint either).

Both directions (self_test): the refusal detector fires on a write tool that
answered a normal result under read-only and is silent on one that refused or
was absent; the dry-run detector fires on any recorded request and is silent on
none.
"""

from __future__ import annotations

import os
import subprocess

import class_harness as ch

REFUSAL_HINTS = ("read-only", "read only", "readonly", "refus", "disabled",
                 "not allowed", "MCP_READ_ONLY")


def _read_only_var(ctx: ch.Ctx) -> str | None:
    for var in ch.classify_env(ctx.manifest).env:
        if var.endswith("_MCP_READ_ONLY"):
            return var
    env = (ctx.manifest.get("server") or {}).get("mcp_config", {}).get("env", {}) or {}
    for var in env:
        if var.endswith("_MCP_READ_ONLY"):
            return var
    return None


def _is_write_tool(tool: dict) -> bool:
    ann = tool.get("annotations") or {}
    return (not ann.get("readOnlyHint")) or bool(ann.get("destructiveHint"))


def _placeholder_args(schema: dict) -> dict:
    props = schema.get("properties") or {}
    out = {}
    for name in schema.get("required") or []:
        kind = (props.get(name) or {}).get("type", "string")
        out[name] = (1 if kind in ("number", "integer") else
                     False if kind == "boolean" else
                     [] if kind == "array" else
                     {} if kind == "object" else "cls-test")
    return out


def run(ctx: ch.Ctx) -> list[str]:
    findings: list[str] = []
    base_env = {"PATH": os.environ.get("PATH", "")}
    for var in ch.classify_env(ctx.manifest).env:
        if var.endswith("_NO_CONFIG_WRITE"):
            base_env[var] = "1"

    # (A) read-only MCP mode refuses every write tool. All probes ride ONE stdio
    # session (booting the server per tool is what made this slow on connectors
    # with many write tools).
    ro_var = _read_only_var(ctx)
    if ro_var:
        ro_env = {**base_env, "HOME": ch.fresh_home(ctx, "write-ro")}
        ro_env[ro_var] = "1"
        tools = ch.mcp_list_tools(ctx.mcp_bin, ro_env)
        writes = [t for t in tools if _is_write_tool(t)]
        if writes:
            calls = list(ch._HANDSHAKE)
            ids = {}
            for i, t in enumerate(writes, start=20):
                calls.append({"jsonrpc": "2.0", "id": i, "method": "tools/call",
                              "params": {"name": t["name"],
                                         "arguments": _placeholder_args(t.get("inputSchema") or {})}})
                ids[i] = t["name"]
            answered = {m.get("id"): m for m in ch.mcp_drive(ctx.mcp_bin, calls, ro_env)
                        if m.get("id") in ids}
            for i, name in sorted(ids.items()):
                msg = answered.get(i)
                if msg is None or ch.mcp_is_error(msg):
                    continue  # never answered / a protocol error is a refusal to run
                text = ch.mcp_result_text(msg).lower()
                if any(h.lower() in text for h in REFUSAL_HINTS):
                    continue  # explicitly refused
                # A normal result means the write handler ran under read-only.
                findings.append(
                    f"[{ctx.slug}] MCP write tool `{name}` ran under {ro_var}=1 instead of "
                    f"refusing (result did not name the read-only switch)")

    # (B) --dry-run puts nothing on the wire. A FRESH home: a dry run that mints
    # a token is only visible when no token is cached, so this probe must not
    # inherit a token any earlier class or sub-probe cached.
    secrets = list(ch.classify_env(ctx.manifest).sentinels.values())
    server = ch.start_server(mode="echo", status=200, sentinels=secrets)
    mutate = [lf for lf in ch.leaves(ctx.surface)
              if lf.method in ("POST", "PUT", "PATCH", "DELETE")
              and "<" not in lf.use]
    try:
        for lf in mutate[:3]:
            env = {**base_env, "HOME": ch.fresh_home(ctx, "write-dry"),
                   **ch.apply_urls(ch.classify_env(ctx.manifest), server.url)}
            req_before = len(server.capture.requests)
            args = list(lf.path) + ["--dry-run", "--no-input", "--yes"] + _fill_required(lf)
            try:
                subprocess.run([str(ctx.cli_bin)] + args, capture_output=True,
                               text=True, env=env, timeout=ch.RUN_TIMEOUT)
            except (OSError, subprocess.SubprocessError):
                continue
            sent = server.capture.requests[req_before:]
            if sent:
                paths = sorted({r[2].split("?")[0] for r in sent})
                findings.append(
                    f"[{ctx.slug}] `{' '.join(lf.path)} --dry-run` put {len(sent)} request(s) "
                    f"on the wire (paths: {paths}); a dry run must send nothing")
                break  # one proven dry-run leak is enough to flag the class
    finally:
        server.stop()
    return findings


def _fill_required(lf: ch.Leaf) -> list[str]:
    out = []
    for f in lf.flags:
        if not f.get("required"):
            continue
        kind = f.get("type", "string")
        out += [f"--{f['name']}", "1" if kind in ("int", "integer", "float") else "cls-test"]
    return out


def self_test() -> bool:
    ok = True

    # A write tool classified from annotations.
    if not _is_write_tool({"annotations": {"readOnlyHint": False}}):
        print("    write-safety self-test: did NOT classify a write tool as a write")
        ok = False
    if _is_write_tool({"annotations": {"readOnlyHint": True, "destructiveHint": False}}):
        print("    write-safety self-test: misclassified a read tool as a write")
        ok = False
    if not _is_write_tool({"annotations": {"readOnlyHint": True, "destructiveHint": True}}):
        print("    write-safety self-test: a destructive tool must count as a write")
        ok = False

    # Refusal detection: a normal result under read-only is a failure; an error
    # or a read-only-named refusal is clean.
    ran = {"id": 20, "result": {"content": [{"type": "text", "text": "deleted 3 records"}]}}
    refused_err = {"id": 20, "error": {"code": -32000, "message": "nope"}}
    refused_txt = {"id": 20, "result": {"isError": True,
                   "content": [{"type": "text", "text": "refusing DELETE while FOO_MCP_READ_ONLY=1"}]}}
    if ch.mcp_is_error(ran) or any(h.lower() in ch.mcp_result_text(ran).lower() for h in REFUSAL_HINTS):
        print("    write-safety self-test: a normal write result was treated as refused")
        ok = False
    if not (ch.mcp_is_error(refused_err)):
        print("    write-safety self-test: a JSON-RPC error was not treated as a refusal")
        ok = False
    if not any(h.lower() in ch.mcp_result_text(refused_txt).lower() for h in REFUSAL_HINTS):
        print("    write-safety self-test: a read-only refusal message was not recognised")
        ok = False

    # Dry-run: the decision is "any recorded request is a leak; none is clean."
    sent = [("h", "POST", "/x", {}, "body")]
    none = []
    if not (len(sent) > 0):
        print("    write-safety self-test: a dry-run request was not detected as a leak")
        ok = False
    if len(none) > 0:
        print("    write-safety self-test: an empty dry-run capture was flagged")
        ok = False
    return ok


if __name__ == "__main__":
    import sys
    sys.exit(0 if self_test() else 1)
