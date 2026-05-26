# MSP Skills

Open-source Claude Code Skills and MCP servers for the PSA, RMM, backup, DR, and
M365 workflows MSPs run every day. Install one, and your AI agent operates the
system directly - it does the clicking, not the narrating.

[![License: Apache 2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Skills](https://img.shields.io/badge/skills-2-green.svg)](./catalog.json)
![Status](https://img.shields.io/badge/status-early-yellow.svg)

## What your agent can do with these

Outcomes, not endpoints. With the two skills available today:

| Outcome | Skill | Command |
| --- | --- | --- |
| Pre-empt SLA breaches before a hand-off | halopsa | `halopsa-cli sla breaching --within 24h` |
| Find stale backups across every client | servosity | `servosity-cli stale-backups --days 7` |
| Build a per-client situational-awareness card | halopsa | `halopsa-cli client card "Acme Corp"` |
| Triage what needs attention across every client | servosity | `servosity-cli attention` |
| Pull a client's full backup picture for a ticket | servosity | `servosity-cli company show 4421` |
| See what changed across your fleet overnight | servosity | `servosity-cli drift --from yesterday --to now` |

## Pick your agent

| Your agent | What you install | How |
| --- | --- | --- |
| Claude Code | Claude Code Skill (plus CLI binary) | `bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)` |
| Codex CLI | Same as Claude Code | (same one-liner) |
| Claude Desktop | MCP server (local binary) | see [skills/halopsa/mcp-install.md](./skills/halopsa/mcp-install.md) |
| ChatGPT (Developer Mode, beta) | Remote MCP connector (not a local binary) | see the ChatGPT section in [mcp-install.md](./skills/halopsa/mcp-install.md) |
| Not sure what you have | start here | [docs/which-agent.md](./docs/which-agent.md) |

## Current skills

<!-- catalog:start -->
| Skill | System | Install (Skill) | Install (MCP) |
| --- | --- | --- | --- |
| [halopsa](./skills/halopsa) | HaloPSA, HaloITSM, HaloCRM | `bash skills/halopsa/install.sh` | [mcp-install](./skills/halopsa/mcp-install.md) |
| [servosity](./skills/servosity) | Servosity backup and DR | `bash skills/servosity/install.sh` | [mcp-install](./skills/servosity/mcp-install.md) |
<!-- catalog:end -->

The table above is regenerated automatically from each skill's `manifest.json`
whenever a PR touches `skills/`. See [`catalog.json`](./catalog.json) for the
machine-readable form. Both skills are **beta**.

## Install in 30 seconds

### Easiest: let your agent install it

You do not have to run anything by hand. Point your AI agent (Claude Code or Codex) at this
repo and let it do the setup - it reads the Skill, installs the binary, and walks you through
authentication. Paste this into Claude Code or Codex:

> Set up the Servosity skill from https://github.com/servosity/msp-skills - read
> skills/servosity/SKILL.md, run its install steps, then run `servosity-cli doctor` to confirm
> it works.

Swap `servosity` for `halopsa` (and `servosity-cli doctor` for `halopsa-cli --version`) for the
HaloPSA skill. You can also just give your agent the repo URL and say "install the HaloPSA skill."

### Or run the installer yourself

**HaloPSA on macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**HaloPSA on Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

**Servosity on macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Servosity on Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Each install drops both the CLI and the MCP server, so you can use either path
(Skill in Claude Code, MCP in Claude Desktop) from one install. Binaries are
published to [GitHub Releases](https://github.com/servosity/msp-skills/releases)
and built from the source under each skill's `cli/` directory. For Claude Desktop
or ChatGPT wire-up, read the per-skill `mcp-install.md`.

## Optional: Claude Code statusline

Not an MSP skill - a small developer convenience that shows model, working directory, git
branch, and context-window usage in your Claude Code status line. Install steps and details
are in [tools/statusline/README.md](./tools/statusline/README.md).

## Safety model

These skills hold privileged, multi-tenant access to systems that run MSP
businesses, so safety is a first-class concern, not a footnote:

- **You supply your own credentials at runtime.** Nothing is stored in this repo.
- **Mutations plan by default.** Each skill's CLI runs in dry-run / discovery mode
  and makes no change until you pass `--confirm`.
- **Every skill ships a permission matrix.** Each skill's `governance.md` tags
  commands read / write / destructive and tells you how to scope an agent.

The safe default for an autonomous agent is **read plus planned (dry-run)
writes**; gate destructive and credential-touching operations behind a human. See
[skills/halopsa/governance.md](./skills/halopsa/governance.md) and
[skills/servosity/governance.md](./skills/servosity/governance.md).

## Tested by MSPs in Build Sessions

These skills are built and tested with real MSPs running them against their own production
systems, live, in our free weekly Build Sessions. They are currently beta and being validated
now. Join a session (see [below](#co-build-a-new-skill-or-mcp-with-us-live)) to watch one run
against a real system, or bring your own to co-build.

## Roadmap

We co-build the next skills live with MSPs in the weekly Build Session. The targets
the MSP community asks for most:

- **M365 governance / Copilot data-exposure pre-check**
- **PSA: Autotask, ConnectWise PSA** (HaloPSA shipped)
- **NinjaOne fleet hygiene** (silent-failure detector)
- **RMM ticket-to-doc** (resolution to IT Glue / Hudu)
- **Datto RMM, Kaseya, Atera, Syncro**

Want one of these next? Bring the system to a Build Session (below) or open an issue.

## Co-build a new Skill or MCP with us live

Every Thursday we host a free Build Session where an MSP brings a system we have
not covered (your PSA, RMM, backup product, or security tool) and we co-build the
Claude Code Skill and MCP server live. You watch, you ask, you walk away with a
working integration.

- Free weekly Build Sessions (Thursdays), co-built with a volunteer MSP.
- Access to every shipped Skill and MCP the day it merges.
- Conversations with other MSP owners about running an MSP as a Compounding Team.

RSVP for the next one at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)**.

## Contribute a Skill or MCP

If your MSP uses a system we have not covered, send a PR. We co-build alongside
contributors in Build Sessions when that is easier than going it alone.

A skill PR includes: `SKILL.md` (with `vendor` frontmatter), a `README.md` with
the non-affiliation banner, `install.sh` + `install.ps1`, `mcp-install.md` if it
ships an MCP server, and ideally `pain-point.md` + `governance.md`. CI enforces
the contract (DCO sign-off, frontmatter schema, required files, no secrets, no
personal paths). See [CONTRIBUTING.md](./CONTRIBUTING.md) for the full checklist
and the non-affiliation banner template.

## About Compounding Teams

MSPs are the channel that brings AI to small business. The durable moat is the
[Compounding Teams](https://compoundingteams.com) methodology for running an MSP
where every interaction with a tool, a customer, or a system makes the next one
better. Loops close, feedback returns to the source, work compounds instead of
evaporating. `msp-skills` is the part of that methodology you can install: the
executable layer that lets your AI agent operate alongside your team in the same
systems, with the same context, every day.

## FAQ

**What is a Claude Code Skill?**
A Claude Code Skill is a markdown file (and usually a binary it drives) that tells
Claude Code how to operate a specific tool or API. When you say "use halopsa" (or any
installed skill), Claude Code loads the Skill, gets a command vocabulary plus an operating
contract, and acts on your behalf in that system. Codex CLI uses the same Skill format.

**What is an MCP server?**
MCP (Model Context Protocol) is the open standard that lets AI apps call tools on
a separate server. Claude Desktop launches an MCP server as a local binary on your
machine. ChatGPT (Developer Mode, beta) connects to a remote MCP server over
HTTPS instead. The HaloPSA MCP server, for example, gives the agent tools like
`tickets_list` and `client_card` it can call directly.

**What is the difference between a Skill and an MCP server?**
A Skill is what skill-capable agents (Claude Code, Codex CLI) use. An MCP server
is what AI apps (Claude Desktop, ChatGPT) use. Both packages here ship both, so
you do not have to pick: install once, use either path.

**How do I connect Claude Code to a system?**
Install the skill for the system you want (let your agent do it, or run the one-liner above).
Claude Code discovers the Skill via `SKILL.md` in `skills/<skill>/` and drives the
`<skill>-cli` binary. Authenticate with that system's credentials; per-skill instructions are in
[skills/halopsa/README.md](./skills/halopsa/README.md) and
[skills/servosity/README.md](./skills/servosity/README.md).

**How do I add a skill's MCP to Claude Desktop?**
The install drops the skill's `<skill>-mcp` binary on your PATH. Add the MCP config block to
your `claude_desktop_config.json` as shown in that skill's `mcp-install.md` (for example
[skills/halopsa/mcp-install.md](./skills/halopsa/mcp-install.md)), and restart Claude Desktop.

**Can I use these with ChatGPT?**
Yes, but it is not the same as Claude Desktop. ChatGPT connects to remote MCP
servers over HTTPS via Developer Mode (beta, on paid plans), not to a local
binary. Each skill's `mcp-install.md` has a ChatGPT section showing how to run the
MCP server in HTTP mode behind a secure tunnel and register it as a custom
connector.

**Is this free?**
Yes. Apache-2.0 licensed - free to use commercially, free to fork. Servosity does not charge
for API access or API calls to run the Servosity skill. Other PSA, RMM, and backup vendors set
their own API-access terms, but the Skills and MCP servers in this repository are always free.

**Can my team contribute a Skill or MCP for ConnectWise, Autotask, or NinjaOne?**
Yes, please. See [CONTRIBUTING.md](./CONTRIBUTING.md). Bring the system to a Build
Session and we will co-build the first version live, or PR a complete skill
directly. Skills for systems we directly compete with are welcome too; this
repository is an MSP capability directory, not a Servosity-product directory.

**Is msp-skills affiliated with HaloPSA, Servosity, or any other vendor named here?**
Servosity Inc. maintains this repository and ships the Servosity skill, so the
Servosity skill is first-party. Every other vendor skill is unofficial and
unaffiliated. Each unofficial skill carries a non-affiliation banner at the top of
its README. See [TRADEMARKS.md](./TRADEMARKS.md) for the full statement.

## Footer

Built by [Servosity](https://www.servosity.com). Maintained by Damien Stevens.
Apache-2.0 licensed. See [TRADEMARKS.md](./TRADEMARKS.md) for vendor
non-affiliation and [SECURITY.md](./SECURITY.md) to report a vulnerability.
Methodology: [Compounding Teams](https://compoundingteams.com).
