# sonicwall skill - governance and safety model

> Unofficial. Community-built skill for the SonicWall APIs. Not affiliated with,
> endorsed by, or sponsored by SonicWall Inc.
> This page tells an MSP owner exactly what the sonicwall skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `sonicwall-cli` binary (and `sonicwall-mcp`). It talks to two
SonicWall services with one credential:

- **MySonicWall** (`https://api.mysonicwall.com`). A MySonicWall API key,
  `SONICWALL_MSW_API_KEY` (or `msw_api_key` in the config file), sent as `X-API-KEY`.
  Licence checks (`licenses renewals`, the licence column of `posture`),
  `blind-spots` and the `msw-*` commands read MySonicWall with it.
- **Network Security Manager (NSM SaaS)** (`https://nsm-<region>.sonicwall.com/api/manager`).
  An NSM session token, sent as `Authorization: Bearer`. `sonicwall-cli nsm-login`
  mints it from the MySonicWall API key through SonicWall's documented exchange
  (list cloud tenants, request an access code, trade it at NSM `/auth/sso`) and saves
  it; it is never printed. The token lasts 8 hours with a 15-minute idle timeout, so
  run `nsm-login` again whenever NSM commands exit 4. `SONICWALL_NSM_TOKEN` overrides
  the saved token; leave it unset, because a pasted token expires and keeps
  overriding the saved one. `SONICWALL_REGION` picks the NSM host (`uswest` by
  default, `eucentral` for EU portals). The firewalls themselves are read through
  NSM's SonicOS passthrough, so no firewall credential is needed and the CLI never
  connects to a firewall directly.

Credentials are never logged and never sent anywhere except the SonicWall service
they belong to. **Two commands write a credential to disk:** `nsm-login` saves the
NSM session token to `credentials.toml` in the CLI's data directory, and `auth set-token` (token on stdin)
saves a token you paste. Run `sonicwall-cli doctor` for the paths.

## Default-safe behavior

- **It is read-only against SonicWall.** No command changes a firewall, NSM or
  MySonicWall configuration. Every NSM and SonicOS read is a GET. The only POSTs are
  read-style MySonicWall queries (`msw-licenses`, `msw-products list`,
  `msw-users groups`, `msw-activity export`) and the token exchanges (`nsm-login`,
  which POSTs to NSM `/auth/sso`, `nsm-session refresh` and `sso`, and
  `msw-tenants access-code`). `import` can only
  target those two token calls; a refresh can invalidate the session token you are
  using.
- **Secret fields are blanked before anything sees them.** SonicWall's own API
  schemas document secret fields on some reads: a password per local user,
  `assign_token`, `authentication_code` and `registration_code` in system status,
  SNMP community names, a device password and `authCode` in NSM's device details,
  and in MySonicWall an `api_key` per firewall, `tcpassword` in service info,
  `authCode` in the product list and `activationKey` in the activity log. The
  client blanks every string field those schemas name as a secret, and the vendor's
  other spellings of registration, authentication and assistance codes and session
  IDs, to `[redacted]` in every
  NSM, SonicOS and MySonicWall response before it is printed, cached, synced into
  the local mirror or returned to an agent, for the CLI and the MCP server alike.
  The token exchanges are the exception: returning a token is their job. Every
  route this connector reads answers JSON, so a response that is not one JSON
  document (text, HTML, a file download such as an NSM tech-support report or
  settings export, binary, or concatenated values) has its whole body withheld (a
  success becomes HTTP 422; an error keeps its status), and so does a response that
  is a bare JSON string naming a credential at all. Secret names match in any
  spelling (`pop3_password`, `pop3-password`, `Pop3Password`, full-width letters)
  on every route, except the generic SonicOS words `key`, `new` and `old`, which
  match in any spelling on SonicOS passthrough reads only; the value is blanked
  whatever its type, and booleans are kept.
  The settings objects SonicOS files under secret names (the
  `administration.password` password policy) keep a child only at a path, and of a
  type, the SonicOS schema documents for a policy field; every other child is
  dropped, key and value. A string that assigns a value to a credential name
  (`authentication_code: ...`, `password=...`, `Temp pw: ...`, also percent-, HTML-
  or backslash-escaped) is blanked whole, and a key that does so is dropped together
  with its value. The separator is `:`, `=` or other separator punctuation: a colon
  or equals lookalike, a bullet, an ellipsis or an arrow, or any character newer
  than the CLI's Unicode tables, such as a recent emoji (dashes, quotes and
  brackets do not count). A name that only contains a secret word (a group called
  `Token Users` or `Hardware Token - Pilot`, an `auth_type` of `password`) is kept;
  separator punctuation right after the word (a bullet, an ellipsis or a recent
  emoji) blanks it.
  posture and mfa-audit never match users or groups on a blanked or repeated name:
  that firewall's account checks read unknown instead. `nsm-get` also redacts any value under a credential-shaped key.
- **`nsm-get` is fenced.** It only sends GETs to routes SonicWall documents in its
  NSM and SonicOS specifications, refuses routes that return credentials and routes
  whose vendor description reads as a state change (reset, sync, export and similar),
  and never caches. That filter is a keyword list, so the MCP server does not mark
  `nsm_get` read-only.
- **`config-audit` prints configuration text as NSM returns it.** Configuration
  diffs and listings cannot be redacted field by field: one that assigns a value to
  a credential name the same way is blanked whole, and anything else is printed as
  NSM returns it, so treat that output as sensitive.
- **`--dry-run` previews API calls only.** It does NOT guard the local writes:
  `auth set-token --dry-run` still saves the token, and `profile save`, the `teach*`
  commands and `learnings forget` still write local files.
- **Unknown is never pass.** `posture`, `mfa-audit`, `firmware drift` and
  `licenses renewals` mark anything they could not read as `unknown` (or list it in
  `fetch_failures`). Those commands only see firewalls onboarded to NSM SaaS;
  `blind-spots` lists the MySonicWall-registered firewalls NSM cannot see.
- **MCP read-only mode.** `SONICWALL_MCP_READ_ONLY=1` makes the MCP server hide tools
  that could change NSM, MySonicWall or firewall state and refuse non-GET execute
  calls. The tools it keeps still write local state: the response cache, `posture`
  snapshots and learning entries.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no
  gating of its own. See AGENTS.md.

## Permission tiers

Nothing here changes a firewall, so the safe default for an autonomous agent is
**the Read tier**; require a human for the tiers below it, which handle tokens or
move data.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, audits, inventory. No change to any device, NSM or MySonicWall; schema-documented secrets blanked. Local writes only: `posture` saves each run, responses are cached, and `export --output` or `--deliver file:<path>` overwrites the file you name. | `posture`, `posture changes`, `exposure`, `mfa-audit`, `firmware drift`, `licenses renewals`, `blind-spots`; every list / get / count under `devices`, `device-groups`, `firmware`, `licenses`, `vulnerabilities`, `tenants`, `alerts` and `nsm-system`; `config-audit count`; the `firewall` reads other than the four in the next row; `msw-products`, `msw-licenses`, `msw-system` and the `msw-tenants` reads other than `access-code`; `search` over the local store, and `export` of any resource other than `config_audit` and `msw_users` | Allow |
| **Raw read (escape hatch)** | GET of any vendor-documented NSM or SonicOS read route without a dedicated command. | `nsm-get` (credential and state-changing routes refused by a keyword list; credential-shaped values redacted) | Allow; review unfamiliar paths |
| **Sensitive read** | Reads data an attacker would want. | `config-audit list` and `config-audit diff` (configuration text as NSM returns it, not redacted field by field); `firewall local-users`, `firewall local-groups`, `firewall user-auth`, `firewall sslvpn-sessions`, `msw-users` and `msw-activity` (account inventory, who is connected and account activity; schema-documented secrets blanked); `export config_audit` and `export msw_users`, which write the same data to a file; `sync` and `workflow archive`, which copy config-audit configuration text and firewall local users and groups (all in the default set) into the local store, where `search` and the MCP `sql` and `search` tools can read it; `tail` of `config_audit` or `msw_users`, which prints the same data as it changes | Human-in-the-loop; keep the output and the local store private |
| **Credential** | Mints, saves or clears a token. | `nsm-login`, `nsm-session refresh`, `nsm-session sso`, `msw-tenants access-code` (returns an NSM access code), `import`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Data egress** | Sends data to a destination you choose. | `--deliver webhook:<url>` on any command (POSTs that command's output to the URL), `feedback --send`, and bare `feedback` when `SONICWALL_FEEDBACK_ENDPOINT` is set and `SONICWALL_FEEDBACK_AUTO_SEND=true` (with no endpoint set, `feedback` only writes a local file) | Human-in-the-loop; a webhook sink moves client data off your machine |
| **MCP execute** | Calls any endpoint of the curated spec by ID. | The MCP `sonicwall_execute` tool: each call falls in the tiers above (`config_audit.list` is a Sensitive read). Its POSTs are the token exchanges (Credential tier) plus four read-style MySonicWall queries (`msw-activity export`, `msw-licenses`, `msw-products list`, `msw-users groups`), and every POST is refused while `SONICWALL_MCP_READ_ONLY=1` | Human-in-the-loop unless `SONICWALL_MCP_READ_ONLY=1` |
| **Write / Destructive / Admin** | Changes a device, NSM or MySonicWall. | (none - no command changes a firewall, NSM or MySonicWall configuration) | n/a |

## How to lock it down

- **Use a MySonicWall account whose NSM role is read-only** for the API key. NSM then
  refuses any write regardless of what a client sends.
- **Keep the token files private.** `nsm-login` and `auth set-token` save tokens in
  `credentials.toml` in the CLI's data directory; do not sync it to a shared drive.
- **Keep `config-audit` output private.** Configuration text is printed unredacted.
- **Never let an agent run Credential or Data-egress tier commands unattended.**
- **Rotate the MySonicWall API key if it is ever exposed** (for example after bridging
  the MCP server to a public endpoint for ChatGPT - see mcp-install.md; the HTTP
  transport refuses to start until `SONICWALL_MCP_HTTP_TOKEN` is set).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the MySonicWall API key, the binary uses it
against your own MySonicWall account and NSM tenant, and you can read every line of
how it does so.
