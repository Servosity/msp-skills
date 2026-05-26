# Servosity MCP - install for Claude Desktop and ChatGPT Desktop

This page is for users who want the Servosity MCP server in a desktop AI like Claude Desktop or ChatGPT Desktop. If you use Claude Code or Codex CLI, install the Skill instead (see [README.md](./README.md)).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `servosity-cli` and `servosity-mcp` on your PATH. `servosity-mcp` is what the desktop agents talk to.

Verify:

```bash
servosity-mcp --help
```

## Get a Servosity partner API token

Log into the Servosity partner portal and generate a partner API token. Save it somewhere safe.

## Claude Desktop

Edit your Claude Desktop config:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add (or merge with your existing `mcpServers` block):

```json
{
  "mcpServers": {
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "<your-partner-token>"
      }
    }
  }
}
```

Quit Claude Desktop completely and reopen. Ask: "Show me stale backups across all my Servosity clients" - if the MCP is wired, Claude will use it.

## ChatGPT (Developer Mode)

ChatGPT connects to MCP servers differently than Claude Desktop. It does **not** launch a local binary - it connects to a **remote MCP server over HTTPS** through Developer Mode (beta; on Pro, Plus, Business, Enterprise, and Education plans). So this is not the same one-liner as Claude Desktop.

The `servosity-mcp` binary can serve the streamable-HTTP transport ChatGPT expects, so the path is:

1. Run the MCP server in HTTP mode with your token in the environment:

   ```bash
   SERVOSITY_MSP_TOKEN=<your-partner-token> servosity-mcp --transport http --addr :7777
   ```

2. Expose `http://localhost:7777` as a public **HTTPS** URL with a secure tunnel (for example Cloudflare Tunnel or ngrok). Treat that URL as sensitive: anyone who reaches it can act on your Servosity partner account, including destructive operations.
3. In ChatGPT: Settings > Apps > Advanced > Developer mode, then create a custom connector pointing at your tunnel's HTTPS URL.

Official OpenAI guidance (beta, plan-dependent, subject to change): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

For the simplest path, use Claude Desktop (above) or the Claude Code / Codex Skill.

## Troubleshooting

- `servosity-mcp: command not found` after install: the install dir is not on your PATH. The installer prints the line you need to add to your shell rc file (macOS / Linux) or sets the user PATH (Windows; open a new terminal after install).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax error. Run it through a JSON linter, fix, save, restart.
- `401 Unauthorized` from the MCP: the partner token is wrong or expired. Regenerate in the partner portal and update the env block.
- Need to update credentials: change the env block in the JSON config and restart Claude Desktop.

For the full CLI command reference (everything the MCP also exposes as tools), see [guide.md](./guide.md).
