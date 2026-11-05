# mimecast skill - governance and safety model

> Unofficial. Community-built skill for the Mimecast API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the mimecast skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `mimecast-cli` binary (and `mimecast-mcp`) with a Mimecast
API 2.0 application: `MIMECAST_CLIENT_ID` and `MIMECAST_CLIENT_SECRET` are
exchanged for a 30-minute bearer token, which is minted again when it expires.
The application acts with the products and the role you gave it in the
Administration Console, so a read-only role cannot release, reject or block
anything. `MIMECAST_ACCOUNT_CODE` (Mimecast delegate access) selects one customer
account on the classic endpoints; while it is set the CLI refuses writes to the
REST endpoints, which have no account selector.

Credentials are read from the environment (or the 0600 credential file written by
`auth login` or `auth set-token`), are never written to logs or the local store,
and are sent only to the configured Mimecast API and token hosts: a request or
redirect that would carry a credential to any other host is refused. An answer
that carries a credential (for example a journaling connection with its SMTP or
POP3 password) is withheld whole from output and never stored.

## Default-safe behavior

- **`held release-safe` checks before it releases.** It refuses impersonation,
  malware, sandbox, phishing and policy holds, and IDs not found in the hold
  queue, unless `--override` gives a reason, and records every decision with the
  ticket in a local ledger. `--plan` shows the decision without releasing. Without
  `--plan` it releases the allowed IDs, so preview first.
- **`held purge-sender` plans by default.** It lists the matching held messages
  and block pairs and changes nothing until you add `--yes`.
- **Other writes send when run.** Typed write commands (`held release`,
  `held reject`, `senders permit-or-block`, `ci-policies update`, `journaling
  delete`, ...) send immediately unless you pass `--dry-run` first. Make your
  agent's policy: preview, show the exact command, get approval, then run the
  write.
- **MCP writes need confirm.** Every MCP tool or endpoint that changes Mimecast,
  and every tool that writes local files, returns a preview until it is called
  again with `confirm: true`. Set `MIMECAST_MCP_READ_ONLY=1` and the MCP server
  refuses every write.
- **Read commands are always safe to run** (tracking, the hold queue, logs,
  `why`, `held triage`, `mailflow`, search); they cannot change anything in
  Mimecast. `held triage --new`, `held release-audit` and `senders suggest` write
  only a local snapshot or cache.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating, and does not imply `--yes`. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previews**; require a
human for anything below the line. Tiers follow each command's HTTP method in
`mimecast-cli agent-context`, not its name: many Mimecast reads are POSTs
(`held list`, `messages search`, the `archive-logs` and `ttp` logs), and
`archive-logs` and `held triage` are reads despite their names.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Tracking, hold queue, logs, verdicts. No change in Mimecast. | `why`, `held triage`, `held list`, `held find`, `held summary`, `held release-logs`, `held release-audit`, `senders suggest`, `mailflow`, `ttp who-clicked`, `ttp url-logs`, `ttp attachment-logs`, `ttp impersonation-logs`, `threats mailbox`, `threats sender`, `messages search`, `messages get`, `rejections`, `queues`, `processing`, `dlp`, `audit events`, `audit categories`, `archive-logs search-logs`, `archive-logs searches`, `archive-logs views`, `gateway details`, `gateway outbound-ips`, `gateway statistics`, `ci-policies get`, `ci-policies get-default`, `journaling list`, `journaling get-service`, `remediation status`, `remediation bulk-status`, `urls`, `sync`, `search`, `analytics`, `export` | Allow |
| **Write (routine)** | Day-to-day mail decisions and policy edits. | `held release-safe` (`--plan` first), `held release`, `held reject`, `senders permit-or-block`, `ci-policies create`, `ci-policies update`, `ci-policies update-default`, `files` (uploads an attachment), `import` | Preview first (`held release-safe --plan`; `--dry-run` elsewhere; MCP `confirm`), then an approved write |
| **Mail flow and sending** | Changes how the tenant's mail is routed, or sends mail as the tenant. | `email`, `gateway set-protection-mode`, `gateway set-outbound-ips`, `gateway update-details` | Human-in-the-loop only |
| **Credential / identity** | Any command that can emit a stored secret, and credential handling. | `journaling get` (returns SMTP and POP3 journaling passwords; the CLI withholds the whole answer when it carries one, but treat the command as credential tier), `messages get-file` (returns a pre-signed download link to a message file), `journaling create` and `journaling update` (take a journaling password), `auth login`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible loss of mail, configuration or local data. | `held purge-sender` (plans until `--yes`), `remediation remediate` and `remediation bulk` (MANUAL_DELETE removes messages from mailboxes), `ci-policies delete`, `gateway delete-outbound-ips`, `journaling delete`, `senders bulk-delete`, `senders bulk-delete-for-recipient`, `senders bulk-delete-internal`, `local-data purge --yes`, `jobs prune` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | `ci-onboarding` (onboards a Cloud Integrated customer account) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Mimecast API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
