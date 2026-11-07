# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - 2026-11-07

### Changed
- Rebuilt on CLI Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).
- HTTP mode (`connectsecure-mcp --transport http`) is documented again, for reference. It refuses to start until `CONNECTSECURE_MCP_HTTP_TOKEN` is set to at least 32 characters, every request must send that token as a bearer header, any address other than loopback also needs `--tls-cert` and `--tls-key`, and browser origins other than loopback are refused unless listed in `CONNECTSECURE_MCP_HTTP_ALLOWED_ORIGINS`.
- New `local-data purge` previews and, with `--yes`, erases the local store, response cache and learn journal. `CONNECTSECURE_NO_LOCAL_DATA=1` runs every read live and writes nothing locally.
- `--csv` prefixes `'` on cells that a spreadsheet would run as a formula. `--deliver file:` writes a private 0600 file; `--deliver webhook:` does not follow redirects.
- `sync` no longer mirrors three report-builder resources (standard report settings, the default template download and the standard report list). None was in the default sync set or read by an analysis command.

### Fixed
- `report-queries registry-problems-remediation-asset-details` reads the rows its own sync writes. Rows saved by an earlier version are not migrated; the next sync of that report fills them in.
- Active Directory computer rows are keyed by `object_guid`, so two clients' computers with the same name stay separate rows in the local store.

### Security
- Credentials are handled by the shared msp-skills credential kit: every output, file, local-store write, MCP result and outbound request that would carry a credential this program holds (the client secret, the API key pair, a pre-minted or freshly minted access token, a saved token, a configured header) is withheld whole (a `[credkit: withheld from ...]` notice), and a credentialed request goes only over https to the configured pod. With no pod configured, no credential is sent anywhere.
- MCP file uploads take the file as base64 content. A path is accepted only when `CONNECTSECURE_MCP_UPLOAD_ROOT` names a folder, and only for files inside it.

## [0.1.4] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `connectsecure-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `connectsecure-cli agents create` ran `connectsecure-cli agents reset`, POST `/w/company/reset_agents`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `connectsecure-cli agents reset` (POST `/w/company/reset_agents`)
  - `connectsecure-cli patching patch-now` (POST `/w/company/patch_now`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.1] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

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
