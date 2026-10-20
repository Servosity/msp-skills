# eset-protect skill - governance and safety model

> Unofficial. Community-built skill for the ESET PROTECT (ESET Connect) API.
> Not affiliated with, endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the eset-protect skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `eset-protect-cli` binary (and `eset-protect-mcp`) as a
dedicated **ESET Connect API user** you create. A Superuser or Root creates it
in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2, gives it
access rights, and turns on the **Integrations** toggle. The Superuser or Root
account itself cannot be given API access.

- `ESET_PROTECT_USERNAME` and `ESET_PROTECT_PASSWORD` are that user's email and
  password. The CLI exchanges them for a one-hour bearer token with ESET's
  OAuth2 password grant at `https://<region>.business-account.iam.eset.systems/oauth/token`
  and refreshes it when it expires or ESET rejects it.
- `ESET_PROTECT_REGION` selects the data center: `eu` (default), `de`, `us`,
  `ca` or `jpn`. A `region` you write under `[template_vars]` in `config.toml`
  is used when the variable is unset; the CLI never writes one there itself.
- `ESET_PROTECT_TOKEN` is optional: a token you minted yourself.

**The API user's access rights are the real permission boundary.** In ESET
PROTECT Hub you can assign permission sets in ESET PROTECT; in ESET Business
Account and ESET MSP Administrator 2 you set the user's access rights when you
create it. The CLI can do only what that user can do, in the companies that user
can see. Give a reporting workflow a user without write rights.

Where the credentials live is your choice:

- **Supplied through the environment** (a secret manager, Keychain or a
  session-scoped variable): the password is never written to disk.
- **The access and refresh tokens** that `auth login` (or any command) mints
  are cached in `credentials.toml` in the CLI's data directory (`doctor` prints
  the path), for that region and user, so the next command does not sign in
  again. `auth logout`
  clears them. `auth set-token` saves a token you pipe in on stdin.
- **Token pinning.** A request that carries the token is refused unless it goes
  to an `*.eset.systems` host or to the base URL you configured, and token
  requests never follow redirects, so a flag that rewrites a URL (for example
  `sync --path-context`) cannot send the token anywhere else.

## Default-safe behavior

- **Response actions preview first.** `respond scan`, `respond isolate`,
  `respond release` and `policies clone` print the exact request and send it only
  with `--yes`. Over MCP, `--yes` is never forwarded, so those tools only preview.
- **Every other write runs when invoked.** Generated commands such as
  `executables block`, `edr-rules delete` or `devices move` do not prompt. Preview
  them with `--dry-run`, which prints the request without sending it. `--dry-run`
  covers the API request only: `--deliver webhook:<url>` still posts the output,
  `feedback --send` still sends, and `import --dry-run` still signs in to ESET to
  mint a token.
- **Read commands are safe to run** (reports, rollups, lists, the local mirror);
  they cannot change anything in ESET. `sync` writes only the local SQLite mirror.
  The mirror holds what the API user can read, so keep the CLI's data
  directory private. Links and files that enroll a device into your tenant
  (installer download links, GPO/SCCM deployment files, mobile enrollment links)
  are never persisted by the CLI: installer rows in the mirror and in `export`
  files carry `downloadUrl` set to `[redacted]`, those responses are never
  cached, and installers are not in the default `sync`. Live `installers list`,
  `installers get`, `gpo-sccm-files-generate` and
  `mobile-devices-batch-get-enrollment-links` still print them, so redirecting
  their output, or `--deliver file:<path>`, writes them. Device identity tokens (`deviceToken`) are replaced with `[redacted]` as
  each response is read, so they are never stored or shown.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating; the preview-then-approve policy still applies. See
  AGENTS.md.
- **MCP read-only switch.** Set `ESET_PROTECT_MCP_READ_ONLY=1` in the MCP
  server's environment for an agent that should only read. No tool that changes
  ESET is offered (including `respond`, `sync` and `isolate_a_machine`), and the
  generic `eset-protect_execute` tool refuses every method other than GET. It
  also refuses the Credential-tier reads: `installers list`, `installers get`,
  `gpo-sccm-files-generate`, `mobile-devices-batch-get-enrollment-links` and the
  two quarantine downloads. Three ESET reads are POST calls
  (`detection-groups-search`, `detections-v2 batch-get` and
  `users-batch-get-users`), so read-only mode refuses those too; each has a GET
  list or get command instead. The local learning tools (`teach`,
  `teach_playbook` and similar) still write to the CLI's own learning store,
  and the mirror cannot be refreshed from a read-only server, so run `sync`
  yourself.
- **No host files over MCP.** In every mode, the MCP server refuses any flag
  that names a file on the machine it runs on (`--file`, `--db`, `--config`,
  `--notes-file` and similar), and it does not offer `import`, so a tool call
  cannot read local files.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**;
require a human for anything below the Read row.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, lists, the local mirror. No change in ESET. | `fleet health`, `fleet gaps`, `fleet stale`, `triage`, `exposure`, `report customer`, `report vulnerabilities`, `patches backlog`, `detections spread`, `detections latest`, `devices profile`, `incidents context`, `executables find`, `policies decode`, `devices list`, `incidents list`, `detections list`, `policies list`, `vulnerable-devices`, `scans`, `sync`, `export` (writes a local file; Credential-tier fields redacted) | Allow |
| **Endpoint response** | Creates tasks that run on real customer machines. | `respond scan`, `respond isolate`, `respond release` (preview unless `--yes`), `device-tasks create`, `device-tasks update-targets`, `device-tasks update-triggers`, `mobile-devices-batch-activate-product` | Human-in-the-loop, explicit confirmation |
| **Protection and policy config** | Changes what protection a device gets. An exclusion, a disabled rule or a move into another group's policy can open a hole. | `executables block`, `executables unblock`, `edr-rules create`, `edr-rules enable`, `edr-rules disable`, `edr-rules update-definition`, `edr-rule-exclusions create`, `edr-rule-exclusions update-definition`, `policies create`, `policies clone`, `policy-assignments assign-policy`, `policy-assignments unassign-policy`, `policy-assignments update-ranking`, `ip-sets update`, `web-address-rules domains web-address-rules-update`, `devices move`, `groups move` | Preview with `--dry-run`, then a human-approved write |
| **Write (routine)** | Day-to-day incident and inventory housekeeping. | `incidents close`, `incidents reopen`, `incidents basic-attributes-update incidents-update-incident-basic-attributes`, `incidents comments incident-create-incident`, `incidents comments incident-update-incident-text`, `detection-groups resolve`, `detections-v2 resolve`, `devices rename`, `groups create`, `groups rename`, `installers create` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Hands out what enrolls a device into your tenant, quarantined content, or access rights. | `installers list`, `installers get`, `gpo-sccm-files-generate`, `mobile-devices-batch-get-enrollment-links`, `quarantined-objects-download`, `quarantined-objects-batch-download`, `roles create`, `roles delete`, `role-assignments-assign-role`, `role-assignments-revoke-role`, `auth login`, `auth set-token` | Human-in-the-loop only |
| **Destructive / high-risk** | Irreversible deletes, or releasing quarantined items back to users. | `groups delete` (and every group under it), `policies delete`, `edr-rules delete`, `edr-rule-exclusions delete`, `device-tasks delete`, `installers delete`, `incidents comments incident-delete-incident`, `quarantined-objects-restore`, `quarantined-objects-batch-restore`, `quarantined-objects-purge`, `quarantined-objects-batch-delete`, `devices-batch-import`, `import` (one create per JSONL line, so it can reach any tier above) | Human-in-the-loop only, explicit confirmation |

Over MCP, the generic `eset-protect_execute` tool can call any ESET Connect
endpoint, so it reaches every write tier above unless
`ESET_PROTECT_MCP_READ_ONLY=1` is set.

## How to lock it down

- **Scope the API user** to only what the workflow needs. A reporting or QBR
  workflow does not need an API user that can create tasks, change policies or
  restore quarantine.
- **Keep autonomous agents to Read plus previewed writes.** Have a human approve
  the actual write for every tier below Read; the gate lives in your agent's
  policy, not in the binary's defaults (except the `respond` and `policies clone`
  preview).
- **Run the MCP server with `ESET_PROTECT_MCP_READ_ONLY=1`** for agents that only
  report.
- **Never let an agent run Credential, Destructive or high-risk commands
  unattended.** Treat them like a production database drop: human, reviewed,
  logged.
- **Rotate the API user's password if it is ever exposed** (for example after
  bridging the MCP server to a public endpoint for ChatGPT, see mcp-install.md).
  A minted token stays valid for up to an hour, so after rotating, run
  `auth logout` on every machine that cached one.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the API user, the binary uses it against
the ESET Connect API, and you can read every line of how it does so. The skill is
read-first, previews its `respond` actions and `policies clone`, and is scoped to
your own account.
