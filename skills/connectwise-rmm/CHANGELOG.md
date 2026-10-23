# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error, an HTML page returned in place of JSON or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **Token requests compare redirect hosts exactly.** The check that keeps a token request on its server case-folded IPv6 interface names, so a redirect to fe80::1%LAN0 passed for fe80::1%lan0. IP addresses and their interface names now must match exactly.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `connectwise-rmm-cli tasks create` ran `connectwise-rmm-cli tasks schedule`, POST `/api/platform/v1/automation/endpoints/schedule-tasks`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `connectwise-rmm-cli tasks schedule` (POST `/api/platform/v1/automation/endpoints/schedule-tasks`)

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
- Initial msp-skills release: a ConnectWise RMM (Asio) CLI and MCP server covering 66
  partner API operations - companies, sites, endpoints, patching, custom fields,
  policies, hypervisors, scripts, tasks, and Asio service tickets.
- Local SQLite mirror (`sync`, `search`) of companies, sites, endpoints and tickets, so
  cross-client questions run against a local copy.
- Fleet roll-ups: `fleet stale` (offline and silent agents across every client),
  `fleet patch-posture` (clients ranked by patch risk), `fleet reboot-due` (the
  patch-week reboot queue), `fleet whois` (which machine a user is logged on to),
  `fleet brief` (one endpoint's card), `fleet scorecard` (one client's per-site health
  card), and `fleet task-failures` (failed script runs grouped by identical error).
- `run` resolves a script and target endpoints by name, previews the request, and only
  schedules with `--confirm`; `--wait` collects per-endpoint results.
- `scopes` finds which OAuth scopes your API key was granted and prints the scope
  setting to use, since one ungranted scope fails the whole token request.
- Region follows the client ID prefix (North America, Europe, Australia); tokens are
  minted with the JSON body ConnectWise requires.
- `doctor` mints a token and verifies it against an authenticated endpoint instead of
  reporting the credential as present but unchecked.
