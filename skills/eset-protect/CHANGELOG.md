# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

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
- The bearer token is sent only to `*.eset.systems` or the configured base URL,
  token requests never follow redirects, and `ESET_PROTECT_MCP_READ_ONLY=1`
  keeps an MCP agent to reads.
