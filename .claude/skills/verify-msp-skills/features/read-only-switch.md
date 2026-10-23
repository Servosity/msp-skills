# Read-only switch

An operator who wants an agent to look but not change anything sets
`<PREFIX>_MCP_READ_ONLY=1` in the MCP server's environment. With it set,
`tools/list` must contain only tools annotated `readOnlyHint: true`; every
write tool is gone from the listing the agent sees.

## Sub-features

- `ro-drops-writes` lists no tool whose `readOnlyHint` is not `true`.
- `ro-keeps-reads` still lists the read-only tools.

## How to get to it (user POV)

- Add `"<PREFIX>_MCP_READ_ONLY": "1"` to the server's `env` block in the agent host's config.
- `<PREFIX>_MCP_READ_ONLY=1 <slug>-mcp` in a shell.

## Driving it with verify.py

Preconditions:

- `verify.py doctor --slug <slug>` names the variable and where the name came from (source, manifest or derived).

- **Default listing.** Run `verify.py run --slug <slug> --features 5`. One
  stdio session lists the tools with no switch: `f5-read-only/tools-default.json`.
- **Switched listing.** A second session with `<PREFIX>_MCP_READ_ONLY=1`:
  `tools-<PREFIX>_MCP_READ_ONLY.json`.
- **Judge.** Any listed tool without `readOnlyHint: true` is FAIL, named in
  the reason; an empty listing is FAIL; otherwise PASS with the count dropped
  and kept.
- **Proof.** PASS reads `<PREFIX>_MCP_READ_ONLY=1 drops <N> write tool(s) (...); <M> read-only tools remain`.

## Gotchas

- A tool with no annotation counts as a write, as the 4.34 server's own drop does.
- The value must be exactly `1`; `true` does not enable the switch.
- On a build without the switch the derived name is used, the listing does not
  change, and the FAIL names the variable and that it was derived.
- CI's `class_write_safety` checks that a call is refused under the switch;
  this feature checks what the agent is shown.
