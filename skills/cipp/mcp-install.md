# CIPP MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build has no password,
> listens on every network interface, and predates the browser-rebinding guard,
> so a web page you open can reach it. The MCP tools can also make changes with
> your credentials. Use the default stdio transport. If you set up a tunnel for
> a remote agent, shut it down. A fix is on the way.
>
> For CIPP: the `cipp_execute` tool can call any of CIPP's 299 write actions
> with no confirmation, so until the fix ships, give the connector a CIPP API
> client with read-only roles only.

**In CIPP, a GET can change things.** 39 CIPP endpoints are called with GET but
named as actions (`Exec...`, `Remove...`). Some of them change your tenants or
your CIPP instance, for example `RemoveStandard`, `ExecSendOrgMessage` and
`ExecRunBackup`. In this build they run through `cipp_execute` with no
confirmation step, like every other call, so do not set your MCP host to
auto-approve `cipp_execute`, and do not treat a GET as safe. The full list is in
the [appendix](#appendix-cipp-get-endpoints-named-as-actions).

This page wires the CIPP MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `cipp-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `cipp-cli`
and `cipp-mcp` on your PATH. `cipp-mcp` is what the agents talk to.

```bash
cipp-mcp --help
```

---

# Local agents (launch the binary directly)

## Claude Desktop

Edit your Claude Desktop config:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add (or merge with your existing `mcpServers` block):

```json
{
  "mcpServers": {
    "cipp": {
      "command": "cipp-mcp",
      "env": {
        "CIPP_API_KEY": "<your-cipp_api_key>",
        "CIPP_BASE_URL": "<your-cipp_base_url>",
        "CIPP_NO_CONFIG_WRITE": ""
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "cipp": {
      "type": "stdio",
      "command": "cipp-mcp",
      "env": {
        "CIPP_API_KEY": "<your-cipp_api_key>",
        "CIPP_BASE_URL": "<your-cipp_base_url>",
        "CIPP_NO_CONFIG_WRITE": ""
      }
    }
  }
}
```

Then open Copilot Chat and switch the mode dropdown to **Agent** - MCP tools are
invisible in Ask/Edit mode.

## Gemini CLI (Google)

Edit `~/.gemini/settings.json` (Gemini CLI's config) and add the same shape as
Claude Desktop:

```json
{
  "mcpServers": {
    "cipp": {
      "command": "cipp-mcp",
      "env": {
        "CIPP_API_KEY": "<your-cipp_api_key>",
        "CIPP_BASE_URL": "<your-cipp_base_url>",
        "CIPP_NO_CONFIG_WRITE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the CIPP tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `cipp-mcp` in HTTP mode. Do not
do that with this build (see the security note at the top of this page), and do
not put this server behind a tunnel or reverse proxy. Use a local agent above,
or the Claude Code / Codex Skill, until the fix release brings these steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# CIPP_API_KEY=<value> CIPP_BASE_URL=<value> cipp-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add cipp -- cipp-mcp

# OpenClaw
openclaw mcp set cipp '{"command":"cipp-mcp"}'
```

Same env vars as the blocks above. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `cipp-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).

## Request discovery

The endpoint search tool returns `params_schema` alongside each result. Read its
`properties` and `required` fields before calling the executor. `x-location` identifies
where an input is sent; `x-wire-name`, when present, names its API parameter.
Keep JSON body key capitalization and nested objects exactly as shown. Known enum
values are included in the schema and can be used as search terms.

## Appendix: CIPP GET endpoints named as actions

The 39 GET endpoints in this build whose path starts with Exec, Remove, Add,
Set, Reset, Run, Invoke, Send, Create, Delete, Update or Assign. Some only read
(for example `ExecAlertsList`); treat all of them as possible changes until each
one is checked. `endpoint_id` is the value `cipp_execute` takes.

| Method | Path | endpoint_id |
| --- | --- | --- |
| GET | `/ExecAddSPN` | `exec-add-spn.list` |
| GET | `/ExecAlertsList` | `exec-alerts-list.list` |
| GET | `/ExecAPIPermissionList` | `exec-apipermission-list.list` |
| GET | `/ExecAppApproval` | `exec-app-approval.list` |
| GET | `/ExecAppInsightsQuery` | `exec-app-insights-query.list` |
| GET | `/ExecAppUpload` | `exec-app-upload.list` |
| GET | `/ExecBackendURLs` | `exec-backend-urls.list` |
| GET | `/ExecBECCheck` | `exec-beccheck.list` |
| GET | `/ExecBulkLicense` | `exec-bulk-license.list` |
| GET | `/ExecCIPPDBCache` | `exec-cippdbcache.list` |
| GET | `/ExecCPVRefresh` | `exec-cpvrefresh.list` |
| GET | `/ExecCreateDefaultGroups` | `exec-create-default-groups.list` |
| GET | `/ExecCSPLicense` | `exec-csplicense.list` |
| GET | `/ExecDeviceCodeLogon` | `exec-device-code-logon.list` |
| GET | `/ExecDurableFunctions` | `exec-durable-functions.list` |
| GET | `/ExecExtensionMapping` | `exec-extension-mapping.list` |
| GET | `/ExecExtensionNinjaOneQueue` | `exec-extension-ninja-one-queue.list` |
| GET | `/ExecExtensionSync` | `exec-extension-sync.list` |
| GET | `/ExecExtensionTest` | `exec-extension-test.list` |
| GET | `/ExecGDAPInviteApproved` | `exec-gdapinvite-approved.list` |
| GET | `/ExecGDAPTrace` | `exec-gdaptrace.list` |
| GET | `/ExecIncidentsList` | `exec-incidents-list.list` |
| GET | `/ExecListAppId` | `exec-list-app-id.list` |
| GET | `/ExecListBackup` | `exec-list-backup.list` |
| GET | `/ExecMailTest` | `exec-mail-test.list` |
| GET | `/ExecMailboxMobileDevices` | `exec-mailbox-mobile-devices.list` |
| GET | `/ExecMaintenanceScripts` | `exec-maintenance-scripts.list` |
| GET | `/ExecMdoAlertsList` | `exec-mdo-alerts-list.list` |
| GET | `/ExecPermissionRepair` | `exec-permission-repair.list` |
| GET | `/ExecRunBackup` | `exec-run-backup.list` |
| GET | `/ExecSchedulerBillingRun` | `exec-scheduler-billing-run.list` |
| GET | `/ExecSendOrgMessage` | `exec-send-org-message.list` |
| GET | `/ExecServicePrincipals` | `exec-service-principals.list` |
| GET | `/ExecStandardConvert` | `exec-standard-convert.list` |
| GET | `/ExecStandardsRun` | `exec-standards-run.list` |
| GET | `/ExecUniversalSearch` | `exec-universal-search.list` |
| GET | `/ExecUniversalSearchV2` | `exec-universal-search-v2.list` |
| GET | `/RemoveStandard` | `remove-standard.list` |
| GET | `/RemoveTenantCapabilitiesCache` | `remove-tenant-capabilities-cache.list` |
