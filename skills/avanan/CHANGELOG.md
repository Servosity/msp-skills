# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **The MCP Registry listing was being rejected.** The one-line description was 107 characters
  against a 100-character limit, so every publish failed validation after the release itself
  had already succeeded. Shortened, and the whole fleet is now checked against the registry's
  own schema before publishing.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `AVANAN_BASE_URL`, `AVANAN_TOKEN`, `AVANAN_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE`.

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
- `remediate --wait` can actually observe the thing it waits for. Both poll
  loops read through the response cache, which is keyed on path and params, so
  every iteration after the first saw the pre-action state: an entity that
  flipped three seconds after submission was still reported unchanged two
  minutes later. They now bypass the cache.
- A submission that returns no task id is confirmed against the entity's own
  state rather than assumed applied. Google Mail applies these actions
  asynchronously without a task, so a 200 means accepted, not landed.
- The mirror walks every page instead of stopping at two. Avanan's `scrollId` is
  a stable server-side cursor handle that repeats on every page while the
  records advance, and the walk treated an unchanged value as a looping server.
  A 24-hour window that previously reported 605 records now stores 3,788.
- `remediate quarantine` and `remediate restore` work at all. They sent a
  hardcoded `entityType: "email"`, which the API answers with HTTP 500
  `internal_error[KeyError]`; the type is per-platform and per-entity, so it is
  now read from the entity itself (with `--entity-type` to override).
- `remediate --wait` polls with the scope the task endpoint requires in its
  query string, reports a poll that could not be made instead of silently
  timing out, treats a synchronous action that returns no task id as applied,
  and no longer implies failure when a task lags an entity that already moved.
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
  - `timeline` reconstructs one message's history from the local mirror, in order: detection, state
    changes, the actions this CLI submitted, task outcomes, and restore disposition, which
    otherwise spans three endpoints and a task record that ages out.
  - `exceptions find` asks whether a domain, sender, URL, or hash is excepted
    anywhere across all seven engines and nine exception tables at once.
  - `exceptions audit` flags exceptions that contradict each other across
    sub-systems, exact duplicates, and entries that have not matched
    traffic in the mirrored window.
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
