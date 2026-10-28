> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/proxmox/

# The Proxmox MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Proxmox Server Solutions GmbH.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Proxmox. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Proxmox to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which client VMs don't have a good, recent, verified backup right now?" and get one row per guest across every client's Proxmox cluster: is it in an enabled job, how old is its newest real restore point, did Proxmox Backup Server verify it. Every request this skill sends to Proxmox is a read; it never starts, stops, restores or deletes anything.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/proxmox/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/proxmox)

## Instead of clicking through Proxmox, just ask

**Instead of** Logging into each client's Proxmox VE and Backup Server web UI every morning to read task lists and backup emails
**just ask:** *"Which client VMs don't have a good, recent, verified backup right now?"*
Your agent runs: `proxmox-cli posture --only problems --agent`

**Instead of** Finding out a new VM was never added to an enabled backup job on the day the client asks for a restore
**just ask:** *"Which VMs or disks are not in any enabled backup job, and why?"*
Your agent runs: `proxmox-cli backup coverage --include-disks --agent`

**Instead of** Opening PBS verify task logs one by one to see how far a verify failure spread
**just ask:** *"Which backup groups failed verification, and do they still have a good restore point?"*
Your agent runs: `proxmox-cli verify-health --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/proxmox/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client VMs lack a good, recent backup (verified, where it is on PBS)? | `proxmox-cli posture --only problems --agent` |
| Which VMs or disks are in no enabled backup job? | `proxmox-cli backup coverage --include-disks --agent` |
| What failed or stopped part-way overnight, by guest, with the error line? | `proxmox-cli failures --since 24h --agent` |
| How far has a PBS verify failure spread? | `proxmox-cli verify-health --agent` |
| Which backup datastores fill up within 30 days? | `proxmox-cli capacity --within 30d --agent` |
| Which PBS-verified restore points does VM 104 have? | `proxmox-cli restore-points "104" --verified-only --agent` |
| Did every scheduled backup produce a restore point this month, per client? | `proxmox-cli rpo --days 30 --by-client --csv` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/proxmox/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/proxmox/guide.md).

## What makes this one different

Most Proxmox MCP servers are built to operate one cluster: list VMs, start and stop them, take snapshots, with the dangerous actions behind a flag. This one sends Proxmox nothing but reads, and its compound commands join each guest's job coverage, the real archives on every backup storage, PBS verify state and the vzdump task logs, across every client cluster, into one verdict per guest.

Proxmox has no built-in AI assistant. The Proxmox VE and Backup Server web interfaces show jobs, tasks and datastores; this adds a per-guest backup verdict joined across both products and across every client, and it changes nothing in either.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=proxmox&utm_content=docs)

## The pain this closes

- A Proxmox shop with many clients runs one cluster and often one Backup Server per client, so "are all my clients' backups good?" means a login per site and a scroll through task lists.
- A green backup task does not mean every guest was backed up. A multi-guest vzdump job only names the guest that failed inside its task log, and a VM created after a job with a fixed VM list was set up is in no job at all.
- Verify failures spread quietly. On a central PBS that more than 100 servers back up to, the weekly verify job showed a growing count of failed snapshots before anyone traced the cause.
- Most MSPs will not hand an AI agent a token that can stop, change or delete a VM, so an agent that has to manage the hypervisor never gets connected at all.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

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

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way proxmox-mcp runs, and shares MCP configuration with Codex: add proxmox-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Proxmox data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Can the AI stop, change or delete anything on my Proxmox hosts?

No. The CLI and MCP server only contain read (GET) calls to Proxmox VE and Proxmox Backup Server; there is no command that starts, stops, migrates, restores, prunes or deletes anything. The only writes are local files: the cluster list, a saved token, the local mirror (which the per-endpoint read commands also update, keeping what they return, comments included) and the agent learning store. There is no response cache. Posture, backup coverage, verify-health, failures, capacity, restore-points and rpo always read Proxmox live; a per-endpoint read command answers from the local mirror only when Proxmox cannot be reached, says so on stderr and in `meta.source: local`, and refuses to with `--data-source live`. Secret-named fields, such as a third-party storage plugin's password in the storage config, are redacted before anything is cached, stored or shown, and each `--cluster` keeps its own local store. One side effect to know: reading a storage's status lets Proxmox mount an enabled storage, which its own status daemon already does every 10 seconds.

### What API token does it need?

A privilege-separated API token for each product. Proxmox Backup Server: the Audit role on / (read-only). Proxmox VE: PVEAuditor on /. One catch: Proxmox VE lists backup files only to a token that also holds Datastore.AllocateSpace and VM.Backup, so if your backups live on dir, NFS or CIFS storage, or you have not configured the Backup Server here, add a custom role with those two privileges. An audit-only token is reported as "storage not read" and the guest as unproven, never as "no backups". If your backups are on Proxmox Backup Server and PROXMOX_PBS_HOST is set (spelled as in the PVE storage's server field: the same IP or hostname), the CLI reads them from the PBS API and the PVE token can stay audit-only. Without it, a PBS storage is read only through Proxmox VE, which lists it with the PBS credential saved in that storage and so may miss backup groups another credential owns: its restore points count, but a guest without one there is reported unproven, not never, and no guest covered by a backup job on that cluster reads green (a fresh one is archives-incomplete, since a newer backup that failed verification may be hidden). On a stock install the local storage also accepts backups: with an audit-only token it is listed under fetch failures until you remove "VZDump backup file" from its content types or grant it the two privileges. Until then fresh guests keep their verdicts, but a guest with no readable or only an old restore point is reported unproven rather than never or stale (the unread storage could hold one), rpo counts its missed runs as unproven, and posture --fail-on-red exits 1. Grant the roles on / (propagating) and put no NoAccess or narrower ACL below it: Proxmox leaves out of every list the guests and storages a token cannot see, and while the CLI flags a missing or narrowed grant, an explicit NoAccess on one guest is invisible to any read. The PVE token is USER@REALM!TOKENID=SECRET; the PBS token uses a colon before the secret.

### Proxmox uses a self-signed certificate. Do I have to turn off certificate checks?

No, and there is no option to. Pin the certificate's SHA-256 fingerprint (shown under the node's Certificates) with PROXMOX_TLS_FINGERPRINT and PROXMOX_PBS_TLS_FINGERPRINT, or per client cluster in clusters.toml. A malformed pin is an error, never silently ignored, and a PVE pin never vouches for the PBS host.

### How do I cover many client clusters?

Add one entry per client with `proxmox-cli clusters add`: its PVE host, optional PBS host and namespace, certificate fingerprints, and the names of the environment variables that hold its tokens (never the tokens themselves). The fleet commands then run across every configured cluster; `--cluster` or `--client` narrows to one. For an MCP client, add those token variables to the MCP server's env block too. On a shared Backup Server, give each client cluster its own PBS namespace and declare it as that entry's pbs_namespace (a separate datastore per client is not enough on its own: only a declared namespace scopes a cluster that cannot be read): VMIDs repeat across clusters, so where two clusters write the same namespace the CLI cannot tell whose vm/100 a snapshot is, reports the overlap, and does not count those restore points. A cluster that cannot be read is assumed to write anywhere in its declared pbs_namespace, or, when it declares none, anywhere on its PBS (on any PBS, if its entry names none), so declaring one keeps a client outage from hiding the others' backups.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Afi](https://msp-skills.compoundingteams.com/skills/afi.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Backup Radar](https://msp-skills.compoundingteams.com/skills/backup-radar.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Synology](https://msp-skills.compoundingteams.com/skills/synology.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Beta. Validated against the Proxmox API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
