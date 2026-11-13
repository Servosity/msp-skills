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

- `tools/list` includes `<slug>_execute` and the `<slug>_search` with the
  same prefix (no `_execute` is SKIP; `_execute` without its `_search` is ERROR).
- The press generates `<slug>_execute` only in code orchestration mode and
  only when the API has a write endpoint: `skills/<slug>/press/tools-manifest.json`
  has `mcp.orchestration == "code"` and a POST, PUT, PATCH or DELETE tool.
  Elsewhere SKIP is the expected verdict at any press version, and
  `tools/maintainer/verify_watch.py` reads the same manifest to tell the two apart.

- **Find a write endpoint.** Run `verify.py run --slug <slug> --features 4`.
  The harness searches `create`, `add`, `update`, `delete`, `remove`, `set`,
  `edit`, `assign`, `reset`, `new` in turn, then the name of each write tool
  in `press/tools-manifest.json` (for an API whose only write matches none of
  those verbs, such as auvik's POST `alert_dismiss-single`), and takes the
  first POST, PUT, PATCH or DELETE result, preferring a path with no
  placeholders. `f4-write-gating/chosen-endpoint.json` records it.
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
- A connector can list other `*_search` tools (zammad's `kb_search` searches
  knowledge-base answers, not endpoints); only the one paired with
  `<slug>_execute` is driven.
- Typed upload tools (press 4.34.1 on) also preview a multipart upload and
  send it only on `confirm`; this feature drives `<slug>_execute` only.
- The static rule `mcp-execute-no-confirm` reads the source; this feature
  observes what reaches the vendor.
