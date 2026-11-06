# domain-cert-tracker skill - governance and safety model

> Published by Servosity Inc. for MSP partners.
> This page tells an MSP owner exactly what the domain-cert-tracker skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

Nothing. The skill drives the `domain-cert-tracker-cli` binary (and `domain-cert-tracker-mcp`),
and every source it reads is public: RDAP registries (through the IANA bootstrap file and
rdap.org), the crt.sh Certificate Transparency index, DNS through your operating system's
resolver, and TLS handshakes with your clients' own hosts. There is no account, API key or
token to grant for those sources. Two secrets are optional: the
`DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` that protects the MCP server's HTTP mode, and an
`auth_header` or `[headers]` value in the config file for your own private RDAP server. It
is sent only to the `base_url` origin, and with no `base_url` set that origin is the public
rdap.org, so always set `base_url` next to it.

## What leaves your machine

- A lookup sends only the domain, host, IP address or AS number being checked to the public
  service that answers for it, plus any `auth_header` or `[headers]` value you configured to
  the `base_url` origin.
- HTTP lookups and TLS probes connect only to public addresses (never loopback, private,
  link-local, CGNAT or cloud metadata ranges), and only on port 443 unless the command line
  passes `--allow-port`. MCP calls cannot open another port, cannot point a lookup at a
  non-public address, and cannot read or write a file on the host (`clients import` is
  command line only). The one exception is the origin you put in
  `DOMAIN_CERT_TRACKER_BASE_URL` (environment or MCP host config, never a tool argument):
  single RDAP lookups go to that server as configured, for example a private RDAP mirror.
- The client inventory and the sweep history stay in the local SQLite store under your user
  account.

## Default-safe behavior

- **Nothing it does can change a registration, a certificate, a DNS record or a server.** Its
  only writes are to its own local store.
- **Removing inventory previews first.** `clients remove` only shows what it would remove and
  exits 2 unless you pass `--yes` (command line) or `confirm: true` (MCP). It removes only the
  `--domain` and `--host` entries you name; removing a whole client needs `--whole-client`, so
  an empty or blank selector removes nothing. Sweep history and events are kept.
- **`local-data purge` is a dry run unless you pass `--yes`.**
- **Changing inventory previews first.** `clients set` only shows what it would add and exits 2
  unless you pass `--yes` (command line) or `confirm: true` (MCP).
- **`DOMAIN_CERT_TRACKER_MCP_READ_ONLY=1`** exposes only read tools: it drops `sweep`, `sync`, `export`, `clients set`, `clients remove`, the learn-loop writers (`teach`, `teach-pattern`, `teach-lookup`, `learnings confirm`, `learnings reject`, `learnings forget`, `playbook amend`) and `workflow archive`
  from the MCP server.
- **A failed check never looks clean.** A lookup that fails is stored as a failure, listed
  under `unverified` in `report`, and never replaces the last good result.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lookups and reports. No change anywhere. | `report`, `renewals`, `issuers audit`, `hosts discover`, `events list`, `clients list`, `domain`, `nameserver`, `ip`, `autnum`, `bootstrap`, `certificates`, `tls probe`, `dns check` | Allow |
| **Write (routine, local store only)** | Records this tool's own sweep history. | `sweep` | Allow |
| **Write (inventory, local store only)** | Adds to the client inventory every sweep checks. | `clients set` (previews unless `--yes` or `confirm: true`), `clients import` (command line only) | Allow after confirm |
| **Destructive (local store only)** | Deletes inventory or the local store. | `clients remove` (previews unless `--yes` or `confirm: true`), `local-data purge` (dry run unless `--yes`) | Human-in-the-loop only |
| **Credential / security** | Touches tokens, keys, MFA. | (none by default: the skill needs no credentials) | n/a |
| **Admin** | Back-office administration. | (none) | n/a |

## How to lock it down

- **Run the MCP server read-only** with `DOMAIN_CERT_TRACKER_MCP_READ_ONLY=1` when an agent
  should only answer questions; run `sweep` yourself on a schedule.
- **Keep HTTP mode on loopback** unless you need a remote agent, and treat
  `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` like a password (see mcp-install.md).
- **Allow other TLS ports deliberately.** A host listed as `host:8443` is probed only when that
  sweep passes `--allow-port 8443` on the command line.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). By default it holds no credential, reads only public sources, connects only
to public addresses, and writes only its own local store. You can read every line of how it
does so.
