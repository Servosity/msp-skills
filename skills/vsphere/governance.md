# vsphere skill - governance and safety model

> Unofficial. Community-built skill for the VMware vSphere API. Not affiliated with,
> endorsed by, or sponsored by Broadcom Inc.
> This page tells an MSP owner exactly what the vsphere skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `vsphere-cli` binary (and `vsphere-mcp`) as one vCenter SSO
user, set with `VSPHERE_BASE_URL`, `VSPHERE_USERNAME` and `VSPHERE_PASSWORD`
(`VSPHERE_TLS_FINGERPRINT` pins a self-signed certificate). A user with the
vCenter **Read-Only** role is enough for every command.

- The password goes only to vCenter's two login endpoints; every data call carries
  the session id vCenter returns. Session ids stay in memory and are never written
  to disk.
- Requests go only to the scheme, host and port of `VSPHERE_BASE_URL`; a redirect
  elsewhere is refused. Credentials are refused over plain `http://`, a loopback
  address included.
- Credentials come from the environment, or from `credentials.toml` (mode 0600)
  when you run `auth set-credentials`. A `config.toml` you write by hand (mode 0600) may hold one too, and so does the `config.toml` of a CLI an `.agentcookie-managed` marker hands to an external store.
- Output, files, requests and MCP results that would repeat the password or a
  session id, whole or as 8 or more characters in a row in the encodings the check
  knows, are withheld whole and replaced by a "[credkit: withheld ...]" notice.
  After a login that includes 8 or more characters of the user name, since the
  login holds user:password as one value.

## Default-safe behavior

- **Nothing changes vCenter.** Every command that talks to vCenter is a read. There
  is no power, delete, move, snapshot or configuration command, in the CLI or in
  the MCP server.
- **Local writes stay local.** `sync`, `export --output` and `rvtools export --out`
  write files on your machine (mode 0600 for the RVTools tabs).
- **Erasing local data previews first.** `local-data purge` is a dry run unless you
  pass `--yes`.
- **Agent mode is explicit.** `--agent` produces JSON for scripting. See AGENTS.md.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reads vCenter. No change. | `vm list`, `migration blockers`, `snapshots audit`, `licence exposure`, `migration capacity`, `migration proxmox-map`, `rvtools export`, `session`, `doctor` | Allow |
| **Write (local files only)** | Writes on your machine, never to vCenter. | `sync`, `export --output`, `rvtools export --out`, `profile save`, `teach` | Allow |
| **Credential** | Saves or clears the vCenter credential. | `auth set-credentials`, `auth logout` | Human-in-the-loop only |
| **Destructive (local data)** | Erases the local store, cache or learnings. | `local-data purge --yes`, `learnings forget`, `learnings purge` | Human-in-the-loop only |
| **Admin** | vCenter administration. | (none: the skill has no vCenter write) | Not applicable |

## How to lock it down

- **Use a Read-Only vCenter user.** The skill needs nothing more, so a leaked
  credential cannot change the estate.
- **Scope the user** to the datacenters or clusters you are assessing with vCenter
  permissions; the skill sees only what the user can see.
- **Keep the MCP server local unless you need a remote agent.** It speaks stdio by
  default. HTTP mode refuses to start without `VSPHERE_MCP_HTTP_TOKEN`, requires that
  bearer token on every request, and needs TLS for any address other than loopback
  (see mcp-install.md). Treat the token like the vCenter password.
- **Rotate the password if it is ever exposed.**

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply a read-only vCenter user, the binary uses
it only against your vCenter, and you can read every line of how it does so.
