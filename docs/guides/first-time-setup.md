---
layout: default
title: "First-time setup for MSP business owners"
description: "Beginner-friendly walkthrough for setting up MSP Skills if you've never used Claude Code, an MCP server, or a CLI. About 10 minutes start to finish."
permalink: /guides/first-time-setup/
---

# First-time setup - for MSP business owners new to all this

This guide is for **MSP business owners** who want to use MSP Skills but haven't used Claude Code, an MCP server, or a CLI before. No coding required, no terminal experience required, no developer experience required. About 10 minutes start to finish.

If you're already comfortable in a terminal, the [main install](/#install-in-60-seconds) is faster.

## What you'll get

By the end of this guide, you'll be able to ask **Claude Desktop** (the most common MSP-owner choice) questions like:

- *"Use halopsa: what's about to breach SLA in the next 24 hours?"*
- *"Use servosity: which clients have backups that haven't succeeded in 7 days?"*
- *"Use halopsa: build a client card for Acme Corp."*

And get real answers from your live HaloPSA or Servosity data - no clicking through portals.

## Before you start

You need three things:

1. **A Mac or Windows computer** (Linux works too, this guide focuses on Mac/Windows).
2. **HaloPSA tenant + OAuth credentials**, OR a **Servosity MSP partner API token**, or both. (If you don't have one, you only install the other - they're independent.)
3. **About 10 minutes.**

## Step 1 - Download Claude Desktop (5 minutes)

Open your browser and go to **[claude.ai](https://claude.ai)**. If you don't have a Claude account, sign up - it's free. The free Claude Desktop is enough for this; you don't need a paid plan.

After signing in, look for a **Download** link on claude.ai (usually in the top-right menu, or there's a banner). Download Claude Desktop for your OS. Install it like any other Mac/Windows app - double-click the downloaded file, drag it to Applications (Mac) or run the installer (Windows).

Open Claude Desktop. You should see a chat window. **You're ready for step 2.**

## Step 2 - Open the Terminal (1 minute, once)

You only need the terminal **once** - for the install. After that, everything happens in Claude Desktop's chat window.

**On Mac:** press `Cmd + Space`, type `Terminal`, press Enter. A black window opens. You'll type one command into it.

**On Windows:** press the Windows key, type `PowerShell`, press Enter. A blue window opens. You'll type one command into it.

## Step 3 - Run the installer (2 minutes)

In the terminal you just opened, **copy and paste one of these commands** depending on what you have:

### If you have HaloPSA credentials:

**Mac:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**Windows:**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

### If you have a Servosity partner token:

**Mac:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows:**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

### If you have both - run both commands, one at a time.

Press Enter after pasting. You'll see some output as the installer downloads and sets up the files. When it's done, the prompt comes back.

## Step 4 - Find your credentials (3 minutes)

### For HaloPSA:

Open your HaloPSA portal in a browser → **Configuration → Integrations → Halo PSA API**. Create an API application:

- **Authentication Method**: Client ID and Secret (Services)
- Give it a name like "MSP Skills"
- After saving, **copy the Client ID and Client Secret** somewhere safe (a password manager is ideal)

Also note your **tenant name** - it's part of your HaloPSA URL. If your URL is `https://acmemsp.halopsa.com`, your tenant is `acmemsp`.

### For Servosity:

Log into the Servosity partner portal → look for **API** or **Tokens** section → generate or copy your **MSP partner API token**. (If you can't find it, contact Servosity support.)

## Step 5 - Tell Claude Desktop about MSP Skills (3 minutes)

This is the only "technical" step. You'll edit a small text file that tells Claude Desktop where to find MSP Skills.

In Claude Desktop:

1. Click **Claude → Settings** (top menu).
2. Click the **Developer** tab.
3. Click **Edit Config**.

A text file opens in your default editor. **Replace its contents** with this (delete what's there first, paste this in):

**If you only have HaloPSA:**

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "your-tenant-here",
        "HALOPSA_CLIENT_ID": "your-client-id-here",
        "HALOPSA_CLIENT_SECRET": "your-client-secret-here"
      }
    }
  }
}
```

**If you only have Servosity:**

```json
{
  "mcpServers": {
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "your-token-here"
      }
    }
  }
}
```

**If you have both:**

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "your-tenant-here",
        "HALOPSA_CLIENT_ID": "your-client-id-here",
        "HALOPSA_CLIENT_SECRET": "your-client-secret-here"
      }
    },
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "your-token-here"
      }
    }
  }
}
```

**Replace** `your-tenant-here`, `your-client-id-here`, `your-client-secret-here`, and `your-token-here` with the actual values from Step 4. Keep the quotes.

Save the file. (`Cmd+S` on Mac, `Ctrl+S` on Windows.)

## Step 6 - Restart Claude Desktop

Quit Claude Desktop completely. On Mac, `Cmd+Q` or **Claude → Quit Claude**. On Windows, right-click the Claude icon in the system tray and pick **Quit**.

Reopen Claude Desktop. After a few seconds, you should see a small **MCP icon** appear at the bottom-right of the chat input. **That means it worked.**

## Step 7 - Ask a real question

In the Claude Desktop chat, try:

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

Claude will run the right command, return the data, and explain what it found. You're done.

## If something didn't work

**No MCP icon appears after restart.** Your `claude_desktop_config.json` has a typo. Common issues:

- A missing comma between blocks
- A trailing comma (don't put a comma after the last item)
- Curly braces don't match - count opening `{` and closing `}`, they should be equal

Use a free JSON validator like [jsonlint.com](https://jsonlint.com) - paste the contents of your config file there and it'll point at the error.

**"halopsa-mcp: command not found" in Claude Desktop logs.** The installer didn't put the binary on your PATH. On Mac:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

…then quit and reopen Claude Desktop. On Windows, run PowerShell as Administrator and:

```powershell
$env:Path += ";$HOME\.local\bin"
[Environment]::SetEnvironmentVariable("Path", $env:Path, "User")
```

…then restart Claude Desktop.

**Authentication errors when Claude asks HaloPSA / Servosity.** Open a terminal and run:

- HaloPSA: `halopsa-cli doctor`
- Servosity: `servosity-cli doctor`

If those don't work, the env-var values in your config are wrong. Compare them to what you saved in Step 4. (`doctor` is safer than calling a real command first - it just tests the connection.)

## You're set. What's next?

- **Try a real workflow live with peers.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions) - we'll work it live with the MSP cohort.
- **Want a different PSA / RMM / backup tool?** [Request a Skill →](/requesting-a-skill/).
- **Switching to a different AI?** Same skills work in [ChatGPT](/integrations/chatgpt/), [Claude Code](/integrations/claude-code/), [Codex](/integrations/codex/), [Cursor and others](/which-agent/). No reinstall.

[← Back home](/)
