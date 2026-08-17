# Reprint survival - servosity connector

The servosity connector is engine-swapped from the `servosity-msp` mint via a de-msp
ritual re-applied on every reprint. `handfixes.json` pins the load-bearing invariants.

## admin-only-endpoints (removal invariant)

The partner connector ships with the **reseller-scoped** `SERVOSITY_MSP_TOKEN`. Two
generated endpoint commands are **admin-scoped** and return **HTTP 403** for that token,
so they are removed from the partner surface:

- `issues archived`  → `GET /issues/archived/`  (admin only)
- `issues ignored`   → `GET /issues/ignored/`   (admin only)

These live in Servosity's separate internal admin CLI instead (admin-token-scoped;
not part of this partner connector).

**Reprint durability:** these endpoints are generated from the OpenAPI spec, so a naive
reprint reintroduces them on **every** surface, not just the CLI. The de-msp ritual must
re-strip all of the following after generation, or the admin-only surface returns:

- `internal/cli/issues_archived.go`, `internal/cli/issues_ignored.go` - delete both files.
- `internal/cli/issues.go` - remove the two `newIssuesArchivedCmd` / `newIssuesIgnoredCmd`
  `AddCommand` registrations.
- `internal/mcp/code_orch.go` - remove the `issues.archived` and `issues.ignored`
  `codeOrchEndpoint` entries (the MCP `servosity-msp_search`/`_execute` tools surface and
  run them otherwise - the twin surface agents actually drive).
- `internal/cli/sync.go` - remove the `issues-archived` / `issues-ignored` entries from the
  sync resource map (bulk `sync` would 403 on them).
- `internal/store/store.go` - remove the `issues-archived` / `issues-ignored` primary-key
  map entries.
- `internal/cli/channel_workflow.go` - remove `issues-archived` / `issues-ignored` from the
  workflow resource list.
- `guide.md`, `SKILL.md` - drop the two `issues archived` / `issues ignored` doc bullets.

Note: the `issues` table's `ignored_until` column and the `archive`/`ignore`/`reactivate`
**action** commands are partner-scoped and unrelated - keep them.

> `attention` is NOT admin-only here: it was adapted to walk `/resellers/{id}/issues/`
> (partner-scoped) instead of `/admin/*`. Keep it.

## explicit-config-credential-writes (PR #248)

The generated `cliutil` credentials layer is config-path-aware on **read** but was not
on **write**. `LoadCredentialsForConfigWithStatus` resolves
`<dir-of-config>/data/credentials.toml` and `config.Load` prefers that sibling store
whenever `--config` or `SERVOSITY_MSP_CONFIG` is set, but `SaveCredentials` and
`RemoveCredentials` always resolved the default data-directory store.

Two user-visible consequences, both silent:

- `auth logout` printed `Logged out. Credentials cleared.` while the sibling token
  stayed on disk. `auth status` still reported authenticated and the token kept
  reaching the wire.
- `auth set-token` wrote the rotated token to the default store while the sibling
  kept serving the OLD one, so a credential rotation the operator believed had
  happened had not. This is the more dangerous half: a revoked or leaked token
  keeps being used.

**Reprint durability:** all four files below are generated (`DO NOT EDIT`), so a
reprint on an unfixed press restores the false logout. The ritual must re-apply:

- `internal/cliutil/credentials.go` - `SaveCredentialsForConfig` and
  `RemoveCredentialsForConfig`, both resolving through
  `CredentialsFilePathForConfig` exactly as the read path does.
- `internal/config/config.go` - the unexported `explicitConfigFile` field recorded in
  `Load`, the `usesColocatedCredentials` predicate, the exported `CredentialsPath`
  accessor, and the routing in `saveCredentialsFirst` / `ClearTokens`. `ClearTokens`
  clears the sibling **and** the default store: `Load` falls back to the default when
  the sibling holds nothing, so both can authenticate the same config.
- `internal/cli/auth.go` - `credentialSavePath` asks `cfg.CredentialsPath()` so
  `auth set-token` names the file it actually wrote.
- `internal/config/config_test.go` - hand-written, not generated. Carries the two
  regression tests that fail without the fix.

## mcp-filesystem-path-flags (PR #248)

The MCP server shells out to the companion CLI as the server account, so any
forwarded flag that names a local path lets a tool caller choose where that
account reads or writes. The generated `blockedDestinationFlags` list covered
`audit-dir` / `o` / `output` / `receipt-file` and missed:

| Flag | Commands | Effect |
| --- | --- | --- |
| `--db` | every local-store command (21 tools) | attacker-chosen SQLite file path |
| `--out` | `qbr`, `qbr-all` | attacker-chosen report destination |
| `--input` | `import` | server-side file read |
| `--notes-file`, `--playbook-file`, `--playbook-notes-file` | `teach`, `teach playbook` | server-side file read and write |
| `--reconcile` | `bill` | server-side CSV read |

28 of 155 tools exposed at least one. Driving `learnings_list` with
`{"db": "<any path>"}` through the MCP server created a 1.8 MB SQLite file there.

**Reprint durability:** both files are generated, so a reprint reopens all 28.
The ritual must re-apply:

- `internal/mcp/cobratree/shellout.go` - the added `db` / `input` / `out` entries
  in `blockedDestinationFlags`, plus `filesystemPathFlagPhrases` and
  `isFilesystemPathFlag`.
- `internal/mcp/cobratree/typemap.go` - the `isFilesystemPathFlag` call inside
  `blockedStructuredArgsForCommand`'s local-flag walk.
- `internal/mcp/cobratree/filesystem_flags_test.go` - hand-written, not generated.

Matching the flag's **usage text** rather than its **name** is load-bearing. This
CLI ships generated API body fields called `--path`, `--seed-path`,
`--source-paths`, `--exclude-paths`, `--filename` and `--working-dir` whose values
go to the vendor API and never touch the server's filesystem. A name-based rule
would block those real API surfaces and still miss the next generated path flag.

## cross-host-redirect-strips-all-credentials (PR #248)

The generated `CheckRedirect` deleted only `Authorization` on a host-changing
redirect while Go replayed every other header verbatim. The 4.30 surface added
26 `--x-servosity-mfa` flags, so `X-Servosity-Mfa` is a second live credential on
the wire, and `config.headers` lets an operator put an API key in any header name
they choose. A 3xx from the API (an open redirect, or a partner handoff) handed
those credentials to a host the operator never authenticated against.

**Reprint durability:** `internal/client/client.go` is generated. The ritual must
re-apply `isCredentialHeader` + `stripCredentialHeaders` and the
`stripCredentialHeaders(req.Header)` call on the cross-host branch. The
same-host branch is unchanged: it still re-derives `Authorization` so nonce-bound
schemes keep working and in-host redirects do not start returning 401.
`internal/client/redirect_credentials_test.go` is hand-written, not generated.
