# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - unreleased

### Added
- Initial msp-skills release: `mimecast-cli` and the `mimecast-mcp` MCP server
  for Mimecast API 2.0 (OAuth2 client credentials), built on CLI Printing Press
  4.34.2.
- `why`: one verdict for one message (held, rejected at the gateway, released or
  rejected from hold, delivered, bounced or not found) with the policy, reason,
  spam score, SPF/DKIM/DMARC and who released or rejected it.
- Hold-queue workflow: `held triage` groups the queue by reason, policy or sender
  domain with a release-risk class and ready-to-act ID batches (`--new` shows only
  what arrived since the last run); `held release-safe` checks the hold reason
  before releasing and records the ticket in a local ledger; `held purge-sender`
  rejects every held message from one sender, optionally blocking it, and only
  plans until `--yes`; `held release-audit` flags risky releases with no ticket.
- Phishing response: `ttp who-clicked` lists who clicked a raw or
  Mimecast-rewritten URL; `threats mailbox` shows every threat event for one
  mailbox; `threats sender` lists every recipient one sender or domain reached,
  who clicked and what is still held.
- `mailflow`: one OK/WARN/FAIL verdict for queue backlog, inbound gaps and stuck
  deliveries, with a monitor-friendly exit code; `senders suggest` proposes narrow
  permits and blocks from cached release-log history.
- Every covered API 2.0 endpoint (hold queue, message tracking, TTP, DLP, audit,
  archive logs, gateway, journaling, managed senders, Cloud Integrated policies
  and remediation) as a typed command.
- Delegate access: `MIMECAST_ACCOUNT_CODE` or `--account-code` selects one
  customer account on the classic endpoints; REST endpoint writes are refused
  while it is set.

### Security
- Built on the shared credential kit: credentials are held from every source
  before any output, and every output path (terminal, MCP results, errors, local
  files, the local store) withholds a unit that carries a held value in any
  encoding the credential leak contract lists. An answer that carries a
  credential, such as a journaling connection's password, is withheld whole and
  never stored.
- A request or redirect that would carry a credential to any host other than the
  configured Mimecast API and token hosts is refused.
- Every MCP tool that changes Mimecast previews until called with
  `confirm: true`; `MIMECAST_MCP_READ_ONLY=1` refuses every write.
- A Mimecast HTTP 200 answer that carries a `fail[]` entry is reported as a
  failure, not a success.
