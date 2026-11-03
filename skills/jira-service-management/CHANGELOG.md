# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - unreleased

### Changed
- Rebuilt on CLI Printing Press 4.35.1.
- HTTP mode (`jira-service-management-mcp --transport http`) for remote agents is documented again. It refuses to start until `JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN` is set to at least 32 characters, every request must send that token as a bearer header, any address other than loopback also needs `--tls-cert` and `--tls-key`, and browser origins other than loopback are refused unless listed in `JIRA_SERVICE_MANAGEMENT_MCP_HTTP_ALLOWED_ORIGINS`.
- New `local-data purge` previews and, with `--yes`, erases the local store, response cache and learn journal. `JIRA_SERVICE_MANAGEMENT_NO_LOCAL_DATA=1` runs every read live and writes nothing locally.
- `--csv` prefixes `'` on cells that a spreadsheet would run as a formula. `--deliver file:` writes a private 0600 file; `--deliver webhook:` does not follow redirects.
- `sync` stores Assets object type attributes in their own table.
- Six POST commands that do not create anything (`request approval answer`, `request transition perform`, `request feedback post`, `importsource token generate`, `objecttype position change`, `servicedesk attach-temporary-file upload-temporary-file`) no longer answer to the `create` alias.

### Fixed
- A one-page list read whose JSM or Assets page says more pages follow now reports the result as truncated and suggests `--all`.
- `export` and `sync` read the rows inside JSM's paged `values` envelope, and Assets object schemas, schema usage, request type groups, SLAs and transitions keep one row per item.
- The local store is kept per Atlassian site. A store saved before site scoping, or one no site has claimed, is left in place and not adopted by whichever site runs first; that site syncs fresh.

### Security
- MCP tools mirrored from CLI write commands, such as `request_note`, return a preview and send nothing unless the call passes `confirm: true`.
- No MCP tool takes `--path-context`, so a tool call cannot point the configured credential at another site.
- Headers from the config file are sent only to the configured site: a redirect to another origin drops them, and a request carrying one is refused over plain HTTP to a non-loopback host.
- Both binaries run on the shared msp-skills credential kit: the API token, config-file credentials and headers, the MCP HTTP token and saved profile credentials are held from startup and kept out of every output (stdout, stderr, MCP results, the local store, response cache and learn journal), and a credential is sent only to the configured site and `api.atlassian.com`, over HTTPS, never across a redirect to another origin.

## [0.1.4] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `jira-service-management-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.2] - 2026-10-22

### Security
- The MCP server no longer passes `cmdb reconcile --file` to the CLI, so an MCP tool call cannot point it at a file on the machine that runs the server. The flag still works on the command line.

## [0.1.1] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: Jira Service Management Cloud CLI + MCP server covering the
  request API (servicedeskapi) and Assets (CMDB), with a local SQLite copy for offline search.
- `breaches`: open tickets across every queue ranked by SLA time remaining, breached first,
  each ticket counted once.
- `request note`: agent-only internal notes that always post public=false and error if JSM
  stores the comment as public; `request timeline` labels every comment PUBLIC or INTERNAL.
- `roster` and `client`: client organization hygiene and a per-client open-ticket view.
- `sla-report`: breach rate and median time vs goal per desk, request type or SLA metric.
- `cmdb discover`, `cmdb tickets` and `cmdb reconcile`: find the Assets workspace, join Assets
  objects to their tickets, and diff an object type against an RMM or PSA export.
- Assets calls are refused unless their cloudId and workspaceId belong to the current site
  (`JIRA_SERVICE_MANAGEMENT_SITE`), so ids left over from another client never reach that
  client's CMDB.
- Pagination sends the real wire names (`start` for JSM; `startAt` and `resultPerPage` for
  Assets), clamps JSM page size to 50, and follows a page the server capped short
  (`isLastPage: false`) to the end, in sync, export, `--all` and the built-in commands.
- Windows builds: `cmdb reconcile` opens its input file through per-OS code, so the connector
  ships Windows binaries.
