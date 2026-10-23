# cloudally skill - governance and safety model

> Unofficial. Community-built skill for the CloudAlly API. Not affiliated with,
> endorsed by, or sponsored by CloudAlly or OpenText.
> This page tells an MSP owner exactly what the cloudally skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cloudally-cli` binary (and `cloudally-mcp`). Every request to
the CloudAlly API (`https://api.cloudally.com`) carries three things:

- **An API application key pair.** `CLOUDALLY_CLIENT_ID` and `CLOUDALLY_CLIENT_SECRET`,
  sent as the `client-id` and `client-secret` headers. They are created in the CloudAlly
  partner portal under Integrations > REST API and identify the integration, not a user.
- **A bearer access token from one of two logins.**
  - `cloudally-cli auth login` signs in as **one company account**
    (`CLOUDALLY_EMAIL` / `CLOUDALLY_PASSWORD`). Every command is then limited to that
    one company. This is the default.
  - `cloudally-cli auth login --partner` signs in to the **partner portal**
    (`CLOUDALLY_PARTNER_EMAIL` / `CLOUDALLY_PARTNER_PASSWORD`). That token can see and
    change every customer account, and it is the one the `distributor-*` and
    `reseller-*` commands need. From it, `auth use-account <accountId>` mints a
    customer-scoped token (`POST /v2/distributors/accounts/{id}/partner-access-token`,
    or the reseller path with `--reseller`) so account-scoped commands act inside that
    one customer; `auth use-account --partner` restores the partner token.
  Tokens are saved to `credentials.toml` in the CLI's data directory and refreshed
  automatically. `CLOUDALLY_ACCESS_TOKEN` overrides the saved token when set; leave it
  unset.

Credentials are never logged and never sent anywhere except CloudAlly. **Two commands
write a credential to disk:** `auth login` (and `auth use-account`, which swaps in a
customer-scoped token) saves tokens to `credentials.toml`, and `auth set-token` saves a
token you paste. Run `cloudally-cli doctor` for the paths.

## Default-safe behavior

- **Previews before writes.** Every command that calls the API takes `--dry-run`, which
  prints the request without sending it. Use it before any write, restore or delete
  and show the human what would be sent.
- **The documented export endpoint creates nothing.** `POST .../recovery/export` (the
  generated `distributor-recovery export`, `reseller-recovery export` and
  `recovery export`) answers 200 with an empty body and creates no job on the live API
  (seen 2026-10-15). The commands exist because they are in CloudAlly's API reference;
  do not rely on them for downloads.
- **`--dry-run` previews API calls only.** It does NOT guard local writes:
  `auth set-token --dry-run` still saves the token, and `profile save`, the `teach*`
  commands and `learnings forget` still write local files.
- **MCP read-only mode.** `CLOUDALLY_MCP_READ_ONLY=1` makes the MCP server hide tools
  that could change CloudAlly state and refuse non-GET execute calls. The tools it keeps
  still write local state: the response cache and learning entries.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but never implies
  `--yes`. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **the Read tier**. Everything below it
needs a human, and the Destructive tier needs a human who has read the command twice.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports and lookups. No change to CloudAlly. Local writes only: responses are cached, `sync` fills the local store, and `export --output` or `--deliver file:<path>` overwrites the file you name. | Every `list`, `get`, `details`, `bills`, `tasks`, `status`, `backup-statuses` and `secondaries` read under `distributor`, `distributor-accounts`, `distributor-resellers`, `distributor-account-tasks`, `reseller`, `reseller-accounts`, `reseller-account-tasks`, `tasks` and `user`; `sync`, `search`, `export` | Allow |
| **Write** | Creates jobs or changes backup settings; reversible. | `backup-now`, `backup-entity-now`, `activate`, `activate-all`, `activate-entity`, `pause`, `pause-entity`, `add-entity`, `cancel`, `cancel-entity` under `tasks`, `distributor-account-tasks` and `reseller-account-tasks`; `distributor-accounts create`, `reseller-accounts create`, `distributor-resellers create`, `create-account`, `update`; `user create-primary`, `create-secondary`, `update-settings`, `update-permissions`; `distributor create-user`, `reseller create-user`, `distributor-resellers create-user`; `distributor-recovery export`, `reseller-recovery export`, `recovery export` (documented, but create nothing on the live API) | Human-in-the-loop |
| **Restore (writes into live customer data)** | Writes backed-up items back into a live mailbox, site or drive. | `distributor-recovery restore`, `reseller-recovery restore`, `recovery restore` | Human-in-the-loop; run `--dry-run` first |
| **Destructive** | Deletes backed-up data or a customer account. Not reversible. | `tasks delete`, `distributor-account-tasks delete`, `reseller-account-tasks delete` (`DELETE .../backup-tasks/{id}`: deletes the backup task and its data; to stop a running job use `cancel`, never `delete`); `remove-entity` under the same three groups (removes an account or site and its data); `distributor-accounts unsubscribe`, `reseller-accounts unsubscribe`, `distributor-resellers unsubscribe-account`, `user delete` (cancel the subscription and delete the CloudAlly account); `distributor delete-user`, `reseller delete-user`, `distributor-resellers delete-user` | Human only; never on an agent's own initiative |
| **Credential** | Mints, saves, swaps or clears a token. | `auth login`, `auth login --partner`, `auth use-account`, `auth set-token`, `auth logout`, `session login`, `session partner-login`, `session refresh`, `distributor-accounts access-token`, `distributor-accounts sign-in-token`, `reseller-accounts access-token`, `reseller-accounts sign-in-token`, `user update-password`, `import` | Human-in-the-loop only |
| **Data egress** | Sends data to a destination you choose. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL), `feedback --send`, and bare `feedback` when `CLOUDALLY_FEEDBACK_ENDPOINT` is set and `CLOUDALLY_FEEDBACK_AUTO_SEND=true` (with no endpoint set, `feedback` only writes a local file) | Human-in-the-loop; a webhook sink moves client data off your machine |
| **MCP execute** | Calls any endpoint of the CloudAlly spec by ID. | The MCP `cloudally_execute` tool: each call falls in the tiers above, including the Destructive ones, and every non-GET call is refused while `CLOUDALLY_MCP_READ_ONLY=1` | Human-in-the-loop unless `CLOUDALLY_MCP_READ_ONLY=1` |

## How to lock it down

- **Prefer the company login.** `auth login` (one company account) cannot reach any
  other customer. Use `auth login --partner` only on the machine and for the session
  that needs partner-wide work.
- **Run the MCP server with `CLOUDALLY_MCP_READ_ONLY=1`** unless the agent is meant to
  start backups or restores.
- **Keep the token file private.** `auth login` saves tokens in `credentials.toml` in
  the CLI's data directory; do not sync it to a shared drive. The partner token reaches
  every customer.
- **Never let an agent run Destructive, Restore, Credential or Data-egress tier
  commands unattended.**
- **Rotate the API key pair if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md; the HTTP transport
  refuses to start until `CLOUDALLY_MCP_HTTP_TOKEN` is set).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the CloudAlly API key pair and login, the
binary uses them against your own CloudAlly partner or company account, and you can
read every line of how it does so, including the account switch in
[`cli/internal/cli/auth_use_account.go`](./cli/internal/cli/auth_use_account.go).
