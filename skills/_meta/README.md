# msp-skills Concierge - tell it your stack, it picks the right connectors

> First-party. The concierge Skill for the msp-skills catalog, by Servosity. Apache-2.0.

The concierge reads the live msp-skills catalog, learns your MSP stack, and recommends the connectors that fit - then installs only the ones you approve. It is a Claude Code Skill (and works with any agent that reads Skills); it does not connect to a tool itself, it routes you to the connectors that do.

## The magic prompt

Once msp-skills is installed, paste this into Claude Code or Codex:

> You have msp-skills installed. Using everything you know about me and how I work, recommend which connectors I should install - and install the ones I approve.

The concierge fetches the live catalog, matches it to your PSA / RMM / backup / security / billing stack, shows you a shortlist with an honest verification badge per connector, and installs only what you say yes to. After each install it runs the connector's `--version` (and `doctor` if you've set credentials) and reports the output - receipts, not claims.

## Install the concierge

```text
/plugin marketplace add Servosity/msp-skills
/plugin install msp-skills-concierge@msp-skills
```

## Markdown-only (no binary)

The concierge is markdown-only: it ships SKILL.md and this README and nothing else. There is no `install.sh`, no CLI, and no MCP server to install, because the concierge has no tool of its own to connect to - it drives the other connectors' installers. Each connector it recommends ships its own binary, its own one-line installer, and its own `mcp-install.md` wire-up doc.

## The honesty model

Every recommendation carries a verification badge taken straight from the catalog, stated honestly:

- **Live-verified** (with a date) - a real MSP ran the connector against a real tenant and confirmed it works.
- **Awaiting live verification - passes every mechanical gate** - the connector builds, its docs match its binary, and it ships clean, but no MSP has confirmed it against a live tenant yet.

The concierge never inflates a badge. "Awaiting live verification" is the truthful state for most connectors, and your feedback is exactly the signal that closes it. The concierge also never installs anything you did not approve and never writes your credentials anywhere - you enter your own credentials following each connector's README.
