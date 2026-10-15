# Proxmox + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Proxmox
> API. Not affiliated with, endorsed by, or sponsored by Proxmox Server Solutions GmbH.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/proxmox/">
    <img src="../../docs/assets/video/proxmox/animated-og.gif" alt="Proxmox demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/proxmox/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Read-only Proxmox VE and Backup Server CLI that tells you, across every client cluster, which guests have a good, verified, recent restore point and which do not. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Proxmox skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `proxmox-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `proxmox-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `proxmox-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, set `PROXMOX_MCP_HTTP_TOKEN`, run `proxmox-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `proxmox-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Proxmox MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/proxmox-v0.1.0/proxmox-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Proxmox release on the [releases page](https://github.com/servosity/msp-skills/releases?q=proxmox).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install proxmox@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Proxmox Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex`. Then authenticate per the README and run `proxmox-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh)
```

The installer drops both `proxmox-cli` (the CLI) and `proxmox-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
proxmox-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proxmox/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update proxmox@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/proxmox --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/proxmox --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `proxmox-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the proxmox skill from https://github.com/servosity/msp-skills/tree/main/skills/proxmox. The skill defines how its required CLI (`proxmox-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create a privilege-separated API token for each product: PBS gets the `Audit` role on `/`; PVE gets `PVEAuditor` on `/`, plus `Datastore.AllocateSpace` and `VM.Backup` if your backups live on PVE storage (see [What API token does it need?](#what-api-token-does-it-need)). Then:

```bash
export PROXMOX_HOST=pve1.example.lan
export PROXMOX_API_TOKEN='monitor@pve!agent=<secret>'      # USER@REALM!TOKENID=SECRET
export PROXMOX_TLS_FINGERPRINT='<PVE certificate SHA-256>'  # stock Proxmox certificates are self-signed
# Optional, for PBS verify, datastore and sync data:
export PROXMOX_PBS_HOST=pbs.example.lan
export PROXMOX_PBS_API_TOKEN='monitor@pbs!agent:<secret>'  # colon before the secret
export PROXMOX_PBS_TLS_FINGERPRINT='<PBS certificate SHA-256>'
proxmox-cli doctor
```

Managing several clients? Add one entry per client cluster instead; `clusters.toml` stores only the *names* of the env vars that hold each token:

```bash
proxmox-cli clusters add acme --client "Acme Dental" --pve-host pve1.acme.lan --pve-token-env ACME_PVE_TOKEN --pve-fingerprint '<sha256>' --pbs-host pbs.acme.lan --pbs-token-env ACME_PBS_TOKEN --pbs-fingerprint '<sha256>'
proxmox-cli clusters list
```

`doctor` checks config, paths, API reachability and whether the PVE token authenticates. Run `proxmox-cli posture --agent` to confirm the token can also see guests, jobs and backups: anything it cannot read is listed under `fetch_failures`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client VMs don't have a good, recent, verified backup? | `proxmox-cli posture --only problems --agent` |
| Which VMs or disks are in no enabled backup job? | `proxmox-cli backup coverage --include-disks --agent` |
| What failed or stopped part-way overnight, by guest, with the error line? | `proxmox-cli failures --since 24h --agent` |
| How far has a PBS verify failure spread? | `proxmox-cli verify-health --agent` |
| Which backup datastores fill up within 30 days? | `proxmox-cli capacity --within 30d --agent` |
| Can we restore VM 104, and from which verified points? | `proxmox-cli restore-points "104" --verified-only --agent` |
| Did every scheduled backup produce a restore point this month, per client? | `proxmox-cli rpo --days 30 --by-client --csv` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Proxmox MCP servers are built to operate one cluster: list VMs, start and stop them, take snapshots, with the dangerous actions behind a flag. Many MSPs will never hand an AI agent that token. This one sends Proxmox nothing but reads (HTTP GET), so there is no write command to misfire, and with backups on Proxmox Backup Server both tokens can be audit-only.

The answer is the join. "Is this VM backed up?" needs four things Proxmox keeps in different places: whether an *enabled* job includes it, the newest real archive on any backup storage, whether Proxmox Backup Server verified that archive, and what the last vzdump task log said about that guest. `posture` and `backup coverage` do that join for every guest across every client cluster, `failures` and `rpo` for every guest a job touches, and all of them read live on every run. For inventory questions, `sync` keeps a **local SQLite mirror** with full-text search.

## The pain this closes

A Proxmox shop with many clients runs one cluster, and often one Backup Server, per client. A green backup task does not mean every guest was backed up: one vzdump job backs up many guests and names the one that failed only inside its task log. Verify failures spread quietly too. In a [Proxmox forum thread](https://forum.proxmox.com/threads/proxmox-backup-server-a-lot-of-verify-failed.165129/), a provider whose central PBS takes backups from 100 to 200 servers watched the weekly verify job report a growing number of failed snapshots before the cause was traced. And when an AI-agent management layer for Proxmox was [shared on the same forum](https://forum.proxmox.com/posts/858319/), administrators pushed back hard on letting an agent near their hosts.

This skill answers the morning questions without asking for that trust:

- `posture --only problems` - every guest that is uncovered, has no restore point, is stale, failed verification, or whose last attempt failed
- `backup coverage --include-disks` - every guest or disk no enabled job includes, with the reason
- `failures --since 24h` - failed and silently incomplete tasks, one row per guest or datastore with the error line
- `verify-health` - how far a verify failure spread, and the newest verified-good point left
- `rpo --days 30 --by-client` - per-client proof that every scheduled run produced a restore point

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Proxmox MCP server runs locally, but it speaks HTTP natively: set `PROXMOX_MCP_HTTP_TOKEN` to a long random value, start it with `proxmox-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default and refuses to start without that token) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Proxmox credentials once.

### Is my Proxmox data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Can the AI stop, change or delete anything on my Proxmox hosts?

No. The CLI and MCP server only contain read (GET) calls to Proxmox VE and Proxmox Backup Server; there is no command that starts, stops, migrates, restores, prunes or deletes anything. The only writes are local files: the cluster list, a saved token, the local mirror (which the per-endpoint read commands also update), the agent learning store, and a response cache of raw API answers (owner-only files; the backup commands never answer from it). One side effect to know: reading a storage's status lets Proxmox mount an enabled storage, which its own status daemon already does every 10 seconds.

### What API token does it need?

A privilege-separated API token for each product. Proxmox Backup Server: the `Audit` role on `/` (read-only). Proxmox VE: `PVEAuditor` on `/`. One catch: Proxmox VE lists backup files only to a token that also holds `Datastore.AllocateSpace` and `VM.Backup`, so if your backups live on dir, NFS or CIFS storage, or you have not configured the Backup Server here, add a custom role with those two privileges. An audit-only token is reported as "storage not read" and the guest as `unproven`, never as "no backups". If your backups are on Proxmox Backup Server and `PROXMOX_PBS_HOST` is set (spelled as in the PVE storage's server field: the same IP or hostname), the CLI reads them from the PBS API and the PVE token can stay audit-only. On a stock install the `local` storage also accepts backups: with an audit-only token it is listed under fetch failures until you remove "VZDump backup file" from its content types or grant it the two privileges. Until then fresh guests keep their verdicts, but a guest with no readable or only an old restore point is reported unproven rather than never or stale (the unread storage could hold one), `rpo` counts its missed runs as unproven, and `posture --fail-on-red` exits 1. Grant the roles on `/` (propagating) and put no NoAccess or narrower ACL below it: Proxmox leaves out of every list the guests and storages a token cannot see, and while the CLI flags a missing or narrowed grant, an explicit NoAccess on one guest is invisible to any read. The PVE token is USER@REALM!TOKENID=SECRET; the PBS token uses a colon before the secret.

### Proxmox uses a self-signed certificate. Do I have to turn off certificate checks?

No, and there is no option to. Pin the certificate's SHA-256 fingerprint (shown under the node's Certificates) with `PROXMOX_TLS_FINGERPRINT` and `PROXMOX_PBS_TLS_FINGERPRINT`, or per client cluster in `clusters.toml`. A PVE pin never vouches for the PBS host.

### How do I cover many client clusters?

Add one entry per client with `proxmox-cli clusters add`: its PVE host, optional PBS host and namespace, certificate fingerprints, and the names of the environment variables that hold its tokens (never the tokens themselves). The fleet commands then run across every configured cluster; `--cluster` or `--client` narrows to one. For an MCP client, add those token variables to the MCP server's `env` block too. On a shared Backup Server, give each client cluster its own PBS namespace (or datastore) and declare it as that entry's `pbs_namespace`: VMIDs repeat across clusters, so where two clusters write the same namespace the CLI cannot tell whose `vm/100` a snapshot is, reports the overlap, and does not count those restore points. A cluster that cannot be read is assumed to write anywhere in its declared `pbs_namespace`, or anywhere on its PBS when it declares none, so declaring one keeps a client outage from hiding the others' backups.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

Every request this skill sends to Proxmox is a read, so the tiers are about what happens on your machine and where output goes.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (Proxmox) | `posture`, `backup coverage`, `failures`, `verify-health`, `capacity`, `restore-points`, `rpo`, and every `vm`, `ct`, `nodes`, `storage`, `tasks`, `cluster` and `pbs-*` command | Allow |
| Local writes | `sync`, `workflow archive`, `export`, `profile save`, `teach` | Allow |
| Credential routing (local) | `clusters add` (where each client's tokens go and which certificate is trusted), `auth set-token`, `auth logout` | A human runs these; none is an MCP tool |
| Data leaving the machine | `--deliver webhook:<url>` or `--deliver file:<path>` on any command, `feedback --send`, the MCP server bridged to a public HTTPS endpoint | Human-in-the-loop only |
| Proxmox writes (start, stop, restore, prune, delete) | None - the skill has no such command | Not possible with this skill |

The strongest control is still the **scope you grant the tokens**: with backups on Proxmox Backup Server both tokens can be audit-only, so Proxmox itself would refuse a write even if one were ever sent. Backups on PVE storage need two backup privileges before Proxmox VE will list them; governance.md explains the trade-off. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Proxmox VE and Proxmox Backup Server API surface with mocked responses, and being validated with MSPs running it live against their own clusters in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-14._
