# Domain and Certificate Tracker CLI

**Every client's domains and certificates in one inventory: what expires, unlocks or changes.**

Keep a per-client list of domains and hosts, then let sweep check registrations (RDAP), Certificate Transparency, live TLS and DNS for all of them and diff against the last run. report, renewals, issuers audit and hosts discover answer the weekly questions from the local store, and events list hands each change to an agent once. No account or credential is needed anywhere.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `domain-cert-tracker-cli` and `domain-cert-tracker-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex
   ```
3. Verify: `domain-cert-tracker-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=domain-cert-tracker). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/domain-cert-tracker --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/domain-cert-tracker --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `domain-cert-tracker-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the domain-cert-tracker skill from https://github.com/Servosity/msp-skills/tree/main/skills/domain-cert-tracker. The skill defines how its required CLI (`domain-cert-tracker-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=domain-cert-tracker).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "domain-cert-tracker": {
      "command": "domain-cert-tracker-mcp"
    }
  }
}
```

</details>

### Remote MCP over HTTP

`domain-cert-tracker-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `DOMAIN_CERT_TRACKER_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
domain-cert-tracker-mcp --transport http
```

Set `DOMAIN_CERT_TRACKER_MCP_READ_ONLY=1` (exactly `1`) to expose only read tools: it drops `sweep`, `sync`, `export`, `clients set`, `clients remove`, the learn-loop writers (`teach`, `teach-pattern`, `teach-lookup`, `learnings confirm`, `learnings reject`, `learnings forget`, `playbook amend`) and `workflow archive` from the MCP server.

## Authentication

No credentials. RDAP (through rdap.org and the IANA bootstrap file) and crt.sh are public. HTTP lookups and TLS probes connect only to public addresses, over https or TLS on port 443 unless the command line passes --allow-port; DNS checks use the resolver your operating system is configured with.

## Quick Start

```bash
# Safe first step that needs no network; drop --dry-run to see the store path, cache and reachability.
domain-cert-tracker-cli doctor --dry-run

# One live RDAP lookup: registrar, expiry, lock statuses and nameservers.
domain-cert-tracker-cli domain example.com --json

# What certificate the host serves right now and when it expires.
domain-cert-tracker-cli tls probe example.com --agent

# Adds a client to the local inventory that sweep walks.
domain-cert-tracker-cli clients set "Acme Dental" --domain example.com --host www.example.com --yes

# Checks every inventoried domain and host and records the diff.
domain-cert-tracker-cli sweep --agent

# The weekly per-client view of what expires, unlocked or changed.
domain-cert-tracker-cli report --days 30 --agent

```

## Unique Features

### Per-client estate
- **`clients list`**  -  Keep each client's domains and hosts in one local inventory that every sweep and report walks.

  _Reach for it first: sweep, report, renewals and hosts discover all iterate this inventory._

  ```bash
  domain-cert-tracker-cli clients list --agent
  ```
- **`report`**  -  See, per client, what expires within N days, which registrations are not transfer-locked, new CT certificates and DNS drift, plus what the last sweep could not check.

  _Use it for the weekly estate check; its unverified section keeps a failed check from reading as a clean result._

  ```bash
  domain-cert-tracker-cli report --days 30 --agent
  ```
- **`renewals`**  -  Find hosts still serving an old certificate after a newer one was logged, or past their renewal point with no successor in CT.

  _As lifetimes fall toward 47 days, this is where silently failed renewal automation shows up first._

  ```bash
  domain-cert-tracker-cli renewals --agent
  ```
- **`issuers audit`**  -  Separate certificates from issuers a domain's CAA does not permit, or a first-ever issuer, from routine renewals.

  _Use it to triage the week's new-certificate events so shadow IT and mis-issuance stand out from renewals._

  ```bash
  domain-cert-tracker-cli issuers audit --days 7 --agent
  ```
- **`hosts discover`**  -  List hostnames that have certificates in CT for a client's domains but are not in that client's inventory.

  _Run it at onboarding and weekly so the forgotten subdomain is watched before its certificate expires._

  ```bash
  domain-cert-tracker-cli hosts discover --agent
  ```

### Sweep and change feed
- **`events list`**  -  Read typed events from each sweep diff (domain.expiring, cert.expiring, ct.new_cert, dns.changed, registrar.unlocked) after a cursor.

  _Poll it after the nightly sweep to open tickets once per change instead of re-reporting the whole estate._

  ```bash
  domain-cert-tracker-cli events list --since 0 --agent
  ```
- **`sweep`**  -  Check every inventoried domain's registration, new CT certificates and DNS, and every inventoried host's live TLS certificate, in one run; then store the results and diff them against the last run.

  _Schedule it nightly; it is what fills report, renewals and the events feed._

  ```bash
  domain-cert-tracker-cli sweep --agent
  ```

### Live checks
- **`tls probe`**  -  Handshake with a live host and get its chain, expiry, SANs, key, issuer, hostname match and OCSP must-staple as JSON.

  _Use it on a certificate-warning ticket to see exactly what one host serves right now._

  ```bash
  domain-cert-tracker-cli tls probe example.com --agent
  ```
- **`dns check`**  -  Get NS, MX, SPF, DMARC, CAA and DNSSEC for a domain as one JSON document through the system resolver.

  _Use it for deliverability and onboarding reviews instead of several one-record lookups._

  ```bash
  domain-cert-tracker-cli dns check example.com --agent
  ```

## Recipes

### Narrow an RDAP answer to what matters

```bash
domain-cert-tracker-cli domain example.com --agent --select ldhName,status,events.eventAction,events.eventDate
```

RDAP domain objects carry long notices and vCards; --select keeps the status list and the registration and expiration dates.

### Monday estate check

```bash
domain-cert-tracker-cli report --days 30 --agent
```

Per client: what expires in 30 days, unlocked registrations, new CT certificates, DNS drift, and what could not be checked.

### Hand changes to a ticketing agent

```bash
domain-cert-tracker-cli events list --since 0 --agent
```

Returns typed events after the cursor plus the next cursor to pass next time.

### Find renewals that never reached the server

```bash
domain-cert-tracker-cli renewals --agent
```

Every inventoried host with a status; deploy_stalled rows (a newer certificate from the same CA logged but not served) come first.

### Check one client's renewals

```bash
domain-cert-tracker-cli renewals --client "Acme Dental" --agent
```

Only that client's hosts: stalled deployments first, then hosts whose renewal is overdue.

## Usage

Run `domain-cert-tracker-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `DOMAIN_CERT_TRACKER_CONFIG_DIR`, `DOMAIN_CERT_TRACKER_DATA_DIR`, `DOMAIN_CERT_TRACKER_STATE_DIR`, or `DOMAIN_CERT_TRACKER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `DOMAIN_CERT_TRACKER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export DOMAIN_CERT_TRACKER_HOME=/srv/domain-cert-tracker
domain-cert-tracker-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DOMAIN_CERT_TRACKER_HOME = "C:\ProgramData\domain-cert-tracker"
domain-cert-tracker-cli doctor
```

Under `DOMAIN_CERT_TRACKER_HOME=/srv/domain-cert-tracker`, the four dirs resolve to `/srv/domain-cert-tracker/config`, `/srv/domain-cert-tracker/data`, `/srv/domain-cert-tracker/state`, and `/srv/domain-cert-tracker/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:DOMAIN_CERT_TRACKER_HOME = "C:\srv\domain-cert-tracker"
domain-cert-tracker-cli doctor
```

Under `$env:DOMAIN_CERT_TRACKER_HOME = "C:\srv\domain-cert-tracker"`, the four dirs resolve to `C:\srv\domain-cert-tracker\config`, `C:\srv\domain-cert-tracker\data`, `C:\srv\domain-cert-tracker\state`, and `C:\srv\domain-cert-tracker\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "domain-cert-tracker": {
      "command": "domain-cert-tracker-mcp",
      "env": {
        "DOMAIN_CERT_TRACKER_HOME": "/srv/domain-cert-tracker"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `DOMAIN_CERT_TRACKER_DATA_DIR` overrides an explicit `--home` for that kind. Use `DOMAIN_CERT_TRACKER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `DOMAIN_CERT_TRACKER_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `domain-cert-tracker-cli doctor --fail-on warn` to check path warnings in automation.

### Local data

Sweep results and the client inventory live in the SQLite store (`data.db`) in the data directory. Some single lookups are saved there too: a response with an id or name the store can key it by (`ip` and `autnum`, for example) is stored whole, while `domain` and `nameserver` lookups are not. `--data-source local` finds a saved lookup only by that stored key (`autnum GOOGLE`, not `autnum 15169`), so read sweep results with `report`, `renewals`, `issuers audit`, `hosts discover` and `events list`. GET responses are cached for five minutes under `http/` in the cache directory. The learn journal lives under `learn/` in the state directory. `domain-cert-tracker-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `domain-cert-tracker-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config is kept. The store also holds the client inventory and sweep history, so use `--only cache` to clear only the response cache.
- **Run without a local copy:** set `DOMAIN_CERT_TRACKER_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new from an API is written to the store, and `sync`, `sweep` and `workflow archive` refuse to run (the client inventory commands still work). Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `DOMAIN_CERT_TRACKER_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### autnum

Look up autonomous system numbers over RDAP

- **`domain-cert-tracker-cli autnum <asn>`** - Look up who holds an autonomous system number over RDAP

### bootstrap

Read the IANA RDAP bootstrap registry

- **`domain-cert-tracker-cli bootstrap`** - Read the IANA list of which RDAP server answers for each top-level domain

### certificates

Search Certificate Transparency for certificates naming a domain

- **`domain-cert-tracker-cli certificates --domain <domain>`** - Search the Certificate Transparency log index for every certificate naming a domain

### domain

Look up domain registrations over RDAP

- **`domain-cert-tracker-cli domain <name>`** - Look up a domain's registrar, expiry, lock statuses, nameservers and DNSSEC over RDAP

### ip

Look up IP address networks over RDAP

- **`domain-cert-tracker-cli ip <address>`** - Look up which network and registry hold an IP address over RDAP

### nameserver

Look up nameserver host objects over RDAP

- **`domain-cert-tracker-cli nameserver <name>`** - Look up a nameserver host object and its addresses over RDAP


### clients


- **`domain-cert-tracker-cli clients list`** - Keep each client's domains and hosts in one local inventory that every sweep and report walks.


### dns


- **`domain-cert-tracker-cli dns check <domain>`** - Get NS, MX, SPF, DMARC, CAA and DNSSEC for a domain as one JSON document through the system resolver.


### events


- **`domain-cert-tracker-cli events list`** - Read typed events from each sweep diff (domain.expiring, cert.expiring, ct.new_cert, dns.changed, registrar.unlocked) after a cursor.


### hosts


- **`domain-cert-tracker-cli hosts discover`** - List hostnames that have certificates in CT for a client's domains but are not in that client's inventory.


### issuers


- **`domain-cert-tracker-cli issuers audit`** - Separate certificates from issuers a domain's CAA does not permit, or a first-ever issuer, from routine renewals.


### renewals


- **`domain-cert-tracker-cli renewals`** - Find hosts still serving an old certificate after a newer one was logged, or past their renewal point with no successor in CT.


### report


- **`domain-cert-tracker-cli report`** - See, per client, what expires within N days, which registrations are not transfer-locked, new CT certificates and DNS drift, plus what the last sweep could not check.


### sweep


- **`domain-cert-tracker-cli sweep`** - Check every inventoried domain's registration, new CT certificates and DNS, and every inventoried host's live TLS certificate, in one run; then store the results and diff them against the last run.


### tls


- **`domain-cert-tracker-cli tls probe <host[:port]>`** - Handshake with a live host and get its chain, expiry, SANs, key, issuer, hostname match and OCSP must-staple as JSON.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`domain-cert-tracker-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`domain-cert-tracker-cli teach --query <q> --resource <id> --resource-type <type>`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`domain-cert-tracker-cli learnings list`** - Inspect taught rows
- **`domain-cert-tracker-cli learnings forget <query>`** - Undo a teach
- **`domain-cert-tracker-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`domain-cert-tracker-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`domain-cert-tracker-cli teach-pattern --query-template <t> --resource-template <t> --resource-type <type> --entity-kind <kind>`** - Install a query/resource template up front
- **`domain-cert-tracker-cli teach-lookup --kind <kind> --canonical <name> --value <alias>`** - Add an entity mapping (e.g. a CA short name such as LE for Let's Encrypt) for pattern substitution

Pass `--no-learn` or set `DOMAIN_CERT_TRACKER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `domain-cert-tracker-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
domain-cert-tracker-cli autnum 15169

# JSON for scripting and agents
domain-cert-tracker-cli autnum 15169 --json
# Filter to specific fields
domain-cert-tracker-cli autnum 15169 --json --select country,endAutnum,entities

# Dry run  -  show the request without sending
domain-cert-tracker-cli autnum 15169 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
domain-cert-tracker-cli autnum 15169 --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources; it writes only its local inventory and store; it writes only its local inventory and store
- **Offline-friendly** - report, renewals, issuers audit, hosts discover and events list read the local store that sweep fills
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` a check or command failed (every sweep check failed, a DNS lookup failed, a TLS handshake failed), `2` usage error, `3` not found, `5` API error, `7` rate limited or blocked, `10` config error.

## Health Check

```bash
domain-cert-tracker-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `domain-cert-tracker-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/domain-cert-tracker-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\domain-cert-tracker-cli\config.toml` on Windows; `--home`, `DOMAIN_CERT_TRACKER_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `DOMAIN_CERT_TRACKER_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `DOMAIN_CERT_TRACKER_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `domain-cert-tracker-cli clients list` to see the inventoried clients, domains and hosts

### API-specific
- **crt.sh answers 502 or 503**  -  crt.sh is a free shared service; rerun sweep later. Until a CT read succeeds the domain is listed under unverified in report (kind ct) and in sweep's fetch_failures, and its stored CT history does not move, so nothing is missed.
- **RDAP answers 429 Too Many Requests**  -  sweep routes each domain to its registry with the IANA bootstrap file; for single lookups keep --rate-limit at 1 or lower.
- **refused: <address> is not a public address (or: <host> resolves to a non-public address)**  -  By design TLS probes and HTTP lookups never connect to loopback, private, link-local, CGNAT or metadata addresses. Check that the host resolves only to public addresses.
- **refused: port <N> is not allowed (only 443)**  -  Pass --allow-port <N> to tls probe or sweep on the command line; MCP calls cannot open other ports.
- **rdap_unavailable for a ccTLD**  -  That registry publishes no https RDAP server; check expiry in the registrar's portal and keep the domain in the inventory for TLS, CT and DNS.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**openrdap rdap**](https://github.com/openrdap/rdap)  -  Go
- [**Domain Monitor MCP Server**](https://github.com/ipythoning/domain-monitor-mcp-server)  -  JavaScript
- [**checkdmarc**](https://github.com/domainaware/checkdmarc)  -  Python
- [**crt-mcp**](https://github.com/Randark-JMT/crt-mcp)  -  TypeScript
- [**ssl-watch**](https://github.com/idesyatov/ssl-watch)  -  Go
- [**crt.sh (certwatch_db)**](https://github.com/crtsh/certwatch_db)  -  PLpgSQL

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
