# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.10] - unreleased

### Security
- Rebuilt on CLI Printing Press 4.34.0 ([GHSA-jh26-vhqh-692c](https://github.com/mvanhorn/cli-printing-press/security/advisories/GHSA-jh26-vhqh-692c)). The MCP server's HTTP mode now refuses to start until `ACRONIS_MCP_HTTP_TOKEN` is set, rejects any request without that bearer token, serves `http://127.0.0.1:7777/mcp` by default, needs `--tls-cert` and `--tls-key` for any other address, and refuses a request body over 4 MiB or one slower than 30 seconds. The remote-agent instructions (ChatGPT, Copilot Studio, the Gemini app) are back in `mcp-install.md`.
- A datacenter value (from `ACRONIS_DATACENTER`, the config file, or a `sync` call's `--path-context`) must be one host label such as `us-cloud`; anything else is refused before a request is built.
- A credential passed in a request (`idp request-token --client-secret`, `idp revoke-token --token`, any credential-named body or query field, and their MCP tools) is withheld from any error that repeats it, like the configured credentials, whether it was sent as text or as a number.
- No credential field is written to the local store or the response cache: the `client_secret` that `clients create` returns, identity-provider token answers, and any secret-named field in a synced or cached row are dropped before they reach disk, at any depth and under plural names such as `secrets`, `tokens`, `api_keys` and `credentials`, including inside a wrapper object, and a list a response wraps under such a name is never stored as rows. A secret-named field is dropped whatever its value; the only values kept are a number under a count name such as `total_tokens`, and under `code` one of the six task result words Acronis documents (`ok`, `error`, `warning`, `cancelled`, `abandoned`, `timedout`), which the `result_code` column reads. The command's own output still shows them.
- `ACRONIS_MCP_READ_ONLY=1` now removes every MCP tool not marked read-only, including the typed endpoint tools. The install configs on `mcp-install.md` and the Claude Desktop bundle preset it to `1`.

### Added
- `ACRONIS_NO_LOCAL_DATA=1` keeps no local copy of API responses (reads go to the API, the response cache is off and `sync` refuses to run). It, `ACRONIS_MCP_READ_ONLY`, `ACRONIS_MCP_HTTP_TOKEN` and `ACRONIS_USER_AGENT` are now declared on all three install channels.
- Commands from the 4.34.0 engine: the local learning loop (`recall`, `teach`, `teach-lookup`, `teach-pattern`, `teach-playbook`, `learnings`, `playbook`), which reads and writes only this machine's local store; `export`, which writes paginated API data to a local JSONL or JSON file; and `local-data purge`, which lists what it would delete and deletes nothing without `--yes`.

### Fixed
- A read or write answer is stored only in the table for the endpoint it came from: `task-manager get-activity` no longer adds an activity to the tasks that `failures` and `health` count, and hardware-node reads and the agent update commands no longer add rows to the agents. Hardware nodes and agent update settings are not stored at all, and `--data-source local` refuses them instead of answering from another table.
- `doctor` no longer reports the API as reachable when no datacenter was chosen: the built-in `eu2-cloud` fill-in is reported as not configured, naming `ACRONIS_DATACENTER` and `ACRONIS_BASE_URL`.
- `sync` exits non-zero when any resource stops before its last page, including at a `--max-pages` cap, and when rows could not be written to the rollup tables, so an incomplete mirror can no longer feed `health`, `failures`, `coverage` or the usage rollups.
- `governance.md`, the README and the docs page named the bearer-token variable `ACRONIS_CYBER_PROTECT_BEARER_AUTH`; the binary reads `ACRONIS_BEARER_AUTH`.

## [0.1.9] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `acronis-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers the token endpoint errors of `acronis-cli auth login` too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.8] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.7] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `acronis-cli agent-manager create` ran `acronis-cli agent-manager force-agent-update`, POST `/api/agent_manager/v2/agents/update:force`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `acronis-cli agent-manager force-agent-update` (POST `/api/agent_manager/v2/agents/update:force`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.6] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.5] - 2026-09-28

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

## [0.1.4] - 2026-09-11

### Fixed
- Resolve the API client's tenant subtree for default sync and search; sync agents,
  per-tenant usages, and offering items as well as the existing resources.
- Follow Acronis nested cursors, including short pages with a next cursor. Reject
  partial pages, failed child requests, and dropped rows; incomplete mirrors cannot
  drive fleet rollups. Existing mirrors require a successful unfiltered full sync.
- Translate task filter and sort syntax in CLI and MCP, applying exact tenant
  filtering locally across all pages. Translate remote search parameters.
- Map nested task tenant/result fields and preserve distinct billing editions and
  infrastructure rows. Report unmatched legacy agent tenant identities explicitly.
- Persist the datacenter API URL during login and stop mislabeling parameter
  validation errors as credential failures.
- Thanks @asterisk79 for the multi-tenant runtime report in #325. Offline fixtures
  cover these contracts; confirmation against the partner's tenant remains pending.

## [0.1.3] - 2026-09-10

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
  It also dialled the shipped placeholder base URL (`https://{datacenter}.acronis.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `ACRONIS_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ACRONIS_BASE_URL`, `ACRONIS_CLIENT_ID`, `ACRONIS_CLIENT_SECRET`.

### Changed
- Regenerated the vendored `acronis-cli` / `acronis-mcp` source from
  cli-printing-press 4.24.0 and re-rendered the templated skill surfaces. No
  changes to command names or flags; the local mirror, search, and cross-tenant
  rollups behave as before.

## [0.1.0]

### Added
- Initial msp-skills release: the `acronis-cli` CLI and `acronis-mcp` MCP server
  for Acronis Cyber Protect Cloud, with an offline SQLite mirror (`sync`) and
  full-text `search`.
- Cross-tenant backup rollups: `health`, `failures`, `freshness`, and
  `agents stale` answer "whose backups failed" and "which agents went offline"
  across every customer tenant in one table.
- Billing and posture views: `coverage --unprotected`, `reconcile usages`,
  `usages drift`, `agents compliance`, `tenants offering-items inventory`, and
  the `customer` 360 card.
- Tenant operations: `tenants` (list/get/create/update/delete/audit), `clients`,
  `agent-manager`, `task-manager`, plus full API coverage via `api`.
