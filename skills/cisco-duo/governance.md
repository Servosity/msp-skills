# cisco-duo skill - governance and safety model

> Unofficial. Community-built skill for the Cisco Duo Admin API. Not affiliated with,
> endorsed by, or sponsored by Cisco Systems, Inc.
> This page tells an MSP owner exactly what the cisco-duo skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cisco-duo-cli` binary (and `cisco-duo-mcp`) as a **Duo Admin API
application**: `CISCO_DUO_API_HOST` (the API hostname), `CISCO_DUO_IKEY` (integration
key) and `CISCO_DUO_SKEY` (secret key). The secret key is used only to compute a Duo v5
HMAC-SHA512 signature for each request; it is never sent to Duo or anywhere else.
Credentials come from the environment or the CLI's own config file.

For an MSP, create the application in the **parent** account with subaccount
permissions. `--account-id` (or `CISCO_DUO_ACCOUNT_ID`) then routes any command to one
customer: the CLI looks the subaccount up in the parent's own list, sends the request to
that customer's API host, and keeps a separate local store per customer. `fleet summary`,
`fleet risk` and `fleet find-user` always cover every subaccount.

## Default-safe behavior

- **The MFA fixes preview by default.** `lockout fix`, `phone-swap` and
  `enrollment chase` read Duo and print the plan; nothing changes until you add
  `--apply`. `lockout fix` refuses to unlock anything but a `locked out` user, refuses an
  administrator hold (a Trust Monitor lock, a lock set through the Admin API, a
  disabled user, or a user in the Trash) without `--force`, and reports a Disabled group that overrides the user
  instead of working around it.
- **Bypass codes are narrow.** A code from `lockout fix --bypass` is single-use, expires
  after one hour by default (`--valid-secs`), and is created with
  `preserve_existing=true` so earlier codes are not wiped.
- **Generated write commands send immediately.** `users update`, `phones delete` and the
  other endpoint commands have no preview of their own, and `--yes` does not gate them;
  pass `--dry-run` first to see the request without sending it.
- **A held user stays held.** For a user under an administrator hold or in a Disabled
  group, `lockout fix` issues no bypass code or phone reactivation and `phone-swap` adds
  no phone and sends no activation, unless you pass `--force`. `lockout triage` suggests
  none of them for that user (nor a quarantine release or an enrollment code). What it
  can still suggest grants no way through MFA by itself: the reviewed `--force` unlock,
  an unlock that the Disabled group still blocks, taking the user out of that group after
  review, or restoring a user from the Trash (Duo restores them as disabled).
- **A shared phone is not activated.** `lockout fix --reactivate-phone` and `phone-swap`
  refuse, without `--force`, a phone that other users share or whose users Duo did not
  confirm: Duo Mobile activation binds the phone, and so every user on it, to whichever
  device completes it.
- **Read commands are always safe to run** (triage, audits, fleet views, logs, search);
  they cannot change anything. `accounts list` is a POST in Duo's API but only reads.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any
  write gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previews**; require a human for
anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Diagnosis, audits, rollups, search. No change. | `lockout triage`, `fleet summary`, `fleet risk`, `fleet find-user`, `audit bypass`, `audit stale`, `failures`, `sync`, `search`, `export`, every `list` / `get`, `accounts list`, and `lockout fix` / `phone-swap` / `enrollment chase` **without** `--apply` | Allow |
| **Write (routine)** | Day-to-day changes that grant no access. | `users send-verification-push`, `phones send-sms-installation`, `groups create`, `tokens resync`, `identity-verification start` / `cancel` | Preview with `--dry-run` (or the command's own preview), then a reviewed write |
| **MFA access / credential** | Gives someone a way through MFA. | `lockout fix --apply` (unlock, bypass code, reactivation), `phone-swap --apply`, `users create-bypass-codes`, `phones create-activation-url`, `phones send-sms-activation`, `phones send-sms-passcodes`, `phones set-quarantine-status` (releasing quarantine lets a device authenticate again), `users add-phone`, `users add-token`, `phones create`, `phones update` (a new number redirects SMS and phone-call factors), `users bulk-create` (can create users with status bypass), `groups delete` (deleting a Disabled group lifts its hold on every member), `directory-sync sync-user` (re-syncing a directory user can re-enable them, attach phones and send enrollment), `import <resource> --input file.jsonl` (an import can set any field a create can, status bypass included), `users enroll` (emails a single-use enrollment code to any address you give and returns the code to the caller), `enrollment chase --apply` (emails an enrollment link to each unenrolled user's address on file; skips users in a Disabled group), `users create` / `users update` (either can set a user to bypass), `users add-group` / `users remove-group` / `groups update` (a Bypass or Disabled group overrides a user's own status), `registered-devices block-device` / `unblock-device` | Human-in-the-loop only, after verifying the caller through a separate channel |
| **Destructive** | Irreversible loss of a user or factor. | `users delete`, `users bulk-trash`, `users remove-phone`, `users remove-token`, `phones delete`, `tokens delete`, `bypass-codes delete`, `webauthn delete`, `desktop-authenticators delete`, `phone-swap --delete-old` | Human-in-the-loop only, explicit confirmation |
| **Admin / billing** | Administrators, billing and whole customer accounts. | `admins create` / `update` / `delete` / `reset` / `clear-inactivity`, `accounts create`, `accounts delete` (removes a whole customer subaccount), `billing set-edition`, `billing set-user-limit`, `billing set-telephony-credits` | Operator-only, not for agents |
| **Bulk write** | Many writes from one call. | `users bulk-restore` (Duo restores users as disabled) | Human-in-the-loop only, explicit confirmation. Never unattended |

The MFA access tier is the one that matters most. A help-desk caller who says "I'm locked
out" may be the attacker the MFA was there to stop, so an agent must never issue a bypass
code, an activation link or a new phone on the strength of a ticket alone. Verify the
caller first (`users send-verification-push` sends a Duo Push to a device they already
have; a call-back to a number on file also works), then have a human approve the plan.

## How to lock it down

- **Scope the Admin API application.** This is the strongest control. Read-only work
  needs `Grant resource - Read`, `Grant read log` and `Grant read information`. As an
  MSP, `--account-id` and the fleet commands also need `Grant accounts - Read` (the CLI
  looks each customer up in the parent's subaccount list), and `fleet summary`'s
  user-limit column needs `Grant user limits - Read`. Without `Grant resource - Write`
  the application cannot unlock, issue codes or touch phones at all. Grant
  `Grant administrators - Write`, `Grant accounts - Write` and `Grant user limits - Write`
  only to an application a human operates.
- **Run the MCP server read-only for agents that only diagnose.** Set
  `CISCO_DUO_MCP_READ_ONLY=1`: the server hides `lockout fix`, `phone-swap`,
  `enrollment chase`, `sync`, `import` and `export`, and refuses every non-GET call made
  through its API tool.
- **Keep autonomous agents to Read and previews.** The gate for a real change lives in
  your agent's policy, not in the binary's defaults.
- **Never let an agent run the MFA access, Destructive or Admin tiers unattended.**
  Treat them like a production database drop: human, reviewed, logged.
- **Rotate the secret key if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0), including the request signer
(`cli/internal/client/duo_signer.go`). You supply the Admin API application, the binary
signs each request with it, and you can read every line of how it does so. The skill is
read-first, previews every MFA fix by default, and is scoped to the permissions you
grant.
