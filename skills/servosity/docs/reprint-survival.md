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
