# kaseya-vsa skill - governance and safety model

> Unofficial. Community-built skill for the Kaseya VSA 10 API. Not affiliated with,
> endorsed by, or sponsored by Kaseya US LLC.
> This page tells an MSP owner exactly what the kaseya-vsa skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `kaseya-vsa-cli` binary (and `kaseya-vsa-mcp`) with a VSA 10 API
token: `KASEYA_VSA_SERVER` (your instance host, for example `yourcompany.vsax.net`),
`KASEYA_VSA_TOKEN_ID` and `KASEYA_VSA_TOKEN_SECRET`, sent as HTTP Basic auth over HTTPS.
The token reaches only that host: a request or redirect to any other host is refused
before it is sent. Credentials come from the environment, or from
`kaseya-vsa-cli auth set-credentials`, which keeps them in one credentials file readable
only by your user (mode 0600). They are never written anywhere else, and any output that
repeats them, in any encoding, is withheld.

## What it keeps on disk

`kaseya-vsa-cli sync` keeps a local SQLite mirror of what it reads so fleet questions run
offline. Custom-field values (where MSPs keep BitLocker keys and client passwords),
webhook signing keys and the Windows product key are never written to it, and the on-disk
response cache is off. `kaseya-vsa-cli local-data purge --yes` deletes the mirror, cache
and learn journal.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** CLI write commands send as soon as they run unless
  you pass `--dry-run` first to preview the request. Make your agent's policy: preview,
  show the exact command, get approval, then run the write.
- **The MCP server previews first.** Every MCP tool that changes VSA 10 or runs
  automation (`kaseya-vsa_execute`, `import`, `scripts_run_wait`, `tasks_run_wait`)
  returns a preview and acts only when called again with `confirm: true`.
- **`scripts run-wait` and `tasks run-wait` run immediately on the CLI.** They have no
  confirmation prompt: treat them as endpoint execution (below).
- **Read commands do not change anything** (reports, rollups, search), but some return
  stored secrets: see the credential tier.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a
human for anything below the line. Tiers follow each command's HTTP method, not its name.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lists, details, reports, rollups, search. No change. | `devices list`, `devices get`, `health`, `patch-exposure`, `software`, `audit-logs digest`, `devices brief`, `patch-management get-policy`, `sync`, `search`, `export` | Allow |
| **Write (routine)** | Day-to-day changes to VSA 10 records. | `organizations create`/`update`, `sites create`/`update`, `groups create`/`update`, `devices move`, `devices publish`, `notifications create`, `custom-fields assign`/`unassign`/`update-assigned`, `webhooks create`/`update`, `import` | Preview with `--dry-run`, then an approved write |
| **Endpoint execution** | Runs code on customer devices. | `scripts run`, `scripts run-wait`, `tasks run`, `tasks run-wait`, `workflows run`, `workflows cancel-executions` | Human-in-the-loop only, for every run |
| **Credential / secret-bearing** | Any command that can return a stored secret or an enrollment link. | `custom-fields get`/`list`, `devices list-custom-fields`, `organization-custom-fields`, `site-custom-fields`, `group-custom-fields`, `assets list`/`get-device` (AssetInfo holds the Windows product key), `groups get-package` (agent install link), `webhooks get`/`list`/`regenerate-secret-key`, `auth` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `organizations delete`, `sites delete`, `groups delete`, `notifications delete`, `webhooks delete`, `local-data purge` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the API token** to only what your workflow needs. A reporting workflow does not
  need a token that can run scripts or delete organizations.
- **Keep autonomous agents to Read plus previewed writes.** The gate lives in your agent's
  policy, not in the CLI's defaults.
- **Never let an agent run Endpoint execution, Credential, or Destructive tier commands
  unattended.** Treat them like a production change: human, reviewed, logged.
- **Use MCP read-only mode** (`KASEYA_VSA_MCP_READ_ONLY=1`) for agents that only answer
  questions; it hides every tool that writes or runs automation.
- **Rotate the token if it is ever exposed**, for example after hosting the MCP server's
  HTTP mode (see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token, the binary uses it against your VSA 10
instance only, and you can read every line of how it does so.
