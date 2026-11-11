# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - unreleased

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.6] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.5] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.4] - 2026-11-07

### Security
- **Rebuilt on Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode now listens on `127.0.0.1:7777` by default, refuses to start until `SONICWALL_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback. Web-only AI apps (ChatGPT on the web, Copilot Studio, the Gemini app) are still not supported by msp-skills; [mcp-install.md](./mcp-install.md) documents HTTP mode for reference.
- Each credential now goes only to its own service: the MySonicWall API key only to MySonicWall and the NSM token only to NSM, including when a request is redirected or a base URL override is set.
- Over MCP, the tools that sign in or can change state (`nsm_login`, `nsm_get`, `import`, and the NSM token exchange and refresh endpoints) return a preview and run only when called again with `confirm` set to `true`.

### Fixed
- `posture` and `exposure` decide whether SSL-VPN is reachable from the internet by each zone's security type (`untrusted`), not its name: a WAN zone with any name is graded, and a trusted zone whose name contains WAN is not reported as exposed. A name containing WAN counts only when a zone's type is not reported.
- `blind-spots` never reports "every registered firewall is visible" when nothing was compared: an empty registry, or one with no firewall to compare (pass `--all-products` to include access points and other products), exits 3 and says so.
- `--tenant` no longer silently skips a firewall whose tenant NSM did not report (so `licenses renewals --tenant` could miss one): the command stops, says how many firewalls report no tenant, and suggests rerunning without `--tenant` or with `--serial`.

## [0.1.3] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `sonicwall-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.2] - 2026-10-22

### Security
- The MCP server no longer passes `import --input` to the CLI, so a tool call cannot point the import at a file on the machine that runs the server.

## [0.1.1] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.0] - 2026-10-16

### Added
- Initial msp-skills release: SonicWall CLI (`sonicwall-cli`) and MCP server
  (`sonicwall-mcp`), read-only against NSM SaaS, the firewalls through NSM's
  SonicOS passthrough, and MySonicWall.
- `nsm-login` mints and saves the NSM session token from a MySonicWall API key
  (SonicWall's documented access-code exchange); `doctor` verifies the token with
  one authenticated NSM read.
- Insurance posture: `posture` grades every firewall pass / warn / fail / unknown
  for firmware, end of support, advisories, licence expiry, admin TOTP, SSL-VPN
  exposure and SSL-VPN users without MFA, and saves each run so
  `posture changes` shows what got worse.
- Advisory triage: `exposure` ranks the advisories hitting the most firewalls,
  or lists every firewall one SNWLID or CVE hits and whether SSL-VPN is on a
  WAN-named zone; `firmware drift` shows builds behind and what the upgrade
  clears.
- `mfa-audit` (admin TOTP and local SSL-VPN users without a one-time password),
  `licenses renewals` (MySonicWall licence expiry with a replace-instead flag)
  and `blind-spots` (MySonicWall-registered firewalls NSM cannot see).
- Typed list / get commands across NSM, SonicOS passthrough and MySonicWall,
  `nsm-get` for documented NSM read routes, and a local SQLite mirror
  (`sync`, `search`).
- Secret fields SonicWall's API schemas document (local-user and device
  passwords, `assign_token`, registration and authentication codes, SNMP
  community names, MySonicWall `api_key`, `tcpassword`, `authCode`,
  `activationKey`) are blanked in every NSM, SonicOS and MySonicWall response
  before any output, cache, sync or MCP result.
