# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-10-16

### Fixed
- `firebox devices changes` no longer drops a fleet snapshot when two runs land in the
  same clock tick. Windows' clock can return one reading to two fast calls; the second
  snapshot collided with the first and was silently not recorded. A tie now steps the
  timestamp forward, while a genuinely stale run is still refused. Found by the new
  Windows CI job.

## [0.1.0] - 2026-10-16

### Added
- Initial msp-skills release: `watchguard-cli` and `watchguard-mcp` for 208 operations
  from 13 of WatchGuard's published OpenAPI specifications - Firebox Management and
  Reports, FireCloud, WatchGuard Endpoint Security, ThreatSync, NDR assets and Smart
  Alerts, AuthPoint authentication, and the platform's accounts, operators, activations,
  license allocation and authorization APIs. Authenticates with a WatchGuard Cloud Access ID, its password and the
  API key, in the North America, EMEA or APAC region (`WATCHGUARD_BASE_URL`,
  `WATCHGUARD_TOKEN_URL`).
- Service Provider fan-out: `accounts health` (one ranked row per customer across
  Firebox, endpoints, ThreatSync and licenses), `endpoint-devices posture` and
  `endpoint-patches exposure` mint a separate token for each managed account where
  WatchGuard requires one, so one credential reaches every customer;
  `threatsync-incidents queue` ranks open incidents across them, and `allocations
  reconcile` matches allocations to the Fireboxes that report in.
  `WATCHGUARD_AUDIENCE` sends every request with one customer's token (a convenience, not
  a security boundary).
- Change control and fleet history: `firebox-transactions wait` exits 0 when a
  deployment completes, 8 when it fails, times out or is canceled, and 9 when it is still
  running at `--max-wait`;
  `firebox-devices changes` and `firmware-lag` compare synced snapshots of the Firebox
  and access point fleet.
- Secret handling: BOVPN pre-shared keys, certificate private keys and passwords,
  Firebox user, PPPoE, Dynamic DNS and WebBlocker override passwords, Wi-Fi passphrases,
  activation keys and installer enrollment links are never written to the local mirror
  or the HTTP response cache. `--dry-run` never requests a login token, and region URLs
  or an account ID set in the environment are never saved into `config.toml`.
- Credential safety: a cached token is reused only by a process with the same Access ID,
  scope and token URL, so a read-only Access ID never rides a read-write token; `auth
  status` and `doctor` agree on whether this process can authenticate, and `doctor`
  verifies the credential with an authenticated read of your account. Cross-origin
  redirects drop the Authorization and `WatchGuard-API-Key` headers and never replay a
  write, `--deliver webhook:` never follows a redirect, and an API error that echoes a
  secret you sent is withheld instead of printed.
- MCP server: `WATCHGUARD_MCP_READ_ONLY=1` refuses every non-GET call; `tail` and
  `import` are not MCP tools, and no MCP tool takes a local file path for the CLI to read.
