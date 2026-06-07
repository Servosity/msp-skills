# appdirect skill - governance and safety model

> Unofficial. Community-built skill for the AppDirect API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the appdirect skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `appdirect-cli` binary (and `appdirect-mcp`),
authenticating with `APPDIRECT_CLIENT_ID` and `APPDIRECT_CLIENT_SECRET` (OAuth2
client_credentials). `APPDIRECT_OAUTH_SCOPE` is optional - the binary defaults to
`ROLE_PARTNER ROLE_PARTNER_READ` and only sends a different scope if you set one.
Credentials are read from the environment only - never written to disk, never
logged, never sent anywhere except the AppDirect marketplace API. Point at a
white-label marketplace with `APPDIRECT_BASE_URL`.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything. This includes the cross-entity views (`reconcile`, `payments
  unpaid`, `subs changed`, `company show`, `pipeline`) and a handful of API
  endpoints that POST a query body but only return data (for example
  `checkout get-pricing-summary`, `checkout preview-shopping-cart`).
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line. Tiers are derived from the binary's
real HTTP method per command (GET/HEAD = read; POST/PUT/PATCH = write;
DELETE/expire = destructive), not guessed from the command name - so a write that
happens to be named `finalize-opportunity` or `upload-and-link-image` is correctly
gated, not waved through as a read.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, and every GET. No change. | `payments unpaid`, `reconcile`, `subs changed`, `company show`, `pipeline`, `search`, and all 160+ read endpoints | Allow |
| **Write (routine)** | Day-to-day mutations: create/update companies, users, memberships, groups, and opportunities; invite users; request a purchase; apply a discount; finalize or clone an opportunity. | `account resource-company-create-company-post`, `account resource-company-invite-users-post`, `account resource-company-request-purchase-post`, `assisted-sales add-items`, `assisted-sales apply-discount`, `assisted-sales finalize-opportunity`, `products upload-and-link-image`, ... (118 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / financial** | Touches passwords and payment instruments / methods (non-delete writes). | `account resource-user-set-temporary-password-put`, `app-market create-payment-method`, `app-market create-payment-method-token`, `app-market set-default-payment-method`, `app-reseller resource-v1-payment-method-api-post`, `billing resource-other-create-payment-instrument-post`, `billing resource-other-update-payment-instrument-put` (7 total) | Human-in-the-loop only |
| **Destructive** | Irreversible data or access loss: every delete, subscription cancellations, account expiry. | `account resource-company-delete-company-membership-delete`, `account resource-group-delete-delete`, `account resource-subscription-delete-subscription-assignment-delete`, `billing resource-subscription-cancel-subscription-delete`, `app-market delete-payment-method`, `appdirect-sync resource-developer-account-expire-developer-account-post`, ... (34 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers; the
  default `ROLE_PARTNER_READ` scope is a good starting point for read-only agents.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults. Remember Write includes money-moving operations
  like `request-purchase` and discount changes.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the AppDirect API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
