# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.17] - unreleased

### Fixed
- **The `zammad_search` MCP tool's description now names this API's resources.** It used to be one sentence shared by every connector, so an MCP host that picks tools by matching words ranked other tools above it for a request like `<vendor> <resource>`. The description now ends with the resource families from the endpoint list, most endpoints first, at most 300 characters ([#629](https://github.com/Servosity/msp-skills/issues/629)).

## [0.1.16] - 2026-11-11

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.15] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.14] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.13] - 2026-11-08

### Fixed
- The first sync finishes in a 1 GB container; the store-write credential check no longer holds memory near its decode budget ([#612](https://github.com/Servosity/msp-skills/issues/612)).
- A record the credential check withholds from the local database no longer stops the sync of its resource: the record is skipped, every other record is stored, and the sync names the skipped records and exits non-zero so the next run fetches them again ([#612](https://github.com/Servosity/msp-skills/issues/612)).

## [0.1.12] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.11] - 2026-11-04

Reprinted on cli-printing-press **4.34.0** (from 4.32.5). Every command, flag and MCP tool that shipped in 0.1.10 is still here; the list below is what changed for an operator or an agent.

### Security
- **HTTP mode for remote agents is available again.** `zammad-mcp --transport http` refuses to start until `ZAMMAD_MCP_HTTP_TOKEN` is set, answers 401 to any request without that token as a bearer header, serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert` and `--tls-key` for any address other than loopback. The install guide documents this mode for reference; msp-skills does not support a web-only AI app on it yet.
- **Every output is checked for credentials.** Stdout, stderr, files the CLI writes, its local database, MCP tool results and child processes all pass one shared check: text that repeats a credential the program holds, whole, as a piece of 8 or more characters, or in a decoded form, is withheld whole and replaced by a notice that says so. The check can withhold ordinary text it cannot finish reading; it says so the same way, so rephrase and retry.
- **Every Zammad write over MCP previews first.** `ticket note` is no longer an MCP tool; an agent adds a note through `zammad_execute`, which previews the call and sends it only with `confirm: true`. The CLI command is unchanged.
- **A credential is withheld wherever it comes from and however the answer splits it.** Before the command line is parsed, the CLI and the MCP server read every credential source they can reach: `ZAMMAD_API_TOKEN` and `ZAMMAD_MCP_HTTP_TOKEN`, a user name or password written into `base_url` or `ZAMMAD_BASE_URL`, every config file `--config`, `--home` or `ZAMMAD_CONFIG` names (including its `[headers]` table, and a file the CLI refuses to trust), and the saved credentials. A value is withheld however short it is. Answers that split a token across several JSON values, encode it in groups separated by spaces, or put it after binary bytes are caught too.
- **A configuration or credentials file that does not parse stops the program at startup.** The CLI and the MCP server exit before doing anything else, naming the file and never showing its content; fix or remove the file to continue. This includes `version` and `--help`.
- **The learn journal and the local learn data keep no credential.** `ZAMMAD_LEARN_SESSION` is stored hashed, the journal records only flag names the command declares (anything else is a placeholder, and nothing after `--`), and a profile name or description, a recall or teach query, note or resource, an entity alias, a pattern or a feedback text that repeats a credential the program holds is refused instead of saved.
- **A credential is only sent to your Zammad instance, over https.** A request that carries a credential is refused if it goes to any other host (a redirect off your instance included) or over plain `http://`. An instance reached only over plain `http://` needs https in front of it. A `ZAMMAD_FEEDBACK_ENDPOINT` with a password inside its URL is refused the same way.
- **Errors printed before a command runs pass the check too,** such as a bad flag value or an unknown command.
- **MCP tools cannot point a command at a host file.** Any flag whose help text names a file or directory is removed from the tool's schema and refused at request time, along with the press's fixed list of path flags.
- Updated `golang.org/x/text` to v0.42.0 and `mark3labs/mcp-go` to v0.57.0.

### Added
- **Webhooks instead of polling.** `webhooks register` creates one Zammad webhook and one trigger per event (ticket created, ticket updated, article added), signed with a secret it stores beside the API token; `ZAMMAD_WEBHOOK_SECRET` overrides it, and nothing prints it. `webhooks listen` receives them on 127.0.0.1, port 8789, by default (any other address needs `--tls-cert` and `--tls-key`), refuses to start without the secret, checks `X-Hub-Signature` before reading a request, drops a delivery it has already seen, caps bodies at 1 MiB, fetches the ticket through the API instead of trusting the webhook body, and on every start records what changed while it was down; it refuses to run while `ZAMMAD_NO_LOCAL_DATA=1`. `events list --since <cursor>` and the read-only MCP tool `events_list` read the recorded events. `webhooks register` and `webhooks unregister` change Zammad only with `--yes` (CLI) or `confirm: true` (MCP), and `unregister` removes only what `register` created; `webhooks status` shows what is registered. `webhooks listen` is CLI-only.
- `ZAMMAD_MCP_READ_ONLY=1` exposes only read-only MCP tools and refuses any state-changing call; `ZAMMAD_NO_LOCAL_DATA=1` keeps no local copy of API responses. Both are optional and declared in the Claude Desktop extension, the MCP Registry entry and the install guide.

### Fixed
- **Analytics no longer answer from a partial local copy.** `agent-load`, `overdue`, `stale` and the other local analytics commands refuse a store with no finished sync, even when a single `tickets get` cached a few rows, and say to run `zammad-cli sync`.
- **Local analytics no longer answer from a partial store.** `agent-load`, `agent-trend`, `churn-risk`, `customer-health`, `escalate`, `feedback-scan`, `overdue` and the `load`, `orphans` and `stale` workflows refuse to answer when the last sync of the data they read did not finish or never ran, and name the sync command to run. `articles sync --limit N` no longer marks the article set complete.
- **Removed articles leave the local store.** Refreshing a ticket's articles deletes stored articles Zammad no longer returns.
- **`--data-source` is honored.** The store-only analytics commands refuse `--data-source live` instead of quietly answering from the local store; `kb browse`, `kb get`, `kb search` and `ticket note` are live-only.
- **`doctor` reports reachable and authenticated only when Zammad answered with JSON**, and recognizes the shipped placeholder URL as not configured.
- `load` and `orphans` with `--dry-run` work before the first sync.
- `load`, `stale` and `orphans` are local-only and refuse `--data-source live`. `articles sync` always reads Zammad (never the response cache) and refuses to run while `ZAMMAD_NO_LOCAL_DATA=1`.
- The guide's troubleshooting tip names `zammad-cli articles sync` (there is no `sync --articles` flag), and the read-only MCP mode is described as it behaves: the learn-store tools are dropped too.

## [0.1.10] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `zammad-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.9] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.8] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.28.0). Every command, flag and
MCP tool that shipped in 0.1.5 is still here; the list below is what changed for
an operator or an agent.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. The two
  security fixes this connector carried by hand since September are now generated code
  and no longer depend on a hand-fix surviving a reprint: every MCP tool-call value is
  joined to its flag as one word (`--flag=value`, the 0.1.5 fix, press #4647) and the
  MCP server refuses argument names carrying `=` and the local-store destination flags
  (`--db`, `--output`, `--audit-dir`, ...) at the schema as well as at run time.
- **`--agent` no longer implies `--yes`.** `--agent` still sets `--json --compact
  --no-input --no-color`; a command that asks for confirmation now needs an explicit
  `--yes`. Scripts that relied on `--agent` to auto-confirm a write must add `--yes`.
- **New global flags:** `--receipt` / `--receipt-file` / `--audit-dir` write a private
  run receipt; `--client-profile` selects a tenant-gated client profile;
  `--rate-limit` now defaults to `auto` (paces to the server's rate-limit headers).
- **MCP tool surface:** `kb_browse`, `kb_get`, `kb_search`, `articles_sync` and
  `zammad_get` are now exposed as tools, and the `read_the_kb_before_answering` recipe
  is registered. The three no-op parent tools (`learnings`, `playbook`, `workflow`),
  which only returned their own help text, are gone. `stale`, `orphans` and `load` are
  unchanged.
- **Three new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `ZAMMAD_MCP_HTTP_TOKEN` (bearer token, only for
  `--transport http`), `PRINTING_PRESS_CLIENT_PROFILE` (client-profile binding) and
  `ZAMMAD_USER_AGENT` (overrides the User-Agent sent to Zammad; blank keeps the
  built-in one). `ZAMMAD_URL`, `ZAMMAD_BASE_URL` and `ZAMMAD_API_TOKEN` are unchanged.
- **Remote (HTTP) launch recipe.** `mcp-install.md` and the ChatGPT answer now say
  `ZAMMAD_MCP_HTTP_TOKEN=<value> ... zammad-mcp --transport http` with no `--addr`:
  the server listens on `127.0.0.1:7777` by default and refuses a non-loopback
  `--addr` (such as the old `--addr :7777`) unless `--tls-cert` and `--tls-key` are
  given, and refuses to start `--transport http` without the bearer token. Put the
  loopback listener behind your HTTPS tunnel or reverse proxy as before.
- **Release artifacts:** first zammad release cut after the 2026-09-18 pipeline change,
  so the `.mcpb` bundle carries the companion `zammad-cli` and `zammad-mcp` reports its
  real version (`0.1.6`) instead of `0.0.0-dev`.

### Fixed
- **`ZAMMAD_URL` kept working.** The 4.32.5 generation dropped the hand-wired read of
  `ZAMMAD_URL` (instance root, `/api/v1` appended), which every install doc names as the
  way to point the CLI at your instance; the read is restored, pinned by a test and now
  recorded in the hand-fix ledger.
- **`stale`, `orphans` and `load` kept working.** The 4.32.5 profiler no longer emits
  the three project-management workflow commands for this API; they are carried as
  hand-authored files so the CLI and MCP surfaces stay the same.
- **MCP `--transport http` listener** now sets a request-header read deadline, so one
  client cannot hold the listener open by dribbling headers.
- **`doctor`, `recall` / `playbook list` honesty fixes preserved** (0.1.3 / 0.1.4):
  the placeholder-`base_url` refusal, the explicit no-credential row and the
  `mcp:local-write` annotations were re-applied on the fresh tree; the doctor
  information-key handling is now generated.

## [0.1.5] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.4] - 2026-09-01

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://your-instance.zammad.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `ZAMMAD_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ZAMMAD_BASE_URL`, `ZAMMAD_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.0]

### Added
- Initial msp-skills release: Zammad CLI + MCP server with an offline SQLite mirror.
- Full ticket surface: list, get, search (Zammad query syntax), create, update, delete, plus articles (read/add) and a one-line `ticket note`.
- Knowledge Base: browse, search, and read answers (parsed from the init bundle) plus create/publish/set-internal/delete.
- Team-management analytics the API can't answer in one call: `agent-load`, `agent-trend`, `customer-health`, `overdue`, `escalate`, `churn-risk`, and `feedback-scan`.
- Reference reads: organizations, users, groups, states, priorities, tags, overviews.
- Per-instance config (`ZAMMAD_URL` + `ZAMMAD_API_TOKEN`); works with any self-hosted or hosted Zammad.
