# HaloPSA MCP - install for Claude Desktop and ChatGPT Desktop

> Unofficial. Community-built MCP server for the HaloPSA, HaloITSM, and HaloCRM
> APIs. Not affiliated with, endorsed by, or sponsored by Halo Service
> Solutions Ltd.

This page is for users who want the HaloPSA MCP server in a desktop AI like Claude Desktop or ChatGPT Desktop. If you use Claude Code or Codex CLI, install the Skill instead (see [README.md](./README.md)).

## Prerequisite: install the MCP binary

Run the install command from [README.md](./README.md). It drops both `halopsa-cli` and `halopsa-mcp` on your PATH. `halopsa-mcp` is what the desktop agents talk to.

Verify:

```bash
halopsa-mcp --help
```

## Get HaloPSA API credentials

You need three values from your HaloPSA tenant:

- `HALOPSA_TENANT` - your tenant subdomain (the `acme` part of `acme.halopsa.com`)
- `HALOPSA_CLIENT_ID` - from Configuration > Integrations > Halo PSA API
- `HALOPSA_CLIENT_SECRET` - generated alongside the client ID

In your Halo tenant: Configuration > Integrations > Halo PSA API > New Application. Authentication Method: Client ID and Secret (Services). Save the client ID and secret somewhere safe before you leave the page; Halo will not show the secret again.

## Claude Desktop

Edit your Claude Desktop config:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add (or merge with your existing `mcpServers` block):

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<your-tenant>",
        "HALOPSA_CLIENT_ID": "<your-client-id>",
        "HALOPSA_CLIENT_SECRET": "<your-client-secret>"
      }
    }
  }
}
```

Quit Claude Desktop completely (Command-Q on macOS, right-click in tray on Windows) and reopen. Ask: "List my open HaloPSA tickets" - if the MCP is wired, Claude will use it.

## ChatGPT (Developer Mode)

ChatGPT connects to MCP servers differently than Claude Desktop. It does **not** launch a local binary - it connects to a **remote MCP server over HTTPS** through Developer Mode (beta; on Pro, Plus, Business, Enterprise, and Education plans). So this is not the same one-liner as Claude Desktop.

The `halopsa-mcp` binary can serve the streamable-HTTP transport ChatGPT expects, so the path is:

1. Run the MCP server in HTTP mode with your credentials in the environment:

   ```bash
   HALOPSA_TENANT=<your-tenant> \
   HALOPSA_CLIENT_ID=<your-client-id> \
   HALOPSA_CLIENT_SECRET=<your-client-secret> \
   halopsa-mcp --transport http --addr :7777
   ```

2. Expose `http://localhost:7777` as a public **HTTPS** URL with a secure tunnel (for example Cloudflare Tunnel or ngrok). Treat that URL as sensitive: anyone who reaches it can drive your Halo tenant.
3. In ChatGPT: Settings > Apps > Advanced > Developer mode, then create a custom connector pointing at your tunnel's HTTPS URL.

Official OpenAI guidance (beta, plan-dependent, subject to change): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta

For the simplest path, use Claude Desktop (above) or the Claude Code / Codex Skill.

## Troubleshooting

- `halopsa-mcp: command not found` after install: the install dir is not on your PATH. The installer prints the line you need to add to your shell rc file (macOS / Linux) or sets the user PATH (Windows; open a new terminal after install).
- Claude Desktop does not see the MCP after restart: the JSON config has a syntax error. Run it through a JSON linter, fix, save, restart.
- `401 Unauthorized` from the MCP: client ID or secret is wrong, or your tenant has the API integration disabled. Recheck Configuration > Integrations > Halo PSA API.
- Need to update credentials: change the env block in the JSON config and restart Claude Desktop. There is no credential cache outside the MCP process.

For the full CLI command reference (everything the MCP also exposes as tools), see [guide.md](./guide.md).
