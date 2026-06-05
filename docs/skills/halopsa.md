---
layout: default
title: "HaloPSA + AI - for Claude, ChatGPT, Codex, Cursor, and any MCP agent"
description: "Free Claude Code skill and MCP server for HaloPSA tickets. Triage queues, pre-empt SLA breaches, build per-client cards, run cross-client analytics. Local SQLite mirror so QBR-time questions don't hit rate limits. Built for MSP owners."
permalink: /skills/halopsa/
skill_name: "HaloPSA MCP"
image: /assets/social/halopsa/wide-1200x630.png
---

# HaloPSA + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the HaloPSA, HaloITSM, and HaloCRM APIs. Not affiliated with Halo Service Solutions Ltd. HaloPSA, HaloITSM, and HaloCRM are trademarks of Halo Service Solutions Ltd.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Add **HaloPSA ticket triage, SLA-breach pre-emption, per-client situational awareness, and cross-client analytics** to the AI you already use - **Claude Code**, **Claude Desktop**, **ChatGPT** (Plus/Pro+), **Codex**, **Cursor**, **Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open source, runs on your laptop. A **local SQLite mirror** means your AI can answer cross-client questions the live HaloPSA API can't return in one shot - **no rate-limit hits during QBR prep**. Built for MSP owners. No code required.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/halopsa){: .btn}

## What it does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's about to breach SLA in the next 24 hours? | `halopsa-cli sla breaching --within 24h` |
| What's the dispatcher view across all agents and teams? | `halopsa-cli triage --team Support` |
| Who's overloaded right now? | `halopsa-cli agent workload` |
| What's the whole story for this client, on one screen? | `halopsa-cli client card "Acme Corp"` |
| How much contract time is left across every contract? | `halopsa-cli contracts burn` |
| Which clients have stale tickets aging out? | `halopsa-cli tickets age-out` |
| What changed in Halo since this morning? | `halopsa-cli tickets changed-since 9:00` |
| Which KB article should the tech link to? | `halopsa-cli kbarticle suggest <ticket>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/halopsa/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/halopsa/guide.md).

## What makes this different

Most HaloPSA MCP servers and AI integrations proxy each question into a live API call. That's fine for one ticket. It dies at QBR time, when you're asking *"how many backup-failure tickets across all 47 clients last quarter, grouped by contract type?"* - that's 47 paginated REST calls, HaloPSA rate-limit headaches (limits aren't publicly documented; they vary by hosting), and a context window full of raw JSON the model has to re-read.

This skill syncs HaloPSA into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `client card`, and `contracts burn` join across tickets, clients, contracts, time, and assets - work a stateless API wrapper can't do.

It **complements** HaloPSA's built-in ChatGPT integration (which is great for per-ticket work like rewriting replies and sentiment-flagging). The two are non-overlapping.

## The pain this closes

Two pains MSP owners name repeatedly:

- **Cost-to-serve is rising faster than contract value.** The squeeze is operational: queue overload, SLA fires, unpriced work eating margin on every ticket.
- **Key-person dependency.** "If your best technician left tomorrow, would your clients notice within 30 days?" For most MSPs the answer is yes, because situational awareness lives in one person's head - and in a PSA nobody else triages quickly.

This skill puts the PSA's cross-entity truth one sentence away from any team member or their agent: `triage` / `sla breaching` surface what will breach before it does; `client card` gives any teammate one situational-awareness panel on demand; `contracts burn` catches unpriced work early; `agent workload` makes the queue legible across the whole team.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

After install: get HaloPSA API credentials from Configuration → Integrations → Halo PSA API (Authentication Method: Client ID and Secret, Services), then:

```bash
HALOPSA_TENANT=<yourtenant> halopsa-cli auth login --client-id <id> --client-secret <secret>
halopsa-cli --version
```

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `triage`, `sla breaching`, `client card`, `contracts burn` | Allow |
| Write (routine) | ticket updates, notes, assignment | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | deletes, configuration changes | Human-in-the-loop only |

The strongest control is the **scope you grant the Halo OAuth application** - the CLI can only do what that application is permitted to do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/halopsa/governance.md).

## Status

Beta. Validated against the HaloPSA API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
