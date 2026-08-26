# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **The credential-precedence tests could never pass, so nothing was watching which secret goes on
  the wire.** The four tests that pin the order - a saved credentials file beats an old secret left
  in `config.toml`, a corrupt credentials file falls back to that config and then to the
  environment, an empty one clears nothing - looked for the raw secret inside the `Authorization`
  header. This connector uses HTTP Basic, so the header carries the secret base64-encoded and the
  check could never match, whatever the code did. The tests now decode the header first and assert
  which credential it was built from. Runtime behaviour is unchanged: the precedence order was
  always correct, and is now actually verified.

## [0.1.2] - 2026-08-26

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
  authenticate. Now declared on every install channel: `WORDPRESS_BASE_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.0] - 2026-07-01

### Changed

- feat(wordpress): add WordPress pages/posts/media connector (#171)

