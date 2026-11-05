# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-05

### Changed
- Describe the changes in this release.

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
