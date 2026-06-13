# connectwise-automate skill - governance and safety model

> Unofficial. Community-built skill for the ConnectWise Automate API. Not affiliated
> with, endorsed by, or sponsored by ConnectWise, LLC.
> This page tells an MSP owner exactly what the connectwise-automate skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `connectwise-automate-cli` binary (and `connectwise-automate-mcp`),
authenticating with `CONNECTWISE_AUTOMATE_SERVER` (your Automate host),
`CONNECTWISE_AUTOMATE_CLIENT_ID` (your registered integration GUID, sent as the
`clientId` header on every request for v2020.11+), and `CONNECTWISE_AUTOMATE_TOKEN`
(a short-lived bearer token). Credentials are read from the environment only -
never written to disk by the skill, never logged, never sent anywhere except your
ConnectWise Automate server.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (roll-ups, triage, inventory, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read only**; require a human for
anything that changes an endpoint, deploys patches, or touches a credential.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Fleet roll-ups, triage, inventory, search. No change. | `fleet-health`, `stale-agents`, `patch-compliance`, `alert-triage`, `client-rollup`, `os-inventory`, `since`, and every `list` / `get` / `search` | Allow |
| **Endpoint and fleet actions** | Changes real endpoints or deploys across the fleet. High impact, hard to undo. | `computers command-execute` (runs a real command on an agent), `patching deploy-approved`, `patching deploy-security`, `patching reattempt-failed` (fleet-wide patch deployment) | Human-in-the-loop, explicit confirmation. Preview with `--dry-run`, then an approved run. |
| **Write (data)** | Creates or upserts records via the API. | `import <resource>` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Mints or stores tokens. | `apitoken mint`, `apitoken refresh`, `auth set-token` | Human-in-the-loop only |
| **Local config** | Changes only local CLI state, never the server. | `profile save` / `delete`, `auth logout`, `sync`, `feedback` | Allow (no server effect) |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a token whose Automate permissions can run commands on agents or
  deploy patches.
- **Keep autonomous agents to the Read tier.** Have a human approve any Endpoint /
  fleet action, Write, or Credential command - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Treat `computers command-execute` and `patching deploy-*` like a production
  change window.** They run real commands and push real patches across live client
  endpoints: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). Bearer tokens are
  short-lived; refresh with `apitoken refresh`.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
your ConnectWise Automate server, and you can read every line of how it does so. The
skill is read-first, plan-by-default, and scoped to your own account.
