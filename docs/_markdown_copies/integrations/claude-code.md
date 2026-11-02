> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/claude-code/

# HaloPSA and Servosity in Claude Code

Claude Code is Anthropic's CLI agent - a terminal-native AI that reads `SKILL.md` files directly and runs shell commands. For a **technical-leaning MSP owner** or a **senior tech**, Claude Code is the fastest install path: paste one sentence, your agent does the rest.

If you don't already use Claude Code in a terminal, [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) is the easier starting point.

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
- **Switching agents?** Same skills work in [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md), [Codex CLI](https://msp-skills.compoundingteams.com/integrations/codex.md), [Claude Cowork](https://msp-skills.compoundingteams.com/integrations/cowork.md), and [ChatGPT (Plus/Pro+)](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) - no reinstall needed.
- **Want a different platform?** [Request a Skill →](https://msp-skills.compoundingteams.com/requesting-a-skill.md).

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
