# Device Warranty CLI

**Dell, Lenovo and Apple warranty end dates for every client's devices, in one local mirror.**

Look up any serial against Dell TechDirect, Lenovo eSupport or Apple Business with whichever vendor credentials you have, keep every result in a local per-client warranty mirror (data-warranty.db), and answer fleet questions offline: report lists what expires soon, lifecycle counts coverage endings by quarter for refresh planning, unresolved shows what was never verified, and events list feeds coverage changes to your PSA.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `warranty-cli` and `warranty-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.ps1 | iex
   ```
3. Verify: `warranty-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=warranty). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/warranty --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/warranty --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `warranty-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the warranty skill from https://github.com/Servosity/msp-skills/tree/main/skills/warranty. The skill defines how its required CLI (`warranty-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=warranty).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Claude Desktop asks for each vendor's credentials. Every field is optional: fill in the vendors you have (for Apple, the private key field takes the path to the downloaded .pem file).

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "warranty": {
      "command": "warranty-mcp",
      "env": {
        "WARRANTY_DELL_CLIENT_ID": "<optional: Dell client ID>",
        "WARRANTY_DELL_CLIENT_SECRET": "<optional: Dell client secret>",
        "WARRANTY_LENOVO_CLIENT_KEY": "<optional: Lenovo ClientID token>",
        "WARRANTY_APPLE_CLIENT_ID": "<optional: Apple Business client ID, BUSINESSAPI.uuid>",
        "WARRANTY_APPLE_KEY_ID": "<optional: Apple Business key ID>",
        "WARRANTY_APPLE_PRIVATE_KEY": "<optional: path to the Apple Business .pem key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`warranty-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `WARRANTY_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `WARRANTY_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export WARRANTY_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
warranty-mcp --transport http
```

Set `WARRANTY_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available. Read-only here means no vendor-side writes: every vendor command is a read, and `lookup`, `recheck` and `apple-pull` stay available because they write only this CLI's local mirror (`data-warranty.db`).

## Vendor credentials

**Every vendor is optional.** `import`, `report`, `lifecycle`, `unresolved` and `events list` work with no credentials (they read the local mirror). `lookup`, `recheck`, `apple-pull` and the `dell`, `lenovo` and `apple` commands need that vendor's enrollment; a vendor without credentials is reported (`vendor_unconfigured`), never called, and never blocks the others.

## Authentication

Each vendor is optional and needs its own enrollment; a missing vendor never blocks the configured ones (its serials report vendor_unconfigured). Dell TechDirect: request Warranty API access (APIs, API Key Management; approval can take several days), then set WARRANTY_DELL_CLIENT_ID and WARRANTY_DELL_CLIENT_SECRET, or store them with the auth login command (the auth commands cover Dell only). Lenovo eSupport: a ClientID token from your Lenovo account manager or partner representative, environment only: WARRANTY_LENOVO_CLIENT_KEY. Apple Business: an API account (Settings, API) in each customer's organization, environment only: WARRANTY_APPLE_CLIENT_ID, WARRANTY_APPLE_KEY_ID and WARRANTY_APPLE_PRIVATE_KEY (PEM text or a path to the .pem file). One Apple account covers one customer organization, so set that customer's variables and pass --customer. warranty-cli doctor --json shows a vendors block with what each vendor still needs.

## Quick Start

```bash
# Confirms the install without calling anything; drop --dry-run and add --json to see the vendors block
warranty-cli doctor --dry-run

# Loads a client's RMM device export into the local mirror
warranty-cli import devices.csv --customer "Acme Dental"

# Looks up every imported serial with the vendors you have keys for
warranty-cli recheck --older-than 30d

# Lists devices expiring in the next 90 days or already out of warranty
warranty-cli report --within 90d

```

## Unique Features

### Fleet-wide answers from the local mirror
- **`lifecycle`**  -  Per client: devices covered now, expired, unverified (no coverage data yet), coverage ending in each of the next quarters, and devices past an age threshold.

  _Reach for this to count coverage endings by quarter for a refresh plan or a QBR hardware-lifecycle slide, for one or every client._

  ```bash
  warranty-cli lifecycle --quarters 4 --age 4y --agent
  ```
- **`apple-pull`**  -  Load one client's Apple devices from its Apple Business organization (needs that organization's WARRANTY_APPLE_* account; exits 10 without it) and see what is new, gone, or missing from the RMM import.

  _Use this to catch Macs and iPads the RMM export never listed._

  ```bash
  warranty-cli apple-pull --customer "Acme Dental" --agent
  ```
- **`report`**  -  Devices out of warranty or expiring within N days, per client, from the mirror.

  _Use this to list the individual devices that need a renewal quote or replacement._

  ```bash
  warranty-cli report --within 90d --customer "Acme Dental" --agent
  ```

### Keeping the mirror honest
- **`recheck`**  -  Re-check serials already in the mirror: never-checked first, then coverage ending soon, then stale; each vendor's batch size and pace respected, vendors without credentials skipped.

  _Use this on a schedule so reports and coverage events stay current without re-importing files._

  ```bash
  warranty-cli recheck --older-than 30d --expiring 120d --max-calls 50 --dry-run
  ```
- **`unresolved`**  -  Every device whose coverage the report cannot speak for, each with one reason: vendor unknown, vendor not configured, ambiguous, invalid, not found, not in the Apple organization, lookup error, never checked, stale or duplicated.

  _Check this before trusting an empty expiry report: it shows what was never verified._

  ```bash
  warranty-cli unresolved --customer "Acme Dental" --agent
  ```
- **`events list`**  -  Coverage expiring, expired and changed events after a cursor, once each.

  _Poll this to open tickets or quotes for coverage that is ending or changed._

  ```bash
  warranty-cli events list --since 0 --agent
  ```

### Serial lookups
- **`lookup`**  -  Warranty for one or more serials, vendor detected from the serial or named, normalized to one coverage-line shape and saved to the mirror; vendors without credentials are reported as vendor_unconfigured, not called.

  _Use this to answer 'is this device still covered, and until when' for Dell, Lenovo or Apple serials whose vendor credentials are set._

  ```bash
  warranty-cli lookup 1ABC234 PF1ABCDE --agent
  ```
- **`import`**  -  Load a CSV or JSON device export (serial, optional manufacturer, model, hostname) into the mirror under a client name; CLI only, not available over MCP.

  _Start here so report, lifecycle and recheck know which client owns each serial._

  ```bash
  warranty-cli import devices.csv --customer "Acme Dental"
  ```

## Recipes

### QBR lifecycle slide for one client

```bash
warranty-cli lifecycle --customer "Acme Dental" --quarters 4 --csv
```

Coverage ending per quarter and devices past the age threshold, ready for a spreadsheet.

### What expires in the next 90 days, trimmed for an agent

```bash
warranty-cli report --within 90d --agent --select devices.serial,devices.customer,devices.coverage_end,unverified,note
```

Keeps serial, customer and end date, plus unverified and note so an empty list cannot be mistaken for a verified fleet.

### Recheck never-checked, soon-expiring and stale serials within a call budget

```bash
warranty-cli recheck --older-than 30d --max-calls 20
```

Never-checked devices go first, then coverage ending within 120 days, then devices last checked over 30 days ago, up to 20 vendor calls.

### Feed coverage changes to a PSA

```bash
warranty-cli events list --since 0 --agent
```

Returns coverage events and a cursor to pass as --since next time.

## Usage

Run `warranty-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, the warranty mirror `data-warranty.db`, and the generated store `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `WARRANTY_CONFIG_DIR`, `WARRANTY_DATA_DIR`, `WARRANTY_STATE_DIR`, or `WARRANTY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `WARRANTY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export WARRANTY_HOME=/srv/warranty
warranty-cli doctor
```

On Windows (PowerShell):

```powershell
$env:WARRANTY_HOME = "C:\ProgramData\warranty"
warranty-cli doctor
```

Under `WARRANTY_HOME=/srv/warranty`, the four dirs resolve to `/srv/warranty/config`, `/srv/warranty/data`, `/srv/warranty/state`, and `/srv/warranty/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:WARRANTY_HOME = "C:\srv\warranty"
warranty-cli doctor
```

Under `$env:WARRANTY_HOME = "C:\srv\warranty"`, the four dirs resolve to `C:\srv\warranty\config`, `C:\srv\warranty\data`, `C:\srv\warranty\state`, and `C:\srv\warranty\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "warranty": {
      "command": "warranty-mcp",
      "env": {
        "WARRANTY_HOME": "/srv/warranty"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `WARRANTY_DATA_DIR` overrides an explicit `--home` for that kind. Use `WARRANTY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `WARRANTY_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `warranty-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

The warranty mirror (`data-warranty.db` in the data directory) holds every client's devices, coverage lines and coverage events; `import`, `lookup`, `recheck` and `apple-pull` write it. Generated endpoint reads in `--data-source auto` mode are also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential). Vendor responses are not cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `warranty-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `warranty-cli local-data purge` lists the store files (including `data-warranty.db`), response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `WARRANTY_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync`, `workflow archive`, `import`, `recheck` and `apple-pull` refuse to run; `lookup` still answers but does not save to the mirror. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `WARRANTY_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### apple

Apple Business AppleCare coverage and organization devices by serial

- **`warranty-cli apple coverage`** - Returns every coverage line for the device: limited warranty, AppleCare+ or AppleCare+ for Business, with status, payment type, start and end dates and renewal flags.
- **`warranty-cli apple get`** - Returns the device's model, product family, part and order numbers, order date and the date it was added to the organization.
- **`warranty-cli apple list`** - Lists every device the organization enrolls through Automated Device Enrollment, with serial number, model, product family and order date. Pages with meta.paging.nextCursor.

### dell

Dell TechDirect warranty entitlements by service tag

- **`warranty-cli dell entitlements`** - Returns one record per service tag with ship date, product line and every entitlement line (start, end, service level). Tags the gateway cannot match come back with invalid true and no entitlements.
- **`warranty-cli dell summary`** - Returns the asset header, entitlements and shipped components for a single service tag.

### lenovo

Lenovo eSupport warranty, warranty offerings and contracts by serial

- **`warranty-cli lenovo contract`** - Returns the contract description and dates and every covered product with its serial, machine type, SLA, status and coverage dates.
- **`warranty-cli lenovo offering`** - Returns the localized name, description, type, delivery, category and duration for a warranty SDF code such as the ID on a warranty line.
- **`warranty-cli lenovo warranty`** - Returns the product, ship and purchase dates, in-warranty flag, warranty lines and contract lines for the serial. Use `lookup` for several serials. ErrorCode 100 means no warranty data; 101 means several machines share the serial, so pass SERIAL.MACHINETYPE.


### apple-pull


- **`warranty-cli apple-pull`** - Load one client's Apple devices from its Apple Business organization (needs that organization's WARRANTY_APPLE_* account; exits 10 without it) and see what is new, gone, or missing from the RMM import.


### events


- **`warranty-cli events list`** - Coverage expiring, expired and changed events after a cursor, once each.


### import


- **`warranty-cli import`** - Load a CSV or JSON device export (serial, optional manufacturer, model, hostname) into the mirror under a client name; CLI only, not available over MCP.


### lifecycle


- **`warranty-cli lifecycle`** - Per client: devices covered now, expired, unverified (no coverage data yet), coverage ending in each of the next quarters, and devices past an age threshold.


### lookup


- **`warranty-cli lookup`** - Warranty for one or more serials, vendor detected from the serial or named, normalized to one coverage-line shape and saved to the mirror; vendors without credentials are reported as vendor_unconfigured, not called.


### recheck


- **`warranty-cli recheck`** - Re-check serials already in the mirror: never-checked first, then coverage ending soon, then stale; each vendor's batch size and pace respected, vendors without credentials skipped.


### report


- **`warranty-cli report`** - Devices out of warranty or expiring within N days, per client, from the mirror.


### unresolved


- **`warranty-cli unresolved`** - Every device whose coverage the report cannot speak for, each with one reason: vendor unknown, vendor not configured, ambiguous, invalid, not found, not in the Apple organization, lookup error, never checked, stale or duplicated.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`warranty-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`warranty-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`warranty-cli learnings list`** - Inspect taught rows
- **`warranty-cli learnings forget <query>`** - Undo a teach
- **`warranty-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`warranty-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`warranty-cli teach-pattern`** - Install a query/resource template up front
- **`warranty-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `WARRANTY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `warranty-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
warranty-cli apple list

# JSON for scripting and agents
warranty-cli apple list --json
# Filter to specific fields
warranty-cli apple list --json --select id,attributes.serialNumber,attributes.deviceModel

# Dry run  -  show the request without sending
warranty-cli apple list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
warranty-cli apple list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - `report`, `lifecycle`, `unresolved` and `events list` answer from the local warranty mirror
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` error (for example a command refused while `WARRANTY_NO_LOCAL_DATA=1`), `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
warranty-cli doctor
```

Shows which vendors are configured (the `vendors` block in `doctor --json`, with what each still needs) and whether the Dell gateway is reachable. With a Dell credential it spends one read of Dell's entitlement endpoint and reports whether Dell accepted it. It does not prove a Lenovo or Apple key works: run a `lookup` for that.

## Configuration

Run `warranty-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/warranty-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\warranty-cli\config.toml` on Windows; `--home`, `WARRANTY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `WARRANTY_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `WARRANTY_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `WARRANTY_DELL_CLIENT_ID` | per_call | No | Dell TechDirect client ID (Warranty API). Also storable with `auth login`. |
| `WARRANTY_DELL_CLIENT_SECRET` | per_call | No | Dell TechDirect client secret. Also storable with `auth login`. |
| `WARRANTY_LENOVO_CLIENT_KEY` | per_call | No | Lenovo eSupport ClientID token from your Lenovo account manager. Environment only. |
| `WARRANTY_APPLE_CLIENT_ID` | per_call | No | Apple Business API account client ID (BUSINESSAPI.uuid). Environment only. |
| `WARRANTY_APPLE_KEY_ID` | per_call | No | Key ID of the Apple Business API account's private key. Environment only. |
| `WARRANTY_APPLE_PRIVATE_KEY` | per_call | No | Apple Business ES256 private key: PEM text or a path to the .pem file. Environment only. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `warranty-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie` (Dell credentials only). Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `warranty-cli doctor --json` and read `vendors`: it names each vendor's missing variables without printing any value
- Exit 4 comes from the vendor named in the message; the other vendors are unaffected
**Not found errors (exit code 3)**
- Check the serial or ID is correct
- For Apple serials, run `warranty-cli apple list` to see the devices in the configured organization

### API-specific
- **A vendor shows as not configured in doctor**  -  Set that vendor's variables (warranty-cli doctor --json lists what each vendor needs); the other vendors keep working without them
- **Lenovo answers ErrorCode 101 for a serial**  -  Several machines share that serial: look it up as SERIAL.MACHINETYPE, for example warranty-cli lookup PF1ABCDE.21CB --vendor lenovo
- **A serial was not looked up because its vendor could not be detected**  -  Pass --vendor dell|lenovo|apple, or add a manufacturer column to the import file
- **Apple answers 404 for a serial**  -  The device is not in the Apple Business organization whose API account is configured; set that client's Apple variables
- **Dell answers 401 or 403**  -  Check in TechDirect that the API key is approved for the Warranty API, then run warranty-cli doctor
- **Over MCP, import is missing and lookup --file is refused**  -  Both read a file on the machine, so they are CLI-only; run them from a terminal, then use recheck, report or lookup over MCP

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PowerShellWarrantyReports (PSWarranty)**](https://github.com/KelvinTegelaar/PowerShellWarrantyReports)  -  PowerShell (200 stars)
- [**warranty-watcher**](https://github.com/mhaowork/warranty-watcher)  -  TypeScript (24 stars)
- [**Dell-Lookup**](https://github.com/in03/Dell-Lookup)  -  Python (1 stars)
- [**breeze warranty providers**](https://github.com/LanternOps/breeze)  -  TypeScript
- [**enthus-golang/lenovo**](https://github.com/enthus-golang/lenovo)  -  Go
- [**vcf-readiness Dell TechDirect client**](https://github.com/vmware/vcf-readiness)  -  Python
- [**DellWarranty**](https://github.com/zimbres/DellWarranty)  -  C#
- [**Dell-Warranty-API**](https://github.com/umn-umm/Dell-Warranty-API)  -  PHP

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
