# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Security
- Credentials go through the shared credential kit: a line, MCP result, export or
  stored row that carries a credential the CLI holds is withheld whole, and a
  credential is sent only over https to the NAS the process was started for.
  `fleet sync` runs one process per NAS, each with only that NAS's credentials.
- A credential may go only to the address of the config this run really uses: the
  last `--config`, else `SYNOLOGY_CONFIG`, else the default. Other configs on the
  machine are read only so their values cannot print.
- A run profile keeps only flags a command really declares, and a credential in any
  profile field is held. A credential field in a `feedback` body is held, and that
  feedback is withheld rather than stored.
- `credkit_verify` exits 0: nothing that writes runs before the kit starts, and every
  site it cannot see routed is listed with its reason in `credkit-sites.json`.

### Added
- Initial msp-skills release: Synology CLI (`synology-cli`) and MCP server
  (`synology-mcp`) for Synology DSM and Active Backup for Business.
- Multi-NAS fleet: `fleet add` registers each client NAS with its own login
  (read from environment variables, never stored), two-factor sign-in via a
  cached trusted-device token, and certificate pinning for DSM's self-signed
  certificate (`--tls-fingerprint`, with the value checked in DSM; `--pin-cert`
  only displays the fingerprint the NAS presents and saves nothing).
- `fleet sync` mirrors Active Backup tasks, job results, per-device runs and a
  Storage Manager snapshot from every registered NAS into one local SQLite
  database; one unreachable NAS does not stop the others.
- Fleet reports from the mirror: `fleet status` (one row per NAS), `abb failures`
  (failed, partial and missed runs), `abb stale` (devices with no recent good
  backup), `abb device` (find a machine by host name), `abb trend` (success rate,
  run duration and overruns), `fleet health` (degraded pools, bad SMART disks,
  hot disks, nearly full volumes with days to full), `abb unprotected` (machines
  and tasks that dropped out of protection), `fleet capacity` (growth per day and
  days until full for every volume) and `abb change-rate` (devices whose newest
  backup moved far more data than usual, or nothing). A NAS that did not sync reads
  as not synced, never as OK, and the reports exit 6 when anything needs attention.
- `abb triage`: why one task failed, from its newest failed run's detail lines and
  the Active Backup log for that window.
- Live per-NAS reads for Active Backup, Hyper Backup, storage, SMART, system
  health, logs, packages, shared folders, users and groups.
- `abb backup` and `abb cancel` to start or cancel a backup, previewable with
  `--dry-run`; over MCP `abb_backup`, `abb_cancel` and `session_logout` return a
  preview until called with `"confirm": true`, and every MCP tool refuses an
  argument it does not declare. `SYNOLOGY_MCP_READ_ONLY=1` removes every write
  tool from the MCP server, and `SYNOLOGY_NO_LOCAL_DATA=1` keeps no local copy
  (`sync` and `fleet sync` refuse to run).
- Credential safety: the DSM password goes only to an address that answers
  SYNO.API.Info as DSM, never across a redirect; a plain `http://` address on
  any host, loopback included, is refused before anything is sent; every credential the CLI holds is withheld, in any encoding, from
  stdout, stderr, MCP results, the response cache, the local store, exports,
  `--deliver` files and receipts, and secret-named
  fields (passwords, tokens) are redacted in the cache, the store and exports
  (live reads show what the NAS returned).
- Sessions: a cached session is reused only for the exact NAS address and
  account it came from, and no session or credential is sent before the
  address answers SYNO.API.Info as DSM.
- Operator-only registry: `fleet add` and `fleet remove` are CLI commands, not
  MCP tools, and `fleet add <name> --tls-fingerprint <sha256>` re-pins a NAS
  whose certificate was renewed, keeping its history. Re-running `fleet add`
  on a registered NAS changes only the fields you pass and lists them.
- `auth logout` forgets the whole session file of the NAS it names (or every
  NAS with `--all`), lists what it removed, and refuses when no NAS is named.
- `auth logout --all` removes only session files the CLI wrote, and lists
  any other file it kept.
- Fleet and abb reports print one complete card per row in a terminal.
- A missing required argument (`abb triage <task>`, `abb device <host-name>`)
  is a usage error (exit 2) on the CLI and over MCP.
- TLS: `--ca-cert` / `SYNOLOGY_CA_CERT` for a NAS certificate from your own CA,
  and `--tls-server-name` / `SYNOLOGY_TLS_SERVER_NAME`.
- Built on CLI Printing Press 4.34.2; the config file is `config.toml`.
- Reports never show an unknown as zero: rows about a NAS that did not sync
  carry null counts, and unknown durations are null with a note.
