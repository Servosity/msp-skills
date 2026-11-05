# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-04

### Added
- First msp-skills release of the NetSapiens CLI (`netsapiens-cli`) and MCP server
  (`netsapiens-mcp`), built on CLI Printing Press 4.34.2, covering every NetSapiens v2
  endpoint on any white-label platform (`NETSAPIENS_SERVER`).
- Reseller-wide audits over a local SQLite mirror: `fleet seats`, `fleet offline`,
  `fleet dids`, `fleet e911` and `fleet changes`, plus live `fleet missed` and
  `fleet queues` from CDRs and queue statistics.
- `trace` resolves one number, extension, MAC or email to its client, routing, user
  and phones.
- Every MCP write previews first and sends only with `confirm: true`;
  `NETSAPIENS_MCP_READ_ONLY=1` refuses every write. HTTP mode requires
  `NETSAPIENS_MCP_HTTP_TOKEN`.
- Credentials are held from startup by the shared credential kit and filtered out of
  every output: terminal, errors, files, MCP results, the local mirror and the learning
  journal. A credential only goes to the configured server over HTTPS.
