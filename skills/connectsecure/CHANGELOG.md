# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-10-15

### Added
- Initial msp-skills release: `connectsecure-cli` and `connectsecure-mcp` for every
  ConnectSecure (formerly CyberCNS) API operation, authenticating per tenant pod by
  minting a short-lived token from the tenant name, client ID and client secret.
- Seven cross-client commands: `exposure` (open findings ranked across every client
  at asset level by EPSS, CVSS and known-exploited status), `portfolio` (one risk row
  per client), `blind-spots` (assets whose scan data cannot be trusted),
  `suppression-audit` (expired, open-ended and self-approved suppressions),
  `ad-hygiene` (risky Active Directory accounts and weak password policies),
  `cve-impact` (which clients a named CVE hits, with fixes) and `qbr` (one client's
  QBR risk pack).
- Local SQLite mirror: a bare `sync` reads a curated set of 15 reports; credential
  records, integration credentials, integration rules, scheduled jobs (whose rows carry
  a password field), attack-surface results (creds and hashes fields), custom domains
  (undocumented certificate details) and the agent uninstall secret are never written
  to the mirror or the HTTP response cache, and `workflow archive` skips them instead
  of aborting.
- Secret handling on the wire: `--dry-run` previews mask secret fields, an API error
  (or token-mint error) that echoes a secret you sent is withheld instead of printed,
  a request the server refuses with HTTP 200 and `"status": false` is reported as a
  failure (a refused read is never printed or cached as an empty result), and a write is never sent to another origin on a redirect (or reported
  as a success when a redirect turned it into a read).
- `qbr` and `portfolio` leave a client's risk score blank when another client shares
  its name, and `cve-impact` flags a client name several companies share: those
  ConnectSecure reports are keyed by company name only.
- A `sync` narrowed with `--param`, `--global-param` or `--resource-param` neither
  prunes other rows (even with `--full`) nor marks the resource fully synced.
- The MCP `tail` tool is not marked read-only (it can stream credential records), so
  hosts ask before running it and `CONNECTSECURE_MCP_READ_ONLY=1` hides it. `doctor`
  never writes to the response cache.
- MCP recipe tool `which_clients_does_this_cve_hit` refuses a flag-like value before
  the CLI runs, so a tool-call argument cannot smuggle a root flag such as
  `--deliver`.
