# jira-service-management skill - governance and safety model

> Unofficial. Community-built skill for the Jira Service Management API. Not affiliated with,
> endorsed by, or sponsored by Atlassian Pty Ltd.
> This page tells an MSP owner exactly what the jira-service-management skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `jira-service-management-cli` binary (and `jira-service-management-mcp`)
with HTTP Basic auth: your Atlassian account email (`JIRA_SERVICE_MANAGEMENT_EMAIL`) and an
Atlassian API token (`JIRA_SERVICE_MANAGEMENT_API_TOKEN`). `JIRA_SERVICE_MANAGEMENT_SITE` (your
site subdomain) and, for Assets, `JIRA_SERVICE_MANAGEMENT_CLOUD_ID` and
`JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID` say where to send requests; they are not secrets.

Requests go only to `https://<site>.atlassian.net` and, for Assets, `https://api.atlassian.com`.
The token is never logged. It is read from the environment by default; if you run
`jira-service-management-cli auth set-credentials`, the email and token are saved to
`credentials.toml` in the CLI's data directory (`jira-service-management-cli doctor` prints the
path), and that save goes to the default data directory even when you pass `--config`.
`cmdb discover --save` writes the cloudId and workspaceId (not secrets) into the config file.

The token can do whatever the Atlassian account that created it can do. An agent account sees
and changes its service desks, queues, requests and internal notes; a customer-only account sees
only its own requests.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass
  `--dry-run` first to preview the request without sending. Make your agent's policy: preview,
  show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (queues, SLA ranking, timelines, reports, search,
  Assets lookups); they cannot change anything. A few Assets searches (`object find-by-aql`,
  `object count-by-aql`) are POST requests that only read.
- **Internal notes stay internal.** `request note` always posts public=false and exits with an
  error if JSM reports the stored comment as public. `request comment add` is the customer-reply
  command (pass `--public`); it sends no public field when the flag is omitted, so never use it
  for an internal note.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any write
  gating - the preview-then-approve policy above still applies. See AGENTS.md.
- **MCP read-only switch.** With `JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY=1` in the MCP server's
  environment, the server drops `request_note`, `sync`, `import`, `export`, `workflow_archive`,
  `learnings_forget`, `learnings_reject` and `cmdb_reconcile` (which reads a local file); its
  generic `execute` tool refuses any non-GET API call, and `cmdb_discover` refuses `--save`. The
  local learning tools (`teach`, `teach_*`, `playbook_amend`, `learnings_confirm`) stay: they
  write only to the CLI's own local store, never to your site. It does not change what the CLI
  itself can do.

- **Local files under MCP.** MCP tools never take a local path to read for `teach`, `teach-playbook`
  or `import`; `cmdb reconcile` reads an export only from inside the MCP server's working
  directory and refuses a working directory that is a filesystem root or the home folder.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a human for
anything below the line.

| Tier | What it does | Commands | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Queues, SLAs, timelines, reports, search, Assets lookups. No change on your site. | every command not listed below, including `breaches`, `request timeline`, `sla-report`, `client`, `roster`, `cmdb tickets`, `cmdb reconcile` (also reads a local file), `search`, and the POST requests that only read: `object find-by-aql`, `object count-by-aql`, `object navlist-aql`, `object navlist-iql`, `request validate`, `servicedesk requesttype check-permissions` | Allow |
| **Write (routine)** | Day-to-day ticket work. | `request note`, `request comment add`, `request transition perform`, `request create`, `request approval answer`, `request participant add` / `remove`, `request attachment add`, `servicedesk attach-temporary-file upload-temporary-file`, `request feedback post` / `delete`, `request notification subscribe` / `unsubscribe`, `object create`, `object update` | Preview with `--dry-run`, then an approved write |
| **Customers and organizations** | Who can raise and see tickets for a client. | `customer create`, `customer create-skippermissioncheck`, `customer revoke-portal-only-access-for-user`, `servicedesk customer add` / `add-skip-permission-check` / `invite` / `remove`, `servicedesk organization add` / `remove`, `organization create`, `organization user add` / `remove`, `organization property set` / `delete` | Human-approved write |
| **Credential / bulk import** | Mints Assets import tokens or pushes data in bulk. | `importsource token generate`, `api-import`, `importsource executions start` / `submit-data` / `submit-progress` / `cancel` / `create-failed-history`, `importsource importschedule create` / `update` / `delete`, `importsource mapping submit` / `update`, `import` | Human-in-the-loop only |
| **Destructive / config** | Irreversible data loss, or desk and CMDB configuration changes. | `organization delete`, `object delete`, `objectschema create` / `update` / `delete`, `objecttype create` / `update` / `delete`, `objecttype position change`, `objecttypeattribute create` / `update` / `delete`, `config create-referencetype` / `create-statustype` / `update-statustype` / `delete-statustype`, `servicedesk requesttype create` / `delete` / `set-property` / `delete-property`, `global` | Human-in-the-loop only, explicit confirmation |

Local-only writes (no change on your Atlassian site): `sync`, `workflow archive`, `export`, `auth set-credentials`, `cmdb discover --save`, `profile save`, and the `teach`, `teach-*`, `learnings`, `playbook` and `feedback` commands write files under the CLI's own config, data and state directories. `feedback` sends anything upstream only if you opt in.

## How to lock it down

- **Create the token as the right account.** The API token carries the full permissions of the
  Atlassian account that made it. For a read/report workflow, use an agent account that can see
  the desks you report on but is not a project or Jira admin.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the actual write for
  the Write tier and above - the gate lives in your agent's policy, not in the binary's defaults.
- **Never let an agent run the Customers, Credential or Destructive tiers unattended.** Treat them
  like a production database drop: human, reviewed, logged.
- **Use the MCP read-only switch** (`JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY=1`) for an agent that
  should only report.
- **Revoke the token if it is ever exposed** (for example after bridging the MCP server to a
  public endpoint for ChatGPT - see mcp-install.md) at
  id.atlassian.com/manage-profile/security/api-tokens.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token, the binary uses it against your own
Jira Service Management site, and you can read every line of how it does so. The skill is
read-first, preview-before-write, and scoped to the Atlassian account you choose.
