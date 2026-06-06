# mspbots skill - the MSP pain it closes

## The pain

Three pains, each structural to the MSPbots Public API itself - documented in
MSPbots' own API article (the `wiki.mspbots.ai` "Public API" page, archived
April 2024 on the Wayback Machine after the Confluence wiki was decommissioned;
the same article lives on in the current support.mspbots.ai KB):

**1. Two endpoints, zero discoverability, zero tooling.** The entire
programmatic surface is `GET /api/dataset/{resourceId}` and
`GET /api/widget/{resourceId}`. There is no endpoint that lists what is bound
to your key - resource IDs are 19-digit identifiers an admin copies out of
Settings > Public API one at a time. And as of June 2026 there was no published
tool for this API anywhere: no CLI, no SDK on npm or PyPI, no GitHub wrapper,
no MCP server, not even a community curl gist. The data is "open" in the way a
door with no handle is open.

**2. The API only knows _now_.** Datasets and widgets return current state;
there is no history endpoint. So the ops manager who lives in MSPbots
dashboards keeps the week-over-week KPI column by hand - screenshot the widget
on Monday, paste the count into a spreadsheet, repeat. The trend is the most
valuable number in the stack (is the backlog growing? is SLA compliance
drifting?) and it is the one number the platform's own API cannot return. It
dies the week someone forgets.

**3. Filters are a comma-encoded DSL with silent traps.** Operators are
encoded into the parameter value: `price=12.6,56.3` is a range, a trailing
comma (`date=2022-07-01,`) means on-or-after, a leading comma means at-most,
and column names with spaces go straight into query keys. The same official
docs list rate limits with unspecified numbers and intermittent HTTP 502
responses on heavy widget fetches. Every hand-built integration re-learns
these rules the hard way.

## What this skill does about it

- `mspbots-cli registry add open_tickets 1534956341424005122` - name a
  resource once; every other command accepts the alias. The local registry is
  the discovery surface the API never shipped.
- `mspbots-cli snapshot open_tickets` - capture a timestamped copy of any
  dataset or widget into local SQLite. Schedule it (cron, agent loop) and
  history accrues.
- `mspbots-cli trend open_tickets --agg count` - the week-over-week answer,
  computed from stored snapshots, offline, zero API calls.
- `mspbots-cli diff open_tickets` - row-level added/removed/changed between
  any two snapshots of the same resource.
- `mspbots-cli pull open_tickets --where "Update Date >= 2026-06-01"` -
  readable predicates compiled into the comma-encoded wire DSL, correct the
  first time; `describe` infers the column names and types to filter on, since
  the API has no metadata endpoint either.
- `mspbots-cli export open_tickets --format csv` - the full table,
  auto-paginated, with an honest partial-dump flag when `--max-pages` is hit.

## Sources

- MSPbots official Public API documentation - `wiki.mspbots.ai/display/MKB/Public+API`
  (Confluence wiki decommissioned; archived April 2024 at
  web.archive.org/web/20240421120635; same article title present in the current
  support.mspbots.ai KB). Documents the two-endpoint surface, the Settings >
  Public API key/binding flow, the comma-encoded filter rules, unspecified rate
  limits, and the intermittent-502-on-heavy-widgets error case.
- Ecosystem absence, verified June 2026: searches of npm, PyPI, GitHub, and the
  MCP directories (LobeHub, MCP Market, FastMCP) return no MSPbots Public API
  client of any kind. This skill is the first.

## Status

Beta. Validated against the MSPbots Public API surface (auth/error paths
verified against the live endpoint; full data paths exercised against mock
fixtures - we do not operate an MSPbots tenant). The closed-loop receipt (a
named MSP running it live in their production tenant at a Build Session) is
tracked separately and added here as `video.md` once it exists.
