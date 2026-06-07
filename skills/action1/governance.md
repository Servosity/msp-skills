# action1 skill - governance and safety model

> Unofficial. Community-built skill for the Action1 API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the action1 skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `action1-cli` binary (and `action1-mcp`), authenticating with
an Action1 API client: `ACTION1_CLIENT_ID` + `ACTION1_CLIENT_SECRET` (the CLI mints
a short-lived bearer token from them automatically), or a pre-minted `ACTION1_OAUTH2`
token. `ACTION1_REGION` (us/eu/au) selects your data center. Credentials are read
from the environment only - never written to disk, never logged, never sent anywhere
except the Action1 API. The token your client mints is scoped to the permissions you
granted that API client in the Action1 console, so the safest control is to grant the
client only the permission templates your workflow needs (see "How to lock it down").

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you
  pass `--dry-run` first to preview the exact request without sending. Make your
  agent's policy: preview, show the command, get approval, then run the write.
- **Read commands are safe to run** (fleet rollups, lists, reports, search,
  analytics, export); they cannot change anything on an endpoint or in your tenant.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating - the preview-then-approve policy above still applies. See AGENTS.md.
- **`import` is a write.** Despite its read-like name, `action1-cli import` issues
  API create/upsert calls. Treat it as a Write-tier command, not a safe read.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the Write line. The tiers below are classified by
what the command actually does to your tenant and your managed endpoints, not by how
the command name reads - several POST commands (running an automation, approving
updates, minting a token, `import`) are real actions even though their names don't
contain an obvious write verb.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Fleet rollups, lists, reports, search, analytics, export, status. No change. | `fleet patch-posture`, `fleet vuln-triage`, `fleet stale`, `fleet org-scorecard`, `endpoints managed <orgId>`, `updates org-id-get <orgId>`, `vulnerabilities org-id-get <orgId>`, `audit events-get`, `search`, `analytics`, `export`, `doctor` | Allow |
| **Write (routine)** | Day-to-day config edits in your tenant. | `endpoints groups` (create), `endpoints groups-group-id-patch`, `endpoints groups-group-id-contents-post`, `endpoints managed-id-move`, `endpoints managed-id-patch`, `endpoints discovery-org-id-patch`, `reports org-id-custom-post`, `settings org-id-post`, `data-sources all-post`, `software-repository packages-all-post`, `software-repository versions packages-all-package-id-id-upload-post`, `automations policies-schedules-org-id-post`, `me patch`, `import` | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Endpoint / patch execution** | Acts on live managed endpoints: runs an automation, approves patches for deployment, opens a remote session, or defines code that will run on machines. | `automations policies-instances-org-id-post` (run an automation), `automations policies-instances-org-id-instance-id-stop-post`, `updates approvals updates-org-id` (approve updates for deployment), `endpoints managed-id-remote-sessions-post`, `endpoints managed-id-remote-sessions-session-id-patch`, `installed-software requery apps-org-id-post`, `scripts org-id-post`, `scripts org-id-id-patch` | Human-in-the-loop; never unattended |
| **Credential / security** | Mints tokens, or manages who can log in and what they can do. | `oauth2` (mints a bearer token), `users post`, `users id-patch`, `roles post`, `roles id-patch`, `roles clone roles-post`, `roles users roles-role-id-id-post` | Human-in-the-loop only |
| **Destructive / account** | Irreversible data, config, or account loss. | `endpoints managed-id-delete`, `endpoints groups-group-id-delete`, `organizations org-id-delete`, `users id-delete`, `roles id-delete`, `scripts org-id-id-delete`, `settings org-id-id-delete`, `software-repository packages-all-package-id-delete`, `vulnerabilities remediations vulnerabilities-org-id-cve-id-id-delete`, `automations policies-schedules-org-id-id-delete`, `enterprise request-closure`, `enterprise revoke-closure` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the API client.** Action1 lets you grant a client specific permission
  templates (e.g. `view_endpoints`, `view_vulnerabilities`) without the management
  ones (`manage_endpoints`, `manage_automations`, `approve_updates`). A read/report
  workflow does not need a client that can run automations, approve patches, or
  delete endpoints - mint a read-only client for it.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy and
  in the API client's granted permissions, not in the binary's defaults.
- **Never let an agent run Endpoint/execution, Credential, or Destructive tier
  commands unattended.** Running an automation, approving updates, or deleting an
  endpoint is real-world impact on a client's machines; treat them like a production
  database drop: human, reviewed, logged.
- **Treat `oauth2` like a credential operation.** It mints a bearer token from your
  client secret; route it through the same approval as anything that touches auth.
- **Rotate the client secret if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Action1 API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to whatever permissions you granted the API
client - nothing more.
