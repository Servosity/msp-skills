> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/github-copilot/

# HaloPSA and Servosity in GitHub Copilot

GitHub Copilot runs MCP servers in **Agent mode**, generally available since VS Code 1.102 (July 2025). It's the Microsoft surface that takes MSP Skills' local binaries **today** - no hosting, no license gymnastics. This guide installs the HaloPSA and Servosity MSP Skills as MCP servers in GitHub Copilot.

**Two gotchas up front:** the config file is `mcp.json` (not `settings.json`), and the root key is **`servers`** (not `mcpServers` like Claude). Miss either and Copilot won't see the tools.

## What you need

- **VS Code** with **GitHub Copilot** (any Copilot plan, including free)
- A **HaloPSA tenant** with API credentials **or** a **Servosity partner API token** - or both
- A terminal once for install

## Step 1 - Install MSP Skills binaries

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Install only the ones you'll use. Each installer drops the CLI and the MCP server on your PATH.

## Step 2 - Add the servers to `mcp.json`

Open the Command Palette → **MCP: Open User Configuration** (or create `.mcp.json` in your workspace), and add - note the **`servers`** root key and `"type": "stdio"`:

```json
{
  "servers": {
    "halopsa": {
      "type": "stdio",
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<your-tenant>",
        "HALOPSA_CLIENT_ID": "<your-client-id>",
        "HALOPSA_CLIENT_SECRET": "<your-client-secret>"
      }
    },
    "servosity": {
      "type": "stdio",
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "<your-partner-token>"
      }
    }
  }
}
```

Save the file. (You can also install from the Extensions view: search `@mcp` in the gallery.)

## Step 3 - Switch Copilot Chat to Agent mode

Open Copilot Chat and set the mode dropdown to **Agent**. MCP tools are invisible in Ask and Edit modes - this is the single most common "it's not working" cause.

## Step 4 - Ask a real question

In Copilot Chat (Agent mode):

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

Copilot discovers the MCP tools the servers expose and runs them.

## Troubleshooting

**Tools never appear:** you used `mcpServers` instead of `servers`, or you're not in Agent mode. Fix both.

**"halopsa-mcp: command not found":** the binary isn't on your PATH. The installer prints the line to add; restart VS Code after adding it.

**Authentication errors:** confirm `halopsa-cli doctor` or `servosity-cli doctor` works in a terminal first. If the CLI authenticates, the MCP server will too with the same credentials.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **Live in the Microsoft 365 business apps instead?** See the [Microsoft 365 Copilot / Copilot Studio guide](https://msp-skills.compoundingteams.com/integrations/microsoft-365-copilot.md) (remote path).

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
