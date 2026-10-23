# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `maxio-cli subscriptions purge-json create` ran `maxio-cli subscriptions purge-json purge-subscription`, POST `/subscriptions/{subscription_id}/purge.json`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `maxio-cli invoices issue-json issue-invoice` (POST `/invoices/{uid}/issue.json`)
  - `maxio-cli invoices refunds-json refund-invoice` (POST `/invoices/{uid}/refunds.json`)
  - `maxio-cli invoices void-json void-invoice` (POST `/invoices/{uid}/void.json`)
  - `maxio-cli proforma-invoices void-json void-proforma-invoice` (POST `/proforma_invoices/{proforma_invoice_uid}/void.json`)
  - `maxio-cli subscription-groups cancel-json cancel-subscriptions-in-group` (POST `/subscription_groups/{uid}/cancel.json`)
  - `maxio-cli subscription-groups delayed-cancel-json initiate-delayed-cancellation-for-group` (POST `/subscription_groups/{uid}/delayed_cancel.json`)
  - `maxio-cli subscription-groups service-credit-deductions-json deduct-subscription-group-service-credit` (POST `/subscription_groups/{uid}/service_credit_deductions.json`)
  - `maxio-cli subscriptions advance-invoice issue` (POST `/subscriptions/{subscription_id}/advance_invoice/issue.json`)
  - `maxio-cli subscriptions cancel-dunning-json cancel-dunning` (POST `/subscriptions/{subscription_id}/cancel_dunning.json`)
  - `maxio-cli subscriptions delayed-cancel-json initiate-delayed-cancellation` (POST `/subscriptions/{subscription_id}/delayed_cancel.json`)
  - `maxio-cli subscriptions hold-json pause-subscription` (POST `/subscriptions/{subscription_id}/hold.json`)
  - `maxio-cli subscriptions migrations-json migrate-subscription-product` (POST `/subscriptions/{subscription_id}/migrations.json`)
  - `maxio-cli subscriptions prepayments refund` (POST `/subscriptions/{subscription_id}/prepayments/{prepayment_id}/refunds.json`)
  - `maxio-cli subscriptions price-points bulk-reset-subscription-components` (POST `/subscriptions/{subscription_id}/price_points/reset.json`)
  - `maxio-cli subscriptions purge-json purge-subscription` (POST `/subscriptions/{subscription_id}/purge.json`)
  - `maxio-cli subscriptions service-credit-deductions-json deduct-service-credit` (POST `/subscriptions/{subscription_id}/service_credit_deductions.json`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.4] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

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
  It also dialled the shipped placeholder base URL (`https://{site}.chargify.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `MAXIO_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `MAXIO_API_KEY`, `MAXIO_BASE_URL`.

## [0.1.0] - 2026-07-01

### Changed

- feat(maxio): add Maxio Advanced Billing connector (revenue, MRR, retention) (#173)

