---
name: datagate
description: "Every DataGate telecom billing API resource - customers, agreements, invoices, sites, products, and more - plus a local SQLite mirror and offline search. Trigger phrases: `datagate invoices`, `datagate customer lookup`, `pull this month's datagate invoices`, `use datagate`, `run datagate`."
author: "anonymous"
license: "Apache-2.0"
vendor: "DataGate"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - datagate-cli
    install:
      - kind: script
        bins: [datagate-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.ps1
---

# DataGate - Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `datagate-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.ps1 | iex
   ```
3. Verify: `datagate-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `datagate-cli` and `datagate-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) to wire up the MCP server for Claude Desktop,
ChatGPT, or another MCP-speaking agent.

## Authentication

Set two environment variables before running any command that reaches the live API:

```bash
export DATAGATE_API_KEY="<your Bearer token>"
export DATAGATE_CLIENT_ID="<your ClientId GUID>"
```

Both come from your DataGate account. There is no OAuth flow and no token expiry - the
Bearer token and ClientId header together identify your account for every request.
Run `datagate-cli doctor` after setting them to confirm connectivity.

## What this connector covers

Read-only access (list, get, search) across DataGate's resource groups: customers,
customer users, agreements, service items, assignments, rate cards, sites, product
templates, kit templates, invoices, products, delivery methods, product transactions,
and account managers. See [governance.md](./governance.md) for exactly which commands
are read vs. write in this build (all read, in this version).

## Local mirror

`datagate-cli sync` pulls the resource groups into a local SQLite database so
`datagate-cli search "<term>"` can answer offline instead of paging through the live
API by hand. Useful for repeated lookups across a large customer list without
burning API calls against DataGate's rate limit (60 requests/minute, 5,000/day, per
account).

## Common commands

```bash
datagate-cli doctor                                          # verify auth + connectivity
datagate-cli customers list --json
datagate-cli customers get <customer-id>
datagate-cli agreements list --customer-id <customer-id>
datagate-cli invoices --period-start 2026-08-01T00:00:00Z --period-end 2026-08-31T23:59:59Z --json
datagate-cli sync
datagate-cli search "<customer name or invoice number>"
```

See [guide.md](./guide.md) for the full command reference and
[mcp-install.md](./mcp-install.md) for wiring this up as an MCP server.

## Known gaps

- This build is read-only. DataGate's full CRUD surface is documented but this
  connector only implements list/get/search - no create, update, or delete.
- A handful of endpoint paths (sites, products, product-templates, kit-templates,
  delivery-methods, product-transactions, account-managers) were derived from
  DataGate's own naming convention rather than individually confirmed against a
  live tenant - see the "awaiting live verification" note on this connector's
  catalog entry. If you run this against your own DataGate account and hit a
  wrong path, please file it (see CONTRIBUTING.md, rung 2).
- Invoice search is `POST /invoices/search` with a JSON filter body, not a
  documented `GET /invoices` - the CLI models this correctly, but it's worth
  knowing if you're comparing against DataGate's own API docs.
