# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release of the Avanan connector: `avanan-cli` and the
  `avanan-mcp` MCP server for Avanan (Check Point Harmony Email and
  Collaboration), covering the documented API surface: security events, SaaS
  entities, scopes, actions and their async tasks, anti-phishing and spam
  exceptions, all five sectool exception families, MSP tenants, users, licenses,
  add-ons, and monthly and daily usage, mis-classification reporting, SOAR entity
  fetch and notify, and both `.eml` download paths.
- The sectool exception surface and the SOAR and download helpers that the
  published SwaggerHub spec omits entirely, reconstructed from the vendor
  reference guide and cross-checked against the shipping Cortex XSOAR and
  Microsoft Sentinel integrations (about 15 operations).
- Dual-mode authentication chosen from the configured host: the legacy
  `*.avanan.net` signed handshake and the Infinity Portal bearer exchange. The
  signed handshake implements the `requestString` term the published docs leave
  out, signing and sending through one code path so a mismatch cannot surface as
  a 401 that looks like a bad credential. Region selection covers all seven
  isolated farms.
- Offline SQLite mirror with `sync`, `mirror`, and full-text `search`, so
  cross-tenant and what-changed questions answer from local data instead of an
  O(tenants) API fan-out against an endpoint that rate-limits and publishes no
  quota.
- Seven commands the Avanan API cannot answer in one call:
  - `triage` buckets a window of detections by threat type, severity, and state,
    with the sender domains driving the volume.
  - `campaign` clusters detections into candidate phishing campaigns by sender
    domain and normalized subject, with recipient spread and how many are still
    un-remediated.
  - `timeline` reconstructs one message's full history in order: detection, state
    changes, actions submitted, task outcomes, and restore disposition, which
    otherwise spans three endpoints and a task record that ages out.
  - `exceptions find` asks whether a domain, sender, URL, or hash is excepted
    anywhere across all seven engines and nine exception tables at once.
  - `exceptions audit` flags exceptions that contradict each other across
    sub-systems, exact duplicates, and entries that have never matched real
    traffic.
  - `msp fleet` joins licensed seats, add-ons, usage, and detection volume into
    one ranked table across every tenant.
  - `remediate` submits a quarantine or restore, waits for the async task to
    reach a terminal state, and reports the real per-item outcome. It also turns
    the action endpoints' single-scope HTTP 400 into an error naming the scopes
    the credential actually covers.
- `--agent` mode (JSON, non-interactive), `--dry-run` previews on every mutating
  command, typed exit codes, and `export`/`import` for JSONL backup and migration.

### Fixed
- `x-av-date` is now sent as microsecond precision with no zone suffix. The
  server parses this header rather than echoing it, so the previous
  millisecond-plus-`Z` layout made the parse fail and the API answered HTTP 500
  on every request, including requests carrying a deliberately fabricated app
  id. That made a client-side format bug present as a vendor outage or an
  unprovisioned credential. The new layout matches Avanan's own MSP sample
  client (`datetime.datetime.utcnow().isoformat()`) and is recorded in
  `handfixes.json` so a reprint cannot silently revert it.
