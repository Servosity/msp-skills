#!/usr/bin/env python3
"""Boot a skill's MCP server and prove no in-process tool is default-denied.

Why this exists
---------------
A connector printed on cli-printing-press 4.30.1/4.30.2 shipped an
`internal/mcp/platform_gate.go` whose middleware answered

    MCP tenant gate is not configured

for every tool that is NOT a Cobra mirror. Cobra mirrors carry the
`pp:tenant-gate: "child-cli"` meta and passed straight through, so the damage
was invisible to every gate the repo had: it built, it vetted, `go test` was
green, the security gate was clean, and `tools/list` still advertised all the
tools. Only a `tools/call` over stdio showed 6 tools per connector answering an
error instead of running. See issue #249.

What this checks
----------------
Boot the MCP server, list its tools, and call the ones the tenant-gate
middleware actually wraps in-process (everything without the `child-cli` meta).
Fail if any of them comes back with a tenant-gate refusal.

Deliberately narrow, so it cannot be flaky:

* It needs NO credentials. The server boots and the in-process tools answer
  with an empty environment, so CI passes nothing in.
* It only calls tools annotated `readOnlyHint: true` and not
  `destructiveHint: true`. The gate is one middleware wrapping every tool
  alike, so the read-only subset proves the whole surface.
* Only a tenant-gate refusal fails. Any other error (bad placeholder argument,
  unreachable API, empty local store) passes: the point is that the handler
  RAN, not that it succeeded.

Usage:
    python3 tools/maintainer/check_mcp_gate.py --slug threatlocker
    python3 tools/maintainer/check_mcp_gate.py --all
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SKILLS = os.path.join(REPO, "skills")

# The refusal the 4.30.1/4.30.2 gate emitted, matched case-insensitively on a
# substring. This names the KNOWN wording; it is not the only net. A refusal
# phrased differently is still caught by is_protocol_error() whenever it
# surfaces as a JSON-RPC error object, which is the shape a Go-error return
# takes. Verified: a gate mutated to return errors.New("nope, denied for
# unrelated reasons") fails this check on all five probes.
GATE_REFUSAL = "tenant gate is not configured"

GATE_OWNER_KEY = "pp:tenant-gate"
CHILD_CLI = "child-cli"


def mcp_main_dir(slug: str) -> str | None:
    """Return the skill's MCP server main package, or None if it has no CLI."""
    cmd_dir = os.path.join(SKILLS, slug, "cli", "cmd")
    if not os.path.isdir(cmd_dir):
        return None
    for name in sorted(os.listdir(cmd_dir)):
        if name.endswith("-mcp") and os.path.isdir(os.path.join(cmd_dir, name)):
            return name
    return None


def placeholder_args(schema: dict) -> dict:
    """Fill only the schema's required properties, with inert placeholders."""
    props = schema.get("properties") or {}
    args: dict = {}
    for name in schema.get("required") or []:
        kind = (props.get(name) or {}).get("type", "string")
        if kind in ("number", "integer"):
            args[name] = 1
        elif kind == "boolean":
            args[name] = False
        elif kind == "array":
            args[name] = []
        elif kind == "object":
            args[name] = {}
        else:
            args[name] = "mcp-gate-probe"
    return args


def drive(binary: str, messages: list[dict], timeout: int, home: str) -> list[dict]:
    """Run one stdio session and return the JSON-RPC responses."""
    payload = "".join(json.dumps(m) + "\n" for m in messages)
    # Nothing but PATH and a throwaway HOME. The check must never depend on a
    # developer's ambient credentials, an accidental real key must not reach a
    # live API, and the probe must not touch anyone's real local store.
    env = {"PATH": os.environ.get("PATH", ""), "HOME": home}
    proc = subprocess.run(
        [binary], input=payload, capture_output=True, text=True, env=env, timeout=timeout
    )
    out = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


HANDSHAKE = [
    {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "check_mcp_gate", "version": "1"},
        },
    },
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
]


def result_text(message: dict) -> str:
    """Every string the server sent back for one tools/call, result OR error.

    A refusal can arrive two ways. The gate we are hunting returns
    `NewToolResultError(...)`, which is a normal `result` carrying an error
    content block - but a variant that returns a non-nil Go error instead
    surfaces as a JSON-RPC `error` OBJECT with no `result` at all. Reading only
    `result.content[].text` scores that second shape as a clean answer, because
    the response still has an `id` and so counts as answered. That is a
    false-GREEN in the one check whose entire job is to catch a default-deny:
    verified by mutating the gate to `return nil, errors.New("MCP tenant gate
    is not configured")`, which denied every tool while this script printed
    `ok` for all five probes and exited 0.
    """
    parts = []
    result = message.get("result") or {}
    for block in result.get("content") or []:
        if isinstance(block, dict) and "text" in block:
            parts.append(str(block["text"]))
    error = message.get("error")
    if isinstance(error, dict):
        for key in ("message", "data"):
            val = error.get(key)
            if val:
                parts.append(str(val))
    return "\n".join(parts)


def is_protocol_error(message: dict) -> bool:
    """True when the server answered with a JSON-RPC error object.

    A tool that cannot answer at all is a failure regardless of wording: it is
    either the gate refusing under a phrasing we did not predict, or the server
    breaking. Matching the known refusal string is the specific signal;
    this is the backstop that does not depend on guessing the words.
    """
    return isinstance(message.get("error"), dict)


def check_slug(slug: str, timeout: int, verbose: bool) -> tuple[str, list[str]]:
    """Return (status, failures). status is one of pass / skip / fail."""
    main_dir = mcp_main_dir(slug)
    if main_dir is None:
        return "skip", []

    cli_dir = os.path.join(SKILLS, slug, "cli")
    tmp = tempfile.mkdtemp(prefix="mcp-gate-")
    binary = os.path.join(tmp, main_dir)
    home = os.path.join(tmp, "home")
    os.makedirs(home, exist_ok=True)
    try:
        build = subprocess.run(
            ["go", "build", "-o", binary, "./cmd/" + main_dir],
            cwd=cli_dir,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        if build.returncode != 0:
            return "fail", [
                f"[{slug}] could not build ./cmd/{main_dir}: {build.stderr.strip()[:400]}"
            ]

        listed = drive(
            binary,
            HANDSHAKE + [{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}],
            timeout,
            home,
        )
        tools = []
        for message in listed:
            if message.get("id") == 2:
                tools = (message.get("result") or {}).get("tools") or []
        if not tools:
            return "fail", [f"[{slug}] MCP server listed no tools over stdio"]

        # The tenant-gate middleware wraps every tool except the Cobra mirrors,
        # which the child CLI gates itself. Probe the wrapped, read-only ones.
        probes = []
        wrapped = 0
        for tool in tools:
            meta = tool.get("_meta") or {}
            if meta.get(GATE_OWNER_KEY) == CHILD_CLI:
                continue
            wrapped += 1
            hints = tool.get("annotations") or {}
            if not hints.get("readOnlyHint") or hints.get("destructiveHint"):
                continue
            probes.append(tool)

        if wrapped == 0:
            # Every tool is a Cobra mirror, so the middleware gates nothing and
            # the #249 failure mode cannot occur here.
            return "skip", []
        if not probes:
            return "fail", [
                f"[{slug}] {wrapped} in-process MCP tool(s) exist but none is annotated "
                "read-only, so the tenant gate cannot be probed safely. Widen this "
                "check rather than leaving the surface unverified."
            ]

        calls = list(HANDSHAKE)
        ids = {}
        for offset, tool in enumerate(probes, start=10):
            calls.append(
                {
                    "jsonrpc": "2.0",
                    "id": offset,
                    "method": "tools/call",
                    "params": {
                        "name": tool["name"],
                        "arguments": placeholder_args(tool.get("inputSchema") or {}),
                    },
                }
            )
            ids[offset] = tool["name"]

        answered = {m.get("id"): m for m in drive(binary, calls, timeout, home) if "id" in m}
        failures = []
        for call_id, name in sorted(ids.items()):
            message = answered.get(call_id)
            if message is None:
                failures.append(f"[{slug}] MCP tool `{name}` never answered tools/call")
                continue
            text = result_text(message)
            if GATE_REFUSAL in text.lower():
                failures.append(
                    f"[{slug}] MCP tool `{name}` is DEFAULT-DENIED by the tenant gate: "
                    f"{' '.join(text.split())[:160]}"
                )
            elif is_protocol_error(message):
                failures.append(
                    f"[{slug}] MCP tool `{name}` answered with a JSON-RPC error instead of "
                    f"a result, so it never reached its handler: "
                    f"{' '.join(text.split())[:160]}"
                )
            elif verbose:
                print(f"    ok  {name}")

        if failures:
            failures.append(
                f"[{slug}] The gate must fall through when no platform source is "
                "registered. See issue #249 and docs/reprint-survival.md; the fix "
                "landed in cli-printing-press 4.30.3."
            )
            return "fail", failures
        if verbose:
            print(f"    {len(probes)} of {wrapped} in-process tool(s) probed")
        return "pass", []
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--slug", help="check one skill")
    parser.add_argument("--all", action="store_true", help="check every skill with a CLI")
    parser.add_argument("--timeout", type=int, default=300, help="per-step timeout in seconds")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    if not args.slug and not args.all:
        parser.error("pass --slug <slug> or --all")

    if args.slug:
        slugs = [args.slug]
    else:
        slugs = sorted(
            d for d in os.listdir(SKILLS)
            if not d.startswith("_") and os.path.isdir(os.path.join(SKILLS, d))
        )

    if shutil.which("go") is None:
        print("SKIP: go is not on PATH; cannot boot any MCP server.")
        return 0

    failures = []
    checked = skipped = 0
    for slug in slugs:
        if args.verbose:
            print(f"==> {slug}")
        status, slug_failures = check_slug(slug, args.timeout, args.verbose)
        if status == "skip":
            skipped += 1
            continue
        checked += 1
        failures.extend(slug_failures)

    if failures:
        print("FAIL: MCP tools are default-denied by the tenant gate:\n")
        for failure in failures:
            print(f"  - {failure}")
        return 1

    print(f"PASS: no default-denied MCP tool in {checked} skill(s) ({skipped} without one).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
