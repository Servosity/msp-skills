# MCP stdio surface

An agent host (Claude Desktop, Copilot, Gemini CLI) launches `<slug>-mcp`
with no arguments and speaks JSON-RPC over stdin and stdout: it initializes,
lists the tools, and calls them. A read call must reach the vendor at the
configured base URL with the configured credential and come back as an answer.

## Sub-features

- `stdio-initialize` returns `serverInfo` (name, version).
- `stdio-tools-list` returns a non-empty tool list with annotations.
- `stdio-read-call` sends one read request to the vendor and answers without an error.

## How to get to it (user POV)

- Configure the host with `"command": "<slug>-mcp"` and the env block from
  `skills/<slug>/mcp-install.md`, then ask the agent something that needs the API.
- Endpoint connectors: the agent calls `<slug>_search`, then `<slug>_read`
  (press 4.34) or `<slug>_execute` (older builds) with an `endpoint_id`.
- Typed connectors: the agent calls a typed tool annotated `readOnlyHint: true`.

## Driving it with verify.py

Preconditions:

- Feature 1 passed, or the run placed the built binaries in the scratch HOME.
- `manifest.json` names a base-URL or host variable (otherwise ERROR: the fake vendor cannot stand in).

- **Initialize.** Run `verify.py run --slug <slug> --features 2`. The session
  sends `initialize` (protocol `2024-11-05`, the handshake from
  `check_mcp_gate.py`) then `notifications/initialized`.
  `f2-mcp-stdio/session.jsonl` shows the `serverInfo` answer.
- **List tools.** `tools/list` (following `nextCursor`). `tools.json` holds
  every tool with its annotations; the count is in the verdict.
- **Read call.** With `<slug>_search` present the harness searches `list`,
  then calls `<slug>_read` with the first endpoint (GET first); without
  `_read` it calls `<slug>_execute` with a read endpoint only (a GET, or an id
  ending `.list` or `.get`, which covers JSON-RPC APIs that read with POST).
  Otherwise it calls typed read-only tools, up to ten, until one reaches the
  vendor. Required params get inert placeholders. `read-attempts.json` pairs
  each call with the vendor requests it caused.
- **Proof.** PASS names the tool, the endpoint, the request line and the auth
  scheme the fake vendor saw (`auth=Bearer`, `auth=Basic`).

## Gotchas

- The fake vendor answers GET with `[]` and JSON-RPC with `"result": []`. A
  client that rejects that generic reply after the request left correctly is
  ERROR (the harness cannot judge the answer), not FAIL.
- `<slug>_get` describes an endpoint and calls nothing; it is never the read call.
- Local-store tools (`sql`, `context`, learnings, recall) answer without the
  vendor and do not count.
- A server that exits before answering is FAIL with its stderr; a server that
  hangs past 60 seconds is ERROR.
- CI's `check_mcp_gate.py` already proves in-process tools are not
  default-denied; this feature proves one call end to end through the configured base URL.
