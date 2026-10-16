# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: SonicWall CLI (`sonicwall-cli`) and MCP server
  (`sonicwall-mcp`), read-only against NSM SaaS, the firewalls through NSM's
  SonicOS passthrough, and MySonicWall.
- `nsm-login` mints and saves the NSM session token from a MySonicWall API key
  (SonicWall's documented access-code exchange); `doctor` verifies the token with
  one authenticated NSM read.
- Insurance posture: `posture` grades every firewall pass / warn / fail / unknown
  for firmware, end of support, advisories, licence expiry, admin TOTP, SSL-VPN
  exposure and SSL-VPN users without MFA, and saves each run so
  `posture changes` shows what got worse.
- Advisory triage: `exposure` ranks the advisories hitting the most firewalls,
  or lists every firewall one SNWLID or CVE hits and whether SSL-VPN is on a
  WAN-named zone; `firmware drift` shows builds behind and what the upgrade
  clears.
- `mfa-audit` (admin TOTP and local SSL-VPN users without a one-time password),
  `licenses renewals` (MySonicWall licence expiry with a replace-instead flag)
  and `blind-spots` (MySonicWall-registered firewalls NSM cannot see).
- Typed list / get commands across NSM, SonicOS passthrough and MySonicWall,
  `nsm-get` for documented NSM read routes, and a local SQLite mirror
  (`sync`, `search`).
- Secret fields SonicWall's API schemas document (local-user and device
  passwords, `assign_token`, registration and authentication codes, SNMP
  community names, MySonicWall `api_key`, `tcpassword`, `authCode`,
  `activationKey`) are blanked in every NSM, SonicOS and MySonicWall response
  before any output, cache, sync or MCP result.
