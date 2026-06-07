# tactical-rmm skill - governance and safety model

> Unofficial. Community-built skill for the Tactical RMM API. Not affiliated with,
> endorsed by, or sponsored by AmidaWare LLC.
> This page tells an MSP owner exactly what the tactical-rmm skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `tactical-rmm-cli` binary (and `tactical-rmm-mcp`),
authenticating with `TRMM_API_KEY` against your own self-hosted instance at
`TACTICAL_RMM_BASE_URL`. Credentials are read from the environment (or a local config
file you control) - never written into this repo, never logged, never sent anywhere
except your Tactical RMM API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Routine mutating commands send immediately
  unless you pass `--dry-run` first to preview the request without sending. Make your
  agent's policy: preview, show the exact command, get approval, then run the write.
- **The cohort actions preview by default.** `agents bulk-run` and `maintenance set`
  resolve and print the target cohort and do **nothing** until you re-run with
  `--execute`. That safety default is in the binary, not just the docs.
- **Read commands are always safe to run** (reports, rollups, search) with one
  exception below: a few read endpoints return stored secrets and are treated as
  credential-tier.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

Tactical RMM is a remote-control plane, not just a database: it can run scripts on
endpoints, reboot machines, install Windows updates, and manage credentials. The safe
default for an autonomous agent is **read plus planned (dry-run) writes**; require a
human for anything in the bottom three tiers.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `fleet health`, `triage`, `patch posture`, `clients scorecard`, `coverage`, `since`, `checks worst`/`flapping`, `alerts digest`, `services down`, `software find`, `agents`/`clients`/`checks`/`alerts` get & list, `search`, `sync`, `analytics` | Allow |
| **Write (routine config)** | Day-to-day config mutations. | Create/update clients, sites, checks, alerts & templates, scripts & snippets, automation policies & patch policies, autotasks, custom fields, schedules, agent notes, deployments, software/service records; `import` (bulk create/upsert from JSONL) | Preview with `--dry-run`, then an approved write |
| **Endpoint & script execution** | Runs code or actions on managed machines. | `agents bulk-run`, `scripts test`, autotasks/automation run, reboot/shutdown/wake/recover agents, install or scan Windows updates, uninstall software, reset checks, `maintenance set`, remote sessions (meshcentral, webvnc), server test actions (SMS/email, OpenAI generate) | Human-in-the-loop only - never unattended |
| **Credential & identity** | Touches keys, tokens, accounts, MFA. | API keys (create/list/delete/update), keystore & codesign tokens, core settings, users, roles, sessions, password/2FA/TOTP resets | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | Delete agents, clients, sites, checks, scripts & snippets, alerts & templates, automation & patch policies, autotasks, custom fields, schedules, deployments, agent processes, pending actions | Human-in-the-loop only, explicit confirmation |

### Reads that return secrets (credential-tier, not Read)

A few endpoints are GET/read-only at the HTTP layer but **return stored secrets**, so
they are classified credential-tier and must not run unattended:

- **`accounts accounts`** (`GET /accounts/apikeys/`) - lists API keys including their
  key values.
- **`core core-keystore`** (`GET /core/keystore/`) - returns stored keystore values
  (used to hold secrets for scripts).
- **`core core-codesign-2`** (`GET /core/codesign/`) - returns the code-signing token.
- **`core core-settings`** (`GET /core/settings/`) - can include integration
  credentials (SMTP, SMS, MeshCentral) in server settings.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a key that can run scripts on endpoints, delete records, or read other
  keys. Tactical RMM API keys inherit the assigned user's role - give the integration
  user a least-privilege role.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy, and
  for the cohort actions also in the binary's `--execute` requirement.
- **Never let an agent run Endpoint-execution, Credential, or Destructive tier
  commands unattended.** Running a script on a cohort, rebooting machines, or reading
  API keys deserves the same care as a production change window: human, reviewed,
  logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential and point it at your own
server, the binary uses it against your Tactical RMM API, and you can read every line
of how it does so. The skill is read-first, plan-by-default, scopes to your own
instance, and previews every cohort action before it runs.
