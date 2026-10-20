# saas-alerts skill - governance and safety model

> Unofficial. Community-built skill for the SaaS Alerts API. Not affiliated with,
> endorsed by, or sponsored by Kaseya or SaaS Alerts.
> This page tells an MSP owner exactly what the saas-alerts skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `saas-alerts-cli` binary (and `saas-alerts-mcp`) with your SaaS
Alerts **partner API key**, shown under **Settings > API** in the SaaS Alerts dashboard
(**Show Api Key**). It is the only credential: `SAAS_ALERTS_API_KEY`. The key goes out as the `api_key` header to
the Reports, Manage and Webhooks APIs, and the CLI exchanges it with
`POST /respondApi/api/v1/auth` for the short-lived Respond access token, which it keeps
in memory for that process only and re-mints once if Respond rejects it. `SAAS_ALERTS_BASE_URL` is
optional; leave it unset to use the SaaS Alerts API host.

**One key reaches every customer and every API.** SaaS Alerts documents a single partner
key for all four APIs, with no read-only or per-customer variant, so the same key that lists events can approve
Respond triggers, block a Microsoft 365 sign-in, delete a customer or turn Respond off
for the whole partner. The limits below therefore live in your agent's policy and in the
MCP read-only switch, not in the key.

Where the key lives is your choice, and the CLI respects it:

- **Supplied through the environment** (a secret manager, Keychain, or a session-scoped
  variable): the key is **never copied to disk**.
- **Saved with `auth set-token`** (the key is piped on stdin, so it never appears in
  process arguments or shell history): it is written to a permission-restricted (`0600`)
  `credentials.toml` in your user data directory. Run `saas-alerts-cli doctor` to see the
  resolved paths, and `auth logout` to clear it.

The key is never logged and goes only to the API base URL. An environment
`SAAS_ALERTS_BASE_URL` applies to that run only and is never saved to `config.toml`. Two
commands send data elsewhere only when you ask: `--deliver webhook:<url>` posts a
command's output to that URL (the MCP server does not accept `--deliver`), and
`feedback --send` (or `SAAS_ALERTS_FEEDBACK_AUTO_SEND`) posts your feedback note to
`SAAS_ALERTS_FEEDBACK_ENDPOINT`, which is unset by default.

**What is stored locally.** `sync` copies customers, users, events, Respond rule
triggers, rules, organizations, suppressions, devices and webhook subscriptions into a
local SQLite store in your data directory, and GET responses are cached there for five
minutes.
That store holds your customers' account emails, sign-in IPs and countries and event
details, so treat the data directory like any other copy of your SaaS Alerts data. Each
webhook subscription's `token` (the string SaaS Alerts attaches to every delivery so your
receiver can trust it) is replaced with `[withheld]` before it is printed, cached, synced
or returned to an MCP client; the delivery URL is kept.

## Default-safe behavior

- **The three workflow writers preview by default.** `rule-triggers decide`, `contain`
  and `subscriptions health --reenable` fetch live data and print the exact calls they
  would make, and change nothing until you add `--confirm`. `decide` skips expired and
  superseded triggers and refuses to approve a rule that resets a password or deletes a
  user unless you pass `--allow-destructive`. `contain` acts on Microsoft 365 accounts
  only, stops when an email matches more than one account, and exits non-zero when any
  action's outcome is unclear.
- **Every other write sends immediately.** The generated commands (`rule-triggers
  approve`, `response-actions execute`, `customers delete`, `respond disable` and the
  rest) do not prompt. Pass `--dry-run` first: it prints the method, URL and body and
  sends nothing, not even the Respond token request.
- **Read commands are safe to run** (triage, investigate, cross-tenant, coverage, the
  Respond queue, lists, gets and reports); they cannot change anything in SaaS Alerts.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating. See AGENTS.md.
- **The MCP server has a read-only switch.** With `SAAS_ALERTS_MCP_READ_ONLY=1`,
  `saas-alerts_execute` runs GET endpoints only, `contain`, `rule_triggers_decide` and
  `subscriptions_health` refuse their `confirm` parameter (so they can only preview), and
  `sync`, `import` and `export` are not offered. Without it, `saas-alerts_execute` can
  call any endpoint, including every write below, gated only by your AI app's approval
  prompt.

## Permission tiers

The safe default for an autonomous agent is **Read plus the previews**; require a human
for anything below the first row.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, lists and gets. No change in SaaS Alerts. | `triage`, `investigate`, `cross-tenant`, `coverage`, `rule-triggers queue`, `subscriptions health` (without `--reenable --confirm`), `rule-triggers decide` and `contain` (without `--confirm`), `customers list`, `customers get`, `events list`, `events query`, `events count`, `events count-query`, `events scroll`, `events fileshare`, `users`, `partner-users`, `reports customers`, `reports customer`, `reports mfa-microsoft`, `reports mfa-google`, `reports mailbox-rules`, `reports billing-details`, `reports scheduled-list`, `rules list`, `rules get`, `rule-triggers list`, `rule-triggers get`, `iocs list`, `ioc-triggers`, `organizations list`, `respond status`, `response-actions jobs`, `suppressions list`, `devices mapped`, `subscriptions list`, `webhook-domains list`, `webhook-queue list`, `alert-types`, `search`, `analytics` | Allow |
| **Account response** | Acts on real users in your customers' Microsoft 365 tenants: blocks sign-in, expires sessions, resets passwords, forces a password change, or approves a Respond trigger that does. | `contain --confirm`, `rule-triggers decide --confirm`, `rule-triggers approve`, `rule-triggers reject`, `rule-triggers ignore`, `rule-triggers manually-remediate`, `response-actions execute` | Human-in-the-loop, explicit confirmation. These lock real people out. |
| **Detection and Respond config** | Changes what SaaS Alerts alerts on or what Respond does automatically. A whitelist or suppression stops alerts for that condition with nothing announcing it, and the whitelist commands *replace* the customer's list rather than adding to it. | `customers set-whitelists`, `customers set-account-whitelists`, `suppressions create`, `suppressions update`, `suppressions delete`, `rules update`, `rules draft`, `rules duplicate`, `rules convert-to-ioc`, `rule-templates use`, `iocs update`, `iocs draft`, `iocs duplicate`, `iocs archive`, `iocs unarchive`, `respond enable`, `application-connections connect`, `subscriptions create`, `subscriptions update`, `subscriptions health --reenable --confirm`, `webhook-domains verify` | Preview with `--dry-run`, then a human-approved write. Read the current list before any whitelist change. |
| **Write (routine)** | Day-to-day records with no detection effect. | `devices map`, `devices unmap`, `psa map`, `psa map-suggested`, `customers create`, `customers update`, `reports scheduled-create`, `reports update-branding`, `subscriptions test-event`, `files upload` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Returns or stores a live secret. `files url` returns a signed link that opens the stored file without any credential. | `files url`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive and outbound** | Irreversible loss, ends monitoring or automated response, or sends email from SaaS Alerts' mail service. `respond disable` turns Respond off for the whole partner. `webhook-queue delete` drops events that were never delivered. `import` sends one create call per JSONL line to whichever resource you name, so it can reach any tier above. | `respond disable`, `customers delete`, `rules delete`, `application-connections delete`, `subscriptions delete`, `webhook-domains delete`, `webhook-queue delete`, `reports scheduled-delete`, `files delete`, `reports send-email`, `import` | Human-in-the-loop only, explicit confirmation |

The local learning commands (`teach`, `teach-lookup`, `teach-pattern`,
`teach-playbook`, `learnings`, `playbook`, `profile`) write only to files in your data
directory and never call SaaS Alerts.

## How to lock it down

- **Run the MCP server with `SAAS_ALERTS_MCP_READ_ONLY=1`** for any agent that should
  only look. It is the one control that holds even when the agent asks for a write.
- **Keep autonomous agents to Read plus the previews.** Have a human approve the actual
  `--confirm` run for `contain` and `rule-triggers decide`, and any write below Read.
  Generated writes send immediately, so the gate lives in your agent's policy.
- **Never let an agent run Account response, Credential or Destructive tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the partner key if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md). It reaches every
  customer.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the SaaS Alerts API, and you can read every line of how it does so. The workflow
commands are preview-first, the MCP server can be pinned read-only, and nothing leaves
your machine except calls to SaaS Alerts and the destinations you name.
