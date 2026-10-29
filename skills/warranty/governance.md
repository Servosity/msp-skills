# warranty skill - governance and safety model

> Unofficial. Community-built skill for the Dell TechDirect, Lenovo eSupport and Apple Business
> APIs. Not affiliated with, endorsed by, or sponsored by Dell Inc., Lenovo or Apple Inc.
> This page tells an MSP owner exactly what the warranty skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `warranty-cli` binary (and `warranty-mcp`). Each vendor is optional
and uses its own credential:

- Dell: `WARRANTY_DELL_CLIENT_ID` and `WARRANTY_DELL_CLIENT_SECRET` (TechDirect Warranty API,
  OAuth client credentials). From the environment, or stored with `warranty-cli auth login`
  in the CLI's credentials file (owner-only permissions).
- Lenovo: `WARRANTY_LENOVO_CLIENT_KEY` (eSupport ClientID token). Environment only.
- Apple: `WARRANTY_APPLE_CLIENT_ID`, `WARRANTY_APPLE_KEY_ID` and `WARRANTY_APPLE_PRIVATE_KEY`
  (Apple Business API account; the key is PEM text or a path to the .pem file). Environment only.

Each credential is sent only to its own vendor's host. Requests to any other host, and
redirects, are refused, and credentials are withheld from output, errors and the local mirror.

## Default-safe behavior

- **Nothing changes at any vendor.** Every vendor command is a read.
- **Local mirror writes are previewable.** `import`, `lookup`, `recheck` and `apple-pull`
  write only this CLI's warranty mirror (`data-warranty.db`); `--dry-run` previews them.
  Over MCP, `lookup`, `recheck` and `apple-pull` save only with `confirm: true`, every other
  tool that writes this machine's local data runs only with `confirm: true`, and `import` and
  `lookup --file` are not available (they read a file on the host). Without `confirm: true` an
  MCP call writes nothing at all: no store, no usage journal, no cached token.
- **Agent mode is explicit.** `--agent` produces JSON for scripting. See AGENTS.md.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Vendor lookups and mirror reports. No change. | `report`, `lifecycle`, `unresolved`, `events list`, `doctor`, `dell entitlements`, `dell summary`, `lenovo warranty`, `lenovo offering`, `lenovo contract`, `apple coverage`, `apple get`, `apple list`, `search` | Allow |
| **Write (local mirror only)** | Saves devices and coverage to the local mirror. | `import`, `lookup`, `recheck`, `apple-pull`, `sync`, `workflow archive` | Preview with `--dry-run`, then an approved write; over MCP a save needs `confirm` |
| **Credential** | Stores, mints or clears credentials. | `auth login`, `auth set-token`, `auth logout`, `auth status` | Human-in-the-loop only |
| **Destructive** | Deletes local data. | `local-data purge` (dry run unless `--yes`) | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Configure only the vendors you use.** A vendor without credentials is never called.
- **Keep autonomous agents to Read plus previewed mirror writes.**
- **Never let an agent run Credential or Destructive tier commands unattended.**
- **Set `WARRANTY_NO_LOCAL_DATA=1`** to stop the CLI writing any local data (`lookup` still
  answers, mirror writers refuse).
- **Rotate a credential if it is ever exposed** (for example after bridging the MCP server
  to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credentials, the binary uses each one against its
own vendor, and you can read every line of how it does so.
