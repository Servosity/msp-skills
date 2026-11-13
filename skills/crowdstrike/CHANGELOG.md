# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.16] - unreleased

### Fixed
- **The `crowdstrike_search` MCP tool's description now names this API's resources.** It used to be one sentence shared by every connector, so an MCP host that picks tools by matching words ranked other tools above it for a request like `<vendor> <resource>`. The description now ends with the resource families from the endpoint list, most endpoints first, at most 300 characters ([#629](https://github.com/Servosity/msp-skills/issues/629)).

## [0.1.15] - 2026-11-11

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.14] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.13] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.12] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.11] - 2026-11-05

### Security
- **Rebuilt on Printing Press 4.34.2 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode listens on `127.0.0.1:7777` by default, refuses to start until `CROWDSTRIKE_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback.
- **Every MCP write previews first.** A tool call that changes CrowdStrike (containing a host, updating alerts, editing policies, host groups or Flight Control groups) returns a preview and sends nothing until it is called again with `confirm` set to boolean true. A tool argument can never name a file to read or write.
- **The client ID and secret go only to your cloud's token endpoint.** Setting `CROWDSTRIKE_BASE_URL` to US-2, EU-1 or GovCloud now sends the token request to `/oauth2/token` on that same cloud (unless `CROWDSTRIKE_TOKEN_URL` names another), for `auth login`, the automatic sign-in, token refresh and `fleet sync`. The token request sends the client pair in the form body, as CrowdStrike documents.
- **Credentials stay in one place.** Only `credentials.toml` (mode 0600) holds a credential; a client pair supplied by the environment, and the token minted for it, are never written to disk. Responses are never cached on disk, and any output, file or MCP result that would repeat a credential this program holds is withheld.
- **Credentials are guarded by the shared credential kit.** Output, files, requests, SQL statements, child processes and MCP messages that carry a credential this program holds (the client pair, a minted token, the MCP HTTP token, or the password or query values of a URL setting) are withheld whole and replaced by a "[credkit: withheld ...]" notice, from before the first line is printed. A learn session label is stored only as a hash, and a mistyped flag name is never written to the learn journal.

### Added
- `CROWDSTRIKE_MCP_READ_ONLY` (preset to `1` in the Claude Desktop bundle and the install configs) makes the MCP server refuse every write. `CROWDSTRIKE_NO_LOCAL_DATA=1` keeps no local copy of API responses, and `local-data purge` erases the local store, cache and learn journal. Both switches are declared on every install channel.
- `sync` can also store Spotlight installed patches and vulnerabilities, and two MSSP member queries.

### Changed
- `CROWDSTRIKE_OAUTH_SCOPE` is optional and not a secret in every install prompt; leave it blank, CrowdStrike grants scopes to the API client in the Falcon console.
- The generic `sync` partitions now hold records: `devices` stores host records (keyed by device ID) instead of bare IDs, `incidents` stores CrowdScores and `policy` stores prevention-policy members. Run `sync --full` once after upgrading to replace the old ID rows. The `fleet` commands use their own tables and are unaffected.

### Fixed
- **`fleet sync` reads every page** of each tenant's hosts, alerts and vulnerabilities, and replaces a tenant's stored rows only after a read it can prove complete. A filtered read (`--vuln-filter`) or a failed, short or capped read adds and updates rows but deletes none, and a posture snapshot is recorded only when hosts, alerts and vulnerabilities all read completely.
- **Fleet rows from an older version no longer count as current.** Hosts, alerts, vulnerabilities and policies an older version stored for the authenticated CID are left out of every `fleet` rollup. `fleet sync` and each rollup print one line saying how many rows are excluded, and the next complete `fleet sync` of that kind replaces them.
- **Each child tenant uses its own token.** `fleet sync` keeps every child CID's member_cid token in memory and sends it only for that CID, so one tenant's data is never stored under another.
- `auth login` saves a token only beside the API client that minted it. When the credential store holds another client, a login from `FALCON_CLIENT_ID` and `FALCON_CLIENT_SECRET` now saves nothing and says how to store that client instead.
- `fleet sync` of the authenticated CID (no `--cids`, `--member-cid` or `--all-cids`) now removes the hosts, alerts, vulnerabilities and policies a complete read no longer returns. Those rows are tied to the API client that read them, so a sync never removes another client's rows from a shared `--db`.
- `fleet sync` reads each kind and each CID once, however it is named: a kind given twice, an alias such as `spotlight`, another letter case, or a CID listed twice no longer doubles the counts or the posture snapshot `fleet trend` reports.
- `doctor` checks `api.crowdstrike.com` as the real US-1 API instead of reporting it as an unset placeholder, and names `FALCON_CLIENT_ID` and `FALCON_CLIENT_SECRET` in its auth hint.

## [0.1.10] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **The token request no longer follows a redirect to another server.** `auth login` and the automatic sign-in sent the token request, with your client secret, on to whatever host a redirect from the token URL named. A redirect off the token server now fails instead.
- **`fleet sync --member-cid` no longer sends your client secret to another server.** The child-tenant sign-in followed a redirect from the token URL to any host and resent the token request, client secret included. It now uses the same token client as the other sign-ins, which stays on the token server.

### Fixed
- **`--dry-run` sends nothing, not even a token request.** With no saved token, a dry run used to request one first, sending your client ID and secret to the token URL and saving the token, although a dry run promises that no request is sent. `auth login --dry-run` now also only shows the request it would make, and `auth set-token --dry-run` and `auth logout --dry-run` no longer save or clear your stored credentials.

## [0.1.9] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.8] - 2026-10-22

### Security
- The MCP server no longer passes `fleet tenants --expected` to the CLI, so an MCP tool call cannot point it at a file on the machine that runs the server. The flag still works on the command line.

## [0.1.7] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.6] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

## [0.1.5] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.4] - 2026-09-01

### Added
- **`CROWDSTRIKE_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
  invocation instead of caching one in `config.toml`. The MCP server honours the same switch,
  which it previously could not: it resolved its own config path and ignored `--config`
  entirely, so a Claude Desktop setup kept a plaintext token cache even when the CLI had been
  told not to write one. `auth logout` still clears a config that already exists - the switch
  suppresses credential writes, not the erase.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CROWDSTRIKE_BASE_URL`, `CROWDSTRIKE_TOKEN_URL`.

## [0.1.1] - 2026-08-22

### Fixed
- `doctor`, the CLI's HTTP 400/401/403 error hints and the MCP server's tool-error
  hints no longer tell operators to run `crowdstrike-cli auth setup`. This CLI's `auth`
  parent defines `login`, `status`, `set-token` and `logout`; it has never defined
  `setup`. Cobra does not reject an unknown subcommand of a non-root parent, and the
  generated parent returns `cmd.Help()` with a nil error, so `crowdstrike-cli auth setup`
  printed the auth help and exited 0 - no "unknown command" error and no non-zero
  exit anywhere in the journey. An operator followed doctor's advice, saw a success
  code and was no closer to configured credentials; because these are agent-first
  CLIs with typed exit codes and an `--agent` mode, an agent performing setup read
  exit 0 as success and moved on unauthenticated. The hints fire exactly when they
  matter - fresh install, rotated secret, new machine, expired token - and this skill
  emits no `auth_instructions` field, so they were the only setup guidance it gave.
  `doctor` now names the variables `auth login` actually reads (`FALCON_CLIENT_ID` and `FALCON_CLIENT_SECRET`, plus
  `CROWDSTRIKE_OAUTH_SCOPE` when the instance needs a scope), and the
  CLI and MCP error paths point at `crowdstrike-cli auth login` to re-authenticate. Seven
  occurrences across three files; the same defect affected appdirect, crowdstrike,
  halopsa and ninjaone, the four connectors that scaffold `auth login` rather than
  `auth setup`. Reported by @geekbrownbear (#278).
- OAuth client credentials supplied through the environment (`FALCON_CLIENT_ID` and
  `FALCON_CLIENT_SECRET`) are no longer written to
  `~/.config/crowdstrike-cli/config.toml`. `Load()` marks env-supplied values so
  `configForSave()` can keep them off disk, but `SaveTokens()` cleared those
  markers before saving, so every token mint - including the automatic one on an ordinary
  authenticated command, with no `auth login` step involved - rewrote the live client ID and secret into
  the config file in cleartext. That defeats the setups this is meant to
  support, where the secret lives only in the OS credential store (Keychain,
  Windows Credential Manager) and reaches the process as an environment
  variable at launch. The minted access token is still cached there, and an
  explicit `auth login --client-id <id> --client-secret <secret>` still stores
  what you passed it. **Deliberate behaviour change:** credentials that came
  from the environment are no longer persisted by `auth login` either, so a
  shell that later drops those variables must set them again or pass the flags.
  Thanks to @Xenith-B for the report (#266).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `crowdstrike-cli` CLI and `crowdstrike-mcp` MCP server.
- Cross-tenant Flight Control fleet rollups over a CID-keyed local SQLite store:
  `fleet sync`, `fleet scorecard`, `fleet alerts`, `fleet vulns`, `fleet stale`,
  `fleet policy-drift`, `fleet remediate`, `fleet trend`, `fleet tenants`, `fleet search`.
- Per-CID Falcon coverage: alerts (modern Alerts API), devices/hosts, Spotlight
  vulnerabilities, prevention policies, and MSSP CID/user-group management.
- Offline sync to local SQLite with full-text `search` and `analytics`, plus
  agent-native JSON (`--agent`) and `--dry-run` preview on every command.
