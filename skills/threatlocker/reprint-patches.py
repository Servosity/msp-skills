#!/usr/bin/env python3
"""Replay every post-generation patch threatlocker needs. Idempotent.

    python3 skills/threatlocker/reprint-patches.py skills/threatlocker/cli

Run this after `printing-press generate --spec <spec> --research-dir <dir>` and
after the -pp- strip, BEFORE building. `handfixes.json` asserts the results; this
script is how you get back to them without rediscovering each one by hand.

The 4.30.2 generator already produces, straight from the enriched spec, the POST
sync path, the paged request bodies and the per-entity id fields. Everything
below is what it still gets wrong for this connector, each filed upstream in
mvanhorn/cli-printing-press#4165:

  1. applySyncCandidates picks the canonical sync resource by SHORTEST PATH, so
     the portal's picker/detail endpoints beat the declared collection and the
     real rows land under a sibling table name.
  2. The default set ships resources that cannot enumerate (a duplicate dropdown
     twin, a by-file-path detail route, parent-keyed children).
  3. UpsertBatch reports generic-row counts, hiding typed-table failures.
  4. sync aggregates zero-stored, API-declared-failure and abnormally-ended
     walks as plain success -- the silent success that made #208 invisible.
  5. Body fields with a spec default are only sent when the user typed the flag,
     so `--help` advertises a default the API never receives.
  6. `workflow archive` reports every resource as synced regardless of outcome.

Only the SYNC-defining blocks are edited: dropping a resource anywhere else (for
example reconcileTypedTables, which maps a resource to its typed table) would
break machinery for resources that still have working CLI commands.
"""
import re
import sys
import pathlib

# ---------------------------------------------------------------- config ----

# Sibling name emitted by the profiler -> the canonical name offline readers use.
RENAMES = [
    ("computers-computer-get-by-all-parameters", "computers"),
    ("computers_computer_get_by_all_parameters", "computers"),
    ("organizations-organization-get-child-organizations-by-parameters", "organizations"),
    ("organizations_organization_get_child_organizations_by_parameters", "organizations"),
    ("scheduled-actions-get-by-parameters", "scheduled-actions"),
    ("scheduled_actions_get_by_parameters", "scheduled_actions"),
]

# Picker / detail endpoints that won a canonical name on path length alone.
DROP_PATHS = [
    "/Computer/ComputerGetForNewComputer",             # install-info, an object
    "/Organization/OrganizationGetForMoveComputers",   # a move-target picker
    "/ScheduledAgentAction/List",                      # HTTP 417 without scheduledType
]

# Resources that can never enumerate from a flat sync.
DROP_RESOURCES = [
    "computer-groups-computer-group-get-dropdown-by-organization-id",  # duplicate of computer-groups
    "audit",                                                           # by-file-path detail route
    "applications-application-get-research-details-by-id",             # by-ID detail route
]
# NOT dropped: application-files and maintenance are parent-keyed and cannot be
# walked flat, but internal/cli/sync_dependents.go fans them out over their
# parent tables, so they stay in the sync set and are intercepted before the
# generated flat walk runs.

# Body fields carrying a spec default that must reach the API even when the user
# did not type the flag: the difference between an MSP seeing 5 devices and 58.
TREE_FLAGS = {
    "computers_list.go":         ("child-orgs", "childOrganizations"),
    "organizations_list.go":     ("all-children", "includeAllChildren"),
    "applications_search.go":    ("child-orgs", "includeChildOrganizations"),
    "approvals_list.go":         ("child-orgs", "showChildOrganizations"),
}

SYNC_BLOCKS = (
    "func defaultSyncResources()",
    "func knownSyncResourceNames()",
    "func syncResourcePath(",
    "var resourceIDFieldOverrides = map[string]string{",
    "func syncResourceMethod(",
)

log = []


def in_sync_block(lines, idx):
    for j in range(idx, -1, -1):
        t = lines[j].lstrip()
        if any(t.startswith(b) for b in SYNC_BLOCKS):
            return True
        if (lines[j].startswith("func ") or lines[j].startswith("var ")) and j != idx:
            return False
    return False


def patch_sync_surface(paths):
    """1 + 2: canonical names, and only enumerable resources in the sync set."""
    for p in paths:
        if not p.exists():
            continue
        src = orig = p.read_text()
        lines = src.splitlines(keepends=True)
        kept = []
        for i, line in enumerate(lines):
            t = line.strip()
            if in_sync_block(lines, i):
                if any(dp in line for dp in DROP_PATHS) and re.match(r'"[a-z-]+":\s*"', t):
                    log.append(f"{p.name}: dropped picker {t[:64]}")
                    continue
                if any(f'"{r}"' in line for r in DROP_RESOURCES) and re.match(r'^"[a-z-]+",?$|^"[a-z-]+":\s*"', t):
                    log.append(f"{p.name}: dropped non-enumerable {t[:64]}")
                    continue
            kept.append(line)
        src = "".join(kept)
        for old, new in RENAMES:
            src = src.replace(old, new)

        # Renaming a sibling onto its canonical name can duplicate a literal
        # inside a slice; collapse repeats within each literal block. Restricted
        # to the renamed names ONLY: a blanket dedupe would also collapse
        # legitimate repeats of ordinary literals (column names such as "name"
        # appear once per typed table and must all survive).
        renamed = {new for _, new in RENAMES}
        out, seen = [], None
        for line in src.splitlines(keepends=True):
            t = line.strip()
            if re.fullmatch(r'"[a-z-]+",', t) and t.strip('",') in renamed:
                if seen is None:
                    seen = set()
                if t in seen:
                    log.append(f"{p.name}: deduped {t}")
                    continue
                seen.add(t)
            elif t in ("}", ")", "};"):
                seen = None
            out.append(line)
        src = "".join(out)

        if src != orig:
            p.write_text(src)
            log.append(f"{p.name}: rewritten")


def patch_store(store):
    """3: report typed-table failures instead of burying them in stderr."""
    s = store.read_text()
    if "func (s *Store) UpsertBatchDetailed(" in s:
        return
    old = "func (s *Store) UpsertBatch(resourceType string, items []json.RawMessage) (int, int, error) {"
    new = '''// UpsertBatch preserves the original three-value contract for callers that do
// not inspect typed-table health. Prefer UpsertBatchDetailed on the sync path:
// `stored` counts GENERIC resources rows, so a batch whose typed projections all
// failed still reports a healthy count while the typed tables that power
// `devices health`, `applications hunt` and analytics stay empty.
func (s *Store) UpsertBatch(resourceType string, items []json.RawMessage) (int, int, error) {
\tstored, extractFailures, _, err := s.UpsertBatchDetailed(resourceType, items)
\treturn stored, extractFailures, err
}

// UpsertBatchDetailed additionally reports how many items landed a generic row
// but failed their typed-table projection. Callers reporting success to a user
// MUST surface a non-zero typedFailures: it is the difference between "synced 58
// computers" and "synced 58 computers that `devices health` cannot see" (#208).
func (s *Store) UpsertBatchDetailed(resourceType string, items []json.RawMessage) (int, int, int, error) {'''
    assert old in s, "UpsertBatch anchor missing"
    s = s.replace(old, new, 1)
    start = s.index("func (s *Store) UpsertBatchDetailed(")
    end = s.index("\nfunc ", start + 10)
    body = s[start:end]
    body = body.replace('return 0, 0, fmt.Errorf("starting batch transaction',
                        'return 0, 0, 0, fmt.Errorf("starting batch transaction')
    body = re.sub(r"return stored, extractFailures, fmt\.Errorf",
                  "return stored, extractFailures, typedFailures, fmt.Errorf", body)
    body = body.replace("return 0, extractFailures, err", "return 0, extractFailures, typedFailures, err")
    body = body.replace("return stored, extractFailures, nil", "return stored, extractFailures, typedFailures, nil")
    store.write_text(s[:start] + body + s[end:])
    log.append("store.go: UpsertBatchDetailed added")


def patch_sync_honesty(cli):
    """4: never aggregate a run that stored nothing as a success."""
    s = cli.read_text()
    if "typedFailureTotal" in s:
        return

    s = s.replace('''func upsertResourceBatch(db *store.Store, resource string, items []json.RawMessage) (int, int, error) {
\tif _, ok := discriminatorDispatchers[resource]; !ok {
\t\treturn db.UpsertBatch(resource, items)
\t}''', '''// upsertResourceBatch returns (stored, extractFailures, typedFailures, err).
// typedFailures counts rows that landed a generic resources row but failed their
// typed-table projection; the caller MUST surface it, because a sync reporting
// rows while the typed tables stay empty is the silent success #208 was about.
func upsertResourceBatch(db *store.Store, resource string, items []json.RawMessage) (int, int, int, error) {
\tif _, ok := discriminatorDispatchers[resource]; !ok {
\t\treturn db.UpsertBatchDetailed(resource, items)
\t}''', 1)

    s = s.replace('''\tvar stored, extractFailures int
\tfor _, target := range order {
\t\ttargetStored, targetExtractFailures, err := db.UpsertBatch(target, grouped[target])
\t\tif err != nil {
\t\t\treturn stored, extractFailures + targetExtractFailures, err
\t\t}
\t\tstored += targetStored
\t\textractFailures += targetExtractFailures
\t}
\treturn stored, extractFailures, nil''', '''\tvar stored, extractFailures, typedFailures int
\tfor _, target := range order {
\t\ttargetStored, targetExtractFailures, targetTypedFailures, err := db.UpsertBatchDetailed(target, grouped[target])
\t\tif err != nil {
\t\t\treturn stored, extractFailures + targetExtractFailures, typedFailures + targetTypedFailures, err
\t\t}
\t\tstored += targetStored
\t\textractFailures += targetExtractFailures
\t\ttypedFailures += targetTypedFailures
\t}
\treturn stored, extractFailures, typedFailures, nil''', 1)

    s = s.replace("\t\tstored, extractFailures, err := upsertResourceBatch(db, resource, items)\n",
                  "\t\tstored, extractFailures, typedFailures, err := upsertResourceBatch(db, resource, items)\n", 1)

    s = s.replace("\tvar extractFailureTotal int\n\tvar hydrateFailureTotal int\n",
                  "\tvar extractFailureTotal int\n"
                  "\t// typedFailureTotal accumulates rows that landed a generic resources row\n"
                  "\t// but failed their typed-table projection (#208).\n"
                  "\tvar typedFailureTotal int\n"
                  "\tvar hydrateFailureTotal int\n", 1)

    anchor = "\t\tconsumedTotal += fetchedThisPage\n\t\textractFailureTotal += extractFailures + hydrateFailures"
    s = s.replace(anchor,
                  '\t\t// A row that landed only its generic resources row is NOT a synced row\n'
                  '\t\t// for any typed-table reader (devices health, applications hunt,\n'
                  '\t\t// analytics). Surface it rather than letting `stored` imply health.\n'
                  '\t\ttypedFailureTotal += typedFailures\n'
                  '\t\tif typedFailures > 0 && !humanFriendly {\n'
                  '\t\t\tfmt.Fprintf(syncEvents, `{"event":"sync_anomaly","resource":"%s","consumed":%d,"stored":%d,"typed_failures":%d,"reason":"typed_table_projection_failed"}`+"\\n", resource, fetchedThisPage, stored, typedFailures)\n'
                  '\t\t}\n' + anchor, 1)

    # An envelope that declares its own failure is not a natural end of enumeration.
    s = s.replace('''\t\t\tif isEmptyPageResponse(data, responsePathForResource(resource, path)...) {
\t\t\t\t// Natural end: the API legitimately returned an empty page.
\t\t\t\toutcome.complete = true
\t\t\t\tbreak
\t\t\t}''', '''\t\t\t// An envelope that declares its own failure ({"success":false,...}) is
\t\t\t// NOT a natural end: isEmptyPageResponse treats a null data field on a
\t\t\t// failed envelope as a legitimate empty page, which would mark the walk
\t\t\t// complete, advance the watermark and report success on an API error.
\t\t\tif responseReportsFailure(data) {
\t\t\t\toutcome.reason = "api_reported_failure"
\t\t\t\tif humanFriendly {
\t\t\t\t\tfmt.Fprintf(os.Stderr, "\\nwarning: %s: the API returned a failure envelope; no rows were stored.\\n", resource)
\t\t\t\t} else {
\t\t\t\t\tfmt.Fprintf(syncEvents, `{"event":"sync_anomaly","resource":"%s","reason":"api_reported_failure"}`+"\\n", resource)
\t\t\t\t}
\t\t\t\tbreak
\t\t\t}
\t\t\tif isEmptyPageResponse(data, responsePathForResource(resource, path)...) {
\t\t\t\t// Natural end: the API legitimately returned an empty page.
\t\t\t\toutcome.complete = true
\t\t\t\tbreak
\t\t\t}''', 1)

    s = s.replace("\nfunc envelopeReportsFailure(", '''
// responseReportsFailure reports whether a 200 body declares its own failure
// (e.g. {"success": false} or {"status": "error"}). The generated empty-page
// classifier treats a failed envelope with a null data field as a legitimate
// empty page, which would end the walk "complete" and report a clean sync on an
// API error. See skills/threatlocker/handfixes.json (sync-typed-table-honesty).
func responseReportsFailure(data []byte) bool {
\tvar envelope map[string]json.RawMessage
\tif json.Unmarshal(data, &envelope) != nil {
\t\treturn false
\t}
\treturn envelopeReportsFailure(envelope)
}

func envelopeReportsFailure(''', 1)

    # Report rows stored by THIS run, not the whole mirror.
    s = s.replace('fmt.Fprintf(syncEvents, `{"event":"sync_complete","resource":"%s","total":%d,"duration_ms":%d}`+"\\n", resource, cachedCount, time.Since(started).Milliseconds())',
                  'fmt.Fprintf(syncEvents, `{"event":"sync_complete","resource":"%s","stored":%d,"total":%d,"duration_ms":%d}`+"\\n", resource, totalCount, cachedCount, time.Since(started).Milliseconds())', 1)

    final_old = "\treturn syncResult{Resource: resource, Count: cachedCount, Duration: time.Since(started)}\n}"
    final_new = '''\t// totalCount is rows stored by THIS run; cachedCount is the whole mirror,
\t// including earlier syncs. Guarding on cachedCount would let a resource that
\t// stored nothing today pass because yesterday's rows are still there.
\t// The API returned rows and we stored none: always a defect, never data.
\tif consumedTotal > 0 && totalCount == 0 {
\t\treturn syncResult{
\t\t\tResource: resource,
\t\t\tCount:    0,
\t\t\tWarn:     fmt.Errorf("%s consumed %d items but stored 0 rows", resource, consumedTotal),
\t\t\tDuration: time.Since(started),
\t\t}
\t}

\t// An abnormal end to the walk (non-JSON 200, API-declared failure, stuck
\t// cursor) means the enumeration was never trustworthy.
\tif outcome.reason != "" && !outcome.complete {
\t\treturn syncResult{
\t\t\tResource: resource,
\t\t\tCount:    totalCount,
\t\t\tWarn:     fmt.Errorf("%s ended enumeration abnormally (%s); stored %d rows and the result may be incomplete", resource, outcome.reason, totalCount),
\t\t\tDuration: time.Since(started),
\t\t}
\t}

\tif typedFailureTotal > 0 {
\t\treturn syncResult{
\t\t\tResource: resource,
\t\t\tCount:    totalCount,
\t\t\tWarn:     fmt.Errorf("%s stored %d rows but %d failed their typed-table projection; offline features reading typed tables will be incomplete", resource, totalCount, typedFailureTotal),
\t\t\tDuration: time.Since(started),
\t\t}
\t}

\treturn syncResult{Resource: resource, Count: totalCount, Duration: time.Since(started)}
}'''
    assert final_old in s, "final return anchor missing"
    s = s.replace(final_old, final_new, 1)

    # A failed resume checkpoint must be classified critical like any state save.
    s = s.replace('fmt.Errorf("saving sync progress for %s: %w", resource, err)',
                  'fmt.Errorf("saving sync state for %s: %w", resource, err)', 1)

    # --no-prune advertised pruning no ThreatLocker resource can perform.
    s = s.replace('cmd.Flags().BoolVar(&noPrune, "no-prune", false, "Disable deletion reconciliation on --full (by default a full sync prunes local rows the API no longer returns for a fully-enumerated parent partition)")',
                  'cmd.Flags().BoolVar(&noPrune, "no-prune", false, "Disable deletion reconciliation on --full. '
                  'Note: no ThreatLocker resource declares the tenant partition that flat reconcile requires, so a full sync '
                  'currently re-fetches without pruning; rows deleted in the portal persist locally until the store is rebuilt")', 1)

    # Do not recommend a resource sync rejects.
    s = s.replace("threatlocker-cli sync --resources application-files,applications",
                  "threatlocker-cli sync --resources approvals,applications", 1)

    cli.write_text(s)
    log.append("sync.go: honesty patches applied")


def patch_archive(p):
    """6: workflow archive must not report every resource as synced regardless."""
    s = p.read_text()
    if "archiveOutcomeError" in s:
        return
    stale = re.search(r'\t+resources := \[\]string\{"[^\n]*\}\n', s)
    if stale:
        s = s.replace(stale.group(0),
                      '\t\t\t// One source of truth with `sync`. The generated literal here goes\n'
                      '\t\t\t// stale and carried a duplicate entry, the computer-groups dropdown\n'
                      '\t\t\t// twin and scheduled-actions.\n'
                      '\t\t\tresources := defaultSyncResources()\n', 1)

    s = s.replace('''\t\t\t\tif res.Warn != nil {
\t\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: warning: %v\\n", resource, res.Warn)
\t\t\t\t\tcontinue
\t\t\t\t}''', '''\t\t\t\tif res.Warn != nil {
\t\t\t\t\tarchivedWarned++
\t\t\t\t\t// Rows that DID land still count: page-cap and typed-projection
\t\t\t\t\t// warnings carry a positive Count, and dropping it would let the
\t\t\t\t\t// summary claim nothing was archived after rows were committed.
\t\t\t\t\ttotalSynced += res.Count
\t\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: warning: %v (%d archived)\\n", resource, res.Warn, res.Count)
\t\t\t\t\tcontinue
\t\t\t\t}''', 1)
    s = s.replace('''\t\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: error: %v\\n", resource, res.Err)
\t\t\t\t\tcontinue''', '''\t\t\t\t\tarchivedErrored++
\t\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: error: %v\\n", resource, res.Err)
\t\t\t\t\tcontinue''', 1)
    s = s.replace("\t\t\ttotalSynced := 0\n",
                  "\t\t\ttotalSynced := 0\n"
                  "\t\t\t// Outcomes are counted, not just printed: an archive that logged an\n"
                  "\t\t\t// error for every resource and then reported them all as synced is\n"
                  "\t\t\t// the same false success #208 was about, one command over.\n"
                  "\t\t\tarchivedOK, archivedWarned, archivedErrored := 0, 0, 0\n", 1)
    s = s.replace('''\t\t\t\ttotalSynced += res.Count
\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: %d synced\\n", resource, res.Count)''',
                  '''\t\t\t\tarchivedOK++
\t\t\t\ttotalSynced += res.Count
\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "  %s: %d synced\\n", resource, res.Count)''', 1)
    s = re.sub(r'return enc\.Encode\(map\[string\]any\{\n\t+"resources_synced": *len\(resources\),',
               'if err := enc.Encode(map[string]any{\n\t\t\t\t\t"resources_synced":  archivedOK,\n'
               '\t\t\t\t\t"resources_warned":  archivedWarned,\n\t\t\t\t\t"resources_errored": archivedErrored,', s, count=1)
    s = s.replace('''\t\t\t\t\t"timestamp":        time.Now().UTC().Format(time.RFC3339),
\t\t\t\t})''', '''\t\t\t\t\t"timestamp":        time.Now().UTC().Format(time.RFC3339),
\t\t\t\t}); err != nil {
\t\t\t\t\treturn err
\t\t\t\t}
\t\t\t\treturn archiveOutcomeError(archivedOK, archivedWarned, archivedErrored)''', 1)
    # Plain replace, not re.sub: a regex replacement string would interpret the
    # \n inside the format string as a real newline and break the literal.
    s = s.replace(
        'fmt.Fprintf(cmd.OutOrStdout(), "Archived %d items across %d resources to %s\\n", totalSynced, len(resources), dbPath)\n\t\t\treturn nil',
        'fmt.Fprintf(cmd.OutOrStdout(), "Archived %d items across %d of %d resources to %s\\n", totalSynced, archivedOK, len(resources), dbPath)\n'
        '\t\t\treturn archiveOutcomeError(archivedOK, archivedWarned, archivedErrored)', 1)
    # `workflow archive` walks the resource list sequentially, and the default
    # list now begins with the parent-keyed dependents. Ordering matters here for
    # the same reason it does in sync: a dependent fans out over ids already in
    # the store, so running it first finds an empty (or yesterday's) parent table
    # and silently omits every child of a parent discovered later in the run.
    s = s.replace("\t\t\tresources := defaultSyncResources()\n",
                  "\t\t\tresources := orderDependentsLast(defaultSyncResources())\n", 1)

    s = s.rstrip("\n") + '''

// orderDependentsLast puts parent-keyed resources after the flat ones. `sync`
// gets this from its two-wave scheduler; `workflow archive` walks the list
// sequentially, so it needs the ordering baked into the slice.
func orderDependentsLast(resources []string) []string {
\tflat := make([]string, 0, len(resources))
\tdependent := make([]string, 0, len(resources))
\tfor _, r := range resources {
\t\tif _, ok := dependentSyncSpecs[r]; ok {
\t\t\tdependent = append(dependent, r)
\t\t\tcontinue
\t\t}
\t\tflat = append(flat, r)
\t}
\treturn append(flat, dependent...)
}

// archiveOutcomeError converts per-resource outcomes into the command's exit
// status; the generated body returned nil unconditionally. Dogfood deliberately
// caps the walk at one page, so paginated resources legitimately warn there and
// gating on that would turn an intentional smoke-test shortcut into a red run.
func archiveOutcomeError(ok, warned, errored int) error {
\tif cliutil.IsDogfoodEnv() {
\t\treturn nil
\t}
\tif ok == 0 && (warned > 0 || errored > 0) {
\t\treturn fmt.Errorf("archive stored nothing: %d resource(s) errored, %d warned", errored, warned)
\t}
\tif errored > 0 || warned > 0 {
\t\treturn fmt.Errorf("archive incomplete: %d resource(s) archived, %d errored, %d warned", ok, errored, warned)
\t}
\treturn nil
}
'''
    p.write_text(s)
    log.append("channel_workflow.go: outcomes gate the exit status")


STORE_REGISTER_HOOK = (
    "\n\n"
    "// RegisterDependentKey declares a parent-keyed resource at init time: idField\n"
    "// is the child row's own key and parentColumn is the field carrying its parent\n"
    "// id, which resourceStorageID appends so a child that is only unique WITHIN\n"
    "// its parent (a maintenance window on a computer) still gets a unique storage\n"
    "// key. Hand-added for the query-param dependents the profiler cannot detect;\n"
    "// see internal/cli/sync_dependents.go and skills/threatlocker/handfixes.json\n"
    "// (sync-dependent-fanout).\n"
    "func RegisterDependentKey(resource, idField, parentColumn string) {\n"
    "\tif idField != \"\" {\n"
    "\t\tresourceIDFieldOverrides[resource] = idField\n"
    "\t}\n"
    "\tif parentColumn != \"\" {\n"
    "\t\tresourceParentKeyColumns[resource] = []string{parentColumn}\n"
    "\t}\n"
    "}"
)

SYNC_DEPENDENT_DISPATCH = (
    "\n"
    "\t// Parent-keyed resources cannot be walked flat: their collection endpoint\n"
    "\t// is scoped by a required QUERY parameter the generated loop never sends,\n"
    "\t// so the API rejects every call. See sync_dependents.go.\n"
    "\tif spec, ok := dependentSyncSpecs[resource]; ok {\n"
    "\t\tif !humanFriendly {\n"
    "\t\t\tfmt.Fprintf(syncEvents, `{\"event\":\"sync_start\",\"resource\":\"%s\"}`+\"\\n\", resource)\n"
    "\t\t}\n"
    "\t\treturn syncDependentResource(ctx, c, db, resource, spec, maxPages, syncEvents, started)\n"
    "\t}\n"
)


def patch_dependents(cli_dir, store):
    """Wire the parent-keyed fan-out (sync_dependents.go) into generated code."""
    s = store.read_text()
    if "func RegisterDependentKey(" not in s:
        anchor = "var resourceParentKeyColumns = map[string][]string{}"
        assert anchor in s, "resourceParentKeyColumns anchor missing"
        store.write_text(s.replace(anchor, anchor + STORE_REGISTER_HOOK, 1))
        log.append("store.go: RegisterDependentKey added")

    sync = cli_dir / "sync.go"
    s = sync.read_text()
    if "syncDependentResource(" in s:
        return
    anchor = "\tstarted := time.Now()\n\tif syncEvents == nil {\n\t\tsyncEvents = io.Discard\n\t}\n"
    assert anchor in s, "syncResource preamble anchor missing"
    s = s.replace(anchor, anchor + SYNC_DEPENDENT_DISPATCH, 1)

    # The worker pool consumes one unordered channel, so a dependent could run
    # before the parent whose ids it fans out over. Enqueue in two waves.
    feed_old = ("\t\t\t// Enqueue all resources\n"
                "\t\t\tfor _, resource := range resources {\n"
                "\t\t\t\twork <- resource\n"
                "\t\t\t}\n"
                "\t\t\tclose(work)\n")
    feed_new = ('\t\t\t// Enqueue in two waves: parent-keyed resources fan out over ids\n'
                '\t\t\t// already in the store, so a dependent scheduled alongside its\n'
                '\t\t\t// parent would find an empty parent table and warn. Flat\n'
                '\t\t\t// resources drain first, then the dependents.\n'
                '\t\t\tvar flatWave, dependentWave []string\n'
                '\t\t\tfor _, resource := range resources {\n'
                '\t\t\t\tif _, ok := dependentSyncSpecs[resource]; ok {\n'
                '\t\t\t\t\tdependentWave = append(dependentWave, resource)\n'
                '\t\t\t\t\tcontinue\n'
                '\t\t\t\t}\n'
                '\t\t\t\tflatWave = append(flatWave, resource)\n'
                '\t\t\t}\n'
                '\t\t\tgo func() {\n'
                '\t\t\t\tfor _, resource := range flatWave {\n'
                '\t\t\t\t\twork <- resource\n'
                '\t\t\t\t}\n'
                '\t\t\t\tif len(dependentWave) > 0 {\n'
                '\t\t\t\t\tdependentBarrier.Wait()\n'
                '\t\t\t\t\tfor _, resource := range dependentWave {\n'
                '\t\t\t\t\t\twork <- resource\n'
                '\t\t\t\t\t}\n'
                '\t\t\t\t}\n'
                '\t\t\t\tclose(work)\n'
                '\t\t\t}()\n')
    assert feed_old in s, "work-feed anchor missing"
    s = s.replace(feed_old, feed_new, 1)

    # The barrier a worker signals once the flat wave is fully drained.
    pool_old = ("\t\t\t\t\tfor resource := range work {\n"
                "\t\t\t\t\t\tres := syncResource(cmd.Context(), c, db, resource, sinceTS, full, maxPages, effectiveLatestOnly, prune, userParams, syncEventWriter)\n"
                "\t\t\t\t\t\tresults <- res\n"
                "\t\t\t\t\t}\n")
    pool_new = ("\t\t\t\t\tfor resource := range work {\n"
                "\t\t\t\t\t\tres := syncResource(cmd.Context(), c, db, resource, sinceTS, full, maxPages, effectiveLatestOnly, prune, userParams, syncEventWriter)\n"
                "\t\t\t\t\t\tif _, isDependent := dependentSyncSpecs[resource]; !isDependent {\n"
                "\t\t\t\t\t\t\t// A parent that errored or warned leaves a partial table,\n"
                "\t\t\t\t\t\t\t// and a dependent fanning out over it would report clean\n"
                "\t\t\t\t\t\t\t// success over a subset. Record the outcome so the\n"
                "\t\t\t\t\t\t\t// dependent wave can inherit it.\n"
                "\t\t\t\t\t\t\tif res.Err != nil || res.Warn != nil {\n"
                "\t\t\t\t\t\t\t\tparentOutcomes.Store(resource, true)\n"
                "\t\t\t\t\t\t\t}\n"
                "\t\t\t\t\t\t\tdependentBarrier.Done()\n"
                "\t\t\t\t\t\t} else if spec, ok := dependentSyncSpecs[resource]; ok && res.Err == nil {\n"
                "\t\t\t\t\t\t\tif _, degraded := parentOutcomes.Load(spec.parent); degraded {\n"
                "\t\t\t\t\t\t\t\twarn := fmt.Errorf(\"%s fanned out over a partial %s table (its sync did not complete cleanly); some children may be missing\", resource, spec.parent)\n"
                "\t\t\t\t\t\t\t\tif res.Warn != nil {\n"
                "\t\t\t\t\t\t\t\t\twarn = fmt.Errorf(\"%w; %v\", warn, res.Warn)\n"
                "\t\t\t\t\t\t\t\t}\n"
                "\t\t\t\t\t\t\t\tres.Warn = warn\n"
                "\t\t\t\t\t\t\t}\n"
                "\t\t\t\t\t\t}\n"
                "\t\t\t\t\t\tresults <- res\n"
                "\t\t\t\t\t}\n")
    assert pool_old in s, "worker-pool anchor missing"
    s = s.replace(pool_old, pool_new, 1)

    barrier_anchor = "\t\t\twork := make(chan string, len(resources))\n"
    barrier_new = (barrier_anchor +
                   "\t\t\t// Records flat resources whose sync did not complete cleanly, so a\n"
                   "\t\t\t// dependent fanning out over a partial parent table inherits the\n"
                   "\t\t\t// warning instead of reporting success over a subset.\n"
                   "\t\t\tvar parentOutcomes sync.Map\n"
                   "\t\t\t// Released once every flat resource has finished, so the\n"
                   "\t\t\t// dependent wave sees a populated parent table.\n"
                   "\t\t\tvar dependentBarrier sync.WaitGroup\n"
                   "\t\t\tfor _, resource := range resources {\n"
                   "\t\t\t\tif _, ok := dependentSyncSpecs[resource]; !ok {\n"
                   "\t\t\t\t\tdependentBarrier.Add(1)\n"
                   "\t\t\t\t}\n"
                   "\t\t\t}\n")
    assert barrier_anchor in s, "work-channel anchor missing"
    s = s.replace(barrier_anchor, barrier_new, 1)

    # The profiler marks parent-keyed resources skip-by-default (it has no way
    # to supply their parent key). The fan-out can, so put them back in the
    # default set; the two-wave scheduler guarantees their parents run first.
    defaults_anchor = 'func defaultSyncResources() []string {\n\treturn []string{\n'
    if defaults_anchor in s and '"application-files",' not in s.split(defaults_anchor, 1)[1].split("}", 1)[0]:
        s = s.replace(defaults_anchor,
                      defaults_anchor + '\t\t"application-files",\n\t\t"maintenance",\n', 1)
        log.append("sync.go: dependents added to the default sync set")

    sync.write_text(s)
    log.append("sync.go: dependent fan-out dispatch + two-wave scheduling added")


def patch_tree_flags(base):
    """5: a body field with a spec default must reach the API unconditionally."""
    for fname, (flag, field) in TREE_FLAGS.items():
        p = base / fname
        if not p.exists():
            continue
        src = p.read_text()
        guarded = f'\t\t\t\tif cmd.Flags().Changed("{flag}") {{\n\t\t\t\t\tbodyMap["{field}"] = body'
        idx = src.find(guarded)
        if idx == -1:
            continue
        tail = src[idx:]
        end = tail.index("\n\t\t\t\t}\n") + len("\n\t\t\t\t}\n")
        var = tail[:end].split(f'bodyMap["{field}"] = ')[1].split("\n")[0].strip()
        src = src[:idx] + (
            f'\t\t\t\t// Always sent: --help advertises a default for --{flag}, so the\n'
            f'\t\t\t\t// value must reach the API even when the user did not type the\n'
            f'\t\t\t\t// flag. Without this a bare call returns one organization\n'
            f'\t\t\t\t// instead of the whole managed tree (#208).\n'
            f'\t\t\t\tbodyMap["{field}"] = {var}\n'
        ) + src[idx + end:]
        p.write_text(src)
        log.append(f"{fname}: {field} always sent")


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    C = pathlib.Path(sys.argv[1])
    cli_dir = C / "internal/cli"
    patch_sync_surface([cli_dir / "sync.go", cli_dir / "channel_workflow.go", C / "internal/store/store.go"])
    patch_store(C / "internal/store/store.go")
    patch_sync_honesty(cli_dir / "sync.go")
    patch_archive(cli_dir / "channel_workflow.go")
    patch_dependents(cli_dir, C / "internal/store/store.go")
    patch_tree_flags(cli_dir)
    for line in log:
        print("  " + line)
    print(f"  {len(log)} patch(es) applied")


if __name__ == "__main__":
    main()
