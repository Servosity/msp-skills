---
layout: default
title: "VMware vSphere MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Read-only vCenter inventory for migration planning: blockers, snapshots, per-core licence exposure and Proxmox mapping, as JSON an agent can use."
permalink: /skills/vsphere/
skill_name: "VMware vSphere MCP"
image: /assets/social/vsphere/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for VMware vSphere?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for VMware vSphere, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the VMware vSphere MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `VSPHERE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `VSPHERE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the VMware vSphere MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my VMware data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does it change anything in vCenter?"
    a: "No. Every vCenter call is a read. A Read-Only role on the vCenter SSO user is enough."
  - q: "Which vCenter versions does it support?"
    a: "Inventory reads need vCenter 7.0 Update 2 or later. Snapshots, licence exposure, host hardware and capacity need vCenter 8.0 Update 1 or later; on older vCenters those checks are reported as unknown, never as a pass."
  - q: "Our vCenter uses a self-signed certificate. Will it connect?"
    a: "Yes. Set VSPHERE_TLS_FINGERPRINT to the certificate's SHA-256 fingerprint and the CLI accepts only that certificate. Without it, normal certificate checks apply."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your VMware vSphere credentials once, then run vsphere-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a VMware vSphere question in plain language; it runs vsphere-cli for you."
---

# The VMware vSphere MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Broadcom Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for VMware vSphere. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects VMware vSphere to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which client VMs can leave VMware today, which are blocked and why, how many cores Broadcom will bill for, and what each VM looks like on Proxmox. vsphere-cli reads vCenter with a Read-Only login and answers in seconds, as JSON your agent can act on. It never changes anything in vCenter.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/vsphere){: .btn}

## Instead of clicking through VMware vSphere, just ask

**Instead of** Run RVTools on a Windows jump box, then sort a spreadsheet by hand to find snapshots, mounted ISOs and PVSCSI disks
**just ask:** *"Which of Acme's VMs are blocked from migrating, and why?"*
<sub>Your agent runs: <code>vsphere-cli migration blockers --severity block --agent</code></sub>

**Instead of** Count sockets and cores per host and apply Broadcom's 16-core minimum in a spreadsheet
**just ask:** *"How many cores will Broadcom bill us for at Northwind?"*
<sub>Your agent runs: <code>vsphere-cli licence exposure --price-per-core 350 --agent</code></sub>

**Instead of** Click through every VM's snapshot manager to find forgotten snapshots before a cutover
**just ask:** *"Show me every snapshot older than a week and how much space it holds"*
<sub>Your agent runs: <code>vsphere-cli snapshots audit --older-than 7d --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/vsphere/wide-1200x630.png" src="/assets/video/vsphere/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/vsphere/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which VMs are blocked from migrating, and what is blocking each one? | `vsphere-cli migration blockers --severity block --agent` |
| Which snapshots are older than a week, and how much delta disk does each of those VMs carry? | `vsphere-cli snapshots audit --older-than 7d --agent` |
| How many licensable cores does this vCenter have under the 16-core-per-CPU minimum? | `vsphere-cli licence exposure --agent` |
| How much vCPU, RAM and storage does the Proxmox target need? | `vsphere-cli migration capacity --powered-on-only --agent` |
| Which Proxmox VE settings does this VM map to? | `vsphere-cli migration proxmox-map --vm vm-1021 --agent` |
| Can I get an RVTools-format export without a Windows machine? | `vsphere-cli rvtools export --out ./acme-rvtools` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/vsphere/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/vsphere/guide.md).

## What makes this one different

This connector answers migration questions directly: blockers with reason codes, snapshot age and size, licensable cores and Proxmox mappings come back as one JSON answer per vCenter, built from both of vCenter's JSON APIs. Tools such as govc and PowerCLI expose the raw objects well, and you still assemble these answers yourself.

VMware's own tooling manages vSphere; it does not tell you what leaving costs or what blocks a move. This adds read-only migration and licence answers that your AI can ask for in plain English, and it complements the vSphere Client rather than replacing it.

## The pain this closes

- Broadcom's per-core subscriptions raised VMware bills several times over for many MSPs and their clients (ServeTheHome reported 10x or more for cloud service providers), so every client now asks what staying costs and what leaving takes.
- The migration inventory still means RVTools on a Windows jump box and an afternoon of spreadsheet work per vCenter before anyone knows which VMs can move.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (desktop app) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/vsphere/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/vsphere/install.ps1 | iex
```

After install, authenticate once with your VMware vSphere credentials, then verify with `vsphere-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | vm list, migration blockers, snapshots audit, licence exposure, migration capacity, migration proxmox-map, session, doctor | Allow |
| Write (local files only) | sync, export --output, rvtools export --out, profile save, teach | Allow; they write only on your machine, never to vCenter |
| Credential | auth set-credentials, auth logout | Human-in-the-loop only |
| Destructive (local data) | local-data purge --yes, learnings forget, learnings purge | Human-in-the-loop only; local-data purge previews unless --yes |

Every command that talks to vCenter is a read: the skill cannot power off, delete, move or reconfigure anything. It does write on your own machine (the local store, export files, saved credentials), and local-data purge erases that local data. Let agents run the read commands freely; keep credential and local-erase commands with a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/vsphere/governance.md).

## Frequently asked questions

### Is there an MCP server for VMware vSphere?

Yes - this one. A free, open source MCP server and Claude Code Skill for VMware vSphere, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the VMware vSphere MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `VSPHERE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `VSPHERE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the VMware vSphere MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my VMware data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does it change anything in vCenter?

No. Every vCenter call is a read. A Read-Only role on the vCenter SSO user is enough.

### Which vCenter versions does it support?

Inventory reads need vCenter 7.0 Update 2 or later. Snapshots, licence exposure, host hardware and capacity need vCenter 8.0 Update 1 or later; on older vCenters those checks are reported as unknown, never as a pass.

### Our vCenter uses a self-signed certificate. Will it connect?

Yes. Set VSPHERE_TLS_FINGERPRINT to the certificate's SHA-256 fingerprint and the CLI accepts only that certificate. Without it, normal certificate checks apply.


## More Other connectors

Run more than one Other tool, or comparing options? These connectors work the same way: [Resource Guru](/skills/resourceguru/)

## Status

Beta. Validated against the VMware vSphere API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
