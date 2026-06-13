# veeam skill - governance and safety model

> Unofficial. Community-built skill for the Veeam Service Provider Console API.
> Not affiliated with, endorsed by, or sponsored by Veeam Software.
> This page tells an MSP owner exactly what the veeam skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `veeam-cli` binary (and `veeam-mcp`), authenticating with
`VEEAM_BASE_URL` (your VSPC appliance REST API base URL, per-instance) and
`VEEAM_TOKEN` (a bearer token from `POST /token`). Credentials are read from the
environment only - never written to disk by the skill, never logged, never sent
anywhere except your own VSPC appliance.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (rollups, triage, inventory, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

VSPC exposes a very large surface (~1000 commands). The safe default for an
autonomous agent is **read only**; require a human for any write, and treat the
infrastructure, destructive, and credential tiers as production-change-window work.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** (~610 cmds) | Rollups, triage, inventory, search. No change. | `fleet-health`, `stale-backups`, `at-risk`, `alarms-triage`, `company-overview`, `license-usage`, `since`, and every `get` / `list` / `search` | Allow |
| **Write (config)** | Creates or modifies jobs, policies, rules, mappings. | `configuration create-windows-backup-policy` / `patch-backup-policy`, `infrastructure create-backup-server-backup-vm-vsphere-job` / `assign-backup-server-job`, `discovery create-windows-custom-rule`, `alarms acknowledge-active` / `resolve-active` | Preview with `--dry-run`, then an approved write |
| **Infrastructure and agent execution** | Installs or runs software on real customer machines and infrastructure. | `discovery install-backup-agent-on-computer` / `reboot-computer` / `start-rule`, `infrastructure activate-backup-agent` / `force-collect-backup-server`, `deployment wait-task` | Human-in-the-loop, explicit confirmation |
| **Destructive** | Irreversible data, job, or tenant loss. | `infrastructure delete-tenant` / `delete-backup-agent` / `delete-vb365-server` / `delete-backup-server-job`, `configuration delete-backup-policy`, `discovery delete-rule`, `alarms delete-active` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Mints, stores, decrypts, or reads tokens, keys, passwords, certificates. | `authentication generate-totp-secret` / `generate-new-pkcs12-key-pair` / `decrypt-pkcs12-container`, `infrastructure create-backup-server-encryption-password` / `create-backup-server-standard-credentials`, the secret-returning reads `infrastructure get-backup-server-credentials-by-server` / `get-backup-server-encryption-passwords-by-server`, `users tokens revoke-authentication` | Human-in-the-loop only |
| **Admin (multi-tenant)** | Back-office tenant, organization, user, and permission management. | `infrastructure create-tenant` / `enable-tenant`, `organizations` management, `users` create/delete, `permissions`, `subscription-plans` | Operator-only, not for agents |

> Note: a handful of `get-*-credentials` / `get-*-encryption-passwords` commands
> return secret material. They are read-only to the API but are listed under
> **Credential / security**, not Read - never let an agent run them unattended.

## How to lock it down

- **Scope the token** to only what your workflow needs. A monitoring/report workflow
  does not need a VSPC token that can delete tenants, install agents, or read stored
  credentials.
- **Keep autonomous agents to the Read tier.** Have a human approve any Write,
  Infrastructure, Destructive, Credential, or Admin command - the gate lives in your
  agent's policy, not in the binary's defaults.
- **Treat infrastructure, destructive, and admin commands like a production change
  window.** They install software, reboot machines, delete jobs, and remove tenants
  across live customer estates: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token and base URL, the binary uses
them against your own VSPC appliance, and you can read every line of how it does so.
The skill is read-first, plan-by-default, and scoped to your own console.
