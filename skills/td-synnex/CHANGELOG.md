# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.4] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.3] - 2026-11-07

### Security
- **Rebuilt on press 4.35.1.** The MCP server's HTTP mode (`--transport http`, for remote agents) now serves `127.0.0.1:7777` by default, refuses to start until `TD_SYNNEX_MCP_HTTP_TOKEN` is set (at least 32 characters), rejects any request without that bearer token, refuses browser origins other than loopback unless listed in `TD_SYNNEX_MCP_HTTP_ALLOWED_ORIGINS`, and needs TLS for any address other than loopback. An unfilled Claude Desktop placeholder no longer counts as a token. Web-only AI apps (ChatGPT on the web, Microsoft 365 Copilot, the Gemini app) are not supported by msp-skills yet; the ChatGPT desktop app runs the connector locally ([mcp-install.md](./mcp-install.md)).
- **One credential guard for every output.** The CLI and the MCP server now share the fleet's credential kit: every token the program holds (environment, config file, credential store, saved profiles, piped refresh tokens and `--stdin` bodies, tokens `auth mint` receives) is held before anything prints, and any output that would carry one, in any common encoding, is replaced by a notice. This covers the terminal, `--json`, MCP tool results and confirmation previews, files, the local store, the response cache, the learn journal and child processes. A held token is sent only to the configured ION API over https, never to another host or across a redirect to one. `auth mint --print-access-token` now prints a withheld notice instead of the token; read it from the credentials file. A password inside a saved profile's base URL is held too, and `--deliver spacefast:` no longer prints the page's secret claim links; it names the private record file that keeps them.
- **MCP writes preview first.** Every MCP tool that can change ION (order create and cancel, cart checkout and edits, customer create and update, product enable and disable, provisioning, report save and delete, `subscriptions set`) returns a preview and sends nothing until the agent repeats the call with `confirm` set to true. Endpoint tools also refuse any argument they do not declare.
- `--deliver file:` writes a private 0600 file and never follows a planted symlink; `--deliver webhook:` no longer follows redirects. `--csv` escapes cells a spreadsheet would run as a formula.

### Added
- `TD_SYNNEX_NO_LOCAL_DATA=1` keeps no local copy of ION data (no response cache, no store writes, live reads), declared on every install channel. `local-data purge` previews and, with `--yes`, erases the local store, cache and learn journal.
- `--ca-cert` / `TD_SYNNEX_CA_CERT` and `--tls-server-name` / `TD_SYNNEX_TLS_SERVER_NAME` for a private CA.

### Fixed
- **The licence-book commands answer only from a complete list.** `customers show`, `margin`, `subscriptions renewals` and `changes`, `orders stuck` and `position export` / `trueup` refuse (exit 10, naming the sync to run) unless the last sync of every resource they read walked the whole, unfiltered list. A sync limited by `--max-pages`, `--param`, `--global-param` or `--resource-param`, or one interrupted before it finished, no longer counts, and an unfiltered sync after a filtered one starts over instead of resuming the filtered position. A list that ends on an exactly full page is now recognized as complete. After upgrading, run one unfiltered `td-synnex-cli sync` before using these commands; a store synced by an earlier version is refused until then. They also answer only for the configured account (`TD_SYNNEX_ACCOUNT_ID`): `sync` refuses an account ID from `--path-context` or a sync parameter that names another account, and a list walked for another account is emptied before it is reused. Two syncs of different accounts into one store (`--db`) at the same time can no longer mix their rows: a row is saved only while its list still belongs to the syncing account, and the commands read only rows a sync of the configured account saved.

## [0.1.2] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers TD_SYNNEX_REFRESH_TOKEN from the environment too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.1] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.0] - 2026-10-20

### Added
- Initial msp-skills release: `td-synnex-cli` and `td-synnex-mcp` for the
  StreamOne ION v3 reseller API (customers, catalog, pricing, carts, orders,
  subscriptions, cloud profiles, reports), with a local SQLite store.
- Licence book: `subscriptions renewals`, `subscriptions changes`, `margin`,
  `orders stuck` and `customers show` answer whole-book questions offline.
- Cross-distributor true-up: `position export` writes licence-position/v1 rows and
  `position trueup` joins them with Pax8, Sherweb or PSA exports.
- `subscriptions set` builds the ION UPDATE order for seats, renewal setting, name,
  status or tier, and places it only with `--confirm`.
- `auth mint` / `auth check` handle ION's single-use refresh tokens: the rotated
  token is saved under a lock before anything prints.
- `TD_SYNNEX_MCP_READ_ONLY=1` removes every MCP tool that can change ION.
- Credentials ION returns in customer, order and provisioning data are blanked
  before anything is printed, cached, synced or handed to an agent.
- Not yet run against a live ION tenant; margin assumes ION prices per subscription
  (see the README's Known gaps).
