# crowdstrike skill - governance and safety model

> Unofficial. Community-built skill for the CrowdStrike API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the crowdstrike skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `crowdstrike-cli` binary (and `crowdstrike-mcp`),
authenticating with `CROWDSTRIKE_OAUTH_SCOPE`, `FALCON_CLIENT_ID`, `FALCON_CLIENT_SECRET`.

- The client ID and secret go only to the token URL of the configured Falcon cloud
  (`/oauth2/token` on the `CROWDSTRIKE_BASE_URL` host); every API request and the
  bearer token go only to that cloud. A redirect to any other host is refused.
- Credentials come from the environment, or from `credentials.toml` (mode 0600)
  when you run `auth login` or `auth set-token`. No other file holds a credential;
  a pair from the environment, and the token minted for it, are never written to disk.
- Output, files, requests and MCP results that would repeat a credential this
  program holds, whole or as 8 or more characters in a row in the encodings the
  check knows, are withheld whole and replaced by a "[credkit: withheld ...]" notice.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** From the CLI, mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Over MCP, every write tool returns a preview and sends nothing until it is called again with `confirm` set to boolean true, and `CROWDSTRIKE_MCP_READ_ONLY=1` (preset in the install configs) refuses every write. Make your agent's policy: preview, show the exact command, get approval, then run the write.
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
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `alerts patch-entities-v3`, `devices create-host-groups`, `devices update-host-groups`, `devices update-tags`, `mssp create-cidgroups`, `mssp create-user-groups`, `mssp update-cidgroups`, `mssp update-user-groups`, ... (12 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `devices delete-host-groups`, `mssp delete-cidgroup-members-v2`, `mssp delete-cidgroups`, `mssp delete-user-group-members`, `mssp delete-user-groups`, `mssp deleted-roles`, `policy delete-prevention-policies` | Human-in-the-loop only, explicit confirmation |
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
the CrowdStrike API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
