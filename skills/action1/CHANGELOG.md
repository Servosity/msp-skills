# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.11] - unreleased

### Security
- **Rebuilt on Printing Press 4.34.2 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode listens on `127.0.0.1:7777` by default, refuses to start until `ACTION1_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback. HTTP mode is available again for ChatGPT, Copilot Studio and the Gemini app.
- **Every MCP write previews first.** A tool call that changes Action1 (`action1_execute`, `import`) returns a preview and sends nothing until it is called again with `confirm` set to boolean true. A tool argument can never name a file to read or write.
- **The credentials go only to your Action1 data center.** The token is minted from `<ACTION1_BASE_URL>/oauth2/token`, so an EU or Australia base URL mints from its own region, and a redirect to another host is refused.
- **Credentials stay in one place.** Only `credentials.toml` (mode 0600) holds a credential; a client pair supplied by the environment, and the token minted for it, are never written to disk. Responses are never cached on disk, and any output, file or MCP result that would repeat a credential this program holds is withheld.
- **One shared credential layer.** The CLI and MCP server now use the msp-skills credential kit: every credential the program holds is known before anything prints, and any output unit that repeats one in any encoding (stdout, stderr, files, the local store, MCP results) is replaced whole by a notice. A very short credential, one split across several MCP replies, and one that matches the learn journal's own timestamp are all covered. The client ID's unique part is held; `api-key-` and `@action1.com` are not secrets and stay printable.
- **The learn journal no longer records text typed in place of a flag name,** and an `ACTION1_LEARN_SESSION` value is stored hashed.
- **A token is only sent for the client it was minted for.** A stored token is used only when the record that holds it names the client in effect, so a pair from the environment or from `--config` never authenticates with another client's token, and minting for it never overwrites the stored client's record.

### Added
- `ACTION1_MCP_READ_ONLY=1` makes the MCP server refuse every write, and `ACTION1_NO_LOCAL_DATA=1` keeps no local copy of API responses. Both are declared on every install channel, with `ACTION1_USER_AGENT` and `ACTION1_MCP_HTTP_TOKEN`.

### Changed
- List commands page with `--offset` (was `--from`), the name every other paging flag uses; the request still sends Action1's `from` parameter.
- `ACTION1_REGION` is gone from the docs (the binary never read it): set `ACTION1_BASE_URL` to `https://app.eu.action1.com/api/3.0` or `https://app.au.action1.com/api/3.0` for the EU or Australia data center.

### Fixed
- **`sync` reads every page and every organization.** Paging follows Action1's `next_page`, so sync no longer stops after the first page, and the per-organization lists (managed endpoints, vulnerabilities, automation instances, installed software) are walked for every organization. The `fleet` commands, which had read empty tables, now see the synced data.
- **An explicit token opens its own local store.** With `ACTION1_OAUTH2` set, the fleet commands read the data synced for that token, never the store of a client pair saved on the same machine. The local store of a client pair no longer changes when its token is re-minted.
- **`fleet patch-drift` never reports a missing endpoint as remediated.** An endpoint absent from the latest sync, or synced without its missing-updates summary, is `not_observed`, and a sync that returned no endpoints is still the current snapshot.
- The `fleet` views read Action1's documented shapes: `last_seen` timestamps, online status, report-data `fields` and vulnerability `product_name`.
- `auth set-token`, `auth logout` and `export --output` change nothing under `--dry-run`, and `--dry-run` never mints a token.

## [0.1.10] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers the errors of the client-credentials token request too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.9] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.8] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `action1-cli automations create` ran `action1-cli automations policies-instances-org-id-instance-id-stop-post`, POST `/automations/instances/{orgId}/{instanceId}/stop`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `action1-cli automations policies-instances-org-id-instance-id-stop-post` (POST `/automations/instances/{orgId}/{instanceId}/stop`)
  - `action1-cli enterprise request-closure` (POST `/enterprise/request-closure`)
  - `action1-cli updates approvals updates-org-id` (POST `/updates/{orgId}/approvals`; approves or declines update packages for deployment to endpoints)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.7] - 2026-10-15

### Changed
- Rebuilt with Go 1.26.9 (was 1.26.6). Go 1.26.9 includes upstream Go security fixes,
  among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and GO-2026-6612.
  No functional changes.

## [0.1.6] - 2026-10-03

### Fixed
- Software installer discovery: searching the MCP for `installer` now returns the package and version read/update tools, with guidance on Enterprise vs organization scope, version object IDs vs displayed version numbers, where edited properties go, and the separate approval/EULA permissions. Thanks @jomplox for the fix in #353.
- Governance now tiers both installer PATCH endpoints: package metadata as a routine write, and version configuration (install switches, approval, EULA) as endpoint execution, human-in-the-loop.

## [0.1.5] - 2026-10-02

### Fixed
- Remote-assistance discovery now names the required `connection_type: assistance` input, uses a valid CLI example, and explains scoped Remote Connect permission, bounded polling, desktop verification, and connection URL privacy. Thanks @jomplox for the fix and pilot verification in #348.

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
- **A client secret supplied through the environment was written to disk in cleartext.**
  `ACTION1_CLIENT_SECRET` was read into the config struct without being marked as an
  environment override, and the save path restored only `ACTION1_OAUTH2` from the on-disk copy
  - so the live secret was copied into `config.toml` under `client_secret` on the next save.
  `ACTION1_CLIENT_ID` had the same gap. Both are now marked and restored, so a value supplied
  by the environment stays in the environment. **If you have run this CLI with either variable
  set, a cleartext copy may already be on disk. Upgrading does not remove it:** inspect
  `~/.config/action1-cli/config.toml`, delete it, and rotate the credential.

### Added
- **`ACTION1_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
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
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ACTION1_BASE_URL`, `ACTION1_OAUTH2`, `ACTION1_ORG_ID`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `action1-cli` + `action1-mcp` covering the full
  Action1 API (endpoints, updates, vulnerabilities, automations, scripts,
  software repository, organizations, users, roles, reports, audit).
- Fleet-wide views the org-siloed API cannot return in one call: `fleet
  patch-posture`, `fleet vuln-triage` (CVSS + CISA KEV), `fleet stale`, `fleet
  org-scorecard`, `fleet reboot-pending`, `fleet health-score`, `fleet
  software-rollup`, `fleet patch-drift`, and `fleet automation-health`.
- Offline SQLite mirror with `sync`, full-text `search`, `analytics`, and JSONL
  `export`; `--agent` JSON mode and `--dry-run` previews for safe automation.
- OAuth2 client-credentials auth that mints and refreshes the bearer token
  automatically, with `us`/`eu`/`au` region selection.
