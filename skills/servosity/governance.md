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

The token is read only from the environment (or the MCP `env` block). It is
never written to disk by the skill, never logged, and never sent anywhere except
the Servosity API.

## Default-safe behavior

Two classes of write commands, with different gating - know which one you are
running:

- **Compound write commands plan by default.** `triage`, `clear`, and
  `stale-issues` run in PLAN mode unless you pass `--confirm`; the global
  `--dry-run` flag is an extra safety net that keeps PLAN mode even if
  `--confirm` is passed. A plan prints what *would* change and exits without
  mutating.
- **Raw CRUD writes prompt, and the prompt is skippable.** Commands like
  `*-notes create/update` and `issue-comments update/delete` use an interactive
  confirmation prompt by default - but `--yes` (and `--agent`, which implies
  `--yes`) SKIPS that prompt, and `--dry-run` is opt-in. Gate these at your
  agent's policy level: preview with `--dry-run`, show the exact command, get
  approval, then run it.
- **Read commands are always safe to run.** `attention`, `drift`,
  `stale-backups`, `backup-facts`, `find`, `search`, `company show`, and
  `restore-queue list` only read; they cannot modify anything.
- **Agent mode skips prompts.** `--agent` produces JSON for scripting AND
  implies `--yes`, which bypasses the interactive confirmations on raw CRUD
  writes. The plan-by-default gate on `triage`/`clear`/`stale-issues` still
  holds; for everything else the gate must live in your agent's policy. See
  [AGENTS.md](./AGENTS.md).

## Permission tiers

Classify what you let an agent do by tier. The safe default for an autonomous
agent is **read-only plus planned (dry-run) writes**; require a human for
anything below the line.

| Tier | What it does | Example commands | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `attention`, `drift`, `stale-backups`, `backup-facts`, `find`, `company show`, `restore-queue list`, `sync` | Allow |
| **Write (routine)** | Day-to-day mutations on issues and notes. | `triage` (ignore/archive/reactivate/comment), `clear`, `stale-issues`, `*-notes create/update`, `issue-comments` | Compound commands: review the PLAN, then `--confirm`. Raw CRUD: preview with `--dry-run`, approve, never blanket `--yes` |
| **Credential / security** | Touches tokens, keys, MFA. Can change who has access. | `credentials rotate`/`delete`, `current-user api-token-delete`, `current-user verified-mfa-delete`, `current-user mfa-backup-codes-update`, `resellers agent-install-token`, `resellers postmark-rotate`, `*-backups encryption-key update`, `reissue-spx-key`, `*-backups agent-token` | Human-in-the-loop only. Do not allow an autonomous agent. |
| **Destructive** | Irreversible data or config loss. | `companies delete`, `companies c2c companies-delete`, `backups delete`, `backup-sets delete`, `dr-backups delete`, `restic-backups delete`, `restic-backups restic-prune`, `backups unlock`, `users delete`, `resellers delete` | Human-in-the-loop only, with an explicit out-of-band confirmation step. |
| **Admin (hidden)** | Back-office worker/token administration. | `admin ...` (hidden from normal help) | Operator-only. Not for agent use. |

## How to lock it down

- **Scope the token.** Use a partner token with only the access your workflow
  needs. If you only want read/report workflows, do not hand the agent a token
  that can delete companies.
- **Keep autonomous agents to Read + previewed writes.** Let the agent produce
  plans (`--dry-run` / PLAN mode), and have a human approve the actual write for
  anything in the Write tier and above - on raw CRUD commands the binary's
  prompt is skippable, so the gate must live in your agent's policy.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** These are the multi-tenant-credential-cascade and data-loss
  risks the MSP industry worries about. Treat them like production database
  drops: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP
  server to a public HTTPS endpoint for ChatGPT - see
  [mcp-install.md](./mcp-install.md)).

## Why an MSP owner can be comfortable

The skill is read-first, plan-by-default, single-reseller-scoped, and the full
source of both the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). Nothing about the credential path is hidden: you
supply the token, the binary uses it against the Servosity API, and you can read
every line of how it does so.
