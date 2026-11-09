# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.10] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.9] - 2026-11-06

### Changed
- **Default reads verify the AWS account first.** With `--data-source auto` (the default), a read calls sts:GetCallerIdentity before it trusts the local store, so the store never answers for a different account than the credentials in use. Offline reads need `--data-source local`.
- **Re-sync after upgrading.** Rows synced by 0.1.8 carry unscoped IDs and are never read by this version: run `sync` again.
- A custom CA bundle (`AWS_CA_BUNDLE`, or `ca_bundle` in the AWS config) is refused with an error; unset it.
- A container credential endpoint that needs an authorization token over plain HTTP (EKS Pod Identity, or ECS with `AWS_CONTAINER_AUTHORIZATION_TOKEN`) is refused instead of being sent the token.
- A `credential_process` program's stderr is not shown while the AWS SDK retrieves credentials, so a prompt it prints there is not visible.
- EC2 instance-metadata (instance role) credentials are not used; on EC2 use a profile, SSO or environment keys.
- `waste` (every subcommand), `report` and `ask` answer from a stored inventory scan only for the one region a live read scans (`--region`, `AWS_REGION`, `AWS_DEFAULT_REGION`, the profile's region, else us-east-1); a stored scan of another region never answers. With `--data-source local` and no complete scan of that region stored, the read is an error naming the region to sync.

### Security
- Every credential that signs an AWS request is held before the request is sent, whatever produced it: in an assume-role chain the source profile's credentials (a `credential_process`, static keys, SSO or another role) sign the STS call, and an STS error that quotes their access key ID is now withheld. A request to an AWS endpoint whose `Authorization` header is not a SigV4 signature is refused.
- The CLI and the MCP server now use the shared credential kit (the same implementation of the repository's credential-leak contract every kit-wired connector uses) in place of this connector's own masking. Every AWS credential the program can read is held from startup, and every credential the AWS SDK retrieves is held as it arrives. Every output, file, local-store row, MCP message, helper program and request is checked, not only errors: a unit that would carry a held credential, in any encoding, is withheld whole and replaced with "[credkit: withheld from <where>: it carried a credential this program holds]".

## [0.1.8] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `aws-billing-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show your AWS credentials.** AWS answers can quote the caller's access key ID (a signature error does), and the CLI printed AWS errors unchanged; the MCP server's endpoint tools did not mask the AWS credentials at all. Now an AWS error or API error whose text carries the access key ID, secret access key or session token in use, as written, HTML-entity-encoded, JSON-escaped, URL-encoded at any depth, base64 or hex, shows the operation name followed by `[text withheld: it repeated a secret value this program holds]`. Access-denied and throttling errors are still recognized. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.7] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.6] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.5] - 2026-10-15

### Changed
- Rebuilt with Go 1.26.9 (was 1.26.6). Go 1.26.9 includes upstream Go security fixes,
  among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and GO-2026-6612.
  No functional changes.

## [0.1.4] - 2026-09-18

### Changed
### Fixed
- **Windows bundle could not find its companion CLI.** The sibling lookup checked only the
  bare `aws-billing-cli` name; on Windows it now tries `aws-billing-cli.exe` first, then
  the bare name, then `AWS_BILLING_CLI_PATH`, then `PATH`.

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)

## [0.1.3] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  The `report --post-slack` hand-off to the Slack CLI joins its values the same way.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.2] - 2026-08-26

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

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `AWS_ACCESS_KEY_ID`, `AWS_BILLING_BASE_URL`, `AWS_BILLING_SLACK_CHANNEL`, `AWS_REGION`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`.

## [0.1.0] - 2026-07-01

### Changed

- feat(aws-billing): add AWS billing & Cost Explorer connector (#175)

