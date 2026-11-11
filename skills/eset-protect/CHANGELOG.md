# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.2] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.1] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.0] - 2026-11-07

### Added
- Initial msp-skills release: `eset-protect-cli` and `eset-protect-mcp` for
  ESET PROTECT through the ESET Connect API, covering all 101 operations across
  device, incident, automation, vulnerability, patch, policy, quarantine,
  installer and IAM services, in the eu, de, us, ca and jpn regions.
- Sign-in with an ESET Connect API user (OAuth2 password grant): one-hour tokens
  are minted, cached per region and user, and refreshed on their own; requests
  ESET answers with 202 and a response id are polled until the request
  `--timeout` runs out.
- Per-customer views over a local SQLite mirror of the ESET PROTECT tree:
  `fleet health`, `fleet gaps`, `fleet stale`, `triage` and `detections spread`,
  each row attributed to the customer (COMPANY group) the device belongs to.
- Live per-customer reports: `exposure` (who has a CVE or vulnerable app),
  `patches backlog`, `report customer` (QBR one-pager) and
  `report vulnerabilities`.
- One-call lookups: `devices profile`, `incidents context`, `detections latest`,
  `executables find` and `policies decode` (base64 and LZMA policy settings to
  JSON).
- `respond scan`, `respond isolate` and `respond release` resolve a device by
  name and print the exact ESET task; they send it only with `--yes`, and over
  MCP they only preview. `policies clone` previews the same way.
- The bearer token is sent only to `*.eset.systems` hosts of the configured
  region or to the configured base URL, token requests never follow redirects,
  and `ESET_PROTECT_MCP_READ_ONLY=1` keeps an MCP agent to reads.
- Credentials are held by the shared credential kit from the first instruction of
  each binary: a password, token or MCP HTTP token never reaches stdout, stderr,
  an MCP result, a file, the local store or a request to another host, in any of
  the encodings the kit reads.
- Over MCP, `eset-protect_read` runs reads (including the three POST search and
  batch-get reads) and `eset-protect_execute` previews every state-changing
  call, sending it only when the call carries `confirm: true`.
- HTTP mode (`eset-protect-mcp --transport http`) refuses to start without an
  `ESET_PROTECT_MCP_HTTP_TOKEN` of at least 32 characters, binds loopback by
  default and needs TLS for any other address.
- `--dry-run` sends nothing: no token request, no API call, no `--deliver`
  webhook. `ESET_PROTECT_NO_LOCAL_DATA=1` runs without a local copy of API
  responses, and `local-data purge` erases the mirror, cache and learn journal.
- Built on CLI Printing Press 4.34.2.
