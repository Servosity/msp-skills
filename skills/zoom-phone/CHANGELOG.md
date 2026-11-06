# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-06

### Added
- Zoom Phone CLI and MCP server covering the Zoom Phone API (420 MCP tools), with a local SQLite mirror filled by `sync`.
- `e911 audit`: users, common areas and sites with a missing or unverified emergency address, sites with no emergency service location, BYOC numbers needing a carrier update.
- `routing trace` and `routing audit`: where one number rings for business and closed hours, and an account-wide sweep for dead IVR targets, empty call queues, orphaned auto receptionists and missing holiday hours.
- `numbers inventory`, `devices health`, `footprint` and `callbacks`: number and desk-phone hygiene by site, everything one user holds, and external callers still waiting for a call back.
- `offboard`: an ordered removal plan for a departing user; `--apply` sends it after a live re-check of the user and of every group family.
- `accounts connect` and `fleet`: one saved profile per client Zoom account, and any read (or sync) run across all of them with an account column.
- Server-to-Server OAuth (`account_credentials` grant) with the Account ID, Client ID and Client Secret.
- The shared credential kit on both binaries: output, local files, the local store, child processes and outbound requests all pass it, and a credential only goes to the configured API origin and Zoom's token hosts.
