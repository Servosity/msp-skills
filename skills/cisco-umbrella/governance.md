# cisco-umbrella skill - governance and safety model

> Unofficial. Community-built skill for the Cisco Umbrella API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the cisco-umbrella skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cisco-umbrella-cli` binary (and `cisco-umbrella-mcp`),
authenticating with an Umbrella API key and secret (`CISCO_UMBRELLA_API_KEY`,
`CISCO_UMBRELLA_API_SECRET`). It exchanges them for a short-lived bearer token at
`https://api.umbrella.com/auth/v2/token`.

Any command that calls the API with those credentials (`doctor`, `sync`, `fleet health`, any
live read or write, any MCP tool that does) saves the key, the secret and the bearer token to
`credentials.toml` in the data directory (`doctor` prints it), as `auth login` and
`auth set-token` do. The file is owner-only (0600) and not encrypted, and later runs use it
after the variables are unset. `auth logout` deletes it; while the variables stay set, the
next API call saves them again. A key named on the run itself (`CISCO_UMBRELLA_API_KEY`, or the config file the run loads) wins over a different key, bearer or `auth set-token` token saved by an earlier run, so naming another customer's key reaches that customer.
`--dry-run`, `--data-source local` and `auth status` do not
write it. Beyond that file, only a `config.toml` you put a credential in holds one.

With `--org <child org ID>` (or `CISCO_UMBRELLA_ORG_ID`) from a parent MSP or
Multi-org console key, it mints a token for that one customer, keeps it in memory only, and
keeps that customer's synced data in its own local store.

Credentials go only over HTTPS, and only to the configured Umbrella API host or
`https://api.umbrella.com`: a request, redirect or token URL aimed at any other host is
refused, and so is plain http. Output that repeats a credential the program holds, plain or
encoded, is replaced whole by
"[credkit: withheld from <sink>: it carried a credential this program holds]"; a write to a
file, the local store or the learn journal that repeats one is refused. Credential-named
fields in API responses (`clientSecret`, `clientId`, `secretAccessKey`, `psk`, `token`, a
string-valued `key` and the like) are replaced by "[redacted]" first, so a listing stays
readable. API responses are not cached.

Umbrella returns a newly generated secret only once and cannot show it again, and this
program withholds it, so the only copy would be lost and a refresh would break whatever uses
the key now. Five actions are therefore refused before anything is sent, over the CLI and
MCP alike: `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no
secret of your own, and `tunnels update-credentials` with auto-rotate or with no secret of
your own. Do them in the Umbrella dashboard, where the secret is shown. `--dry-run` still
previews them.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** CLI write commands send immediately unless you pass
  `--dry-run` first to preview the request without sending. `auth login`, `auth set-token`
  and `auth logout` honour it too: nothing is sent, saved or deleted. Make your agent's
  policy: preview, show the exact command, get approval, then run the write.
- **The MCP server previews every write to Umbrella.** Over MCP such a write returns its
  preview until the call carries `confirm: true`, and local learn-store tools (`teach`,
  `learnings_*`, `playbook_*`) run with no preview and change only this machine; `CISCO_UMBRELLA_MCP_READ_ONLY=1` removes writes entirely.
  A preview changes nothing on this machine: `destination_lists_diff` with `apply` runs
  nothing until `confirm: true` and returns its arguments (the same call without `apply`
  returns the plan).
- **Read commands are always safe to run** (reports, rollups, search, `fleet health`,
  `verdict`, `allowlist audit`, `destination-lists diff` without `--apply`); they cannot
  change anything.
- **`destination-lists diff --apply` adds and removes entries with no prompt.** Run it
  without `--apply` first to read the plan; add `--no-remove` to only add. A source with no
  entries is refused: to empty a list on purpose, use the Umbrella dashboard.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `fleet health`, `hygiene`, `verdict`, `allowlist audit`, `report-card`, `destination-lists diff` (without `--apply`), `activity`, `reports`, `investigate`, every `list` and `get`, `sync`, `search` | Allow |
| **Write (routine)** | Day-to-day changes. | `destination-lists add-destinations`, `destination-lists create`/`update`, `destination-lists diff --apply`, `networks`, `sites`, `internal-networks`, `internal-domains`, `network-devices` `create`/`update`, `tags create`/`add-to-devices`, `application-lists create`/`update`, `policies update-policy-identities`, `swg-device-settings set`, `roaming-computers update`, `virtual-appliances update`, `tunnels update`, `managed-customers` and `provider-customers` `create`/`update`, `console-config create-*`/`update-*`, `import` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Changes a credential, or sets one you chose. | `api-keys update`, `tunnels add` and `tunnels update-credentials` with a secret you chose, `password-resets`, `users create`, `auth login`, `auth set-token` | Human-in-the-loop only |
| **Refused here** | Umbrella would return a new secret once, and this program withholds it. | `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no secret of your own, `tunnels update-credentials` with auto-rotate or no secret of your own | Do it in the Umbrella dashboard, where the secret is shown |
| **Destructive** | Irreversible data or config loss. | `api-keys delete`, `destination-lists delete`/`remove-destinations`, `networks`, `sites`, `internal-networks`, `internal-domains`, `network-devices` `delete`, `roaming-computers delete`, `virtual-appliances delete`, `tunnels delete`, `tags remove-from-devices`, `users delete`, `managed-customers delete`, `provider-customers delete`, `application-lists delete`, `console-config delete-*`, `policies delete-policy-identities`, `swg-device-settings remove`, `local-data purge` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none beyond the tiers above) | Operator-only, not for agents |

## How to lock it down

- **Scope the API key** to only what your workflow needs. Umbrella scopes are per key
  (Reports, Deployments, Policies, Investigate, Admin) and each can be read-only. A
  reporting workflow needs read-only Reports and Deployments, not Admin.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential or Destructive tier commands unattended.** Treat
  them like a production database drop: human, reviewed, logged.
- **Rotate the key if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Cisco Umbrella API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
