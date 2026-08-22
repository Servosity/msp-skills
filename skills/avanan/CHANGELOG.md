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
  a 401 that looks like a bad credential. `--region` covers all seven isolated
  legacy farms plus the two confirmed Infinity Portal gateways (`infinity`,
  `infinity-us`); a tenant on any other Infinity gateway points `auth login` at
  it with `--base-url` or `AVANAN_BASE_URL`, since guessing an unverified
  gateway host fails identically to a bad credential.
- Entity records are stored instead of silently dropped. Their identifier is
  nested at `entityInfo.entityId` and absent from the top level, so the flat
  identifier lookup skipped every one: a live mirror fetched 343 entities,
  stored none, and exited 0, leaving every offline command that reads entities
  answering from a table that had never been written.
- `mirror` also fails when a resource returns records and stores none of them.
  That is a record-shape mismatch rather than a thin day of data, and it is what
  hid the entity bug above behind a successful-looking run.
- The signing transport authenticates only the configured host. It runs once
  per redirect hop and after `CheckRedirect`, so a cross-host 3xx previously got
  a freshly signed request even though `CheckRedirect` had just stripped the
  token - and because the Infinity handshake endpoint was derived from the
  redirect target and the host test was a `cloudinfra` substring match, a
  redirect to a lookalike host was POSTed the raw client secret. `IsInfinityHost`
  is now anchored to `.portal.checkpoint.com`, and a cross-host hop is refused
  with the target host named rather than followed unauthenticated.
- `mirror` exits non-zero, and leaves the sync stamp alone, when the credential
  is rejected or when every requested resource fails and nothing is stored.
  Previously an expired credential 401'd every call, the command exited 0, and
  zero rows were stamped as freshly synced - so every offline command answered
  confidently from an empty mirror.
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
- Commands no longer refuse to run without `--x-av-req-id` or `--scopes`. The
  press had promoted both to required flags on 32 commands: `x-av-req-id` is a
  transport header the client already generates per request, and `--scopes` was
  required on the `scopes` command itself, so listing your scopes required
  already knowing one. The API remains the enforcement point and answers a
  multi-scope client with an HTTP 400 that names the requirement. Both flags
  are still available for pinning a scope or correlating a request id.
- The entity mirror now sends `requestData.entityFilter` with a `saas` value and
  the window inside it, one pass per platform. It previously reused the event
  query's flat body, so `/v1.0/search/query` answered HTTP 422 "entityFilter
  Field required" and entities stored zero rows while the run still reported
  success. A platform the tenant does not license no longer empties the whole
  entity mirror.
- `mirror` resolves an empty `--scope` to the credential's real scope list
  before fanning out. `Options.Scopes` documented empty as "every scope the
  credential can reach", but an empty list produced one unscoped request, which
  a multi-scope app client rejects on all nine exception sub-systems and the
  sectool paths with HTTP 400. Discovery failure warns and continues, since a
  single-scope credential does not need it.
