# bitdefender skill - governance and safety model

> Unofficial. Community-built skill for the Bitdefender GravityZone API. Not
> affiliated with, endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the bitdefender skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `bitdefender-cli` binary (and `bitdefender-mcp`) with a
GravityZone Control Center API key you create: click the user icon in Control
Center, choose **My Account** and use the **Control Center API** section. Calls
use HTTP Basic with the key as the username and an empty password. It needs
`BITDEFENDER_API_KEY`; `BITDEFENDER_BASE_URL` is optional and defaults to
`https://cloud.gravityzone.bitdefender.com/api` (set it to the **Access URL**
shown in that same section for another region or an on-premises console).

**The API areas you enable on the key are the real permission boundary.** Keys
are scoped per API area (Companies, Network, Policies, Incidents, Quarantine,
Licensing, Reports, Packages, Accounts, Push and so on) with no read/write split
inside an area: a key with the Network API can both list endpoints and start
scan tasks, and one with the Incidents API can both list incidents and isolate
machines. Enable only the areas the workflow needs. A partner-level key
reaches every child company; a customer-level key reaches only that company.

Where the key lives is your choice, and the CLI respects it:

- **Supplied through the environment** (a secret manager, Keychain, or a
  session-scoped variable): the key is **never copied to disk**.
- **Saved with `auth set-token`** (the key is piped on stdin, so it never appears
  in process arguments or shell history): it is written to a permission-restricted
  (`0600`) `credentials.toml` in your user data directory. Run
  `bitdefender-cli doctor` to see the resolved paths, and `auth logout` to clear it.

The key is never logged and goes only to the API base URL. An environment
`BITDEFENDER_BASE_URL` applies to that run only and is never saved to
`config.toml`; editing `base_url` in `config.toml` changes it until you change it
back. Two commands send data elsewhere only when you ask: `--deliver webhook:<url>`
posts a command's output to that URL (the MCP server does not accept
`--deliver`), and `feedback --send` (or `BITDEFENDER_FEEDBACK_AUTO_SEND`) posts your
feedback note to `BITDEFENDER_FEEDBACK_ENDPOINT`, which is unset by default.

**What is stored locally.** `fleet health` saves each run as a dated snapshot in a
local SQLite store (companies, endpoint names, IPs, MAC addresses, labels and
protection state), and `companies tree` caches the company tree. Responses to the
generated API commands are not stored: every GravityZone call is a POST, and the
response cache only ever holds GET responses. So the reads that return a credential
(below) are printed to you and nowhere else.

## Default-safe behavior

- **Generated commands run immediately.** Write commands (create, delete, isolate,
  quarantine, tasks) run as soon as they are invoked and do not prompt. `--dry-run`
  is opt-in: it prints the JSON-RPC method (after the `#` in the URL) and the
  parameters it would send, without sending. Make your agent's policy: preview,
  show the exact command, get approval, then run the write.
- **The fleet commands only read.** `fleet health`, `fleet incident-queue`,
  `fleet threats`, `fleet locate`, `fleet rmm-gap`, `fleet changes`,
  `companies tree` and `usage billing` change nothing in GravityZone; some write
  only to your local snapshot store. `fleet incident-queue` and `fleet threats`
  print the ids to act on and leave the action to `incidents set-status`,
  `incidents assign` or `blocklist add`.
- **The MCP server can still write.** Its generic `bitdefender_execute` tool can
  call any GravityZone method, including isolation, kill process, remote command
  execution and deletes, gated only by your AI app's per-tool approval prompt.
  For an agent that should only read, set `BITDEFENDER_MCP_READ_ONLY=1` in the MCP
  server's environment (the value must be exactly `1`): `bitdefender_execute` then
  runs only an explicit list of GravityZone read methods and refuses every write
  and every Credential and access read below. Incident reads stay allowed for
  triage, and an XDR incident's evidence can include connection metadata captured
  on the endpoint (for example a WinRM authorization header), so treat incident
  output as sensitive. The `import` tool and the tools that delete local
  learnings are hidden. A method added by a later regeneration stays refused until it is
  reviewed and listed.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

GravityZone is an endpoint security console: some of its commands reach real
customer machines, and some change what GravityZone blocks or allows. The safe
default for an autonomous agent is **read only**; require a human for anything
below Read.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, list and get. No change in GravityZone. | `fleet health`, `fleet incident-queue`, `fleet threats`, `fleet locate`, `fleet rmm-gap`, `fleet changes`, `companies tree`, `usage billing`, `usage monthly`, `usage monthly-by-product`, `companies list`, `companies get`, `companies find`, `company-folders list`, `endpoints list`, `endpoints get`, `endpoint-tags`, `inventory roots`, `groups list`, `incidents list`, `incidents get`, `incidents get-many`, `incidents similar-emails`, `quarantine list`, `blocklist list`, `custom-rules list`, `custom-fields list`, `policies list`, `patches installed`, `patches missing`, `maintenance-windows list`, `maintenance-windows get`, `maintenance-windows approved-patches`, `tasks list-scans`, `tasks status`, `response-actions status`, `reports list`, `reports download-links`, `investigation file-url`, `phasr recommendations`, `phasr identities`, `phasr resources`, `phasr monitored-rules`, `phasr profiles`, `phasr rule-data`, `integrations list`, `integrators list`, `accounts list`, `accounts get`, `notifications get`, `packages list`, `push stats`, `api-key`, `aws-ec2 company-by-account`, `aws-ec2 hourly-usage`, `aws-ec2 external-id` | Allow |
| **Endpoint response** | Acts on real customer machines. | `isolation isolate`, `isolation restore`, `endpoints kill-process`, `investigation kill-process`, `investigation run-command`, `investigation collect-package`, `investigation retrieve-file`, `tasks scan`, `tasks scan-by-mac`, `tasks reconfigure`, `tasks gather-logs`, `tasks sandbox-submit`, `quarantine add-file`, `quarantine remove`, `quarantine restore`, `quarantine empty`, `quarantine release-exchange`, `quarantine restore-exchange`, `live-search`, `yara`, `response-actions create` | Human-in-the-loop, explicit confirmation |
| **Protection and policy config** | Changes what GravityZone blocks, allows, patches or alerts on, or which policy a machine gets. An exclusion rule or a weaker policy can open a hole. | `policies set-modules`, `endpoints assign-policy`, `blocklist add`, `blocklist remove`, `custom-rules create`, `custom-rules update`, `custom-rules delete`, `phasr apply`, `phasr edit-rules-access`, `phasr request-access-action`, `maintenance-windows create`, `maintenance-windows update`, `maintenance-windows assign`, `maintenance-windows unassign`, `maintenance-windows delete`, `push set`, `push reset-stats`, `push test`, `notifications configure`, `integrations create`, `integrations update`, `integrations manage`, `integrations delete`, `integrators add`, `integrators remove`, `aws-ec2 configure`, `aws-ec2 disable`, `aws-ec2 generate-external-id`, `endpoints move`, `groups move` (a machine or group moved into another custom group can inherit that group's policy) | Preview with `--dry-run`, then a human-approved write |
| **Write (routine)** | Day-to-day records with no direct protection effect. | `incidents note`, `incidents set-status`, `incidents set-priority`, `incidents assign`, `endpoints set-label`, `groups create`, `company-folders create`, `custom-fields create`, `custom-fields update`, `reports create`, `reports delete`, `packages create`, `packages update`, `tasks delete` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Returns or takes a live secret, or changes who can do what. `push get` returns the Authorization value GravityZone sends to your webhook; `license get` returns the license key; `companies get-by-user` takes a user's password; `packages get` returns package settings, which can include the uninstall password; `packages install-links` returns links that enroll a machine into the company; `integrations get` returns an integration's connection settings; `policies get` returns policy settings, which carry the password-protection and power-user password fields; `inventory list` can return a company's license keys. | `push get`, `license get`, `companies get-by-user`, `packages get`, `packages install-links`, `integrations get`, `policies get`, `inventory list`, `accounts create`, `accounts update`, `accounts delete`, `email-verification`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive and commercial** | Irreversible loss, moves endpoints or a company to another parent, or changes a customer's account or what it is billed. `import` sends one create call per JSONL line to whichever resource you name, so it can reach any tier above (an `isolation` import isolates machines, an `accounts` import creates console users). | `import`, `endpoints delete`, `endpoints move-to-company`, `company-folders move`, `companies create`, `companies update`, `companies suspend`, `companies activate`, `companies delete`, `company-folders delete`, `groups delete`, `custom-fields delete`, `packages delete`, `license set-key`, `license add-product-key`, `license remove-product-key`, `license set-monthly-subscription` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the key.** Enable only the API areas a workflow needs. A reporting
  workflow needs Companies, Network, Licensing, Incidents and Quarantine; it does
  not need Accounts, Packages, Push or Integrations. Create a separate key for
  the rare runs that must act.
- **Run the MCP server read-only unless an agent must act.** Set
  `BITDEFENDER_MCP_READ_ONLY=1` and keep your AI app's approval prompt on. Because a
  key has no read/write split inside an area, this switch and the approval prompt
  are the gate on the MCP side, not the key.
- **Keep autonomous agents to Read.** Have a human approve every command below the
  Read tier - the gate lives in your agent's policy and in the key's API areas,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Endpoint response commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the key if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md). Delete it in
  Control Center and create a new one.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the key, the binary uses it against the
GravityZone API, and you can read every line of how it does so. The fleet views
only read, and the skill is scoped to your own account.
