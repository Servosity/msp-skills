# ESET + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ESET
> API. Not affiliated with, endorsed by, or sponsored by ESET, spol. s r.o.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/eset-protect/">
    <img src="../../docs/assets/video/eset-protect/animated-og.gif" alt="ESET PROTECT demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/eset-protect/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every ESET Connect endpoint in one binary, plus per-customer health, exposure and QBR reports built from a local mirror of your MSP tree. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ESET skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `eset-protect-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `eset-protect-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `eset-protect-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `eset-protect-mcp --transport http` with `ESET_PROTECT_MCP_HTTP_TOKEN` set (it refuses to start without one) and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `eset-protect-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ESET MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/eset-protect-v0.1.0/eset-protect-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ESET release on the [releases page](https://github.com/servosity/msp-skills/releases?q=eset-protect).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install eset-protect@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ESET Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex`. Then authenticate per the README and run `eset-protect-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)
```

The installer drops both `eset-protect-cli` (the CLI) and `eset-protect-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
eset-protect-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update eset-protect@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/eset-protect --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/eset-protect --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `eset-protect-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the eset-protect skill from https://github.com/servosity/msp-skills/tree/main/skills/eset-protect. The skill defines how its required CLI (`eset-protect-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create a dedicated ESET Connect API user (a Superuser or Root does it in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2, with the **Integrations** toggle on), then give the CLI its email and password. `ESET_PROTECT_REGION` is the region of your ESET PROTECT console (`eu` by default; `de`, `us`, `ca` or `jpn`):

```bash
ESET_PROTECT_USERNAME=<value> ESET_PROTECT_PASSWORD=<value> ESET_PROTECT_REGION=<value> eset-protect-cli auth login
```

On Windows (PowerShell):

```powershell
$env:ESET_PROTECT_USERNAME = "<value>"
$env:ESET_PROTECT_PASSWORD = "<value>"
$env:ESET_PROTECT_REGION = "<value>"
eset-protect-cli auth login
```

`auth login` exchanges the email and password for a one-hour ESET Connect token, caches the token for that region and user, and proves it with one read call (it exits 4 when ESET rejects the credentials or the API user cannot read devices, and 5 when the token request or the probe fails for another reason, such as a network error or an ESET outage); other commands mint and refresh the token on their own. The password is not written to disk. `ESET_PROTECT_TOKEN` is optional, for a token you minted yourself. `eset-protect-cli doctor` checks config, paths, reachability and the credentials, and exits 0 even when a row fails, so scripts should add `--fail-on error`.

Then fill the local mirror the per-customer views read from:

```bash
eset-protect-cli sync --resources devices,device-groups,detections,incidents
```

Re-run it before a triage or report run (the views read what was last synced), and run `eset-protect-cli sync --full` now and then: a plain sync adds and updates, while `--full` also drops devices and groups that were deleted in ESET.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customers have devices that need attention, are muted or stopped checking in? | `eset-protect-cli fleet health` |
| What should my techs work first this morning, detections and open incidents together? | `eset-protect-cli triage --since 24h --min-severity medium` |
| Which customers and devices are exposed to this CVE, and is a patch available? | `eset-protect-cli exposure --cve CVE-2024-3400` |
| Give me one customer's quarter: device health, detections, incidents, top CVEs and patches | `eset-protect-cli report customer "Acme Dental" --since 90d` |
| Which devices have not checked in for two weeks, most stale first? | `eset-protect-cli fleet stale --days 14` |
| Which security patches are still missing, ranked by the devices and CVE risk they fix? | `eset-protect-cli patches backlog --security-only` |
| Is the same file hash showing up at more than one customer this week? | `eset-protect-cli detections spread --since 7d --min-customers 2` |
| Isolate this machine from the network (show me the exact task first) | `eset-protect-cli respond isolate "FRONTDESK-01"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ESET integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 40 customers have machines that need attention or stopped checking in, worst first".

ESET Connect has no "which customer owns this device" call: the answer is the nearest ancestor device group linked to a company, which only the synced group tree can tell you. This skill syncs your ESET PROTECT tree into a **local SQLite mirror**, so `fleet health`, `triage` and `detections spread` are local joins across devices, device groups, detections and incidents, each row attributed to its customer. Live views such as `exposure`, `patches backlog` and `report customer` read ESET directly and attribute every hit through the same tree. A partial scan reports `truncated` and `rejected_rows` instead of passing for a clean result.

MCP servers such as [maciekaz/ESET-MCP](https://github.com/maciekaz/ESET-MCP) and [Fenrindale/eset-protect-mcp](https://github.com/Fenrindale/eset-protect-mcp) expose the ESET Connect endpoints as tools. This one covers the endpoints too, and adds the per-customer rollups on top. ESET's own [ESET LiveAI](https://www.eset.com/us/business/solutions/liveai/) (formerly AI Advisor) is a generative AI assistant inside ESET PROTECT and ESET Inspect; this skill complements it and the ESET PROTECT console rather than replacing them.

## The pain this closes

ESET PROTECT's [Managed Customers](https://help.eset.com/protect_cloud/en-US/managed_customers.html) page gives an MSP per-customer counts of devices and unresolved detections. The next questions (which machines are muted or stopped checking in, which customers carry a new CVE, what to work first, what goes in this quarter's QBR) are still answered by filtering and copying by hand. An AI agent cannot answer them from the API alone either: ESET Connect returns devices, detections and incidents by device group with no field that names the customer, splits detections between v1 and v2, keeps incidents in another service, mints one-hour password-grant tokens and answers slow requests with 202 and a response id to poll. Integrations trip over it ([elastic/integrations#15335](https://github.com/elastic/integrations/issues/15335), [#20977](https://github.com/elastic/integrations/issues/20977)).

- `eset-protect-cli fleet health` - one row per customer: devices that need attention, problem counts, muted, stale and unenrolled devices, worst first.
- `eset-protect-cli triage --since 24h --min-severity medium` - endpoint detections and open incidents in one ranked queue, with device, customer and a suggested next command.
- `eset-protect-cli exposure --cve CVE-2024-3400` - which customers and devices carry a CVE, and whether a patch is available.
- `eset-protect-cli report customer "Acme Dental" --since 90d` - the QBR one-pager for one customer.
- `eset-protect-cli respond isolate "FRONTDESK-01"` - the exact network isolation task for a named device, sent only with `--yes`.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ESET MCP server runs locally, but it speaks HTTP natively: start it with `eset-protect-mcp --transport http` and `ESET_PROTECT_MCP_HTTP_TOKEN` set to a long random value (it refuses to start without one; it serves `http://127.0.0.1:7777/mcp` by default) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ESET credentials once.

### Is my ESET data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What ESET account do I need?

A dedicated ESET Connect API user. A Superuser or Root creates it in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2, gives it the access rights the workflow needs, and turns on the **Integrations** toggle ([ESET's steps](https://help.eset.com/eset_connect/en-US/create_api_user_account.html)). The Superuser or Root account itself cannot get API access. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once before the API works. Set `ESET_PROTECT_USERNAME` and `ESET_PROTECT_PASSWORD` to that user's email and password.

### Which region should I set?

The region of your ESET PROTECT console: `ESET_PROTECT_REGION=eu` (the default), `de`, `us`, `ca` or `jpn`. ESET Connect has no quarantine or Cloud Office user API in `jpn`, and no v2 detection list there.

### Will this hit ESET Connect rate limits?

ESET Connect allows 10 calls per second for each set of credentials, account and source IP, and answers 429 above that ([rate limits](https://help.eset.com/eset_connect/en-US/rate_limits.html)). Under ESET's Fair Use Policy a very large burst (the page's example is 1000+ requests) is slowed to 2 requests per second with 202 answers, and the rest is rejected with 50x. The CLI slows down and retries when it sees a 429; pass `--rate-limit 8` to stay under the limit from the start, and keep `sync --concurrency` low on a large tree. Requests ESET answers with 202 and a response id are polled until the request `--timeout` runs out (60 seconds by default). ESET answers 504 to a query that is not ready within 120 seconds ([how the API works](https://help.eset.com/eset_connect/en-US/how_api_works.html)), so for large queries pass `--timeout 3m`, and narrow the query (`--customer`, a shorter `--since`) if ESET still answers 504.

### Why do my detections look different from the console?

ESET splits them ([Incident Management](https://help.eset.com/eset_connect/en-US/incident_management.html)): v1 detections cover ESET PROTECT and ESET Inspect endpoints, and the v2 detection list covers ESET Cloud Office Security only. `triage` and `detections latest` use v1, which has no resolved state, so a detection you already handled still lists until it ages out of `--since`. `detections latest --include-cloud-office` adds the v2 Cloud Office detections. Quarantine and Cloud Office user commands need the API user to have signed in to the ESET Cloud Office Security console once.

### Can the AI isolate a machine or change a policy by itself?

The `respond` commands (`scan`, `isolate`, `release`) and `policies clone` only print the request unless you add `--yes`, and over MCP the `respond` tools only preview. Every other write command runs as soon as it is invoked, so preview it with `--dry-run` first. Over MCP, the generic `eset-protect_execute` tool can call any ESET Connect endpoint, including device tasks, deletes and policy assignments, gated only by your AI app's approval prompt. For an agent that should only read, set `ESET_PROTECT_MCP_READ_ONLY=1` in the MCP server's environment and keep the approval prompt on: no tool that changes ESET is offered, and the Credential-tier reads (installer and enrollment links, quarantine downloads) are refused. See [governance.md](./governance.md) for exactly what read-only mode allows.

### Does it work with on-premises ESET PROTECT?

No. It signs in through the ESET Connect cloud API (`*.eset.systems`), so it works with ESET PROTECT (cloud). An on-premises ESET PROTECT server uses a different API and sign-in, which this connector does not support.

### Does it replace the ESET PROTECT console?

No. The console stays the place for policy editing, investigations in ESET Inspect and day-to-day administration. This skill answers the across-every-customer questions and runs previewed actions from your AI agent.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `fleet gaps`, `fleet stale`, `triage`, `exposure`, `report customer`, `report vulnerabilities`, `patches backlog`, `detections spread`, `detections latest`, `devices profile`, `incidents context`, `executables find`, `policies decode`, `devices list`, `incidents list`, `detections list`, `policies list`, `sync` (writes only the local mirror), `export` (writes a local file; Credential-tier fields redacted) | Allow |
| Endpoint response | `respond scan`, `respond isolate`, `respond release` (preview unless `--yes`), `device-tasks create`, `device-tasks update-targets`, `device-tasks update-triggers`, `mobile-devices-batch-activate-product` | Human-in-the-loop, explicit confirmation - these reach real customer machines |
| Protection and policy config | `executables block`, `executables unblock`, `edr-rules create`, `edr-rules enable`, `edr-rules disable`, `edr-rules update-definition`, `edr-rule-exclusions create`, `edr-rule-exclusions update-definition`, `policies create`, `policies clone` (preview unless `--yes`), `policy-assignments assign-policy`, `policy-assignments unassign-policy`, `policy-assignments update-ranking`, `ip-sets update`, `web-address-rules domains web-address-rules-update`, `devices move`, `groups move` | Preview with `--dry-run`, then a human-approved write - an exclusion, a disabled rule or a move into another group's policy can open a hole |
| Write (routine) | `incidents close`, `incidents reopen`, `incidents basic-attributes-update incidents-update-incident-basic-attributes`, `incidents comments incident-create-incident`, `incidents comments incident-update-incident-text`, `detection-groups resolve`, `detections-v2 resolve`, `devices rename`, `groups create`, `groups rename`, `installers create` | Preview with `--dry-run`, then a reviewed write |
| Credential and access | `installers list`, `installers get`, `gpo-sccm-files-generate`, `mobile-devices-batch-get-enrollment-links` (links and files that enroll a device into your tenant), `quarantined-objects-download`, `quarantined-objects-batch-download` (quarantined mail and file content), `roles create`, `roles delete`, `role-assignments-assign-role`, `role-assignments-revoke-role`, `auth login`, `auth set-token` | Human-in-the-loop only |
| Destructive / high-risk | `groups delete` (and every group under it), `policies delete`, `edr-rules delete`, `edr-rule-exclusions delete`, `device-tasks delete`, `installers delete`, `incidents comments incident-delete-incident`, `quarantined-objects-restore`, `quarantined-objects-batch-restore` (releases quarantined items), `quarantined-objects-purge`, `quarantined-objects-batch-delete`, `devices-batch-import`, `import` (one create per JSONL line, so it can reach any tier above) | Human-in-the-loop only, explicit confirmation |

Generated write commands run as soon as they are invoked; preview with `--dry-run`. `--dry-run` covers the API request only: `--deliver webhook:<url>` still posts the output and `feedback --send` still sends. Over MCP, `eset-protect_execute` can reach every write in the table; set `ESET_PROTECT_MCP_READ_ONLY=1` for an agent that should only read.

The strongest control is the **scope you grant the ESET credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ESET API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-19._
