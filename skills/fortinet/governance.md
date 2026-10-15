# fortinet skill - governance and safety model

> Unofficial. Community-built skill for the Fortinet APIs. Not affiliated with,
> endorsed by, or sponsored by Fortinet, Inc.
> This page tells an MSP owner exactly what the fortinet skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `fortinet-cli` binary (and `fortinet-mcp`). It talks to up to
three Fortinet services, each with its own credential:

- **FortiGate (FortiOS REST API).** A REST API administrator token per FortiGate,
  sent as `Authorization: Bearer`. One device: `FORTINET_BASE_URL`
  (`https://<fortigate>[:port]/api/v2`) plus `FORTINET_API_TOKEN`. A fleet: a
  `devices.toml` file (`FORTINET_DEVICES` or `--devices`) where each `[[device]]`
  names a `token_file`, a file holding that box's token (on macOS and Linux the
  CLI refuses it unless it is chmod 600; on Windows, protect it with NTFS
  permissions). Token values never go in the devices file, and a `token_env`
  entry is rejected rather than read. A devices file you name that is missing or
  lists no `[[device]]` entries is an error, never a silent fall-back to one box.
- **FortiManager.** `FORTINET_FMG_URL` plus `FORTINET_FMG_API_KEY`, the key of a
  FortiManager REST API admin (FortiManager 7.2.2 or later).
- **FortiGate Cloud.** A FortiCloud IAM API user: `FORTINET_CLOUD_API_USER`,
  `FORTINET_CLOUD_API_PASSWORD`, `FORTINET_CLOUD_REGION` and, with FortiCloud
  Organizations, `FORTINET_CLOUD_ACCOUNT_ID`. The access token is cached in a
  chmod 600 file (a cache other users can read is ignored and replaced) and the CLI
  logs in at most once per run, because repeated failed logins lock the FortiCloud
  account for 15-30 minutes.

Credentials are never logged and never sent anywhere except the Fortinet service
they belong to. Every FortiGate URL must be `https://` (plain `http://` is refused
except to a loopback test host), because the token travels on every request.
**One option writes a credential to disk:** `fortinet-cli auth set-token` (token
on stdin) saves the FortiGate token to the CLI's credentials file
(run `fortinet-cli doctor` for the path). Exporting `FORTINET_API_TOKEN` keeps it
off the filesystem and is the recommended path for an agent.

**Certificate checks stay on by default.** A FortiGate or FortiManager still on its
factory self-signed certificate is refused until you either install a trusted
certificate (the secure fix) or opt in: `insecure = true` for one device in
`devices.toml`, `--insecure` on one command, `FORTINET_SKIP_TLS_VERIFY=true`
(FortiGates AND FortiManager), or `FORTINET_FMG_INSECURE=true` (FortiManager
only). Turning verification off exposes that device's token to anyone on the
network path. Those two variables are deliberately not install prompts, so a
single checkbox cannot turn verification off for every device; in a JSON MCP
config, add them to the server's `env` block by hand.

## Default-safe behavior

- **It is read-only against every Fortinet service.** Every FortiOS call is a GET
  against configuration tables or monitor endpoints (the `settings` and `monitor`
  groups read; they do not set). `fleet backup` asks the FortiGate for its
  configuration file, falling back to the POST form of the same backup call on
  builds that refuse the GET; neither changes device configuration. The
  FortiManager client sends only the JSON-RPC `get` method, and the FortiGate
  Cloud commands only list devices.
- **Stored secrets are redacted before anything sees them.** FortiOS returns its
  stored secrets (RADIUS secrets, LDAP bind passwords, IPsec pre-shared keys, the
  HA password, password hashes, API-user keys) as `ENC <...>` values. Every FortiOS
  client this skill builds (CLI, fleet commands and MCP server) rewrites each one
  to `ENC (redacted)` before the result is printed, cached, synced into the local
  mirror or returned to an agent, and refuses any request carrying
  `plain-text-password`. The one deliberate exception is `fleet backup`, whose
  whole purpose is to keep the configuration file.
- **`--dry-run` previews API calls only.** It does NOT guard the local writes:
  `auth set-token --dry-run` still saves the token, and `profile save`, the
  `teach*` commands and `learnings forget` still write local files. Treat those as
  unguarded and keep a human on the credential ones.
- **Unreadable is never green.** A device the audit or health commands could not
  read is UNKNOWN, never PASS, and `--fail-on fail` (or `warn`) exits 6 on any
  UNKNOWN as well, so a scheduled check cannot pass on silence.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no
  gating of its own. See AGENTS.md.

## Permission tiers

Nothing here changes a firewall, so the safe default for an autonomous agent is
**the Read tier**; require a human for the tiers below it, which move secrets or
data.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, audits, inventory. No change to any device; encrypted secrets redacted. Local writes only: `sync` fills the local store, `fleet audit` records each run's findings in its local history, and `export --output` or `--deliver file:<path>` overwrites the file you name. | `fleet audit`, `fleet health`, `fleet devices`, `fleet changes`, `audit firmware`, `audit sslvpn`, `audit admins`, `audit licenses`, `policies hygiene`, `fmg adoms` / `devices` / `status`, `cloud devices` / `device`, every `list` / `get`, every `monitor` and `settings` command (GET-only despite the name), `sync`, `search`, `export` | Allow |
| **Sensitive read** | Reads data an attacker would want. | `fleet backup` (each FortiGate's full configuration, including encrypted secrets and private keys, unredacted on purpose and written to owner-only files under the data directory or `--out-dir` on macOS and Linux); `administrators`, `api-users`, `local-users` and `monitor fortitokens` (account and token inventory, secret values redacted) | Human-in-the-loop; keep the output and backup directory private |
| **Credential** | Writes or clears a stored token. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Data egress** | Sends data to a destination you choose. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL), `feedback --send`, and bare `feedback` when `FORTINET_FEEDBACK_ENDPOINT` is set and `FORTINET_FEEDBACK_AUTO_SEND=true` (with no endpoint set, `feedback` only writes a local file) | Human-in-the-loop; a webhook sink moves client data off your machine |
| **Write / Destructive / Admin** | Changes a device. | (none - no command changes a FortiGate, FortiManager or FortiGate Cloud) | n/a |

## How to lock it down

- **Give each FortiGate token a read-only admin profile.** System > Administrators >
  Create New > REST API Admin, with a profile that has read access only, and your
  machine's IP as the only trusted host. The FortiGate then refuses any write
  regardless of what the client sends. `audit admins` needs read access to System
  settings, `fleet health` needs read access to monitor data, and `audit sslvpn`
  needs Firewall read access to resolve named SSL-VPN source addresses.
- **Use a read-only FortiManager admin** for the `fmg` commands and a FortiCloud IAM
  API user limited to FortiGate Cloud read for the `cloud` commands.
- **Keep the backup directory private.** `fleet backup` output contains encrypted
  secrets and private keys; do not sync it to a shared drive.
- **Never let an agent run Credential or Data-egress tier commands unattended.**
- **Rotate a token if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md; the HTTP transport
  refuses to start until `FORTINET_MCP_HTTP_TOKEN` is set).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credentials, the binary uses them
against your own FortiGates, FortiManager and FortiGate Cloud account, and you can
read every line of how it does so.
