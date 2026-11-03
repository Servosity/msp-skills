> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/codex/

# HaloPSA and Servosity in OpenAI Codex CLI

Codex CLI is OpenAI's command-line agent - a terminal-native AI that reads `SKILL.md` files (the open Agent Skills spec format) AND speaks MCP natively. For the same audience as Claude Code: a **technical-leaning MSP owner** or a **senior tech**.

If you use Claude Code already, the install is identical - same skills, same SKILL.md, same MCP servers. Switching agents costs nothing.

## Install in 30 seconds

Paste this into Codex CLI:

> Set up the **HaloPSA** skill from https://github.com/servosity/msp-skills - read `skills/halopsa/SKILL.md`, run its install steps, then run `halopsa-cli --version` to confirm. Walk me through authentication.

For both skills:

> Set up both the **halopsa** and **servosity** skills from https://github.com/servosity/msp-skills - read each skill's SKILL.md, run their install steps, then run `halopsa-cli --version` and `servosity-cli doctor` to confirm.

Codex reads each SKILL.md, runs the installer (drops the CLI + MCP binary on your PATH), prompts you for credentials, runs `doctor` to verify.

## Manual install + MCP registration

If you prefer explicit config:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

Then edit `~/.codex/config.toml` (global) or `.codex/config.toml` (project) to register the MCP servers:

```toml
[mcp_servers.halopsa]
command = "halopsa-mcp"
env = { HALOPSA_TENANT = "<your-tenant>", HALOPSA_CLIENT_ID = "<id>", HALOPSA_CLIENT_SECRET = "<secret>" }

[mcp_servers.servosity]
command = "servosity-mcp"
env = { SERVOSITY_MSP_TOKEN = "<token>" }
```

Restart Codex CLI. Both MCP servers connect on startup.

Codex supports stdio + Streamable HTTP and runs concurrent read-only tools when advertised - useful for cross-client / cross-engine analytics that hit multiple MSP Skills tools at once.

[Source: developers.openai.com/codex/mcp](https://developers.openai.com/codex/mcp)

## Use the skill

In Codex CLI:

- *"Use halopsa: triage what needs attention across all clients today, focused on SLA breaches in the next 24 hours."*
- *"Use servosity: show me stale backups across all clients, grouped by backup engine."*

Codex knows the command surface (it just read the SKILL.md), runs the right command with the right flags, and pipes the JSON output into reasoning.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions).
- **Switching agents?** Same skills work in [Claude Code](https://msp-skills.compoundingteams.com/integrations/claude-code.md), [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md), [Claude Cowork](https://msp-skills.compoundingteams.com/integrations/cowork.md), and [the ChatGPT desktop app](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) - no reinstall.
- **Want a different platform?** [Request a Skill →](https://msp-skills.compoundingteams.com/requesting-a-skill.md).

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
