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
  toolchain (report upstream; see `NOTICE`). Upstream has no private
  vulnerability reporting channel today, so send anything exploitable to
  security@servosity.com instead of a public upstream issue, and we will pass
  it on privately.

## Handling credentials safely

These tools never store credentials in this repository. You supply your own at
runtime via environment variables or your agent's config. Per-skill guidance
lives in each skill's `governance.md`.

**HTTP mode.** 77 connectors can serve their MCP server over HTTP
(`--transport http`). Only 24 of them require a bearer token in that mode: backup-radar,
bitdefender, cisco-duo, cisco-meraki, connectsecure, connectwise-rmm, datagate,
datto-bcdr, fortinet, google-workspace, hubspot, huntress,
jira-service-management, proxmox, quotewerks, saas-alerts, sentinelone,
sonicwall, sophos-central, td-synnex, threatdown, watchguard, wordpress, zammad.
The other 53 have no password at all; putting a login on a tunnel or proxy in
front of one protects only that public URL, not the server, which still answers
anything that can reach it directly. Until each connector is reprinted, do not
run any of them in HTTP mode, token or not: use the default stdio transport. The
security note at the top of each connector's `mcp-install.md` says why for that
build. If you already exposed one through a tunnel, shut it down and rotate the
credentials it held.

**What stays on your machine.** Each CLI and MCP server keeps a local SQLite
copy of what it reads under `~/.local/share/<slug>-cli/` and raw API responses
under `~/.cache/<slug>-cli/http` (on Windows, the same folders under
`%USERPROFILE%`); neither is encrypted, the local copy does not expire, and no
command deletes it yet, so delete those folders to remove the data. The MCP
tools can also make changes with your credentials: 53 connectors expose an
`<api>_execute` tool that can call any write endpoint with no confirmation
step. The 17 connectors that support a `<PREFIX>_MCP_READ_ONLY` switch now set
it to `1` in their install guides, and their one-click `.mcpb` bundles preset it
from each connector's next release.
