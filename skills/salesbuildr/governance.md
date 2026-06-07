# salesbuildr skill - governance and safety model

> Unofficial. Community-built skill for the Salesbuildr API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the salesbuildr skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `salesbuildr-cli` binary (and `salesbuildr-mcp`),
authenticating with `SALESBUILDR_API_KEY`, `SALESBUILDR_TENANT`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Salesbuildr API.

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
| **Read** | Reports, rollups, search. No change. | every `public-get` / `public-get-list`, the analytics (`quote stale`, `quote thin`, `quote funnel`, `pricing drift`, `opportunity velocity` / `winrate` / `mrr-forecast`, `product velocity`, `company whitespace`), `reconcile-psa`, `field public-get-values`, the template `public-validate-*` checks, `search`, `sql`, `export`, and `sync` (writes the local mirror only) | Allow |
| **Write (routine)** | Day-to-day mutations through the Public API. | `company` / `contact` / `product` / `pricing-book` / `quote` / `quote-template` / `quote-widget-template` / `quote-discount-group` create & update, the `public-upsert*` commands, `opportunity public-win` / `public-lose` / `public-upsert`, `field public-update-values`, `product inventory`, `product public-create-batch`, the contact undeletes (which **restore** soft-deleted records, not delete them), and the bulk `import` - 30 routine-write commands in all | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Destructive** | Irreversible data loss. | the 6 delete commands: `company public-delete`, `company public-delete-by-external-identifier`, `contact company-public-delete`, `contact company-public-delete-by-id`, `product public-delete`, `product public-delete-by-external-identifier` | Human-in-the-loop only, explicit confirmation |
| **Credential / config** | Sets or clears the local API credential. | `auth setup`, `auth set-token`, `auth logout` (these touch the local token store, never vendor data) | Human-in-the-loop only |

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
the Salesbuildr API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
