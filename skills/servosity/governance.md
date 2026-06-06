# Servosity skill - governance and safety model

> Published by Servosity Inc. for MSP partners. This page tells an MSP owner
> exactly what the Servosity skill can touch, how it is scoped, and which
> operations are dangerous, so you can decide what to allow your AI agent to do.

## What it authenticates as

The skill drives the `servosity-cli` binary (and the `servosity-mcp`
server), authenticating with a single `SERVOSITY_MSP_TOKEN` you generate in the
Servosity partner portal. Every call is scoped to **your reseller account** and
the companies under it. There is no cross-reseller access: the skill cannot see
or touch another partner's tenants. Cross-reseller listing, billing back-office,
and internal support tooling are not part of the public surface.

The token is read from the environment (or the MCP `env` block), or saved to
the CLI's local config file if you opt in via `servosity-cli auth set-token`.
It is never logged and never sent anywhere except the Servosity API.

## Default-safe behavior

Know exactly how writes are gated - the binary does NOT gate them for you:

- **`--dry-run` is opt-in - use it.** Write commands (`triage` mutations,
  `import`, raw create/update/delete under the resource groups) send
  immediately unless you pass `--dry-run` first to preview the request without
  sending. Make your agent's policy: preview, show the exact command, get
  approval, then run the write.
- **Prompts are skippable.** Where a command prompts interactively, `--yes`
  (and `--agent`, which implies `--yes`) skips the prompt. The gate must live
  in your agent's policy, not in the binary's defaults.
- **Read commands are always safe to run.** `attention`, `drift`,
  `stale-backups`, `backup-facts`, `search`, `qbr`, `fleet-health`,
  `unprovisioned`, `bill`, and `restore-queue watch` only read; `email-draft`
  writes nothing and sends nothing - it prints email bodies.
- **Agent mode is explicit.** `--agent` produces JSON for scripting AND implies
  `--yes`. It adds no write gating. See [AGENTS.md](./AGENTS.md).

## Permission tiers

Classify what you let an agent do by tier. The safe default for an autonomous
agent is **read-only plus planned (dry-run) writes**; require a human for
anything below the line.

| Tier | What it does | Example commands | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `attention`, `drift`, `stale-backups`, `backup-facts`, `qbr`, `qbr-all`, `fleet-health`, `unprovisioned`, `bill`, `email-draft`, `restore-queue watch`, `search`, `sync` | Allow |
| **Write (routine)** | Day-to-day mutations on issues and records. | `triage` (`--ignore`/`--archive`/`--reactivate`/`--comment`), `import`, raw create/update subcommands under the resource groups | Preview with `--dry-run`, then an approved write - never blanket `--yes` |
| **Credential / security** | Touches tokens, keys, MFA. Can change who has access. | `credentials rotate`/`delete`, `current-user api-token-delete`, `current-user verified-mfa-delete`, `current-user mfa-backup-codes-update`, `resellers agent-install-token`, `resellers postmark-rotate`, `*-backups encryption-key update`, `reissue-spx-key`, `*-backups agent-token` | Human-in-the-loop only. Do not allow an autonomous agent. |
| **Destructive** | Irreversible data or config loss. | `companies delete`, `companies c2c companies-delete`, `backups delete`, `backup-sets delete`, `dr-backups delete`, `restic-backups delete`, `backups unlock`, `users delete` | Human-in-the-loop only, with an explicit out-of-band confirmation step. |

## How to lock it down

- **Scope the token.** Use a partner token with only the access your workflow
  needs. If you only want read/report workflows, do not hand the agent a token
  that can delete companies.
- **Keep autonomous agents to Read + previewed writes.** Let the agent preview
  with `--dry-run`, and have a human approve the actual write for anything in
  the Write tier and above - the binary's prompts are skippable, so the gate
  must live in your agent's policy.
- **Never let an agent run Credential or Destructive tier commands
  unattended.** These are the multi-tenant-credential-cascade and data-loss
  risks the MSP industry worries about. Treat them like production database
  drops: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP
  server to a public HTTPS endpoint for ChatGPT - see
  [mcp-install.md](./mcp-install.md)).

## Why an MSP owner can be comfortable

The skill is read-first and single-reseller-scoped, and the full
source of both the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). Nothing about the credential path is hidden: you
supply the token, the binary uses it against the Servosity API, and you can read
every line of how it does so.
