# afi skill - governance and safety model

> Unofficial. Community-built skill for the Afi API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the afi skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `afi-cli` binary (and `afi-mcp`),
authenticating with `AFI_API_KEY`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Afi API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, and the local fleet walk. No remote change. | `coverage-gaps`, `backup-stale`, `fleet-health`, `tenant-scorecard`, `reconcile-licenses`, `resolve`, `fleet-sync`, and every `list`/`get` (e.g. `tenants archives list`, `tenants protections list`) | Allow |
| **Write (routine)** | Day-to-day mutations against the Afi API. | `orgs create`, `import`, `tenants resources protections-protect`, `tenants jobs tasks-trigger` | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches stored tokens and credentials. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Releases backup coverage or deletes an archive - irreversible. | `offboard` (final-backup-then-release a resource), `tenants resources protections-unprotect`, `tenants archives delete` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

> **`offboard` is guarded but destructive.** It triggers a final backup, waits,
> verifies a fresh archive exists, and only then removes the protection so the
> Microsoft 365 / Google Workspace seat can be released. It refuses the
> irreversible unprotect step until the backup is verified (`--no-wait` /
> `--skip-backup` override that gate - do not let an agent pass them). Treat the
> whole command as human-in-the-loop.

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
the Afi API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
