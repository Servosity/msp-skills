# Install an MCP server (Claude Desktop, ChatGPT Desktop)

This page is for users of a desktop AI app that speaks MCP (Model Context Protocol). Claude Desktop launches the MCP server as a local binary; ChatGPT (Developer Mode, beta) connects to a remote MCP server over HTTPS instead, so its setup differs - see the per-skill `mcp-install.md` for the ChatGPT path. If you use Claude Code or Codex CLI, install the Skill instead via [install-skill.md](./install-skill.md).

## What MCP gives you

MCP is the open standard that lets a desktop AI app launch a local server and call functions on it. After installing the HaloPSA MCP server, you can ask Claude Desktop "Show me my open HaloPSA tickets" and Claude calls the MCP server to get a real answer from the API. Same idea for Servosity ("Which backups are stale across my fleet?").

## What you install

One binary per skill, sitting on your PATH:

- `halopsa-mcp` for HaloPSA
- `servosity-mcp` for Servosity

The install scripts in each skill directory drop these on your PATH. The same install scripts also drop the matching CLI binary; that is fine and harmless if you only use MCP. You will not see them unless you open a terminal.

## Install in two steps

### 1. Run the install script

```bash
bash skills/halopsa/install.sh
# or
bash skills/servosity/install.sh
```

Windows:

```powershell
.\skills\halopsa\install.ps1
```

This drops `halopsa-mcp` (or `servosity-mcp`) on your PATH and prints the verify command. Re-run later to update.

### 2. Wire the MCP into your desktop AI

Open the per-skill `mcp-install.md` for the exact JSON snippet:

- [skills/halopsa/mcp-install.md](../skills/halopsa/mcp-install.md)
- [skills/servosity/mcp-install.md](../skills/servosity/mcp-install.md)

The two desktop apps wire MCP differently:

- **Claude Desktop:** edit `claude_desktop_config.json`, add a `mcpServers` block pointing at the local MCP binary, set the env vars for credentials, restart the app. This is the simple, supported local path.
- **ChatGPT (Developer Mode, beta):** ChatGPT connects only to remote MCP servers over HTTPS - it does not launch a local binary. Run the MCP binary in HTTP mode (`--transport http`), expose it as a public HTTPS URL via a secure tunnel, and register that URL as a custom connector. Full steps are in each skill's `mcp-install.md`.

## Restart is required

Claude Desktop does not hot-reload its MCP config. After editing the JSON, quit Claude Desktop completely (Command-Q on macOS, right-click the tray icon on Windows) and reopen. Then ask a question that requires the API and confirm Claude reaches for the MCP tool.

## Auth

Each skill's `mcp-install.md` lists the credentials you need (HaloPSA client ID + secret + tenant, Servosity partner token, etc.) and where to get them. The MCP server reads credentials from env vars passed in the JSON config block; there is no separate credential store outside the MCP process.

## Troubleshooting

- Claude Desktop does not see the new MCP after restart: the JSON config is invalid. Open it in a JSON validator (https://jsonlint.com), fix the syntax error, save, restart Claude Desktop.
- `command not found` style error from Claude Desktop's MCP error log: the MCP binary is not on PATH for Claude Desktop. Use the full absolute path in the `command` field of the JSON config, for example `/Users/<you>/.local/bin/halopsa-mcp` on macOS or `C:\Users\<you>\AppData\Local\Programs\msp-skills\halopsa-mcp.exe` on Windows.
- `401 Unauthorized` from the MCP: credentials in the env block are wrong. Recheck against the vendor portal.

If you are not sure which agent you have, read [which-agent.md](./which-agent.md).
