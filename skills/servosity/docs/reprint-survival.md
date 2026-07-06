# Reprint survival — servosity connector

The servosity connector is engine-swapped from the `servosity-msp` mint via a de-msp
ritual re-applied on every reprint. `handfixes.json` pins the load-bearing invariants.

## admin-only-endpoints (removal invariant)

The partner connector ships with the **reseller-scoped** `SERVOSITY_MSP_TOKEN`. Two
generated endpoint commands are **admin-scoped** and return **HTTP 403** for that token,
so they are removed from the partner surface:

- `issues archived`  → `GET /issues/archived/`  (admin only)
- `issues ignored`   → `GET /issues/ignored/`   (admin only)

These live in the admin CLI instead: `github.com/Servosity/servosity-admin-cli`
(uses the admin `SERVOSITY_API_TOKEN`).

**Reprint durability:** these commands are generated from the OpenAPI spec, so a naive
reprint will regenerate `internal/cli/issues_archived.go` + `internal/cli/issues_ignored.go`
and re-add their registrations in `internal/cli/issues.go`. The de-msp ritual must strip
them again after generation (delete the two files + remove the two `AddCommand` lines).
Until the ritual encodes this, a reprint reintroduces the admin-only surface.

> `attention` is NOT admin-only here: it was adapted to walk `/resellers/{id}/issues/`
> (partner-scoped) instead of `/admin/*`. Keep it.
