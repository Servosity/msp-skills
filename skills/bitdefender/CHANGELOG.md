# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-11-10

### Fixed
- **Writing to the local store costs almost no CPU.** The credential check on rows written to the local store read every row through each decoder it knows. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).

## [0.1.6] - 2026-11-09

### Fixed
- **Local store memory no longer climbs.** The credential check on rows written to the local store no longer grows with each page or spikes on one large record. The check is exactly as strict as before (#612).

## [0.1.5] - 2026-11-07

### Security
- **Rebuilt on press 4.35.1.** The MCP server's HTTP mode (`--transport http`, for remote agents) now serves `127.0.0.1:7777` by default, refuses to start until `BITDEFENDER_MCP_HTTP_TOKEN` is set (at least 32 characters), rejects any request without that bearer token, refuses browser origins other than loopback unless listed in `BITDEFENDER_MCP_HTTP_ALLOWED_ORIGINS`, and needs TLS for any address other than loopback. Web-only AI apps (ChatGPT on the web, Microsoft 365 Copilot, the Gemini app) are not supported by msp-skills yet; the ChatGPT desktop app runs the connector locally ([mcp-install.md](./mcp-install.md)).
- **One credential guard for every output.** The CLI and the MCP server now share the fleet's credential kit: every key, token and password the program holds (environment, config file, credential store, saved profiles, piped `--stdin` bodies) is held before anything prints, and any output that would carry one, in any common encoding, is replaced by a notice. This covers the terminal, `--json`, MCP tool results and confirmation previews, files, the local store, the learn journal and child processes. GravityZone fields that carry secrets (license keys, push receiver authorization, uninstall and policy passwords) are withheld in command output and MCP results. A held credential is sent only to the configured GravityZone API over https, never to another host or across a redirect to one.
- **MCP writes preview first.** Reads now go through `bitdefender_read`. `bitdefender_execute` runs only data-changing GravityZone methods, and it returns a preview and sends nothing until the agent repeats the call with `confirm` set to true.
- With `BITDEFENDER_MCP_READ_ONLY=1`, `bitdefender_execute`, `import` and the tools that write the local learn store are removed, and `bitdefender_read` runs only the listed GravityZone read methods: the Credential-tier reads (push settings, license key, package, policy and integration settings, install links, inventory) stay refused.
- `--deliver file:` writes a private 0600 file and never follows a planted symlink; `--deliver webhook:` no longer follows redirects. `--csv` escapes cells a spreadsheet would run as a formula.

### Added
- `BITDEFENDER_NO_LOCAL_DATA=1` keeps no local copy of GravityZone data (no response cache, no store writes, live reads), declared on every install channel. `local-data purge` previews and, with `--yes`, erases the local store, cache and learn journal.
- `--ca-cert` / `BITDEFENDER_CA_CERT` and `--tls-server-name` / `BITDEFENDER_TLS_SERVER_NAME` for an on-premises Control Center with a private CA.

### Fixed
- `usage billing --agreements`: rows for the same company by id and by name now add up instead of the second being ignored, and a name row more than one child company carries is applied to none of them and flagged `agreement_name_ambiguous`.

## [0.1.4] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `bitdefender-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer passes `import --input` to the CLI, so a tool call cannot point the import at a file on the machine that runs the server.

## [0.1.2] - 2026-10-22

### Security
- The MCP server no longer passes `fleet rmm-gap --devices` or `usage billing --agreements` to the CLI, so an MCP tool call cannot point either command at a file on the machine that runs the server. Both flags still work on the command line.

## [0.1.1] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: `bitdefender-cli` and `bitdefender-mcp` for the
  Bitdefender GravityZone Public API (JSON-RPC), cloud or on-premises Control
  Center, authenticated with a Control Center API key.
- 140 GravityZone Partners and Cloud Public API methods as commands:
  endpoints, companies, policies, incidents, quarantine, isolation, scan and
  investigation tasks, patches, PHASR, reports, packages, license and usage.
- Cross-customer views that walk the whole partner tree: `fleet health`,
  `fleet incident-queue`, `fleet threats`, `fleet locate`, `fleet rmm-gap`,
  `fleet changes` (diffs saved snapshots), `companies tree` and `usage billing`
  (own share after the partner rollup, with PSA agreement checks).
- `BITDEFENDER_MCP_READ_ONLY=1` lets the MCP server's generic execute tool run
  GravityZone read methods only; writes and the Credential-tier reads are
  refused (incident reads stay allowed).
- An environment `BITDEFENDER_BASE_URL` is a per-run override and is never saved
  to config.toml.
