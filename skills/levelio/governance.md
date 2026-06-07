# levelio skill - governance and safety model

> Unofficial. Community-built skill for the Level API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the levelio skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `levelio-cli` binary (and `levelio-mcp`),
authenticating with `LEVEL_API_TOKEN` (sent as `Authorization: Bearer <token>`).
The credential is read from the environment only - never written to disk by the
skill, never logged, never sent anywhere except the Level API. Level lets you mint
a **read-only** key, which is all every report, rollup, sync, and search command
below needs.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything. `sync` only writes to the *local* SQLite mirror, never to Level.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `at-risk`, `patch-posture`, `fleet`, `alert-triage`, `stale`, `client-scorecard`, `security-posture`, `group-tree`, `since`, `reboot-due`, `tag-audit`, `cf-coverage`, every `list`/`show`, `search`, `analytics`, `sync` | Allow |
| **Write (routine)** | Day-to-day mutations of Level records. | `devices update`, `alerts resolve alert`, `groups create`, `groups update`, `groups devices assign-group`, `groups devices remove-group`, `tags create`, `tags update`, `tags devices tag`, `tags devices remove-tag-from`, `custom-fields create`, `custom-fields update`, `custom-field-values update`, `import` | Preview with `--dry-run`, then an approved write |
| **Device actions** | Triggers automations that run on real endpoints (scripts, installs, reboots). | `automations` | Human-in-the-loop only - never unattended |
| **Credential / security** | Manages the locally stored API token. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss in Level. | `devices delete`, `groups delete`, `tags delete`, `custom-fields delete`, `custom-field-values delete` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

> Note on the rollups: `at-risk`, `patch-posture`, `alert-triage`, and the other
> cross-entity views are **read-only** - they aggregate synced data and change
> nothing, despite their action-sounding names.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  should use a **read-only** Level API key - it cannot run any Write, Device-action,
  or Destructive command even if asked.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Treat `automations` like running a script on every targeted endpoint** - because
  that is what it does. Never let an agent fire it unattended.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Level API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
