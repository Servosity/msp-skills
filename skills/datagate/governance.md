# DataGate skill - governance and safety model

> Unofficial. Community-built skill for the DataGate telecom billing API.
> Not affiliated with, endorsed by, or sponsored by DataGate.
> This page tells an MSP owner what the skill can touch and how to scope it.

## What it authenticates as

The skill drives `datagate-cli` (and the `datagate-mcp` server), authenticating to
**your own DataGate account** with a Bearer token (`DATAGATE_API_KEY`) and a
`ClientId` header value (`DATAGATE_CLIENT_ID`) that DataGate issues you. Unlike an
OAuth application, there is no separate scoping step in DataGate's own console for
this credential pair - it identifies your whole account, not a narrower role.
Credentials are read from the environment only - never written to disk, never
logged, never sent anywhere except your DataGate endpoint.

Treat the ClientId the same as a password: DataGate documents it as something that
should never be shared or requested by anyone claiming to need it.

## Default-safe behavior

- **Read-only in this version.** Every command in this build is list, get, or
  search. There is no create, update, or delete command to accidentally invoke -
  DataGate's full CRUD surface is documented, but this connector doesn't implement
  it.
- **Discovery first.** `datagate-cli doctor` and `agent-context` report runtime
  truth (auth configured, API reachable) before any command runs.
- **Local mirror is a read cache.** `sync` pulls data into a local SQLite database
  for offline search; it never writes back to DataGate.

## Recommended agent policy

| Tier | What it does | Recommended agent policy |
| --- | --- | --- |
| **Read** | Customer, agreement, invoice, and other resource lookups; local search | Allow |
| **Write / destructive** | Not implemented in this build | N/A |

Because this credential pair is account-wide rather than role-scoped, the
strongest guarantee available today is controlling who has the Bearer token and
ClientId at all, and rotating them if either is ever exposed.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). The credential path is auditable end to end, and
because the connector is read-only there is no write or delete path to reason
about at all.
