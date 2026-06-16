# NinjaOne - reprint survival notes

Connector-specific edits to **generated** (`DO NOT EDIT`) files that a
`cli-printing-press` reprint must not clobber. The ledger is
`skills/ninjaone/handfixes.json`; `tools/maintainer/check_handfixes.py --slug ninjaone`
asserts every marker still exists and fails CI if a reprint dropped one.

## after-id pagination (issue #88)

**Symptom.** `sync devices --full --param pageSize=1000 --max-pages 0` against a
tenant with 1,115 devices synced exactly 1,000 and stopped, reporting
`total:1000` as if complete. `--max-pages 0` had no effect.

**Root cause.** NinjaOne list endpoints (`/v2/devices`, `/v2/organizations`,
`/v2/locations`, …) return a **bare JSON array** and paginate with
`?after=<id of the last entity on the prior page>` - there is no envelope
`cursor` and no `has_more`. The generated sync loop only handled
envelope-cursor / `page` / `offset` paginators: `extractPageItems` returns
`hasMore=false` for a bare array, so the natural-end break
(`if !hasMore || len(items) < pageSize.limit`) fired after the first full page.

**Fix.** A response-shape-driven after-id fallback in `internal/cli/sync.go`
(both `syncResource` and `syncDependentResource`): when a bare-array page is
full and its last item exposes an `id`, set the next cursor to that id and
request the next page under `?after=`. Object-enveloped responses
(`/v2/queries/*`, which carry a `cursor` object) are detected as non-bare and
keep the existing envelope path, so they do not regress.

**Durable fix (upstream).** The press's pagination profiler should learn to
detect after-id / keyset pagination natively (bare-array list + `?after` param +
id-derived cursor). Filed via `printing-press-retro`. Until then this hand-fix
must survive every reprint - keep the markers in `handfixes.json` green.
