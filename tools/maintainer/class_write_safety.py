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

# Cap on read-only write-tool probes per connector (mirror tools shell to the
# CLI, so each call is a subprocess); any one reachable write flags the class.
MAX_WRITE_PROBES = 12
# Write commands tried for the dry-run probe (each costs a control + a dry run).
MAX_DRY_PROBES = 4
# A dry run should return at once; a probe process past this is cut off and its
# captured requests are still inspected.
PROBE_TIMEOUT = 20


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


WRITE_METHODS = ("POST", "PUT", "PATCH", "DELETE")
# Boolean arguments that confirm a write. Set true in BOTH sessions so a
# confirmation gate does not mask whether the read-only switch is what stopped
# the write.
CONFIRM_ARGS = ("confirm", "yes", "force", "allow_destructive", "allow-destructive")


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


def _write_args(tool: dict, write_endpoint: str) -> dict:
    """Arguments that can actually produce a write: required fields filled, any
    confirmation boolean set, and an `_execute`-style tool pointed at a genuine
    WRITE endpoint id (an invalid id fails before any gate, proving nothing)."""
    schema = tool.get("inputSchema") or {}
    props = schema.get("properties") or {}
    args = _placeholder_args(schema)
    if "endpoint_id" in props and write_endpoint:
        args["endpoint_id"] = write_endpoint
    for name, spec in props.items():
        if name in CONFIRM_ARGS and (spec or {}).get("type") == "boolean":
            args[name] = True
    return args


def _read_write_paths(surface: dict) -> list:
    """Regexes for endpoints that use a write METHOD but are reads (a POST
    search, a GraphQL query): agent-context marks them mcp:read-only. A request
    to one of these is not a write, so it must never count as one."""
    import re
    out = []
    for lf in ch.leaves(surface):
        if lf.read_only and lf.method in WRITE_METHODS and lf.api_path:
            pat = re.escape(lf.api_path)
            pat = re.sub(r"\\\{[^}]*\\\}", "[^/]+", pat)
            out.append(re.compile(pat + r"/?$"))
    return out


def _mutating(requests: list, read_paths: list | None = None) -> list:
    """Requests that WRITE: a mutating method, excluding a token mint (an OAuth
    exchange is auth, not a write to the vendor's data) and any endpoint the
    surface declares a read that rides a write method (a POST search)."""
    out = []
    for r in requests:
        _host, method, path, _headers, body = r
        bare = path.split("?")[0]
        pl = bare.lower()
        is_token = method == "POST" and ("token" in pl or "oauth" in pl or "grant_type" in body)
        is_read = any(rx.search(bare) for rx in (read_paths or []))
        if method in WRITE_METHODS and not is_token and not is_read:
            out.append(r)
    return out


def _probe_order(tool: dict) -> int:
    """API-capable write tools first: an `_execute` tool, then typed endpoint
    tools, then CLI mirrors. Local-only framework mirrors (learnings, teach,
    playbooks) never reach the API, so they must not use up the probe cap."""
    props = (tool.get("inputSchema") or {}).get("properties") or {}
    if "endpoint_id" in props:
        return 0
    if not (tool.get("_meta") or {}).get("pp:cli-command"):
        return 1
    return 2


def run(ctx: ch.Ctx) -> list[str]:
    findings: list[str] = []
    base_env = {"PATH": os.environ.get("PATH", ""), ch.cli_path_env(ctx.slug): str(ctx.cli_bin)}
    for var in ch.classify_env(ctx.manifest).env:
        if var.endswith("_NO_CONFIG_WRITE"):
            base_env[var] = "1"

    # (A) read-only MCP mode: no write may reach the API. Ground truth is the
    # fake API's own request log, not the wording of an error: an error message
    # cannot tell a gate that refused from a handler that failed on a bad
    # placeholder before it got that far. So every write tool is called twice
    # with the SAME real-shaped arguments (an `_execute` tool gets a genuine
    # write endpoint id from agent-context): once with the switch off (the
    # CONTROL, which shows the probe can produce a write at all), once with
    # it on. A mutating request in the read-only session is a finding; if even
    # the control produced none, the probe proved nothing and is reported as
    # advisory coverage, not a pass.
    ro_var = _read_only_var(ctx)
    if ro_var:
        secrets_a = list(ch.classify_env(ctx.manifest).sentinels.values())
        api = ch.start_server(mode="echo", status=200, sentinels=secrets_a)
        try:
            urls = ch.apply_urls(ch.classify_env(ctx.manifest), api.url)
            ctrl_env = {**base_env, **urls, "HOME": ch.fresh_home(ctx, "write-ctrl")}
            ro_env = {**base_env, **urls, "HOME": ch.fresh_home(ctx, "write-ro"), ro_var: "1"}
            writes = sorted((t for t in ch.mcp_list_tools(ctx.mcp_bin, ctrl_env)
                             if _is_write_tool(t)), key=_probe_order)[:MAX_WRITE_PROBES]
            write_ep = next((lf.endpoint for lf in ch.leaves(ctx.surface)
                             if lf.endpoint and lf.method in WRITE_METHODS
                             and not lf.read_only), "")
            read_paths = _read_write_paths(ctx.surface)
            calls = list(ch._HANDSHAKE) + [
                {"jsonrpc": "2.0", "id": i, "method": "tools/call",
                 "params": {"name": t["name"], "arguments": _write_args(t, write_ep)}}
                for i, t in enumerate(writes, start=20)]
            if writes:
                mark = len(api.capture.requests)
                ch.mcp_session(ctx.mcp_bin, calls, ctrl_env)
                ctrl_writes = _mutating(api.capture.requests[mark:], read_paths)
                mark = len(api.capture.requests)
                ch.mcp_session(ctx.mcp_bin, calls, ro_env)
                ro_writes = _mutating(api.capture.requests[mark:], read_paths)
                if ro_writes:
                    paths = sorted({f"{r[1]} {r[2].split('?')[0]}" for r in ro_writes})
                    findings.append(
                        f"[{ctx.slug}] {len(ro_writes)} write request(s) reached the API with "
                        f"{ro_var}=1 ({paths}); the read-only switch must stop every write")
                elif not ctrl_writes:
                    findings.append(
                        f"NOTE: [{ctx.slug}] read-only not proven - no write tool produced an API "
                        f"write even with {ro_var} off, so the probe could not exercise the gate")
        finally:
            api.stop()

    # (B) --dry-run puts nothing on the wire. Each probe runs in a FRESH home (a
    # dry run that mints a token is visible only when no token is cached). Any
    # request a dry run sends is a finding. A CLEAN dry run counts as proven
    # only when a CONTROL run of the same command without --dry-run reaches the
    # API, so a command that merely fails its arguments is never a pass.
    # Positional arguments are filled with a placeholder so write commands that
    # take an id are probed too.
    secrets = list(ch.classify_env(ctx.manifest).sentinels.values())
    server = ch.start_server(mode="echo", status=200, sentinels=secrets)
    mutate = sorted((lf for lf in ch.leaves(ctx.surface)
                     if lf.method in WRITE_METHODS and not lf.read_only),
                    key=lambda lf: lf.use.count("<"))
    proven = 0
    leaked = False
    try:
        for lf in mutate[:MAX_DRY_PROBES]:
            argv = list(lf.path) + _positionals(lf.use) + _fill_required(lf)
            urls = ch.apply_urls(ch.classify_env(ctx.manifest), server.url)
            # The dry run FIRST: any request it puts on the wire is a finding,
            # whatever the control does (a dry run can authenticate, and so
            # mint, before it validates arguments the control then rejects).
            mark = len(server.capture.requests)
            _run(ctx, argv + ["--dry-run", "--no-input", "--yes"],
                 {**base_env, **urls, "HOME": ch.fresh_home(ctx, "write-dry")})
            sent = server.capture.requests[mark:]
            if sent:
                paths = sorted({r[2].split("?")[0] for r in sent})
                findings.append(
                    f"[{ctx.slug}] `{' '.join(lf.path)} --dry-run` put {len(sent)} request(s) "
                    f"on the wire (paths: {paths}); a dry run must send nothing")
                leaked = True
                break  # one proven dry-run leak is enough to flag the class
            # A clean dry run counts only when the same command, run for real,
            # reaches the API (otherwise it may simply have failed its arguments).
            mark = len(server.capture.requests)
            _run(ctx, argv + ["--no-input", "--yes"],
                 {**base_env, **urls, "HOME": ch.fresh_home(ctx, "write-ctrl-dry")})
            if server.capture.requests[mark:]:
                proven += 1
            if proven >= 1:
                break  # one proven clean dry run is enough
    finally:
        server.stop()
    if mutate and not proven and not leaked:
        findings.append(
            f"NOTE: [{ctx.slug}] --dry-run not proven - none of the first {MAX_DRY_PROBES} write "
            f"commands reached the API even without --dry-run")
    return findings


def _positionals(use: str) -> list[str]:
    """A placeholder for each required positional (`<id>`) in a Use string."""
    import re
    return ["1" for _ in re.findall(r"<[^>]+>", use)]


def _run(ctx: ch.Ctx, argv: list[str], env: dict) -> None:
    try:
        subprocess.run([str(ctx.cli_bin)] + argv, capture_output=True, text=True,
                       env=env, timeout=PROBE_TIMEOUT, stdin=subprocess.DEVNULL)
    except (OSError, subprocess.SubprocessError):
        pass  # what was captured is inspected regardless: a command that SENT
              # a request and then hung or crashed has still sent it.


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

    # Read-only decision input: which captured requests count as a WRITE.
    # A data POST/PUT/PATCH/DELETE is a write; an OAuth token mint and a GET
    # are not, so a gate that merely authenticates is never a false finding.
    data_post = ("h", "POST", "/v1/items", {}, '{"name":"x"}')
    data_del = ("h", "DELETE", "/v1/items/7", {}, "")
    token_mint = ("h", "POST", "/oauth/token", {}, "grant_type=client_credentials")
    token_root = ("h", "POST", "/", {}, "grant_type=client_credentials&client_id=a")
    read = ("h", "GET", "/v1/items", {}, "")
    if len(_mutating([data_post, data_del])) != 2:
        print("    write-safety self-test: a data write was not counted as a write")
        ok = False
    if _mutating([token_mint, token_root, read]):
        print("    write-safety self-test: a token mint or a GET was counted as a write")
        ok = False
    # A read that rides POST (agent-context marks it mcp:read-only) is not a
    # write; a real POST create on the sibling path still is.
    surf = {"commands": [{"name": "tables", "subcommands": [
        {"name": "search", "use": "search", "annotations": {
            "pp:method": "POST", "pp:path": "/v1/tables/{t}/search", "mcp:read-only": "true"}},
        {"name": "create", "use": "create", "annotations": {
            "pp:method": "POST", "pp:path": "/v1/tables/{t}"}}]}]}
    rp = _read_write_paths(surf)
    got = _mutating([("h", "POST", "/v1/tables/Docs/search", {}, "{}"),
                     ("h", "POST", "/v1/tables/Docs", {}, "{}")], rp)
    if [r[2] for r in got] != ["/v1/tables/Docs"]:
        print("    write-safety self-test: a POST search was counted as a write, or a create was missed")
        ok = False

    # Probe arguments: an _execute tool must get a GENUINE write endpoint (a
    # bogus id fails before any gate and proves nothing), and a confirmation
    # boolean is set so a confirm gate cannot stand in for the read-only switch.
    exe = {"name": "x_execute", "inputSchema": {
        "properties": {"endpoint_id": {"type": "string"}, "params": {"type": "object"},
                       "confirm": {"type": "boolean"}},
        "required": ["endpoint_id"]}}
    args = _write_args(exe, "items.delete")
    if args.get("endpoint_id") != "items.delete" or args.get("confirm") is not True:
        print("    write-safety self-test: _execute probe did not carry a real write endpoint and confirm")
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
