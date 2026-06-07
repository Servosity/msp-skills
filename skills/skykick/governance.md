# skykick skill - governance and safety model

> Unofficial. Community-built skill for the SkyKick API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the skykick skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `skykick-cli` binary (and `skykick-mcp`), authenticating with
SkyKick Partner API client credentials: `SKYKICK_CLIENT_ID` (your API user ID) and
`SKYKICK_CLIENT_SECRET` (your partner subscription key) are the two secrets.
`SKYKICK_OAUTH_SCOPE` is an optional, non-secret scope override that defaults to
`Partner` (set it to `Distributor` for distributor accounts). Credentials are read
from the environment only - never written to disk, never logged, never sent
anywhere except the SkyKick (ConnectWise Cloud Services) API. The CLI reads only
what those partner credentials are already permitted to see.

## Default-safe behavior

- **`--dry-run` is opt-in for most writes - use it.** `alerts complete`,
  `backup discover-mailboxes`, and `backup discover-sites` POST immediately; pass
  `--dry-run` first to preview the request without sending. `import` has a
  `--dry-run` preview flag, and `alert-sweep` is **dry-run by default** - it only
  prints what it would complete unless you add `--apply`. Make your agent's policy:
  preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (posture tables, rollups, audits,
  search); they cannot change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Posture tables, rollups, audits, search. No change. | `fleet-health`, `stale-snapshots`, `coverage-gaps`, `retention-audit`, `autodiscover-audit`, `drift`, `partner-rollup`, `alert-sweep` (without `--apply`), `backup list` / `mailboxes` / `sites` / `storage-settings` / `subscription-settings`, `alerts list`, `identity` | Allow |
| **Write (routine)** | POSTs to the live API: marks alerts complete, triggers discovery, bulk-imports. | `alerts complete <id>`, `alert-sweep --complete <ids> --apply`, `backup discover-mailboxes <id>`, `backup discover-sites <id>`, `import <resource>` | Preview with `--dry-run` (or omit `--apply` on `alert-sweep`), then an approved write |
| **Credential / security** | Touches tokens, keys, MFA. | None - no credential-rotation commands in the wrapped API | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | None - no delete/purge/wipe commands in the wrapped API | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | None | Operator-only, not for agents |

Note: `backup storage-settings` and `backup subscription-settings` are **read-only
GET** commands that show a tenant's settings - they do not change anything.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the SkyKick API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
