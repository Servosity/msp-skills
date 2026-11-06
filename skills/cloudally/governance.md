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
    `reseller-*` commands and the portal-backed commands need. From it,
    `auth use-account <accountId>` mints a customer-scoped token
    (`POST /v2/distributors/accounts/{id}/partner-access-token`, or the reseller path
    with `--reseller`) so account-scoped commands act inside that one customer;
    `auth use-account --partner` restores the partner token.
  Tokens are saved to `credentials.toml` in the CLI's data directory and refreshed
  automatically. The partner login also keeps its access and refresh token in
  `auth-context.json` in the CLI's state directory, so `auth use-account` can switch
  customers without a new password; `auth logout` deletes both files.
  `CLOUDALLY_ACCESS_TOKEN` overrides the saved token when set; leave it unset.
- **The customer portal, for what the documented API cannot do.** Past backup dates,
  download job creation and download links come from the customer portal's own service
  layer. The CLI reaches it the way the partner portal does when an operator clicks a
  customer: the partner token calls
  `POST /v2/distributors/accounts/{id}/partner-access-token`, which returns a
  single-use handoff token; the CLI spends it at `app.cloudally.com/newapp`, and the
  resulting portal session (a session cookie plus a CSRF token) lives only in memory
  for that one command. It is never written to disk.

Credentials are never logged and never sent anywhere except CloudAlly. Every request,
the portal session's included, goes through the shared credential kit. It holds the
key pair, the tokens, the passwords, the portal handoff token, the portal session
cookie and its CSRF token; it sends each one only to the origin it belongs to (the API,
and for the portal values `https://app.cloudally.com` or the API origin, so a portal
anywhere else is refused before anything is sent), redirects included; and it replaces any output, file or MCP result that would
carry one with `[credkit: withheld from <sink>: it carried a credential this program
holds]`. Two things stay outside it by design: a presigned download link is not held,
because showing it is what `download link` is for, and the archive bytes
`download fetch` streams to `--dest` are customer data written outside the kit (the
archive request carries no held value). **Three commands write a credential to disk:** `auth login` (and
`auth use-account`, which swaps in a customer-scoped token) saves tokens to
`credentials.toml`, `auth login --partner` also writes `auth-context.json`, and
`auth set-token` saves a token you paste. Run `cloudally-cli doctor` for the paths.

## Default-safe behavior

- **Only the planned commands wait for `--yes`.** `restore submit`,
  `download submit` and `download start` resolve and print exactly what they would
  create and stop unless `--yes` is passed, also under `--agent`, `--no-input` or a
  pipe. The generated API commands (`recovery restore` and its distributor and reseller
  variants, every `delete`, `remove-entity`, `unsubscribe` and `delete-user`) send their
  request at once with no confirmation; `--dry-run` is their only preview, which is why
  the tiers below mark them human-only.
- **Restores never pick a different backup than the one planned.** `restore plan`
  resolves each mailbox to its newest successful backup at or before the requested
  date from the portal's backup calendar (partner login plus `--account`; under a
  company login it can only use the latest backup). A mailbox with no such backup is a
  `NO_BACKUP` row and is never submitted. `restore submit` records every row in a local
  ledger before the request leaves, and `--resume` always sends with the target account
  and restore options recorded on the batch; a conflicting flag is refused.
- **A restore is sent once per state directory.** `restore submit` claims each restore
  (backup task, mailbox, resolved backup) in a guard, `restore-guard.db` in the CLI
  state directory, before its request leaves. The guard is shared by every login and
  token that uses that state directory (partner, `auth use-account`, company login,
  `CLOUDALLY_ACCESS_TOKEN`, `auth set-token`) and holds across date spellings, scope
  changes, entity lists and concurrent runs. A run that would send a restore already
  accepted, unknown or in flight is refused before anything is sent; only a restore the
  API refused before running it can be sent again. `--new-batch` is the only override:
  it keeps the earlier record and sends again. Another OS user, `--home`,
  `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` is a different state directory and so a
  separate guard; run restores from one account and one state directory. Ledgers
  written by pre-release builds are not imported into the guard: while the data
  directory holds a restore ledger with a batch the guard has no record of, `restore
  submit` refuses before any request (exit 2) and names the file, to be checked with
  `restore history` and moved away by a person; if the ledger directory or any ledger in
  it cannot be read, the run sends nothing. One backup reached two ways (the portal calendar and the task's last-backup
  dates can differ by seconds or minutes) counts as one restore: anything within 30
  minutes for the same task and mailbox is refused, because the CLI cannot tell a
  re-reported time from a separate on-demand backup. The refusal names both times; a
  person who has checked sends the other backup with the printed `--entities <mailbox>
  --new-batch` command. Agents must not run it on their own. The generated
  single-restore `recovery restore` commands and the MCP `cloudally_execute` tool are not
  behind the guard.
- **An archive is created once per state directory.** `download submit` and
  `download start` claim each archive (account, task, mailbox, sub-source, snapshot, format, include flags) in
  `download-guard.db` before creating it, so overlapping or concurrent batches never
  create the same export twice; `--new-batch` is the only override. A download batch
  ledger written by a pre-release build (one the guard has no record of) is not
  imported: while one is in the batch folder, `download submit` and `download start`
  refuse before any request (exit 2) and name the file, to be checked with
  `download list` and moved away by a person; `download fetch --batch` still reads it.
- **Downloads only ever create EXPORT jobs.** The portal endpoint the download
  commands use (`startRestoreOrDownload`) also accepts `RESTORE` (writes into the live
  mailbox) and `DELETEITEMS` (deletes from the backup). The CLI only ever sends
  `EXPORT`: the job type is a constant in its portal package, never taken from a flag,
  a config value or a caller, and a test asserts the type on the created job. Download archives stay in CloudAlly's own
  storage; the CLI never sends one to a bucket you did not choose.
- **Download links are short-lived and never stored.** A link is a presigned URL that
  is valid for about 60 seconds. `download fetch` mints and streams each one in the
  same step and writes the archive to a `.partial` name until it is complete. Do not
  paste a link into a ticket or chat; the customer can collect finished downloads in
  the CloudAlly portal (their account name > Jobs > Download results).
- **The documented export endpoint is not used.** `POST .../recovery/export` (the
  generated `distributor-recovery export`, `reseller-recovery export` and
  `recovery export`) answers 200 and creates no job on the live API. The commands
  exist because they are in CloudAlly's API reference; they change nothing.
- **`--dry-run` previews API calls only.** It does NOT guard local writes:
  `auth set-token --dry-run` still saves the token, and `profile save`, the `teach*`
  commands and `learnings forget` still write local files.
- **MCP read-only mode.** `CLOUDALLY_MCP_READ_ONLY=1` makes the MCP server list only
  read-only tools: `cloudally_execute`, `restore_submit`, `download_submit`,
  `download_start`, `download_link`, `sync`, `export`, `workflow_archive`, `jobs_prune`
  and the learn-store writers are gone, and `cloudally_read` refuses every
  state-changing endpoint. The tools it keeps can still write the learn journal and
  create an empty local store. No MCP tool takes a local file path (`--plan-out`,
  `--plan-file`, `--entities-file`), and `download fetch`, `import` and `local-data purge`
  are CLI-only.
- **No local copy.** `CLOUDALLY_NO_LOCAL_DATA=1` keeps API responses off the disk: reads
  go to the API, the response cache is off, and `sync` and `workflow archive` refuse to
  run. `local-data purge --yes` erases what is already there; it keeps config and
  credentials.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but never implies
  `--yes`. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **the Read tier**. Everything below it
needs a human, and the Destructive tier needs a human who has read the command twice.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports and lookups. No change to CloudAlly. Local writes only: responses are cached, `sync` fills the local store, and `export --output` or `--deliver file:<path>` overwrites the file you name. `restore plan`, `backup snapshots`, `download list` and `download watch` mint a single-use partner handoff token to open a portal session for each run. | `backup health`, `backup snapshots`, `restore plan`, `restore watch`, `restore history`, `download list`, `download watch`; every `list`, `get`, `details`, `bills`, `tasks`, `backup-statuses` and `secondaries` read under `distributor`, `distributor-accounts`, `distributor-resellers`, `distributor-account-tasks`, `reseller`, `reseller-accounts`, `reseller-account-tasks`, `tasks` and `user`; `sync`, `search`, `export` | Allow |
| **Sensitive read** | Returns something that grants access to customer data. | `download link` (a 60-second presigned URL to a mailbox archive; no confirm); `download fetch` (CLI only, no confirm; writes complete mailbox archives, owner-only on macOS and Linux, to the local disk; `--plan-only` previews) | Human-in-the-loop; never paste a link anywhere |
| **Write** | Creates jobs or changes backup settings; reversible. | `download submit`, `download start` (EXPORT jobs only; each needs `--yes`, and without it prints the plan and exits 2; `--new-batch` creates every export of the run again and is a human decision); `backup-now`, `backup-entity-now`, `activate`, `activate-all`, `activate-entity`, `pause`, `pause-entity`, `add-entity`, `cancel`, `cancel-entity` under `tasks`, `distributor-account-tasks` and `reseller-account-tasks`; `distributor-accounts create`, `reseller-accounts create`, `distributor-resellers create`, `create-account`, `update`; `user create-primary`, `create-secondary`, `update-settings`, `update-permissions`; `distributor create-user`, `reseller create-user`, `distributor-resellers create-user`; `distributor-recovery export`, `reseller-recovery export`, `recovery export` (documented, but create nothing on the live API) | Human-in-the-loop |
| **Restore (writes into live customer data)** | Writes backed-up items back into a live mailbox, site or drive. | `restore submit` (needs `--yes`; without it, prints each mailbox and its backup and exits 2; `--new-batch` sends accepted restores again and is a human decision); `distributor-recovery restore`, `reseller-recovery restore`, `recovery restore` (no confirm; send at once) | Human-in-the-loop; run `restore plan` first |
| **Destructive** | Deletes backed-up data or a customer account. Not reversible. | `tasks delete`, `distributor-account-tasks delete`, `reseller-account-tasks delete` (`DELETE .../backup-tasks/{id}`: deletes the backup task and its data; never point it at a download job id, finished download jobs cannot be deleted); `remove-entity` under the same three groups (removes an account or site and its data); `distributor-accounts unsubscribe`, `reseller-accounts unsubscribe`, `distributor-resellers unsubscribe-account`, `user delete` (cancel the subscription and delete the CloudAlly account); `distributor delete-user`, `reseller delete-user`, `distributor-resellers delete-user` | Human only; never on an agent's own initiative |
| **Credential** | Mints, saves, swaps or clears a token. | `auth login`, `auth login --partner`, `auth use-account`, `auth set-token`, `auth logout`, `session login`, `session partner-login`, `session refresh`, `distributor-accounts access-token`, `distributor-accounts sign-in-token`, `reseller-accounts access-token`, `reseller-accounts sign-in-token`, `user update-password`, `import` (CLI only; one create per JSONL record: accounts, users, resellers) | Human-in-the-loop only |
| **Data egress** | Sends data to a destination you choose. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL), `feedback --send`, and bare `feedback` when `CLOUDALLY_FEEDBACK_ENDPOINT` is set and `CLOUDALLY_FEEDBACK_AUTO_SEND=true` (with no endpoint set, `feedback` only writes a local file) | Human-in-the-loop; a webhook sink moves client data off your machine |
| **MCP execute** | Calls any endpoint of the CloudAlly spec by ID. | The MCP `cloudally_execute` tool: each call falls in the tiers above, including the Destructive ones, and the tool is not listed while `CLOUDALLY_MCP_READ_ONLY=1` | Human-in-the-loop unless `CLOUDALLY_MCP_READ_ONLY=1` |

## How to lock it down

- **Prefer the company login.** `auth login` (one company account) cannot reach any
  other customer. Use `auth login --partner` only on the machine and for the session
  that needs partner-wide work, past backup dates or downloads.
- **Run the MCP server with `CLOUDALLY_MCP_READ_ONLY=1`.** Through MCP,
  `restore_submit`, `download_submit` and `download_start` only preview: the server
  exposes no `--yes`. Review the plan, then submit from a terminal with the CLI. With
  read-only off, the only MCP tool that changes CloudAlly is `cloudally_execute` (for
  example `recovery.restore`), which skips the plan, the ledger and the guard and sends
  a write as soon as the caller passes `confirm=true`, so approve each call yourself.
- **Keep the token files private.** `credentials.toml` (data directory) and
  `auth-context.json` (state directory) hold the tokens; do not sync either to a shared
  drive. The partner token reaches every customer.
- **Synced rows and the restore ledger are kept per CloudAlly user.** Each partner user,
  each `auth use-account` customer and each company login has its own local store, so a
  restricted partner user never reads the administrator's synced customer data. The
  download batch ledgers, `download-guard.db` and `restore-guard.db` are shared by every login in the state
  directory. If `auth-context.json` cannot be
  trusted, the restore commands stop instead of opening an empty store.
- **Treat downloaded archives as customer data.** `download fetch` writes complete
  mailbox archives to the directory you name, readable only by your user on macOS and
  Linux; on Windows they inherit that folder's permissions, so pick a private folder.
  Store and delete them under the same policy as the mailboxes themselves.
- **Never let an agent run Destructive, Restore, Credential or Data-egress tier
  commands unattended.**
- **Rotate the API key pair if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md; the HTTP transport
  refuses to start until `CLOUDALLY_MCP_HTTP_TOKEN` is set).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the CloudAlly API key pair and login, the
binary uses them against your own CloudAlly partner or company account, and you can
read every line of how it does so, including the portal handoff in
[`cli/internal/portal`](./cli/internal/portal).
