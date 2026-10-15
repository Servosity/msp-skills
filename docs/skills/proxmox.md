---
layout: default
title: "Proxmox MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Read-only Proxmox VE and Backup Server CLI that tells you, across every client cluster, which guests have a good, verified, recent restore point and which do not."
permalink: /skills/proxmox/
skill_name: "Proxmox MCP"
image: /assets/social/proxmox/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Proxmox?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Proxmox, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Proxmox MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PROXMOX_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PROXMOX_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and proxmox-mcp speaks HTTP natively: set PROXMOX_MCP_HTTP_TOKEN to a long random value, run `proxmox-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` and refuses to start without that token), and put that /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Proxmox data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Can the AI stop, change or delete anything on my Proxmox hosts?"
    a: "No. The CLI and MCP server only contain read (GET) calls to Proxmox VE and Proxmox Backup Server; there is no command that starts, stops, migrates, restores, prunes or deletes anything. The only writes are local files: the cluster list, a saved token, the local mirror (which the per-endpoint read commands also update), the agent learning store, and a response cache of raw API answers (owner-only files; the backup commands never answer from it). One side effect to know: reading a storage's status lets Proxmox mount an enabled storage, which its own status daemon already does every 10 seconds."
  - q: "What API token does it need?"
    a: "A privilege-separated API token for each product. Proxmox Backup Server: the Audit role on / (read-only). Proxmox VE: PVEAuditor on /. One catch: Proxmox VE lists backup files only to a token that also holds Datastore.AllocateSpace and VM.Backup, so if your backups live on dir, NFS or CIFS storage, or you have not configured the Backup Server here, add a custom role with those two privileges. An audit-only token is reported as \"storage not read\" and the guest as unproven, never as \"no backups\". If your backups are on Proxmox Backup Server and PROXMOX_PBS_HOST is set (spelled as in the PVE storage's server field: the same IP or hostname), the CLI reads them from the PBS API and the PVE token can stay audit-only. On a stock install the local storage also accepts backups: with an audit-only token it is listed under fetch failures until you remove \"VZDump backup file\" from its content types or grant it the two privileges. Until then fresh guests keep their verdicts, but a guest with no readable or only an old restore point is reported unproven rather than never or stale (the unread storage could hold one), rpo counts its missed runs as unproven, and posture --fail-on-red exits 1. Grant the roles on / (propagating) and put no NoAccess or narrower ACL below it: Proxmox leaves out of every list the guests and storages a token cannot see, and while the CLI flags a missing or narrowed grant, an explicit NoAccess on one guest is invisible to any read. The PVE token is USER@REALM!TOKENID=SECRET; the PBS token uses a colon before the secret."
  - q: "Proxmox uses a self-signed certificate. Do I have to turn off certificate checks?"
    a: "No, and there is no option to. Pin the certificate's SHA-256 fingerprint (shown under the node's Certificates) with PROXMOX_TLS_FINGERPRINT and PROXMOX_PBS_TLS_FINGERPRINT, or per client cluster in clusters.toml. A malformed pin is an error, never silently ignored, and a PVE pin never vouches for the PBS host."
  - q: "How do I cover many client clusters?"
    a: "Add one entry per client with `proxmox-cli clusters add`: its PVE host, optional PBS host and namespace, certificate fingerprints, and the names of the environment variables that hold its tokens (never the tokens themselves). The fleet commands then run across every configured cluster; `--cluster` or `--client` narrows to one. For an MCP client, add those token variables to the MCP server's env block too. On a shared Backup Server, give each client cluster its own PBS namespace (or datastore) and declare it as that entry's pbs_namespace: VMIDs repeat across clusters, so where two clusters write the same namespace the CLI cannot tell whose vm/100 a snapshot is, reports the overlap, and does not count those restore points. A cluster that cannot be read is assumed to write anywhere in its declared pbs_namespace, or anywhere on its PBS when it declares none, so declaring one keeps a client outage from hiding the others' backups."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Proxmox credentials once, then run proxmox-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Proxmox question in plain language; it runs proxmox-cli for you."
---

# The Proxmox MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Proxmox Server Solutions GmbH.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Proxmox. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Proxmox to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which client VMs don't have a good, recent, verified backup right now?" and get one row per guest across every client's Proxmox cluster: is it in an enabled job, how old is its newest real restore point, did Proxmox Backup Server verify it. Every request this skill sends to Proxmox is a read; it never starts, stops, restores or deletes anything.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/proxmox){: .btn}

## Instead of clicking through Proxmox, just ask

**Instead of** Logging into each client's Proxmox VE and Backup Server web UI every morning to read task lists and backup emails
**just ask:** *"Which client VMs don't have a good, recent, verified backup right now?"*
<sub>Your agent runs: <code>proxmox-cli posture --only problems --agent</code></sub>

**Instead of** Finding out a new VM was never added to an enabled backup job on the day the client asks for a restore
**just ask:** *"Which VMs or disks are not in any enabled backup job, and why?"*
<sub>Your agent runs: <code>proxmox-cli backup coverage --include-disks --agent</code></sub>

**Instead of** Opening PBS verify task logs one by one to see how far a verify failure spread
**just ask:** *"Which backup groups failed verification, and do they still have a good restore point?"*
<sub>Your agent runs: <code>proxmox-cli verify-health --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/proxmox/wide-1200x630.png" src="/assets/video/proxmox/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/proxmox/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client VMs don't have a good, recent, verified backup? | `proxmox-cli posture --only problems --agent` |
| Which VMs or disks are in no enabled backup job? | `proxmox-cli backup coverage --include-disks --agent` |
| What failed or stopped part-way overnight, by guest, with the error line? | `proxmox-cli failures --since 24h --agent` |
| How far has a PBS verify failure spread? | `proxmox-cli verify-health --agent` |
| Which backup datastores fill up within 30 days? | `proxmox-cli capacity --within 30d --agent` |
| Can we restore VM 104, and from which verified points? | `proxmox-cli restore-points "104" --verified-only --agent` |
| Did every scheduled backup produce a restore point this month, per client? | `proxmox-cli rpo --days 30 --by-client --csv` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/proxmox/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/proxmox/guide.md).

## What makes this one different

Most Proxmox MCP servers are built to operate one cluster: list VMs, start and stop them, take snapshots, with the dangerous actions behind a flag. This one sends Proxmox nothing but reads, and its compound commands join each guest's job coverage, the real archives on every backup storage, PBS verify state and the vzdump task logs, across every client cluster, into one verdict per guest.

Proxmox has no built-in AI assistant. The Proxmox VE and Backup Server web interfaces show jobs, tasks and datastores; this adds a per-guest backup verdict joined across both products and across every client, and it changes nothing in either.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- A Proxmox shop with many clients runs one cluster and often one Backup Server per client, so "are all my clients' backups good?" means a login per site and a scroll through task lists.
- A green backup task does not mean every guest was backed up. A multi-guest vzdump job only names the guest that failed inside its task log, and a VM created after a job with a fixed VM list was set up is in no job at all.
- Verify failures spread quietly. On a central PBS that more than 100 servers back up to, the weekly verify job showed a growing count of failed snapshots before anyone traced the cause.
- Most MSPs will not hand an AI agent a token that can stop, change or delete a VM, so an agent that has to manage the hypervisor never gets connected at all.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proxmox/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proxmox/install.ps1 | iex
```

After install, authenticate once with your Proxmox credentials, then verify with `proxmox-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (Proxmox) | posture, backup coverage, failures, verify-health, capacity, restore-points, rpo, and every vm, ct, nodes, storage, tasks, cluster and pbs-* command | Allow |
| Local writes | sync, workflow archive, export, profile save, teach | Allow |
| Credential routing (local) | clusters add (where each client's tokens are sent and which certificate is trusted), auth set-token, auth logout | A human runs these; none is an MCP tool |
| Data leaving the machine | --deliver webhook:<url> or --deliver file:<path> on any command, feedback --send, and the MCP server bridged to a public HTTPS endpoint | Human-in-the-loop only |
| Proxmox writes (start, stop, restore, prune, delete) | None. The CLI has no such command. | Not possible with this skill |

Every request this skill sends to Proxmox VE or Proxmox Backup Server is a read, so no command it runs can change a cluster or a backup. The only writes are local: the cluster list, a saved token, the local mirror and the agent learning store, and the MCP server cannot edit the cluster list. Gate where output goes: --deliver can send any command's output to a file or a webhook URL. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/proxmox/governance.md).

## Frequently asked questions

### Is there an MCP server for Proxmox?

Yes - this one. A free, open source MCP server and Claude Code Skill for Proxmox, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Proxmox MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PROXMOX_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PROXMOX_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and proxmox-mcp speaks HTTP natively: set PROXMOX_MCP_HTTP_TOKEN to a long random value, run `proxmox-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` and refuses to start without that token), and put that /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Proxmox data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Can the AI stop, change or delete anything on my Proxmox hosts?

No. The CLI and MCP server only contain read (GET) calls to Proxmox VE and Proxmox Backup Server; there is no command that starts, stops, migrates, restores, prunes or deletes anything. The only writes are local files: the cluster list, a saved token, the local mirror (which the per-endpoint read commands also update), the agent learning store, and a response cache of raw API answers (owner-only files; the backup commands never answer from it). One side effect to know: reading a storage's status lets Proxmox mount an enabled storage, which its own status daemon already does every 10 seconds.

### What API token does it need?

A privilege-separated API token for each product. Proxmox Backup Server: the Audit role on / (read-only). Proxmox VE: PVEAuditor on /. One catch: Proxmox VE lists backup files only to a token that also holds Datastore.AllocateSpace and VM.Backup, so if your backups live on dir, NFS or CIFS storage, or you have not configured the Backup Server here, add a custom role with those two privileges. An audit-only token is reported as "storage not read" and the guest as unproven, never as "no backups". If your backups are on Proxmox Backup Server and PROXMOX_PBS_HOST is set (spelled as in the PVE storage's server field: the same IP or hostname), the CLI reads them from the PBS API and the PVE token can stay audit-only. On a stock install the local storage also accepts backups: with an audit-only token it is listed under fetch failures until you remove "VZDump backup file" from its content types or grant it the two privileges. Until then fresh guests keep their verdicts, but a guest with no readable or only an old restore point is reported unproven rather than never or stale (the unread storage could hold one), rpo counts its missed runs as unproven, and posture --fail-on-red exits 1. Grant the roles on / (propagating) and put no NoAccess or narrower ACL below it: Proxmox leaves out of every list the guests and storages a token cannot see, and while the CLI flags a missing or narrowed grant, an explicit NoAccess on one guest is invisible to any read. The PVE token is USER@REALM!TOKENID=SECRET; the PBS token uses a colon before the secret.

### Proxmox uses a self-signed certificate. Do I have to turn off certificate checks?

No, and there is no option to. Pin the certificate's SHA-256 fingerprint (shown under the node's Certificates) with PROXMOX_TLS_FINGERPRINT and PROXMOX_PBS_TLS_FINGERPRINT, or per client cluster in clusters.toml. A malformed pin is an error, never silently ignored, and a PVE pin never vouches for the PBS host.

### How do I cover many client clusters?

Add one entry per client with `proxmox-cli clusters add`: its PVE host, optional PBS host and namespace, certificate fingerprints, and the names of the environment variables that hold its tokens (never the tokens themselves). The fleet commands then run across every configured cluster; `--cluster` or `--client` narrows to one. For an MCP client, add those token variables to the MCP server's env block too. On a shared Backup Server, give each client cluster its own PBS namespace (or datastore) and declare it as that entry's pbs_namespace: VMIDs repeat across clusters, so where two clusters write the same namespace the CLI cannot tell whose vm/100 a snapshot is, reports the overlap, and does not count those restore points. A cluster that cannot be read is assumed to write anywhere in its declared pbs_namespace, or anywhere on its PBS when it declares none, so declaring one keeps a client outage from hiding the others' backups.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Proxmox API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
