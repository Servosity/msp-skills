# Write gating

The endpoint connectors expose `<slug>_execute`, which can call any of the
vendor's state-changing endpoints. Called without `confirm`, it must return a
preview (method, path, body) and send nothing; called with `confirm: true`,
it sends the request.

## Sub-features

- `execute-preview` returns `"preview": true, "confirmation_required": true` (or a message naming confirm) for a write endpoint.
- `execute-nothing-sent` sends no request to the vendor without confirm.
- `execute-confirm` sends the request when `confirm` is boolean `true`.

## How to get to it (user POV)

- The agent calls `<slug>_search` with a phrase like "create" and then
  `<slug>_execute` with the returned `endpoint_id` and params.
- The agent repeats the call with `"confirm": true` after the user approves the preview.

## Driving it with verify.py

Preconditions:

- `tools/list` includes `<slug>_execute` and `<slug>_search` (no `_execute` is SKIP).

- **Find a write endpoint.** Run `verify.py run --slug <slug> --features 4`.
  The harness searches `create`, `add`, `update`, `delete`, `remove`, `set`,
  `edit`, `assign`, `reset`, `new` in turn and takes the first POST, PUT,
  PATCH or DELETE result, preferring a path with no placeholders.
  `f4-write-gating/chosen-endpoint.json` records it.
- **Call without confirm.** `<slug>_execute` with the `endpoint_id` and
  placeholder params. `call-without-confirm.json` pairs the answer with the
  vendor requests it caused. Any request with the endpoint's method is FAIL
  (sent at once); a preview with nothing sent continues.
- **Call with confirm.** The same arguments plus `"confirm": true`.
  `call-with-confirm.json` must show the request reaching the fake vendor.
- **Proof.** PASS reads `<slug>_execute previewed POST /<path> without confirm (nothing sent); confirm=true sent it`.

## Gotchas

- Every write goes to the fake vendor only; the sandbox refuses any other host.
- An answer that neither sends nor previews (for example, a path-parameter
  validation error) is ERROR: the gate was not exercised.
- Mutating GETs (an `Exec...` endpoint called with GET) are not chosen here;
  the search filters on the HTTP method.
- The static rule `mcp-execute-no-confirm` reads the source; this feature
  observes what reaches the vendor.
