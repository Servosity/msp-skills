# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - unreleased

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.4] - 2026-11-07

### Security
- Rebuilt on CLI Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)). The MCP server's HTTP mode refuses to start until `WATCHGUARD_MCP_HTTP_TOKEN` is set (32 characters or more), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert` and `--tls-key` on any other address. `watchguard_execute` previews every state-changing call until `confirm` is the JSON boolean `true`. `WATCHGUARD_NO_LOCAL_DATA` is now declared on every install channel next to `WATCHGUARD_MCP_HTTP_TOKEN` and `WATCHGUARD_MCP_READ_ONLY`.
- A managed account's scoped token is sent only to the origin it was minted for, and a scoped token request refuses a plaintext (non-loopback `http://`) token URL.
- The CLI and MCP server now run on the shared credential kit: every credential the program holds (the Access ID, API password and API key, the minted and per-account bearer tokens, saved profile values) is withheld from terminal output, files, the local store, the learn journal and MCP results, and is sent only to the configured WatchGuard Cloud API over https. `auth login --dry-run` no longer prints the Access ID.
- Saved credentials pinned to an IPv6 link-local address are used only on the same zone: an address whose zone differs only in case is a different host.

### Added
- `local-data purge` previews, then (with `--yes`) erases the local store, response cache and learn journal. `WATCHGUARD_NO_LOCAL_DATA=1` runs with no local copy: reads go to the API and `sync` is blocked.
- `--ca-cert` / `WATCHGUARD_CA_CERT` adds a PEM CA bundle, and `--tls-server-name` / `WATCHGUARD_TLS_SERVER_NAME` sets the name checked against the server certificate.

### Changed
- Sync partitions follow the 4.35.1 names: the per-account operator list is `rest_platform_operator_mgmt_v1_operators_by_account_id`, and account and Firebox template rows are kept in the generic resources table. The seven Firebox exception lists are keyed on each exception's `id`, and an account's children on `accountId`.

### Fixed
- `accounts health` reads Firebox and access point expiry per device from the allocated-inventory summary. A license row that carries no expiry evidence is counted in `licenses_expiry_unknown`, marks the account incomplete and exits 6; `allocations reconcile` lists such rows in `expiry_unknown`.
- `accounts health` and `allocations reconcile` take a row's expiry from the earliest of `allocationExpiryDate`, `licenseExpiryDate` and `daysUntilExpiry`, and report which field it came from in `expiry_field`. A present but unreadable expiry value makes the row expiry-unknown.

## [0.1.3] - 2026-10-23

### Security
- **Token requests compare redirect hosts exactly.** The check that keeps a token request on its server case-folded IPv6 interface names, so a redirect to fe80::1%LAN0 passed for fe80::1%lan0. IP addresses and their interface names now must match exactly.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `watchguard-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `watchguard-cli endpoint-devices create` ran `watchguard-cli endpoint-devices action`, POST `/rest/endpoint-security/management/api/v1/accounts/{accountId}/devices/action`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `watchguard-cli endpoint-devices action` (POST `/rest/endpoint-security/management/api/v1/accounts/{accountId}/devices/action`)
  - `watchguard-cli firebox-exceptions disable-global` (POST `/rest/firebox/management/v1/commands/{accountId}/global_exceptions/disable`)
  - `watchguard-cli firebox-templates subscribe` (POST `/rest/firebox/management/v1/configuration/{accountId}/templates/subscribe`; subscribes or unsubscribes Firebox templates, replacing device configuration)
  - `watchguard-cli threatsync-actions perform` (POST `/rest/threatsync/management/v1/{accountId}/actions`)
  - `watchguard-cli threatsync-incident-actions perform` (POST `/rest/threatsync/management/v1/{accountId}/incidents/{incidentId}/actions`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

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
