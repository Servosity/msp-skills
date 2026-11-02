---
layout: default
title: "HaloPSA + Servosity in the ChatGPT desktop app"
description: "Use HaloPSA and Servosity in the ChatGPT desktop app, which runs MCP servers locally (STDIO): run the installer, then add the connector under Settings > MCP servers or once in Codex's config. ChatGPT on the web is not supported today. Free MSP Skills, runs on your machine."
permalink: /integrations/chatgpt/
faqs:
  - q: "Does ChatGPT support HaloPSA?"
    a: "Yes, in the ChatGPT desktop app, via MSP Skills' free HaloPSA MCP server running locally on your machine. Add it under Settings > MCP servers as a STDIO server whose command is the halopsa-mcp binary, or add it once to Codex's ~/.codex/config.toml, which the desktop app shares. ChatGPT on the web is not supported today."
  - q: "Does ChatGPT support MCP?"
    a: "Yes. OpenAI's docs say the ChatGPT desktop app, Codex CLI and the IDE extension support MCP servers and share MCP configuration, and that you add a server as STDIO (a local command) or Streamable HTTP (a URL). ChatGPT on the web uses remote MCP tools through plugins and doesn't read local Codex configuration files."
  - q: "Can I use MSP Skills in ChatGPT on the web?"
    a: "Not today. ChatGPT on the web needs a remote (HTTP) MCP server, and MSP Skills' SECURITY.md says not to run any connector in HTTP mode until it is reprinted. Use the ChatGPT desktop app instead. If your team works in ChatGPT on the web, a hosted gateway or your vendor's own hosted server is the better pick."
  - q: "Do I need to host my own server for ChatGPT to use MSP Skills?"
    a: "No. In the ChatGPT desktop app the connector runs on your machine as a local STDIO server: no tunnel and no public URL."
---

# HaloPSA and Servosity in ChatGPT

The **ChatGPT desktop app** runs MCP servers locally (STDIO), which is how MSP Skills connectors run. It shares MCP configuration with Codex CLI on the same machine ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). This guide adds the MSP Skills HaloPSA and Servosity MCP servers to it.

**ChatGPT on the web is not supported today.** It needs a remote (HTTP) MCP server, and our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says not to run any connector in HTTP mode until it is reprinted. If your team works in ChatGPT on the web, a hosted gateway or your vendor's own hosted server is the better pick ([how they compare](/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick)).

## What you need

- **The ChatGPT desktop app**
- **A HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token** - or both
- A terminal for install (after that, ChatGPT chat handles the rest)

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

Install only the skills you'll use.

## Step 2 - Add the connector to the ChatGPT desktop app

**Option A - in the app.** Open **Settings**, then select **MCP servers**. Add a server, choose **STDIO**, and give the connector's MCP binary as the command: `halopsa-mcp` (or `servosity-mcp`). If it isn't found, use its full path, for example `/Users/<you>/.local/bin/halopsa-mcp` on macOS. The connector reads its credentials from environment variables; each skill's `mcp-install.md` lists them.

**Option B - once in Codex's config.** The desktop app shares MCP configuration with Codex, so a connector in `~/.codex/config.toml` is available in both. Already set one up for [Codex CLI](/integrations/codex/)? It's already there.

```toml
[mcp_servers.halopsa]
command = "halopsa-mcp"
env = { HALOPSA_TENANT = "<your-tenant>", HALOPSA_CLIENT_ID = "<id>", HALOPSA_CLIENT_SECRET = "<secret>" }

[mcp_servers.servosity]
command = "servosity-mcp"
env = { SERVOSITY_MSP_TOKEN = "<token>" }
```

**What we tested (November 2, 2026):** the ChatGPT desktop app (v26.1027) bundles its own Codex engine. We ran that engine with our live-verified [Zammad connector](/skills/zammad/) in local (STDIO) mode: it loaded all 38 of the connector's tools and answered a read-only query. We have not yet walked through the app's settings screen ourselves, so Option A follows OpenAI's docs.

## Step 3 - Ask a real question

In ChatGPT chat:

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

ChatGPT discovers the MCP tools the connector exposes and runs them.

## Cost

- **MSP Skills:** Free (Apache-2.0).
- **ChatGPT:** see [OpenAI's pricing](https://openai.com/chatgpt/pricing).
- **HaloPSA / Servosity access:** your existing license / partner agreement.

## Security

- The connector runs on your machine over STDIO: no listener and no public URL to protect.
- Don't run a connector in HTTP mode or put it behind a tunnel to reach ChatGPT on the web. [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says why.
- HaloPSA + Servosity credentials stay in your local environment or config file and are never transmitted to MSP Skills.

## Troubleshooting

**ChatGPT doesn't show the connector's tools:** test the underlying connector with `halopsa-cli doctor` or `servosity-cli doctor` first. Then check that the command is the MCP binary (`halopsa-mcp`, not `halopsa-cli`), using its full path if the app can't find it.

**`401 Unauthorized` from the connector:** the credentials are wrong. Recheck them against the vendor portal.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **Prefer Claude?** [Claude Desktop](/integrations/claude-desktop/) runs the same local connector.

[← Back to main install](/#install-in-60-seconds)
