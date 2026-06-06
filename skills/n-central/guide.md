# N-central CLI

**Every N-central REST endpoint, plus an offline SQLite mirror of your whole org tree, cross-tenant search, and a JWT-expiry guardian no other N-central tool has.**

A single cross-platform binary for N-able N-central. It mirrors service orgs, customers, sites, devices, active issues, and custom properties into local SQLite so you can search and SQL across every tenant offline. It matches the 87-tool community MCP server's REST coverage and beats it with `fanout` cross-server search, `triage` issue rollups, `props audit` custom-property coverage, and a `guardian` that kills the silent JWT/password-expiry outage.

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

For the short install path see [README.md](./README.md). This file is the command reference.

## Authentication

Authentication uses an N-central API-only user's JSON Web Token. Generate it in N-central (Administration → User Management → Users → API Access → Generate JSON Web Token; MFA must be OFF). Set NCENTRAL_JWT and your tenant base URL N_CENTRAL_BASE_URL (e.g. https://yourmsp.ncod.n-able.com/api). The CLI exchanges the long-lived JWT for a short-lived access token via POST /api/auth/authenticate and auto-refreshes it. Watch the API user's password expiry (default 90 days)  -  it silently invalidates the JWT; `guardian` warns you before it does.

## Quick Start

```bash
# confirm the JWT is valid and the password is not about to expire before anything else
n-central-cli guardian


# mirror the org tree, devices, and active issues into local SQLite
n-central-cli sync


# see today's active issues grouped and ranked across customers
n-central-cli triage --by customer


# locate a device by name across every tenant
n-central-cli whereis DC01

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-tenant intelligence

- **`fanout`**  -  Search every configured N-central server at once  -  find a device, customer, or active issue across all your tenants without clicking through each console.

  _Reach for this when an MSP runs more than one N-central server and needs one answer across all of them._

  ```bash
  n-central-cli fanout "backup failed" --agent
  ```
- **`triage`**  -  Group active monitoring issues by customer, device, or monitor type and rank by severity  -  the daily NOC sweep as one command.

  _Reach for this to start a shift with one ranked view instead of per-customer Active Issues screens._

  ```bash
  n-central-cli triage --by customer --agent
  ```
- **`whereis`**  -  Given a device name fragment, return its full path  -  server, service org, customer, site  -  plus its current active-issue count.

  _Reach for this when a ticket names a device but not which customer or server it lives on._

  ```bash
  n-central-cli whereis DC01 --agent
  ```

### Local state that compounds

- **`props audit`**  -  Report which devices are missing a required custom-property value, as a coverage percentage grouped by customer.

  _Reach for this when custom properties drive automation and you need coverage, not a manual CSV spot-check._

  ```bash
  n-central-cli props audit --required BackupTier --agent
  ```
- **`maint coverage`**  -  List devices and sites with no maintenance window before a reboot/patch wave, so nothing reboots in business hours.

  _Reach for this before a patch wave to find the blind spots the per-device view can't show._

  ```bash
  n-central-cli maint coverage --before 2026-06-15 --agent
  ```

### Reachability mitigation

- **`guardian`**  -  Validate the access token, warn when the API user's password (and thus the JWT) is about to expire, and detect N-central's HTTP-200-with-error-body failures.

  _Reach for this in CI or a cron check so a silent JWT expiry never takes your integrations down at day 90._

  ```bash
  n-central-cli guardian --password-set 2026-03-01
  ```

## Recipes


### Morning NOC sweep across customers

```bash
n-central-cli triage --by customer --agent
```

One ranked, grouped view of every active issue instead of per-customer console screens.

### Find a device anywhere

```bash
n-central-cli whereis DC01 --agent
```

Resolves a device fragment to server → SO → customer → site with its live issue count.

### Audit a required custom property

```bash
n-central-cli props audit --required BackupTier --agent --select customerName,coveragePct
```

Per-customer coverage of a property that drives automation, narrowed to the two fields that matter.

### Pre-patch maintenance-window check

```bash
n-central-cli maint coverage --before 2026-06-15 --agent
```

Lists devices/sites with no window before the reboot wave.

### Guard against silent JWT expiry in CI

```bash
n-central-cli guardian --password-set 2026-03-01 --agent
```

Exits non-zero when the token is invalid or the password is near expiry  -  wire it into a cron/CI check.

## Usage

Run `n-central-cli --help` for the full command reference and flag list.

## Commands

### access-groups

Access groups (device-type and org-unit-type).

- **`n-central-cli access-groups <accessGroupId>`** - Retrieve detailed information for an access group by ID.

### customers

Customers (client organizations) in N-central.

- **`n-central-cli customers get`** - Retrieve a single customer by ID.
- **`n-central-cli customers list`** - List all customers across the instance.
- **`n-central-cli customers registration-token`** - Retrieve the agent registration token for a customer (used to enroll new devices).

### device-filters

Saved device filters (reusable as filterId on device list calls).

- **`n-central-cli device-filters`** - List saved device filters for the API user.

### devices

Devices monitored by N-central (workstations, servers, network devices, probes).

- **`n-central-cli devices assets`** - Retrieve hardware/software asset inventory for a device.
- **`n-central-cli devices get`** - Retrieve a single device by ID.
- **`n-central-cli devices list`** - List all devices visible to the API user, across the org tree.
- **`n-central-cli devices maintenance`** - List patch maintenance windows configured for a device.
- **`n-central-cli devices properties`** - List custom property values for a device (the backbone of MSP automation/documentation).
- **`n-central-cli devices status`** - Retrieve the service-monitoring status (active issues / health) for a device.
- **`n-central-cli devices tasks`** - List scheduled/automation tasks targeting this device.

### org-units

Organization units  -  the unified tree of service orgs, customers, and sites.

- **`n-central-cli org-units access-groups`** - List access groups for an org unit.
- **`n-central-cli org-units active-issues`** - Fetch active monitoring issues for an org unit (the daily NOC triage feed).
- **`n-central-cli org-units children`** - List the direct children of an org unit.
- **`n-central-cli org-units devices`** - List devices scoped to a specific org unit.
- **`n-central-cli org-units get`** - Retrieve a single org unit by ID.
- **`n-central-cli org-units job-statuses`** - Fetch job statuses for an org unit.
- **`n-central-cli org-units list`** - List all organization units (SO, customer, and site nodes).
- **`n-central-cli org-units registration-token`** - Retrieve the agent registration token for an org unit.
- **`n-central-cli org-units user-roles`** - List user roles defined for an org unit.

### scheduled-tasks

Scheduled tasks  -  run scripts/automation policies on devices and track them.

- **`n-central-cli scheduled-tasks get`** - Retrieve general information for a scheduled task.
- **`n-central-cli scheduled-tasks run`** - Create a direct-support scheduled task (run an Automation Policy, Script, or MacScript on a device).
- **`n-central-cli scheduled-tasks status`** - Retrieve aggregated status for a scheduled task.

### server

Server info and health.

- **`n-central-cli server health`** - Return the start and current time of the server (lightweight reachability check).
- **`n-central-cli server get`** - Return version information for the N-central API service and systems.

### service-orgs

Service Organizations  -  the top level of the N-central org tree.

- **`n-central-cli service-orgs customers`** - List all customers under a service organization.
- **`n-central-cli service-orgs get`** - Retrieve a single service organization by ID.
- **`n-central-cli service-orgs list`** - List all service organizations.

### sites

Sites  -  the leaf org-unit level under customers.

- **`n-central-cli sites get`** - Retrieve a single site by ID.
- **`n-central-cli sites list`** - List all sites across the instance.

### users

N-central users.

- **`n-central-cli users`** - List N-central users.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
n-central-cli access-groups 550e8400-e29b-41d4-a716-446655440000

# JSON for scripting and agents
n-central-cli access-groups 550e8400-e29b-41d4-a716-446655440000 --json

# Filter to specific fields
n-central-cli access-groups 550e8400-e29b-41d4-a716-446655440000 --json --select id,name,status

# Dry run  -  show the request without sending
n-central-cli access-groups 550e8400-e29b-41d4-a716-446655440000 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
n-central-cli access-groups 550e8400-e29b-41d4-a716-446655440000 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
n-central-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/n-central-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NCENTRAL_JWT` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `n-central-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `n-central-cli doctor` to check credentials
- Verify the environment variable is set: `echo $NCENTRAL_JWT`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **Every call returns HTTP 500 INTERNAL ERROR**  -  The API user's password likely expired (default 90 days), which invalidates the JWT. Reset the password and regenerate the JWT; run `guardian --password-set <date>` to track it.
- **403 Forbidden on a script or repository item**  -  That item is not API-enabled. In N-central, enable the item for the API (scripts must be ID >= 2000 with 'Enable API' toggled).
- **A call returns HTTP 200 but nothing happened**  -  N-central can return failures as an Error Message inside a 200 body. Run `guardian` or re-check the response; the CLI surfaces these as errors.
- **429 Too Many Requests on a fan-out**  -  N-central enforces per-endpoint concurrency caps (some as low as 1). The CLI throttles per endpoint automatically; lower --concurrency if you still hit it.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PS-NCentral**](https://github.com/ToschAutomatisering/PS-NCentral)  -  PowerShell (42 stars)
- [**NC-API-Documentation**](https://github.com/AngryProgrammerInside/NC-API-Documentation)  -  PowerShell (20 stars)
- [**pyncentral**](https://github.com/RenierM26/pyncentral)  -  Python (6 stars)
- [**NCRestAPI**](https://github.com/theonlytruebigmac/NCRestAPI)  -  PowerShell (4 stars)
- [**N-central_MCP**](https://github.com/theonlytruebigmac/N-central_MCP)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
