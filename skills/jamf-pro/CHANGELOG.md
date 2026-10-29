# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - unreleased

### Added
- Initial msp-skills release: Jamf Pro CLI (`jamf-pro-cli`) and MCP server
  (`jamf-pro-mcp`) covering the Jamf Pro API and the Classic API: inventory, policies,
  scripts, packages, prestages, groups, patch management and MDM commands.
- A registry of client instances (`instance add`, `list`, `test`, `remove`): each client's
  server, API client ID and the name of the variable holding its secret.
- Reports across every client, read in parallel with each client's own credential:
  `fleet find`, `fleet posture`, `fleet patch`, `fleet app`, `fleet policy-hygiene`,
  `fleet health`, `fleet baseline` and `fleet exec`.
- A local SQLite mirror per instance for offline search (`sync`, `search`).
- `doctor` requests a token and reads the Jamf Pro version, so it tells a working API
  client from a rejected one and names the variable that fixes a missing server.

### Security
- Credentials go only to the configured Jamf Pro server; requests and redirects to any
  other host are refused, and a saved token or secret stays with the server it was saved
  for.
- Endpoints that return LAPS passwords, FileVault keys, recovery-lock passwords and lock
  PINs are not included. Password, key, token and PIN fields in other responses are
  redacted, and a downloaded document that carries a credential is withheld whole.
- Any output that repeats a held credential, in any encoding, is withheld whole by the
  shared credential kit: on the CLI, in MCP results, in files, in local store statements
  and in requests.
- MCP tools that change Jamf Pro return a preview and act only with `confirm: true`.
