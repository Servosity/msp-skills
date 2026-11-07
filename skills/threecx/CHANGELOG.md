# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-06

### Added
- First release of the 3CX CLI and MCP server, built on the official 3CX V20
  Configuration API (XAPI) specification: every one of its 606 operations as a
  command, against any single phone system selected with `--pbx`.
- Fleet commands for MSPs with one PBX per client, each PBX with its own API
  credential: `fleet add`, `fleet list`, `fleet remove`, `fleet health`,
  `fleet trunks`, `fleet licences`, `fleet versions`, `fleet backups`,
  `fleet changes` and the read-only `fleet exec`.
- A health verdict is `unknown`, never `pass`, when the phone system did not
  report a fact the check depends on.
- Credentials are withheld from every output and file by the shared credential
  kit; requests and redirects to any host other than the configured phone
  system are refused; `--dry-run`
  sends nothing; MCP writes preview until `confirm: true`.
