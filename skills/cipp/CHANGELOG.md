# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.13] - unreleased

Rebuilt on CLI Printing Press 4.34.0.

### Changed
- **MCP HTTP mode needs a token.** `cipp-mcp --transport http` serves `http://127.0.0.1:7777/mcp` by default, refuses to start until `CIPP_MCP_HTTP_TOKEN` is set, answers only requests that send `Authorization: Bearer <token>`, and needs `--tls-cert`/`--tls-key` for any address other than loopback. stdio stays the default.
- **MCP writes are previewed.** `cipp_execute` returns a preview of any call that changes something and sends it only when the agent passes `confirm: true`. That includes every CIPP endpoint called with GET but named as an action (43 in this build: `ExecRunBackup`, `RemoveStandard`, `ExecSendOrgMessage`, ...); `cipp_read` refuses them. A write CIPP answers with HTTP 200 and a failure inside `Results` comes back from `cipp_execute` as an error.
- **Read-only switch.** `CIPP_MCP_READ_ONLY=1` makes the MCP server drop its write tools. Claude Desktop shows it as the **Read-only MCP mode** setting.
- **Local data you can see and remove.** `cipp-cli doctor` prints where the store, response cache and learn journal live; `cipp-cli local-data purge` lists them and `--yes` deletes them; cached responses expire after five minutes and are deleted on a later run. `CIPP_NO_LOCAL_DATA=1` (Claude Desktop: **Run without a local copy**) keeps no local copy of CIPP data: reads go live, the cache is off, and `fanout --save` and `fanout --resume` refuse.
- **`auth set-token` reads the token from stdin** (`echo "$TOKEN" | cipp-cli auth set-token`) instead of an argument, so it stays out of shell history.
- The `sync` command is gone; `cipp-cli fanout ... --save` fills the local store, as before.
- MCP library `github.com/mark3labs/mcp-go` v0.57.0.

### Fixed
- Search finds short all-caps values such as `WEB`, `SQL` and `DNS` again ([#464](https://github.com/Servosity/msp-skills/issues/464)). The local search index rebuilds once, the first time a command opens the store read-write after upgrading (for example `cipp-cli fanout ... --save`); the MCP server opens the store read-only, so run one such command first.
- **`posture --dimension mfa` reads the right data.** It now counts from the rows `fanout` saves from `/ListMFAUsers`, which carry MFA registration. Earlier builds read `/ListUsers`, which has no MFA fields, so their MFA numbers were wrong. Run `cipp-cli fanout --endpoint /ListMFAUsers --all-tenants --save` before `posture --dimension mfa`.
- `fanout --save` replaces a tenant's stored rows in one step instead of keeping rows CIPP no longer returns, unwraps CIPP's `Results` envelope, and keeps the stored rows when CIPP answers with a failure message. Rows are stored per tenant, so two tenants with the same id (license SKUs, for example) no longer overwrite each other.
- `fanout`, `export` and `tail` accept only `List*` and `Get*` read endpoints.
- `fanout --save` refuses a tenant's response when anything in the response reads as a failure: failure wording or a failed state anywhere under a status-like field (state, status, result, message, text, detail, reason and the like, at any depth: `"status":{"code":"error"}`) or in a bare message, a populated field named like error, exception or fault (`"error":"HTTP 403"`, whatever the wording), a field named like success or ok that is `false`, or a failed state, in the rows or beside them, and keeps the rows already stored. It stores only a list (an array, or `Results` holding an array) whose every entry carries the endpoint's id; JSON `null`, an empty answer, a single object or a message is shown, not stored. In the tenant list, and only there, tenant records keep their own Graph token fields (`LastGraphError`, `GraphErrorCount`), so one tenant with a failed Graph token does not stop the fleet. A response with a duplicate key is refused. A row whose own name holds a failure word (a user named "Error Reporting", a group named "Denied Access") is still a row. The trade-off: a list whose rows report their own failed status is refused too; run the same `fanout` with `--json` to see what CIPP returned. The tenant list and `gdap expiring` refuse any entry that is not a tenant or relationship instead of working from a partial list.
- `bulk` is a command-line tool only; the MCP server no longer offers it.
- A successful response that repeats a configured credential (the API key, the client secret or the minted token) has it masked before it is shown, cached or saved. The check reads the whole response with zero-width and other invisible characters removed, and then reads every value again with all spaces, tabs and line breaks removed too, so a credential cut up by them (`WVdG aFlX ...`) is still found. A single value over 64 KiB once spaces are removed, or a response that is not JSON and is over 64 KiB, withholds the response, because it cannot be decoded whole. A response over 1 MiB is also searched along its whole length for every encoded form of each credential and withheld only if one turns up, so large reads such as a big tenant's `/ListUsers` still come back.
- A retry after a rate-limit wait re-checks the token and mints a new one if it has expired.
- A `bulk --resume` checkpoint belongs to one CSV file's exact contents, so a new or edited batch starts from its first row instead of skipping rows an earlier batch finished.
- A failed token request shows only the OAuth error code and any Azure AD error numbers (AADSTS...); the rest of what the token endpoint sent is withheld. A token response that cannot be read is described by what went wrong ("could not be decoded"), never by its text. A token that arrives already expired is refused. `auth login` checks its connection errors for the client secret in encoded forms too.
- `bulk` and `cipp_execute` recognize more of CIPP's failure wording inside an HTTP 200 (Access denied, Forbidden, Insufficient privileges, not found, invalid input), and read the whole response, not only `Results`; a response with a duplicate key is a failure. A populated field named like error, exception or fault counts as a failure whatever its wording, and so does a field named like success or ok that is `false`. A result that carries both failure and success wording or state is reported as an error and is never resent automatically: `bulk` marks the row needs-review (keeping only hashes and row numbers, never CIPP's response) and skips it on later runs of the same CSV, and refuses to start when that CSV's checkpoint file, or the folder for a new one, cannot be written.

### Docs
- The README, docs page and skill notes no longer say CIPP lacks fleet-wide MFA, drift, stale-account or license views, or that its own MCP server is limited to one tenant: CIPP's All Tenants views and its MCP server cover those. They now describe what this connector adds (a local copy an agent can join and compare offline, and CIPP's writes behind a preview) and where that copy lives on disk.

## [0.1.12] - 2026-10-23

### Fixed
- `--json` turns on JSON output on `add-intune-reusable-setting-template` again: the command declared its own `--json` value flag for the body field. That field is now `--json-field`.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers the token endpoint errors of `cipp-cli auth login` too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.11] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.10] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.9] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.8] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.1.7] - 2026-09-28

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first for this connector to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)
- No connector code changed in this release; it exists so the published bundle and
  binaries pick up the release-pipeline fixes above.

## [0.1.6] - 2026-09-10

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

## [0.1.5] - 2026-09-01

### Added
- **`CIPP_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
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

## [0.1.4] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **Credential handling hardened.** The token exchange refused to run over plain HTTP (the
  request body carries the client secret); a vendor error message can no longer echo the secret
  back; an absurd `expires_in` no longer wraps into the past and re-mint on every call; an
  existing config file has its permissions repaired to owner-only instead of keeping whatever
  it had; and a token that cannot be written to disk is reused for the rest of the process
  rather than re-minted on every single request.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.3] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://your-cipp-instance.example.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `CIPP_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CIPP_BASE_URL`.

- **The connector stopped working after one token lifetime.** `auth login` cached the access token and
  its expiry, but nothing ever read that expiry and no 401 triggered a refresh, so roughly an hour later
  every call failed until you re-ran `auth login` by hand. The client now refreshes from the client
  credentials it already persisted. If a refresh fails while the cached token is still valid it warns and
  carries on rather than breaking a working install, and the token exchange refuses cross-host redirects
  so the client secret cannot be replayed to a redirect target.

  Existing installs need one `cipp-cli auth login` to record the tenant id the refresh needs; until then
  the connector says so explicitly instead of failing with an opaque 401.

## [0.1.2] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.1] - unreleased

### Fixed
- **Cross-tenant fan-out found zero tenants on current CIPP builds.** `/ListTenants`
  now returns its array inside a `{"Results": [...]}` envelope; tenant enumeration
  only understood a bare array, so it treated the whole envelope as a single
  tenant, dropped it for having no domain, and reported an empty fleet against a
  healthy CIPP. Envelope responses are now unwrapped, so `--all-tenants` fan-out
  and `posture` see the whole fleet again.
- **`sync` pointed at a remedy that cannot work.** CIPP's API is tenant-scoped end
  to end and exposes no bulk-list endpoints, so `sync` is a structural no-op here.
  Its warning suggested `--resources`, which always errors "unknown sync resource";
  it now names the mechanism that actually populates the store:
  `cipp-cli fanout --endpoint <endpoint> --all-tenants --save`.

Thanks to Abhi Saini at Bearium Networks, who found both while running the
connector against a live CIPP tenant.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `cipp-cli` CLI + `cipp-mcp` MCP server for the
  CIPP (CyberDrain Improved Partner Portal) Microsoft 365 multi-tenant API.
- **Cross-tenant fan-out** (`fanout`): run one read across every client tenant
  with throttle-aware backoff, optional persistence, and resume-after-halt.
- **Cross-tenant posture matrix** (`posture`): one table of every tenant's MFA,
  Conditional Access, Standards, and BPA posture.
- **License waste reconciler** (`licenses waste`) and **stale-account sweep**
  (`users stale`) across all tenants from the local store.
- **Standards drift report** (`standards drift`): tenants whose security
  baseline regressed between two synced snapshots.
- **Throttle-aware bulk executor** (`bulk`): drive add-user / offboard /
  remove-user / set-forwarding from a CSV with 429 backoff and resume; plans by
  default, writes only with `--execute`.
- Offline SQLite store, `--agent` JSON mode, `--dry-run`, and `doctor` health
  check.
