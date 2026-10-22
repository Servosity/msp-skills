# TD SYNNEX MCP - install for every agent that speaks MCP

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

This page wires the TD SYNNEX MCP server into any MCP client. If you use Claude
Code, Codex CLI, or Cowork, install the Skill instead (see [README.md](./README.md)) -
it's simpler. Everyone else: pick your agent below.

**Two install classes.** *Local* agents (Claude Desktop, GitHub Copilot, Gemini CLI)
launch `td-synnex-mcp` directly on your machine - no hosting. *Remote* agents (ChatGPT,
Microsoft 365 Copilot / Copilot Studio, the Gemini app) only talk to an HTTPS
endpoint, so they need HTTP mode, which is paused for this connector until
it is reprinted (see the security note above).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `td-synnex-cli`
and `td-synnex-mcp` on your PATH. `td-synnex-mcp` is what the agents talk to.

```bash
td-synnex-mcp --help
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
    "td-synnex": {
      "command": "td-synnex-mcp",
      "env": {
        "TD_SYNNEX_ACCOUNT_ID": "<your ION account ID>",
        "TD_SYNNEX_ACCESS_TOKEN": "",
        "TD_SYNNEX_REFRESH_TOKEN": "",
        "TD_SYNNEX_BASE_URL": "",
        "TD_SYNNEX_MCP_READ_ONLY": "1",
        "TD_SYNNEX_MCP_HTTP_TOKEN": "",
        "TD_SYNNEX_USER_AGENT": "",
        "TD_SYNNEX_TAIL_MAX_PAGES": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

`TD_SYNNEX_MCP_READ_ONLY` is `1` in every JSON config on this page. Remove this
line only when you want the agent to make changes.

Before the first start, mint a token with the CLI (the MCP server cannot do it): `td-synnex-cli auth mint --refresh-token-stdin`, piping in the refresh token from **ION portal > Settings > Users > Edit > OAuth credentials > Request New Credentials**. The server reads the token that command saves. Leave `TD_SYNNEX_ACCESS_TOKEN` and `TD_SYNNEX_REFRESH_TOKEN` empty: a pasted access token expires after 2 hours and keeps overriding the saved one. Keep it fresh with `td-synnex-cli auth mint --if-expiring 15m` from cron or Task Scheduler. `TD_SYNNEX_MCP_READ_ONLY` is preset to `1`, which removes every tool that can change ION (any value except blank, `0`, `false`, `no` or `off` turns it on); leave the other empty values empty unless you need them.

Quit Claude Desktop completely and reopen, then ask a question that needs the API.

## GitHub Copilot (VS Code)

GitHub Copilot supports MCP in **Agent mode** (GA since VS Code 1.102, July 2025).
Two gotchas trip people up: the config file is `mcp.json` (not `settings.json`), and
the root key is **`servers`** (not `mcpServers` like Claude).

Create `.vscode/mcp.json` in your workspace (or open the Command Palette > **MCP: Open User
Configuration**) and add:

```json
{
  "servers": {
    "td-synnex": {
      "type": "stdio",
      "command": "td-synnex-mcp",
      "env": {
        "TD_SYNNEX_ACCOUNT_ID": "<your ION account ID>",
        "TD_SYNNEX_ACCESS_TOKEN": "",
        "TD_SYNNEX_REFRESH_TOKEN": "",
        "TD_SYNNEX_BASE_URL": "",
        "TD_SYNNEX_MCP_READ_ONLY": "1",
        "TD_SYNNEX_MCP_HTTP_TOKEN": "",
        "TD_SYNNEX_USER_AGENT": "",
        "TD_SYNNEX_TAIL_MAX_PAGES": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
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
    "td-synnex": {
      "command": "td-synnex-mcp",
      "env": {
        "TD_SYNNEX_ACCOUNT_ID": "<your ION account ID>",
        "TD_SYNNEX_ACCESS_TOKEN": "",
        "TD_SYNNEX_REFRESH_TOKEN": "",
        "TD_SYNNEX_BASE_URL": "",
        "TD_SYNNEX_MCP_READ_ONLY": "1",
        "TD_SYNNEX_MCP_HTTP_TOKEN": "",
        "TD_SYNNEX_USER_AGENT": "",
        "TD_SYNNEX_TAIL_MAX_PAGES": "",
        "PRINTING_PRESS_CLIENT_PROFILE": ""
      }
    }
  }
}
```

Restart Gemini CLI; the TD SYNNEX tools appear in its tool list. (The **Gemini app /
web** is remote-only - see the remote section below.)

---

# Remote agents (paused until this connector is reprinted)

ChatGPT, Microsoft 365 Copilot / Copilot Studio and the Gemini app only connect
to an MCP server over HTTPS, which means running `td-synnex-mcp` in HTTP mode.
Do not do that with this build (see the security note at the top of this page),
and do not put this server behind a tunnel or reverse proxy. Use a local agent
above, or the Claude Code / Codex Skill, until the fix release brings these
steps back.

The HTTP-mode launch command is kept below for reference. It is commented out on
purpose, so pasting it does nothing:

```bash
# TD_SYNNEX_ACCOUNT_ID=<value> TD_SYNNEX_MCP_HTTP_TOKEN=<a long random value> td-synnex-mcp --transport http
```

---

# Skill-native agents

[Hermes](https://hermes-agent.nousresearch.com) and OpenClaw read this skill's
`SKILL.md` directly, and both also speak MCP. Register the server directly:

```bash
# Hermes (also supports `hermes skills install ...` - see README.md)
hermes mcp add td-synnex -- td-synnex-mcp

# OpenClaw
openclaw mcp set td-synnex '{"command":"td-synnex-mcp"}'
```

Same env vars as the blocks above, including `TD_SYNNEX_MCP_READ_ONLY=1`. For the Skill-install path (no MCP wiring), see
the "Install for Hermes" / "Install for OpenClaw" sections in [README.md](./README.md).

For the simplest path overall, use Claude Desktop or the Claude Code / Codex Skill.

## Troubleshooting

- `td-synnex-mcp: command not found`: the install dir is not on your PATH (the
  installer prints the line to add).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax
  error. Validate it, fix, restart.

For the full CLI command reference, see [guide.md](./guide.md).
