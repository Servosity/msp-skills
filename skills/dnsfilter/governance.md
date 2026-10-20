# dnsfilter skill - governance and safety model

> Unofficial. Community-built skill for the DNSFilter API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the dnsfilter skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `dnsfilter-cli` binary (and `dnsfilter-mcp`) with a DNSFilter
API token you create in the DNSFilter dashboard (Account Settings, API Tokens). The
token is sent as the bare value of the `Authorization` header, with no `Bearer`
prefix, to `https://api.dnsfilter.com` (override with `DNSFILTER_BASE_URL`). It needs
`DNSFILTER_API_KEY`.

**The token is the permission boundary.** The CLI can do whatever that token can do
in the dashboard. A token created in your MSP organization reaches every client
organization under it; one created in a client organization reaches only that client.

Where the token lives is your choice, and the CLI respects it:

- **Supplied through the environment** (a secret manager, Keychain, or a
  session-scoped variable): the token is **never copied to disk**.
- **Saved with `auth set-token`** (the token is piped on stdin, so it never appears
  in process arguments or shell history): it is written to a permission-restricted
  credentials file in your user config directory (`0600` on macOS and Linux; on
  Windows the file's ACL is checked to allow only you, Administrators and SYSTEM).
  Run `dnsfilter-cli doctor` to see the resolved paths, and `auth logout` to clear it.

The token is never logged and goes only to the API base URL. Two commands send data
elsewhere only when you ask: `--deliver webhook:<url>` posts a command's output to
that URL (the MCP server does not accept `--deliver`), and `feedback --send` (or
`DNSFILTER_FEEDBACK_AUTO_SEND`) posts your feedback note to
`DNSFILTER_FEEDBACK_ENDPOINT`, which is unset by default.

**What is stored locally.** `sync` mirrors DNSFilter into a local SQLite database
(organizations, sites, policies, roaming clients, unblock requests and the other
default resources), and GET responses are cached on disk for a few minutes to save
repeat calls. Secret fields DNSFilter returns are not written to either: each site's
`secret_key` (in the networks lists) and API key `token` values are removed before a
row reaches the mirror or its full-text index, and a response that carries one is
never cached. The commands that fetched them still print them to you. `api-keys` is
not part of the default sync.

## Default-safe behavior

- **Generated commands run immediately.** Write commands (create, update, delete,
  add or remove a domain from a policy list, an unblock decision) run as soon as
  they are invoked and do not prompt. `--dry-run` is opt-in: it prints the request it
  would send, without sending. Make your agent's policy: preview, show the exact
  command, get approval, then run the write.
- **`allow` previews by default.** Without `--confirm` it resolves the client's
  policies and prints the plan; nothing is written. `--confirm` also requires
  `--note` (the ticket number), refuses a domain in a security (threat) category or
  an unresolvable category unless you add `--force`, skips policies whose `can_edit`
  is not true, and leaves shared global policies alone unless you pass
  `--include-global`. After writing it re-reads each policy and exits 6 if any target
  failed.
- **The troubleshooting and fleet views only read.** `why-blocked`,
  `unblocks triage`, `agents drift`, `agents stale`, `lists where` and `policy-audit`
  change nothing in DNSFilter. `agents stale`, `agents drift` and `lists where` read
  the local mirror; `why-blocked`, `unblocks triage` and `policy-audit` read live.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.
- **Saved profiles never authorize a write.** `--confirm`, `--force` and `--yes` are
  never applied from a saved `--profile` (a warning names the ignored flag), and
  `profile save` refuses to store them; a profile that carries `--dry-run` keeps a
  command in preview.

## The MCP server

Over MCP an agent sees the troubleshooting and fleet tools (`why_blocked`,
`unblocks_triage`, `agents_drift`, `agents_stale`, `lists_where`, `policy_audit`),
`allow`, the local `search` and `sql` tools, and a generic `dnsfilter_execute` tool
that can call **any** DNSFilter API endpoint, deletes included. A write through
`dnsfilter_execute` is sent immediately (there is no dry-run over MCP) and refuses
any parameter outside that endpoint's schema, so a mistyped `dry_run` fails instead
of running live. `import` is never offered over MCP, and MCP callers cannot pass
flags that read or write local files (`--input`, `--deliver` and the like).

**Read-only mode.** Set `DNSFILTER_MCP_READ_ONLY=1` in the MCP server's environment
for an agent that should only read. `true`, `yes` and any other value except `0`,
`false`, `no` or `off` also turn it on, so a typo never grants writes. In read-only
mode `allow`, `sync`, `export` and the archive tool are removed, `dnsfilter_execute`
runs GET endpoints only, the reads that return a secret (API key reads, the site
secret-key endpoints, the roaming client uninstall PIN) are refused, and networks
reads come back with each `secret_key` replaced by `[withheld: credential]`, so the
agent still sees site names and ids. The same switch is enforced inside the HTTP client of the
companion `dnsfilter-cli` the MCP server runs, so no route can send a write or a
credential read while it is on.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Troubleshooting, fleet views, lists and gets. No change. | `why-blocked`, `unblocks triage`, `agents drift`, `agents stale`, `lists where`, `policy-audit`, `allow` without `--confirm`, `organizations index`, `policies index`, `policies show`, `user-agents index`, `user-agents tags`, `unblock-requests index`, `traffic-reports query-logs`, `domains user-lookup`, `search`, `sync` | Allow |
| **Filtering policy and allow lists** | Changes what clients can reach. One allow, a weaker policy or permissive mode opens a site for every site and roaming client on that policy. | `allow --confirm`, `policies add-whitelist-domain`, `policies remove-whitelist-domain`, `policies add-blacklist-domain`, `policies remove-blacklist-domain`, `policies add-blacklist-category`, `policies remove-blacklist-category`, `policies bulk-actions-add-allowlist-domains`, `policies update`, `policies permissive-mode policies-update`, `policies add-allowed-application`, `scheduled-policies update`, `unblock-requests decision unblock-requests`, `block-pages update` | Preview (`allow` without `--confirm`, or `--dry-run`), then a human-approved write |
| **Write (routine)** | Day-to-day changes that do not change filtering. | `networks create`, `networks update`, `organizations create`, `organizations update`, `user-agents update`, `ip-addresses create`, `mac-addresses create`, `scheduled-reports create`, `dashboards create`, `notes domain-update` | Preview with `--dry-run`, then a reviewed write |
| **Credential and access** | Returns, creates, rotates or revokes a secret, or grants access. | `networks index`, `networks show`, `networks all`, `networks msp`, `networks msp-all` (rows carry each site's secret key), `networks secret-key networks-generate`, `networks secret-key networks-rotate`, `networks secret-key networks-revoke`, `api-keys index`, `api-keys show`, `api-keys create`, `api-keys revoke api-keys`, `user-agents uninstall-pin`, `users change-password`, `distributors users-reset-password-url`, `organizations users organization-create`, `psa-integrations` (a GET that creates a PSA integration and returns its link), `auth set-token` | Human-in-the-loop only |
| **Destructive and commercial** | Irreversible deletes, roaming client removal, billing and distributor changes. | `import` (one create per JSONL line, so it can reach any tier above), `policies destroy`, `networks destroy`, `networks bulk-destroy`, `organizations destroy`, `organizations cancel`, `user-agents destroy`, `agent-local-user-bulk-deletes create`, `api-keys destroy`, `billing create`, `billing address-update`, `distributors organizations-cancel`, `distributors msps-update`, `trials` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the token** to what your workflow needs. A read and troubleshooting
  workflow does not need a token that can delete organizations or rotate site keys.
- **Run the MCP server read-only for agents that should only read**
  (`DNSFILTER_MCP_READ_ONLY=1`), and keep your AI app's approval prompt on for
  everything else.
- **Keep autonomous agents to Read plus previewed writes.** Have a human approve the
  actual write for the policy tier and above - the gate lives in your agent's policy,
  not in the binary's defaults (except `allow`, which never writes without
  `--confirm`).
- **Never let an agent run Credential or Destructive tier commands unattended.**
  Treat them like a production database drop: human, reviewed, logged.
- **Rotate the token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md), and rotate a site's
  secret key with `networks secret-key networks-rotate` if a networks list ever
  leaves your machine.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the token, the binary uses it against the
DNSFilter API, and you can read every line of how it does so. The troubleshooting
views read first, `allow` plans before it writes, and secret fields stay out of the
local mirror.
