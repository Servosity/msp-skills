# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.15] - unreleased

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.14] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.13] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.12] - 2026-11-05

### Security
- **The learn journal keeps no text you typed in place of a name.** `IMMYBOT_LEARN_SESSION` is stored as a hash, so sessions still group. A flag name the command does not have is recorded only as `<undeclared>`, and nothing after `--` is recorded. A mistyped flag is never kept as typed. A teach with no `--query` logs how many arguments it got, not the arguments.
- **Every credential is held before the program reads its arguments, and a request's credential fields before any preview.** A flag that failed to parse could print a password inside `IMMYBOT_TOKEN_URL`, `IMMYBOT_FEEDBACK_ENDPOINT`, a proxy variable or a config `token_url`, and `--dry-run` printed a `password` field from `--stdin` or an `import` file. A saved profile's value is held from startup unless it sits under a flag this CLI has and a profile would apply, and so is the password or a query value inside a `base-url` value. A saved value under a credential's name, or a `base-url` value, is held as the program starts, so one that is also the program's fixed text, such as `true`, stops the program from starting instead of printing. A proxy's password goes to that proxy only. An `import` record is held from its raw line. A saved profile no longer turns on `--stdin`, so pass the flag on the command line. A `--stdin` body is held whole, so `--dry-run` prints a notice in place of the preview, and a command whose answer repeats any part of the body prints the notice in place of that answer; the request is still sent.
- **Every credential rule now comes from the shared credential kit** that all MSP Skills connectors use, in place of this connector's own checks. Four things you may notice. A credential goes only to the instance this run uses, chosen by an explicit `--config`, `IMMYBOT_CONFIG` or `--home` over a discovered default, or by the subdomain. The Entra client secret and refresh token go only to Microsoft's sign-in hosts or to the `https://` endpoint `IMMYBOT_TOKEN_URL` names, so a `token_url` in a config file on any other host is refused. A credential never travels over plain `http://`, and a `--deliver` webhook or feedback URL with a user name or password in it is refused. Commands that write to the local mirror take longer while a credential is set, because each row is checked (roughly 30 to 120 ms a row, so a large `sync` is slower).
- **A redirect can no longer reach a GET that changes state.** Only a direct, confirmed call runs one; a server redirecting a read onto one is refused.
- **A config file that fails to load gets a local store of its own.** It used to open the shared `data.db`, whose instance is unknown.
- **The response cache moved to a new folder, and the old one is deleted on upgrade.** An answer an earlier version cached, which could include one a redirect fetched from a credential endpoint, is never served.
- **No credential this program holds reaches any output, file or other server, in any form and at any length.** A client secret, access or refresh token, auth header, cookie, a password in `IMMYBOT_BASE_URL` or `IMMYBOT_TOKEN_URL`, or the `IMMYBOT_MCP_HTTP_TOKEN` was printed or saved when the ImmyBot instance or the token endpoint repeated it back: in command output, `--json`, MCP tool results, the local mirror and the response cache. It now does not matter where the credential came from (environment, config file including a malformed or legacy one, credentials file, `--client-secret`, `auth set-token`, a minted or refreshed token) or how it was repeated (as written, base64, base32, hex, percent, JSON or HTML escapes, ROT13, layered, or split across lines or JSON fields), and a short value is covered too. Messages the MCP server writes itself (such as an error naming an unknown tool) and a credential flag that fails to parse are covered as well. A line of output that carries one is replaced by a withheld notice; a row, cache entry, journal line or file that carries one is not written; and a request to any server other than your ImmyBot instance (or the token URL, for sign-in) that carries one is refused; a username and password in a URL go only to the host they were configured for. Your Entra client id and tenant id are identifiers and are shown as before.
- **The GET endpoints that change state are treated as writes.** Cancelling a maintenance session or a schedule's sessions, the maintenance-email reboot-now, run-now and postpone links, launching an ephemeral agent, signing in, refreshing and signing out, the inbound webhook, ephemeral agent sessions, the script language service and the provider plugin passthrough need `confirm: true` in `immybot_execute`, are refused by `immybot_read` and under `IMMYBOT_MCP_READ_ONLY=1`, and are never answered from the response cache or the local mirror (`--data-source local` refuses them).

### Changed
- **Rebuilt on CLI Printing Press 4.34.0** ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)). Every command keeps its name, flags and MCP tool id. The MCP server's HTTP mode (`--transport http`) now requires a bearer token in `IMMYBOT_MCP_HTTP_TOKEN` and TLS on any address other than loopback, so the earlier advice to avoid HTTP mode is withdrawn. Two optional switches are now offered on every install channel: `IMMYBOT_MCP_READ_ONLY=1` exposes only read-only tools, and `IMMYBOT_NO_LOCAL_DATA=1` keeps no API data on disk.

### Added
- **`schedule-orphans`**: maintenance schedules whose target, tenant or target type no longer exists, plus disabled schedules, from the local mirror.
- **`provider-link-health`**: every RMM or PSA provider link that is disabled, unhealthy or reporting a sync failure, with the failure messages ImmyBot recorded.

### Fixed
- **`assignment-explain` and `script-blast-radius` follow tenant propagation.** An assignment or deployment on a parent tenant with "propagate to child tenants" set now reaches computers in every child tenant. When the tenants are not synced, `assignment-explain` lists such a rule as unverified and `script-blast-radius` marks its reach as a lower bound (`reach_is_lower_bound`), instead of leaving them out.
- **`drift` and `version-spread` count each computer once.** A computer behind on two titles, or on two old builds of one title, counted twice in `computers_behind` and `computers_below_floor`.
- **`session-triage --status` no longer calls the selected actions failed.** The selection is counted in a new `matched_actions` field; `failed_actions` counts only actions that failed.
- **`fleet-diff` no longer hides a new global deployment.** A baseline taken when no global deployment existed was read as an old-format baseline, so a global deployment added later was not listed. Each `--snapshot` now records what it tracked; a baseline from an older version still compares computers and software, and says which kinds it could not compare (`not_compared`).
- **The mirror-reading commands open the local store read-only**, so they no longer run migrations or fail with "database is locked" while a sync runs.
- **A maintenance action's outcome is read one way everywhere**: `session-triage`, `deployment-health` and `computer-dossier` read the documented `result` field and agree on what failed.
- **A rejected sign-in reports the error instead of hanging.** When Microsoft Entra refused the token request (for example a wrong or expired client secret), every command and MCP call stopped responding.
- **`IMMYBOT_SUBDOMAIN` and `sync --path-context` values are checked.** A subdomain must be one host name label (letters, digits and inner hyphens); anything else is refused with an error naming the variable, and a value placed in a request path is escaped.
- **Two ImmyBot instances keep two local mirrors.** The local store is keyed by the instance (`IMMYBOT_SUBDOMAIN` or the base URL) as well as the sign-in (a token-only sign-in, and no sign-in, too), so two instances that share one Entra app registration no longer read each other's data. A mirror kept by an earlier version (`data.db`, or one keyed by the sign-in alone) is not reused: run `immybot-cli sync` once after upgrading. `sync --path-context` no longer accepts `subdomain`: the instance comes from `IMMYBOT_SUBDOMAIN` or `IMMYBOT_BASE_URL` only.
- **A row with a missing or unreadable date no longer passes an age filter.** `onboarding-stalled --older-than`, `session-triage --since` and `fleet-diff --since` count such rows in `undated` / `undated_actions` / `undated_snapshots` instead.
- **Computers listed by the paged computer endpoint are found and counted.** `assignment-explain`, `computer-dossier`, `psa-reconcile` and `fleet-diff` read each mirrored computer list by its own field names, and `version-spread`, `drift` and `session-triage` count computers and tenants by id. `version-spread`'s `tenants_behind[].computer_count` counts only the machines below the floor (`computers_with_title` counts all). `fleet-diff` tracks each computer once, from the record merged across every computer list the mirror holds, so a machine going offline is reported even when another list leaves its state blank; rows with no computer id are counted in `unidentified_computers`. Each field of that record comes from the most recently synced list that carries it, so `computer-dossier` and `fleet-diff` follow a later `sync --resources computers-paged`, and `fleet-diff` reports a change in any one list even when the lists disagree. Rows are matched to a computer by computer id only: `computer-dossier` counts the mirrored actions and software rows that carry no computer id in `unidentified_actions` and `unidentified_software`, and an `assignment-explain` Computer rule reaches a computer only by its id. A `fleet-diff` baseline from an earlier version is no longer compared on computers; record a new one with `--snapshot`.
- **Azure tenant app-registration secrets are CLI only.** `access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id` and `access create-update-azure-tenant-auth-details` print to the terminal that ran them (as a withheld notice while the answer carries the secret: read the secret itself in the ImmyBot console), are never cached, stored, exported or sent with `--deliver`, are fetched only by the command that asks for them (never by another command or a redirect), and are not offered by the MCP server.
- **`version-spread` matches the exact title by default**, and an unknown installed version no longer hides a machine below the floor.

## [0.1.11] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **The token request no longer follows a redirect to another server.** `auth login` and the automatic sign-in sent the token request, with your client secret, on to whatever host a redirect from the token URL named. A redirect off the token server now fails instead.

### Fixed
- **`--dry-run` sends nothing, not even a token request.** With no saved token, a dry run used to request one first, sending your client ID and secret to the token URL and saving the token, although a dry run promises that no request is sent. `auth login --dry-run` now also only shows the request it would make, and `auth set-token --dry-run` and `auth logout --dry-run` no longer save or clear your stored credentials.

## [0.1.10] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.9] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `immybot-cli access create` ran `immybot-cli access create-delete-azure-tenant-auth-details`, POST `/api/v1/auth/delete-azure-tenant-auth-details`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `immybot-cli access create-delete-azure-tenant-auth-details` (POST `/api/v1/auth/delete-azure-tenant-auth-details`)
  - `immybot-cli application-locks create-request-cancellation` (POST `/api/v1/application-locks/request-cancellation`)
  - `immybot-cli billing create-cancel-subscription` (POST `/api/v1/billing/cancel-subscription`)
  - `immybot-cli computers ephemeral-agent-circuit-breaker create-reset` (POST `/api/v1/computers/{computerId}/ephemeral-agent-circuit-breaker/reset`)
  - `immybot-cli maintenance-sessions actions create-by-id-rerun` (POST `/api/v1/maintenance-sessions/{sessionId}/actions/{actionId}/rerun`; re-runs a maintenance action (software install, patch or task) on the computer)
  - `immybot-cli maintenance-sessions create-cancel` (POST `/api/v1/maintenance-sessions/cancel`)
  - `immybot-cli metrics create-circuit-breakers-isolate` (POST `/api/v1/metrics/circuit-breakers/isolate`)
  - `immybot-cli plugins create-api-v1-by-provider-link-id-by-catch-all` (POST `/plugins/api/v1/{providerLinkId}/{catchAll}`; passes an arbitrary POST through to a provider plugin's API)
  - `immybot-cli provider-agents create-bulk-delete-pending` (POST `/api/v1/provider-agents/bulk-delete-pending`)
  - `immybot-cli provider-links cross-references create-by-external-link-id-disable-client-linking` (POST `/api/v1/provider-links/{id}/cross-references/{externalLinkId}/disable-client-linking`)
  - `immybot-cli scripts create-debug-cancel-by-cancellation-id` (POST `/api/v1/scripts/debug/cancel/{cancellationId}`)
  - `immybot-cli system create-disable-immy-support-access` (POST `/api/v1/system/disable-immy-support-access`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.8] - 2026-10-15

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

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.1.5] - 2026-09-18

### Changed
### Fixed
- **`workflow archive` no longer reports success when nothing was archived.** It counted
  successes but exited 0 and printed the summary even when every resource failed
  (for example on an expired credential). Zero of N now exits non-zero with the true count;
  partial success still exits 0.
- The MCP server's config-resolution test that every other connector carries was missing;
  added, proving the server reads the same config file the CLI does.

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)

## [0.1.4] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  The recipe shortcut tools, which build their own command line, now also refuse a value
  that begins with `-` where a plain value is expected.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.3] - 2026-09-01

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Added
- **`IMMYBOT_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
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

## [0.1.2] - 2026-08-26

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

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://{subdomain}.immy.bot`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `IMMYBOT_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `IMMYBOT_BASE_URL`, `IMMYBOT_TOKEN_URL`, `IMMYBOT_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE`.

## [0.1.0] - 2026-08-22

### Added
- Initial msp-skills release of the ImmyBot connector: `immybot-cli` and the
  `immybot-mcp` MCP server, covering the ImmyBot API (computers, tenants,
  software, target assignments, maintenance sessions and actions, scripts,
  schedules, provider and RMM links, persons, roles, and access).
- Offline SQLite mirror with `sync` and full-text `search`, so cross-tenant
  questions answer from local data instead of one API call per tenant.
- Cross-tenant views the console does not offer: `session-triage` (groups a
  maintenance window's failures by root cause), `version-spread` (one software
  title ranked across every tenant with a real semver comparator),
  `assignment-explain` (which deployment rule actually won on a given machine,
  and which are shadowed), `fleet-diff` (what changed between two syncs, from
  history the API does not retain), `deployment-health`, `onboarding-stalled`,
  `computer-dossier`, `drift`, `psa-reconcile`, and `script-blast-radius`.

### Fixed
- `auth login` no longer persists an env-supplied client secret to disk:
  `SaveTokens` keeps the ClientID/ClientSecret env-override markers, so the
  guards added in #268 actually hold.
- `sync` mirrors every computer. `GET /api/v1/computers` exposes no cursor,
  page, skip or offset, so `pageSize` is a ceiling rather than a page size and
  the profiler's spec default of 25 silently truncated the resource that the
  offline commands join on most.
- Three parameter-required lookup endpoints are out of the default sync walk
  (they returned HTTP 400 on every run) and remain reachable via `--resources`.
- Staleness reports `never_synced` separately instead of reading a zero
  watermark as a 292-year-old mirror.
- `doctor` reports the instance subdomain as its own check, and every auth
  hint - in `doctor`, `auth status`, `auth login --help`, the runtime error
  paths, and the MCP tool layer - names the four required `IMMYBOT_*`
  variables and points at `auth login` rather than an `auth setup` subcommand
  this CLI does not define.

Connector contributed by [@geekbrownbear](https://github.com/geekbrownbear)
(PR [#276](https://github.com/servosity/msp-skills/pull/276)), live-verified
against a production ImmyBot tenant.
