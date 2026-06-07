# gradient skill - governance and safety model

> Unofficial. Community-built skill for the Gradient MSP API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the gradient skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `gradient-cli` binary (and `gradient-mcp`), authenticating with
a Synthesize vendor token. Set `GRADIENT_TOKEN` to the base64 of
`<vendorApiKey>:<partnerApiKey>`, or set `GRADIENT_VENDOR_API_KEY` and
`GRADIENT_PARTNER_API_KEY` and the CLI derives the token for you. `GRADIENT_BASE_URL`
is optional and defaults to the public Synthesize endpoint. Credentials are read from
the environment (or a local config file you write with `auth set-token`) - never
logged, never bundled, never sent anywhere except the Gradient MSP API.

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
| **Read** | Reports, rollups, search. No change. | `integration get`, `vendor get`, `services get`, `accounts list`, `usage drift`, `alert trace`, `hygiene unmapped`, `status ready`, `analytics`, `sync` | Allow |
| **Write (routine)** | Day-to-day mutations that push or change vendor data. | `usage push`, `billing`, `alert send`, `alerting`, `import`, `accounts create`, `accounts update`, `accounts update-one`, `services create`, `mappings create`, `mappings update`, `mappings update-bulk`, `integration update-status`, `vendor update` (14 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches the stored token. | `auth set-token` (writes the token to your local config file), `auth logout` (clears it) | Human-in-the-loop only |
| **Destructive** | Irreversible vendor data loss. | (none - the Synthesize vendor API exposes no delete commands; `profile delete` only removes a local saved-flag profile) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

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
the Gradient MSP API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
