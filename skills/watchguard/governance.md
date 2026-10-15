# watchguard skill - governance and safety model

> Unofficial. Community-built skill for the WatchGuard Cloud APIs. Not affiliated with,
> endorsed by, or sponsored by WatchGuard Technologies, Inc.
> This page tells an MSP owner exactly what the watchguard skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `watchguard-cli` binary (and `watchguard-mcp`). It authenticates
the way WatchGuard's API documentation describes: an OAuth2 client-credentials token
minted from a WatchGuard Cloud **Access ID** (`WATCHGUARD_CLIENT_ID`) and its password
(`WATCHGUARD_CLIENT_SECRET`), plus the account's API key (`WATCHGUARD_API_KEY`), sent
as the `WatchGuard-API-Key` header on every request. `WATCHGUARD_ACCOUNT_ID` is the
account the `{accountId}` in each request path defaults to; `--account-id` overrides it.

WatchGuard Cloud issues two Access IDs when you enable API access: a **read-write** one
and a **read-only** one. That choice is the outer limit on anything an agent can do. A
reporting agent should get the read-only Access ID.

Where credentials live:

- Read from the environment (or the agent's MCP config) on every run.
- The token minted for your own account is cached in `credentials.toml` under the data
  directory (mode 0600). `auth login --client-id ... --client-secret ...` also saves that
  Access ID and password there. Removing the environment variables does not remove them;
  run `watchguard-cli auth logout`, or disable the Access ID in WatchGuard Cloud.
- The cached token is reused only by a process with the same Access ID, scope and
  token URL. A process configured with a different Access ID (for example the
  read-only one) mints its own token, and stops with an error rather
  than borrow the cached token when it has no password to mint with.
- Per-customer tokens (one per managed account, minted with that account's audience)
  stay in memory and are never written to disk.
- `WATCHGUARD_AUDIENCE` makes every request use one managed account's token. It is a
  convenience for single-customer work, not a security boundary: WatchGuard's
  documentation says an audience token works for that account "or your own account", so a
  pinned process can still reach your Service Provider account. Do not rely on it to
  contain an agent; use the read-only Access ID for that.
- `WATCHGUARD_BASE_URL`, `WATCHGUARD_TOKEN_URL` and `WATCHGUARD_ACCOUNT_ID` set in the
  environment apply to that run only; they are never written into `config.toml`.

## Secrets the API returns

Some WatchGuard reads return live secrets inside ordinary rows:

| Command | Secret in the response |
| --- | --- |
| `firebox-bovpn-tunnels list` / `get` | BOVPN pre-shared keys (`psk`, `psk_encrypted`) |
| `firebox-certificates list` / `get` | certificate private keys and their passwords |
| `firebox-networks list` / `get` | PPPoE and Dynamic DNS passwords, Wi-Fi passphrases, DHCP auth tokens |
| `firebox-users list` / `get` | Firebox user passwords |
| `firebox-content-filtering list` / `get` | WebBlocker override passwords |
| `activations list-recent` / `status` | product activation keys |
| `endpoint-installers` | a download link for an installer that enrolls devices into the account |
| `allocations list-owned` / `list-assigned` / `summary` | Firebox serial numbers and software license keys (`serialOrLicense`) |

These commands print what the API returns, so treat their output as a credential, and
so does `export` of any of these resources. The local SQLite mirror (`sync`, and every
live read that writes through to it) drops these fields before anything reaches disk,
and the HTTP response cache never stores a response that carries one, any
`endpoint-installers` response, or any license allocation response. The one exception
is `serialOrLicense` in allocation rows: the same field holds a Firebox serial number or
a software license key, and the local mirror keeps it because it is what tells two
otherwise identical allocation rows apart; delete the data directory if that matters
to you.

Through the MCP server these reads return their secrets to the agent, and so to your AI
provider, like any other result: `WATCHGUARD_MCP_READ_ONLY=1` does not block them,
because they are GET calls. Only the read-only Access ID plus not asking for them keeps
them out of a conversation.

## Default-safe behavior

- **Writes send immediately.** No write command asks for confirmation. Preview any
  write with `--dry-run` first: it prints the request and sends nothing, does not
  request a login token, and changes nothing on disk (`auth login`, `auth logout` and
  `feedback` print what they would do; `jobs prune`, `profile save` and `profile delete`
  refuse `--dry-run`, as do `export --output` and `--deliver webhook:` with it).
- **`accounts delete` with no flags targets your own account.** Its path takes
  `--account-id`, which defaults to `WATCHGUARD_ACCOUNT_ID`. Always pass the managed
  account's ID explicitly.
- **Redirects never carry credentials or write bodies.** A cross-origin redirect drops
  the Authorization and `WatchGuard-API-Key` headers, and a write is never replayed to
  another origin. `--deliver webhook:<url>` never follows a redirect.
- **Error output never echoes a secret you sent.** An API error that repeats a password,
  pre-shared key, the API key or the token is withheld instead of printed.
- **Read commands only read.** The cross-customer reports (`accounts health`,
  `endpoint-devices posture`, `endpoint-patches exposure`, `threatsync-incidents queue`,
  `allocations reconcile`), the fleet-history views and `firebox-transactions wait`
  never change anything in WatchGuard Cloud.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the first row.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, sync. No change in WatchGuard Cloud. | `accounts health`, `endpoint-devices posture`, `endpoint-devices list`, `endpoint-devices protection-status`, `threatsync-incidents queue`, `endpoint-patches exposure`, `allocations reconcile`, `firebox-devices`, `firebox-devices changes`, `firebox-devices firmware-lag`, `firebox-transactions wait`, `firebox-policies list`, `endpoint-security-events`, `ndr-assets list`, `operators list`, `accounts list-managed`, `sync`, `search`, `analytics`, `tail`, and every other list / get command not in the Credential row | Allow |
| **Endpoint and incident actions** | Acts on customer computers, users' phones or open incidents. | `endpoint-devices isolate`, `endpoint-devices stop-isolation`, `endpoint-devices scan`, `endpoint-devices action`, `endpoint-devices uninstall`, `endpoint-configurations assign`, `threatsync-actions perform`, `threatsync-incident-actions perform`, `threatsync-transactions`, `threatsync-incidents set-status`, `ndr-smart-alerts close`, `authpoint push` | Human-in-the-loop, explicit confirmation |
| **Firewall config and deployment** | Changes Firebox, FireCloud, Endpoint Security or account configuration, or pushes it to devices. | `firebox-deployments`, `firebox-policies create` / `update` / `set-enabled`, `firebox-blocked-sites`, `firebox-botnet-exceptions`, `firebox-file-exceptions`, `firebox-geo-exceptions`, `firebox-ips-exceptions` and `firebox-webblocker-exceptions` create / update, the same `firecloud-*` creates and updates (plus `firecloud-https-exceptions`), `firebox-exceptions enable-global` / `disable-global`, `firebox-templates subscribe`, `firebox-bovpn-p1-settings create` / `update`, `firebox-transactions update`, `endpoint-risks update-config`, `ndr-assets create` / `update`, `threatsync-comments create` / `update`, `allocations allocate` / `update-hardware` / `update-software`, `accounts create` / `update`, `import` | Preview with `--dry-run`, then a human-approved write |
| **Destructive** | Irreversible removal. | `accounts delete`, `operators delete`, `allocations deallocate`, `firebox-transactions cancel`, `firebox-policies delete`, `firebox-bovpn-tunnels delete` / `delete-by-name`, `firebox-certificates delete`, every Firebox and FireCloud exception `delete`, `ndr-assets delete`, `threatsync-comments delete` | Human-in-the-loop only, explicit confirmation |
| **Credential and admin** | Returns or sets secrets, creates operators, or touches MFA. | Every read in the table above, `export` of those resources, `firebox-bovpn-tunnels create` / `update` / `update-by-name` / `patch` / `patch-by-name`, `firebox-certificates create` / `install`, `activations activate`, `operators create` / `update`, `audiences`, `authpoint check-policy` / `password` / `otp` / `qrcode` / `forgot-token` / `transaction-status`, `auth login`, `auth set-token` | Human-in-the-loop only |

## How to lock it down

- **Give a reporting agent the read-only Access ID.** WatchGuard refuses writes made
  with its token, whatever the agent tries.
- **Run the MCP server read-only.** `WATCHGUARD_MCP_READ_ONLY=1` drops the MCP server's
  write tools and makes `watchguard_execute` refuse every non-GET call. GET calls still
  run, including the credential reads above, so pair it with the read-only Access ID.
  The cross-customer report tools (`accounts health`, `endpoint-devices posture`,
  `endpoint-patches exposure`) still send the audience lookup (a POST that only returns
  a customer's audience value) and mint per-customer tokens in read-only mode.
- **There is no preview through the MCP server.** `--dry-run` is a CLI flag;
  `watchguard_execute` sends a write as soon as it is called, whatever its arguments say.
  An agent that should only plan writes needs `WATCHGUARD_MCP_READ_ONLY=1`.
  `tail` and `import` are not MCP tools, and MCP tools never take a local file path
  for the CLI to read.
- **Keep autonomous agents to Read plus previewed writes.** The gate lives in your
  agent's policy, not in the binary's defaults.
- **Rotate the Access ID password and API key if they are ever exposed**, for example
  after bridging the MCP server to a public endpoint for ChatGPT (see mcp-install.md).
  The HTTP transport refuses to start without `WATCHGUARD_MCP_HTTP_TOKEN`.

`PRINTING_PRESS_CLIENT_PROFILE` and `--client-profile` are not enforced by this
connector: WatchGuard has no tenant-gated client profiles here, so a value does not
limit which customer accounts a call can reach. The Access ID you give it is the only
hard limit.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the WatchGuard API, and you can read every line of how it does so. The skill is
read-first and scoped to the accounts your credential can reach.
