# Security Policy

`msp-skills` ships Skills, CLIs, and MCP servers that operate real MSP systems
(PSA, RMM, backup, DR) holding privileged, multi-tenant access. We take security
reports seriously.

## Reporting a vulnerability

Please do **not** open a public issue for security problems.

- Preferred: use GitHub's private vulnerability reporting on this repository
  (Security tab > "Report a vulnerability"), if enabled.
- Or email **security@servosity.com** with the details.

Include, where you can:

- the affected skill, binary, or doc, and the version (`<binary> --version`),
- your OS and agent (Claude Code, Codex, Claude Desktop, ChatGPT),
- reproduction steps and impact,
- any suggested remediation.

We aim to acknowledge reports within a few business days and will coordinate a
fix and disclosure timeline with you.

## Scope

In scope:

- the install scripts (`skills/*/install.sh`, `*.ps1`, `tools/**`),
- the vendored CLI / MCP source under `skills/*/cli/`,
- credential handling, token scoping, and any path that could leak secrets or
  cross a tenant boundary,
- the release workflow and published binaries.

Out of scope (report to the upstream vendor instead):

- vulnerabilities in HaloPSA, Servosity, or any other vendor's API or product,
- the [canonical statusline repo](https://github.com/servosity/claude-code-statusline)
  (report there),
- generator-level issues in the [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
  toolchain (report upstream; see `NOTICE`).

## Handling credentials safely

These tools never store credentials in this repository. You supply your own at
runtime via environment variables or your agent's config. If you bridge an MCP
server to a public endpoint (for example for ChatGPT), treat that URL as
sensitive and rotate the underlying token afterward. Per-skill guidance lives in
each skill's `governance.md`.
