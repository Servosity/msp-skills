# abnormal skill - governance and safety model

> Unofficial. Community-built skill for the Abnormal Security API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the abnormal skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `abnormal-cli` binary (and `abnormal-mcp`),
authenticating with `ABNORMAL_API_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Abnormal Security API.
The EU host is selected by setting `ABNORMAL_BASE_URL` (the US base URL is the default).

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (triage, reporting, threat/case/vendor/employee lookups, search); they cannot change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line. Note the auto-classifier's keyword pass
is corrected by hand here: `security-settings` is a read-only GET (it only looked like a
write because "settings" contains "set"), and the email/threat remediation commands - which
delete or move real mail and act on live threats - are pulled up into the destructive tier.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Triage, reporting, lookups, search. No change. | `triage`, `report-snapshot`, `threats retrieve`, `cases retrieve`, `vendors`, `vendor-cases retrieve`, `employee-risk`, `vendor-risk`, `aggregations …`, `auditlogs`, `roles`, `users`, `security-settings`, `email-search search-create` (POST-based search), `spm-v2 postures-query-create` (POST-based query) | Allow |
| **Read - token metadata** | Lists API tokens by metadata only (`token_id`, `name`, `status`, `created_at`, `expires_at`, `permissions`); does **not** return secret token values. | `soar` | Allow, but treat the inventory itself as sensitive |
| **Write (routine)** | Day-to-day, reversible mutations. | `cases create` (update a case's status), `detection360 reports-create` (report a misclassification), `api-resources resources-create-create`, `api-resources resources-update-partial-update`, `api-resources resources-actions-create` | Preview with `--dry-run`, then an approved write |
| **Remediation / destructive** | Acts on live mail and threats; can move or delete messages. | `threats create` (remediate/unremediate a threat), `email-search search-remediate-create` (delete / move / submit-for-review messages), `remediate-watch` (remediate and confirm), `import` (bulk create/upsert) | Human-in-the-loop only, explicit confirmation |

There is no Admin tier exposed by this API surface - detection policy, integration
settings, and tenant configuration live in the Abnormal portal, not in this CLI.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a token that can run the Remediation / destructive tier.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for the Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent remediate, move/delete mail, or bulk-import unattended.**
  Treat `threats create`, `email-search search-remediate-create`, `remediate-watch`,
  and `import` like a production change: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Abnormal Security API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
