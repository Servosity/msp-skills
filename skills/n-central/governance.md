# n-central skill - governance and safety model

> Unofficial. Community-built skill for the N-able N-central API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the n-central skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `n-central-cli` binary (and `n-central-mcp`),
authenticating with a JSON Web Token for an API-only N-central user, read from
`NCENTRAL_JWT` in the environment or saved to the CLI's local config via
`n-central-cli auth set-token`. The JWT is exchanged for short-lived access
tokens, never logged, and never sent anywhere except your N-central server
(`N_CENTRAL_BASE_URL`).

## Default-safe behavior

- **Nearly the whole surface is read-only.** Devices, customers, sites, org
  units, issues, properties, maintenance windows, exports, search - none of it
  can change anything in N-central.
- **Two commands write, and `--dry-run` is opt-in - use it.**
  `scheduled-tasks run` creates a task that executes an API-enabled Automation
  Policy or Script on a live device; `import` POSTs records from a JSONL file.
  Both send immediately unless you pass `--dry-run` first. Make your agent's
  policy: preview, show the exact command, get approval, then run.
- **Registration tokens are credentials.** `customers registration-token` and
  `org-units registration-token` are reads, but what they read enrolls new
  devices into your N-central - treat the output like a password.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does
  not add any write gating - the preview-then-approve policy above still
  applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run)
writes**; require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Rollups, lookups, audits, exports. No change. | `triage`, `whereis`, `fanout`, `devices list`, `props audit`, `maint coverage`, `guardian`, `export` | Allow |
| **Write (routine)** | POSTs records to the API. | `import <resource> --input data.jsonl` | Preview with `--dry-run`, then an approved write |
| **Remote execution** | Runs scripts/automation policies on live endpoints. | `scheduled-tasks run --task-type Script --device-id <id> --item-id <id>` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Tokens that enroll devices; stored-credential changes. | `customers registration-token`, `org-units registration-token`, `auth set-token`, `auth logout` | Human-in-the-loop only |

## How to lock it down

- **The API user's role is the real permission boundary.** Create a dedicated
  API-only user in N-central and scope its role to what you want an AI to
  reach. A read/report workflow does not need a role that can create tasks.
  Note the JWT carries the role's permissions - changing the role requires
  regenerating the token.
- **Watch the password clock.** The API user's password expiry (90 days by
  default) silently invalidates the JWT. Wire `guardian --password-set
  <YYYY-MM-DD>` into CI or a scheduled agent loop - it warns at 14 days and
  exits non-zero once expired.
- **Keep `scheduled-tasks run` human-only.** It executes on a live endpoint;
  only items explicitly API-enabled in the N-central Script/Software Repository
  can run, which is a second boundary you control on the N-central side.
- **Rotate the JWT if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). Note that
  regenerating invalidates the previous token wherever it is in use.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it
against your N-central server, and you can read every line of how it does so.
The skill is read-first; its two write paths are previewable and bounded by
the API user's role and N-central's own API-enabled-item gate.
