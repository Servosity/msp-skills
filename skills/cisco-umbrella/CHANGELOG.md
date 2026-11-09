# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release of the Cisco Umbrella CLI and MCP server, built with CLI
  Printing Press 4.34.2: every operation in Cisco's published Umbrella v2 API (admin,
  deployments, policies, reports, investigate, app discovery, managed and provider
  customers), with a local store for offline search.
- MSP child orgs: `--org <child org ID>` (or `CISCO_UMBRELLA_ORG_ID`) mints a token for one
  customer from a parent console key and keeps that customer's synced data in its own store.
- `fleet health`: one row per managed customer with roaming computers off, uninstalled,
  stale or without web protection, virtual appliance health and tunnels down.
- `hygiene`: every deployment problem in one customer as a typed finding, from the local store.
- `verdict`: a domain's category, risk score and security status, every destination list
  that blocks or allows it (parent-domain entries included) and who resolved it.
- `allowlist audit`: allow-list entries now categorized as malicious, allow and block
  conflicts, and recent allows.
- `destination-lists diff`: the plan to make a destination list match a file or an inline
  `--entries` list; `--apply` makes the changes in 500-entry batches.
- `report-card`: a customer's summary, top threat types and blocked categories against the
  prior period, with deployment status.

### Security
- Credentials go only over HTTPS to the configured Umbrella API host or
  `https://api.umbrella.com`; a request, redirect or token URL aimed at any other host is
  refused, and API responses are not cached.
- Built on the shared credential kit. Output that repeats a credential the program holds,
  plain or encoded, is replaced whole by
  "[credkit: withheld from <sink>: it carried a credential this program holds]", and a write
  to a file, the local store or the learn journal that repeats one is refused.
- The first API call with environment credentials saves the key, the secret and the bearer
  token to `credentials.toml` in the data directory (owner-only, not encrypted). Later runs
  use them after the variables are unset, until `auth logout` deletes the file.
- Credential-named response fields print as "[redacted]".
- `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no secret of
  your own and `tunnels update-credentials` with auto-rotate or no secret of your own are
  refused before anything is sent, over the CLI and MCP alike: Umbrella returns that new
  secret only once and this program withholds it, so the only copy would be lost. Do them in
  the Umbrella dashboard, where the secret is shown.
- Over MCP every write to Umbrella previews until `confirm: true` (local learn-store tools
  change only this machine), a preview changes nothing on this machine, and no tool argument can name a local file. Console logo uploads are CLI-only: no
  MCP tool takes a file.
- `auth login`, `auth set-token` and `auth logout` honour `--dry-run`, and `export --dry-run`
  leaves its output file alone.
