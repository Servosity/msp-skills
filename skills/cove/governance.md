# cove skill - governance and safety model

> Unofficial. Community-built skill for the Cove Data Protection API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the cove skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cove-cli` binary (and `cove-mcp`),
authenticating with the credentials documented in mcp-install.md. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Cove Data Protection API.

## Default-safe behavior

- **This skill is read-first against your tenant.** Every named command that
  touches Cove reads: failure sweeps, stale lists, fleet rollups, billing,
  storage trends, and the enumerate commands. None of them change anything in
  backup.management, so they are always safe to run.
- **The local commands write only to your machine.** `sync` and `snapshot`
  populate a local SQLite mirror; `auth login`/`logout` manage a local session
  cache (the visa). They never mutate your Cove tenant.
- **`call` is the one escape hatch.** `cove-cli call <method>` can invoke any of
  the documented JSON-RPC methods, including the few that mutate (add/modify/
  remove on the management API). Treat it like a write: pass `--dry-run` to
  preview the request, show the exact command, get approval, then run it.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy for `call` still
  applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read (tenant)** | Reports, rollups, search against Cove. No change. | `devices failures`, `devices stale`, `fleet health`, `billing usage`, `billing changes`, `storage growth`, `devices changes`, and every `enumerate`/`list`/`get` command | Allow |
| **Local-only writes** | Side effects on your machine only, never the tenant. | `sync`, `snapshot` (local SQLite mirror); `auth login`/`logout` (local visa cache) | Allow |
| **Generic API call** | Escape hatch to any documented JSON-RPC method, a few of which mutate. | `call <method>` | Preview with `--dry-run`, then a human-approved run |
| **Destructive / admin** | Irreversible tenant change via a mutating `call`. | a mutating method invoked through `call` (e.g. an add/modify/remove method) | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the Cove account** to only what your workflow needs. A read/report
  workflow should use a backup.management user whose permissions cannot run the
  mutating management methods at all - then even a `call` to a write method fails
  at the API, not just at your policy.
- **Keep autonomous agents to Read + local-only writes.** Have a human approve any
  `call` to a method that is not a plain enumerate/read - the gate lives in your
  agent's policy, not in the binary's defaults.
- **Never let an agent run a mutating `call` method unattended.** Treat it like a
  production database change: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Cove Data Protection API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
