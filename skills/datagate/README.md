# DataGate + AI - for Claude, ChatGPT, Codex, Cursor, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the DataGate telecom billing API. Not affiliated with, endorsed by, or sponsored by DataGate. DataGate is a trademark of its respective owner.

<!-- media:start -->
<!-- media:end -->

Add **DataGate customer, agreement, and invoice lookups** to the AI you already use -
**Claude Code**, **Claude Desktop**, **ChatGPT** (Plus/Pro+), **Codex**, **Cursor**,
**Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open
source, runs on your laptop. A local SQLite mirror means repeated lookups don't have
to page through the live API by hand, and don't eat into DataGate's per-account rate
limit. Built for MSPs and telecom resellers who bill through DataGate. No code
required.

## Works with your agent

| Your AI agent | How to install the DataGate skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `datagate-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `datagate-mcp` over HTTPS, register as a Developer Mode connector. |
| **Claude Code** | Paste the install prompt below. |
| **Codex CLI** | Paste the install prompt below. |

For ChatGPT, run `datagate-mcp --transport http` and expose that behind HTTPS - no
bridge package is needed. See [mcp-install.md](./mcp-install.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and
> Gemini CLI - plus any MCP-speaking agent, via MCP and the pre-wired skill
> frontmatter. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

## Install

macOS / Linux:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.sh)
```

Windows (PowerShell):

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.ps1 | iex
```

Both installers detect your OS and architecture, download the `datagate-cli` and
`datagate-mcp` binaries from this repo's Releases, write them to `~/.local/bin`
(macOS/Linux) or `%LOCALAPPDATA%\Programs\msp-skills\` (Windows), clear macOS
Gatekeeper quarantine, and honor `DRY_RUN=1`.

## Authenticate

```bash
export DATAGATE_API_KEY="<your Bearer token>"
export DATAGATE_CLIENT_ID="<your ClientId GUID>"
datagate-cli doctor
```

Both values come from your own DataGate account - there's no OAuth flow, no token
expiry, and nothing to refresh. `doctor` confirms both are set and the API is
reachable.

## Why a local mirror instead of a wrapper

Most thin DataGate wrappers proxy every question into a live API call. That's fine
for one customer lookup, but it adds up once you're checking a large customer list
or pulling invoices repeatedly - each call counts against DataGate's rate limit (60
requests/minute, 5,000/day, per account). This skill syncs customers, agreements,
and invoices into a local SQLite mirror with full-text search, so repeated lookups
become one local query: instant, offline, and no extra API calls.

## What it covers

Read-only (list, get, search) across DataGate's resource groups: customers, customer
users, agreements, service items, assignments, rate cards, sites, product templates,
kit templates, invoices, products, delivery methods, product transactions, and
account managers. See [governance.md](./governance.md) for the full read/write
breakdown and [guide.md](./guide.md) for every command.

## Example commands

```bash
datagate-cli customers list --json
datagate-cli agreements list --customer-id <customer-id>
datagate-cli invoices --period-start 2026-08-01T00:00:00Z --period-end 2026-08-31T23:59:59Z --json
datagate-cli sync
datagate-cli search "<customer name or invoice number>"
```

## Known gaps

This build is read-only by design - DataGate's CRUD surface is documented but this
connector doesn't implement create/update/delete. A handful of endpoint paths
(sites, products, product templates, kit templates, delivery methods, product
transactions, account managers) were derived from DataGate's own naming convention
rather than individually confirmed against a live tenant - if you run this against
your own account and something's off, please file it (see
[CONTRIBUTING.md](../../CONTRIBUTING.md), rung 2 - a bug report with real-tenant
evidence is worth more than one we can't reproduce).

## License

Apache-2.0. See [cli/LICENSE](./cli/LICENSE) and [cli/NOTICE](./cli/NOTICE).
