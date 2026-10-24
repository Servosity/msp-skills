# cipp skill - governance and safety model

> Unofficial. Community-built skill for the CIPP API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the cipp skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

> **Version note.** The MCP write preview, `CIPP_MCP_READ_ONLY`, token-protected HTTP mode, `local-data purge` and `CIPP_NO_LOCAL_DATA` ship in cipp 0.1.13 and later. Check with `cipp-cli --version`.

## What it authenticates as

The skill drives the `cipp-cli` binary (and `cipp-mcp`),
authenticating with `CIPP_API_KEY` or a token `auth login` mints from your CIPP API
client. Credentials are sent nowhere except the CIPP API and are never logged. A
minted token and the client secret are kept in the CLI's credentials file; with
`CIPP_NO_CONFIG_WRITE=1`, `auth login` refuses and nothing is stored (use `CIPP_API_KEY` instead).

## Default-safe behavior

- **The MCP server previews every CIPP write (cipp 0.1.13 and later; check `cipp-cli --version`).** `cipp_execute` returns a preview of
  any state-changing call, including every CIPP GET endpoint named as an action
  (`ExecRunBackup`, `RemoveStandard`, ...; 43 in this build), and sends it only when the agent passes
  `confirm: true`. `cipp_read` refuses them. `CIPP_MCP_READ_ONLY=1` drops the write
  tools entirely.
- **On the command line, `--dry-run` is opt-in - use it.** Most mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **`bulk` is the exception - it plans by default.** `cipp-cli bulk --from changes.csv` prints the plan and does **not** execute; it only writes when you add `--execute`. Pair it with `--resume` so a throttled batch checkpoints instead of restarting; a checkpoint belongs to one CSV file as it was, so a new or edited file starts from its first row. A row whose CIPP result carries any failure wording or failure state next to any success wording or success state is reported as an error, marked needs-review and not sent again by later runs of the same CSV; check it in CIPP first. The hold keeps only hashes and row numbers (the CSV's sha256, its path's sha256 and the row index), never CIPP's response or the row's content, and `bulk --execute` refuses to start when the checkpoint folder cannot be written.
- **`list-*` and the rollup commands only read.** Some CIPP endpoints are called with
  GET but change things (`exec-run-backup`, `remove-standard`, `exec-send-org-message`
  and others named `exec-*` or `remove-*`): treat every `exec-*` and `remove-*` command
  as a write, whatever its HTTP method.
- **Typed write commands exit 0 when CIPP reports a failure inside an HTTP 200.** They
  print CIPP's `Results` text; read it before reporting success. `bulk` and the MCP
  server's `cipp_execute` turn such a failure into an error.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, lists and cross-tenant rollups. No change. | `list-users`, `list-mfausers`, `list-tenants`, `posture --dimension mfa`, `ca diff`, `changes`, `scorecard`, `gdap expiring`, `fanout --endpoint /ListUsers --all-tenants`, `doctor` | Allow |
| **Write (routine)** | Day-to-day changes in a tenant or in CIPP. | `add-user`, `edit-user`, `add-group`, `exec-convert-mailbox`, `exec-set-mailbox-quota`, `exec-run-backup`, `remove-standard`, `exec-send-org-message`, and the other `add-*`, `edit-*`, `set-*` and `exec-*` commands | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Touches passwords, MFA, tokens, keys or sessions. | `exec-reset-pass`, `exec-reset-mfa`, `exec-per-user-mfa`, `exec-create-tap`, `exec-revoke-sessions`, `exec-get-local-admin-password`, `exec-get-recovery-key`, `exec-token-exchange`, `exec-update-refresh-token`, `exec-api-client` | Human-in-the-loop only |
| **Destructive** | Removes users, devices, sites, tenants or relationships. | `remove-user`, `exec-offboard-user`, `exec-offboard-tenant`, `exec-remove-tenant`, `exec-device-delete`, `delete-sharepoint-site`, `exec-delete-gdaprelationship`, `exec-groups-delete`, and the other `remove-*` and `delete-*` commands | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above. The MCP server's preview-then-confirm step
  is a gate the agent itself passes, so the approval belongs in your agent's policy;
  set `CIPP_MCP_READ_ONLY=1` where no agent should write at all.
- **Never let an agent run Credential or Destructive tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed.** HTTP mode refuses to start without
  `CIPP_MCP_HTTP_TOKEN` and needs TLS for any address other than loopback; treat that
  token like a password too (see mcp-install.md).
- **Mind the local copy.** The connector keeps an unencrypted local copy of what it
  reads (store, five-minute response cache) plus a learn journal of commands run, and
  `auth login` keeps the API client's secret in `credentials.toml` so it can mint new
  tokens. `cipp-cli doctor` prints the paths, `cipp-cli local-data purge --yes` deletes
  the store, cache and journal, `cipp-cli auth logout` removes the credentials, and
  `CIPP_NO_LOCAL_DATA=1` keeps no local copy of CIPP data. Removing a technician's CIPP
  access does not touch their machine: run logout and purge there, delete the
  `checkpoints` folder in the CLI's state directory, and rotate the API client's secret
  in CIPP.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the CIPP API, and you can read every line of how it does so. The skill is
read-first, previews writes, and is scoped to your own account.
