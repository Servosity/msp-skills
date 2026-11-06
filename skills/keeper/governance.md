# keeper skill - governance and safety model

> Unofficial. Community-built skill for the Keeper Security API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the keeper skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `keeper-cli` binary (and `keeper-mcp`),
authenticating to your own Keeper Commander Service Mode host (`KEEPER_BASE_URL`) with its API key
(`KEEPER_API_KEY`, or the 0600 credential store `auth set-token` writes). The key is sent only to that host over
https, never logged, and withheld from every output, file and MCP result.

## Default-safe behavior

- **No write path to Keeper.** The CLI can send only the Commander commands `keeper-cli allowlist` prints, each with fixed read-only flags. `switch-to-mc` and `switch-to-msp` move Service Mode's session between companies for a read and back; they change no Keeper data. Anything else is refused before a request.
- **No vault records.** Nothing reads records, passwords, attachments or shares, and a response shaped like record content is refused whole.
- **The MCP server writes almost nothing.** `posture_report` saves its run to the local store, and it returns a preview until `confirm` is boolean true. The learning commands are CLI-only, not MCP tools. A read-only tool writes nothing, not even a usage row.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports and stored history. No change anywhere. | `companies list`, `users list`, `users risk`, `users hygiene`, `posture diff`, `posture runs`, `events list`, `billing report`, `security-audit report`, `audit-trail events` | Allow |
| **Local write** | Changes files on this machine only; nothing in Keeper. | `posture report` (reads Keeper, saves the run), `teach`, `playbook amend`, `learnings forget`, `profile save`, `local-data purge` | Allow in the CLI; through MCP only `posture_report` is a tool, and it needs `confirm: true` |
| **Credential** | Stores or removes the API key. | `auth set-token`, `auth logout` | Human-in-the-loop only |

## How to lock it down

- **Set Service Mode's own allowlist.** Create the key with exactly the commands `keeper-cli allowlist` prints (`service-create -c '...'`), so Service Mode itself refuses anything else, even if another tool shares the host.
- **Give this CLI its own Service Mode instance.** Commander's managed-company context is one global per instance. This CLI's own runs take turns on a lock, and a read another tool disturbed is discarded, but a separate API key does not isolate it.
- **Run Service Mode over TLS.** The CLI refuses plain http on every host. Trust a private certificate with `KEEPER_CA_CERT`.
- **Keep the store in mind.** `posture report` saves per-user facts (email, status, 2FA, last login, password-health counts) locally. `KEEPER_NO_LOCAL_DATA=1` refuses it; `local-data purge` removes what is there.
- **Rotate the key if it is ever exposed.**

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Keeper Security API, and you can read every line of how it does so. The skill is
read-only toward Keeper and scoped to your own account.
