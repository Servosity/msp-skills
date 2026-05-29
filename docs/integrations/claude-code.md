---
layout: default
title: "HaloPSA + Servosity in Claude Code (Anthropic CLI)"
description: "Install MSP Skills in Claude Code: paste one sentence and your agent reads the SKILL.md, runs the install, walks authentication. For technical-leaning MSP owners or senior techs."
permalink: /integrations/claude-code/
faqs:
  - q: "Does Claude Code support HaloPSA?"
    a: "Yes. Paste this into Claude Code: 'Set up the HaloPSA skill from https://github.com/servosity/msp-skills - read skills/halopsa/SKILL.md, run its install steps, then run halopsa-cli --version to confirm. Walk me through authentication.' Claude Code does the rest."
  - q: "What's the difference between Claude Code and Claude Desktop?"
    a: "Claude Code is Anthropic's command-line agent that reads Skills directly (markdown SKILL.md files) and can run shell commands. Claude Desktop is the Mac/Windows app with a chat window that uses MCP servers (no shell). Most MSP business owners prefer Claude Desktop; senior techs and engineers often prefer Claude Code."
  - q: "Do I need to know how to code to use Claude Code?"
    a: "You need to be comfortable in a terminal, but you don't need to write code. The recommended install path is to paste one sentence - Claude Code reads SKILL.md and handles everything else."
  - q: "Can Claude Code install both HaloPSA and Servosity?"
    a: "Yes. Paste two prompts (one for each skill), or one prompt that asks for both: 'Set up both the halopsa and servosity skills from https://github.com/servosity/msp-skills.' Claude Code installs both."
---

# HaloPSA and Servosity in Claude Code

Claude Code is Anthropic's CLI agent - a terminal-native AI that reads `SKILL.md` files directly and runs shell commands. For a **technical-leaning MSP owner** or a **senior tech**, Claude Code is the fastest install path: paste one sentence, your agent does the rest.

If you don't already use Claude Code in a terminal, [Claude Desktop](/integrations/claude-desktop/) is the easier starting point.

## Install in 30 seconds

Paste this into Claude Code chat:

> Set up the **HaloPSA** skill from https://github.com/servosity/msp-skills - read `skills/halopsa/SKILL.md`, run its install steps, then run `halopsa-cli --version` to confirm. Walk me through authentication.

For both skills in one go:

> Set up both the **halopsa** and **servosity** skills from https://github.com/servosity/msp-skills - read each skill's SKILL.md, run their install steps, then run `halopsa-cli --version` and `servosity-cli doctor` to confirm. Walk me through authentication for both.

Claude Code reads each SKILL.md, runs the installer (which drops the CLI + MCP binary on your PATH), prompts you for the HaloPSA OAuth credentials (Configuration → Integrations → Halo PSA API in your tenant) or the Servosity MSP partner token, runs `doctor` to verify, and confirms.

## Manual install (skip the prompt)

If you'd rather see what's running:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

Then symlink each skill into Claude Code:

```bash
ln -s "$(pwd)/skills/halopsa" ~/.claude/skills/halopsa
ln -s "$(pwd)/skills/servosity" ~/.claude/skills/servosity
```

Restart Claude Code. Invoke with `use halopsa` or `use servosity`.

## Use the skill

In Claude Code:

- *"Use halopsa: triage what needs attention across all clients today, focused on SLA breaches in the next 24 hours."*
- *"Use servosity: show me stale backups across all clients, grouped by backup engine."*
- *"Use halopsa: build a client card for Acme Corp, then cross-reference any open Servosity backup issues for them."*

Claude Code knows the command surface (it just read the SKILL.md), runs the right command with the right flags, and pipes the JSON output into its reasoning.

## Add the MCP server too (optional)

Each install drops an MCP binary. To register it with Claude Code's MCP support (parallel to the Skill, lets you use MCP-style tool calls):

```bash
claude mcp add halopsa -- halopsa-mcp
claude mcp add servosity -- servosity-mcp
claude mcp list  # verify
```

The Skill path and MCP path coexist. Most users only need one (Skill is recommended).

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions).
- **Switching agents?** Same skills work in [Claude Desktop](/integrations/claude-desktop/), [Codex CLI](/integrations/codex/), and [ChatGPT (Plus/Pro+)](/integrations/chatgpt/) - no reinstall needed.
- **Want a different platform?** [Request a Skill →](/requesting-a-skill/).

[← Back to main install](/#install-in-60-seconds)
