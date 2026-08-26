# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **`LIONGARD_ACCESS_KEY_ID` and `LIONGARD_ACCESS_KEY_SECRET` were ignored, so every call went out
  unauthenticated.** Liongard hands you an Access Key ID and an Access Key Secret and never shows
  the encoded form the API actually wants, and the README, the SKILL and the guide all tell you to
  export those two variables. The config loader read neither: it only ever looked for a
  pre-encoded `LIONGARD_API_KEY`, so an operator who followed the shipped instructions got an empty
  `X-ROAR-API-KEY` header and a 401 on every command, with nothing in `doctor` naming the cause.
  The loader composes the pair into base64 of `accessKeyId:accessKeySecret` again, as it did when
  the connector first shipped; a pre-encoded `LIONGARD_API_KEY` still wins when both are set.
  Both variables are now declared on all three install channels, so Claude Desktop asks for them
  and the copy-paste MCP config blocks carry them. `LIONGARD_API_KEY` is no longer marked required
  in `manifest.json` or `server.json` either: while it was, the MCPB bundle and the registry
  install both forced a value into it, and because the pre-encoded key wins the ID/secret pair
  stayed dead through every official install no matter what the docs said. All three fields now
  state the one-of contract - set the key or set the pair, never both - and the install pages show
  the two setups separately instead of one line that sets all three at once.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://{instance}.app.liongard.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `LIONGARD_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `LIONGARD_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `liongard-cli` and the `liongard-mcp` MCP server,
  covering the full Liongard API surface (environments, systems, launchpoints,
  agents, inspectors, metrics, detections, timeline, users, access keys).
- Offline local SQLite mirror via `sync`, with FTS5 full-text `search` and
  `analytics` over your whole estate.
- Cross-estate rollups that no single API call returns: `drift` (change feed
  joined to environment and system), `health` (one estate scorecard with a typed
  exit code), `launchpoints stale`, `agents offline`, `detections failures`,
  `coverage`, and `inspectors coverage`.
- Reporting helpers: `metrics pivot` (one metric across every system, CSV-ready)
  and `metrics breach` (systems crossing a numeric threshold).
- Per-environment `environments overview` and per-system `systems history` views.
- `--agent` JSON mode on every command, plus `doctor` for auth/connectivity
  checks and `tail` for live change polling.
