# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - unreleased

### Added
- Initial msp-skills release: `prtg-cli` and the `prtg-mcp` MCP server for
  PRTG Network Monitor API v2, built on CLI Printing Press 4.34.2.
- Fleet view across every client's PRTG core: register cores with `core add`,
  then `fleet alarms`, `fleet status`, `fleet handoff`, `fleet silenced`,
  `fleet drift` and `fleet exec` ask every core in one run; `fleet sync` keeps an
  offline mirror per core.
- Previewed bulk actions across cores: `fleet ack`, `fleet pause` and
  `fleet resume` show what they would do on each core and send only with `--yes`
  (over MCP, only with `confirm: true`).
- Every PRTG API v2 operation as a typed command, pointable at one core with
  `--core`.

### Security
- Credentials are held from every source before any output, and every output
  path (terminal, MCP results, errors, local files) withholds a held value in any
  encoding; the license key returned by `system license-info` is never stored.
- Requests go only to the configured PRTG core: a redirect or a request to any
  other host is refused.
- Every MCP tool that changes PRTG previews until called with `confirm: true`.
- A bulk action reports failure when any item in PRTG's result failed.
