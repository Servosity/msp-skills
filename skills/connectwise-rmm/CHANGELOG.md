# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
