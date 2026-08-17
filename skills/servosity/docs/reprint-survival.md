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
