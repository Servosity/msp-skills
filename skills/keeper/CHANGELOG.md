# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-05

### Added
- First release: read-only Keeper admin and MSP reports through Keeper Commander Service Mode, as a CLI and an MCP server.
- `posture report`, `posture diff`, `posture runs` and `events list`: cross-client 2FA, password health, BreachWatch, inactivity and seats, with stored history and a change feed.
- `users risk`, `users hygiene` and `users find` across every managed company, from the newest stored run.
- A fixed command allowlist (`keeper-cli allowlist`); no path to a vault record.
- The shared credential kit: the Service Mode API key is held from the first statement of the program and withheld from every output, file, log and MCP result.

### Security
- The API key goes only to `KEEPER_BASE_URL` over https. Plain http is refused on every host, localhost included, and no redirect is followed.
