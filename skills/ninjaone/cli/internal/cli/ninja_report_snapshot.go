// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ninjaone-pp-cli/internal/store"
)

// Class report-row-identity (hand-fix; reapply/snapshot.py wires it into the flat sync loop).
//
// A /v2/queries/* report is a snapshot of the fleet with no since window: a walk that starts
// at page one and reaches the natural end, unfiltered, is the whole report. Its rows are keyed
// on the identity the row schema documents, or, where it documents none, on the row's ordinal
// among its device's rows (internal/store/ninja_row_identity.go), so a key moves when
// NinjaOne's answer moves and a row NinjaOne stops returning would otherwise stay forever.
//
// One ninjaReportWalk follows one syncResource walk. It stamps the ordinals before a page is
// stored and records the storage ids the page was stored under (both derived from the same
// stamped rows, so the upsert and the seen set cannot disagree). After the walk, on every
// sync (ordinary or --full) unless --no-prune, it replaces the partition: every row whose
// full storage id the walk did not see is removed. Any sign that the walk was not the whole
// report skips the replace and says why on the event stream: a row-narrowing user parameter,
// since window or tenant scope (syncPruneSkipReason; a parameter ninjaReportWalkParams
// classifies as row-preserving, such as pageSize, is not a filter), a walk resumed from a
// saved cursor, an incomplete walk (a --max-pages cap with more pages, a stuck or unprovable
// cursor, a non-JSON page), or a page that was stored as a single object rather than report
// rows. A walk that fails on a page returns before the replace. The generated reconcile cannot
// do this: it compares bare entity ids, which keeps every stale row of a device the walk saw.
// Partitions already flat-reconcilable keep the generated reconcile.
//
// A report keyed by row ordinal (position-keyed rows) is written only by a walk that also
// replaces the whole report, atomically. A row-narrowing walk of one is refused before any
// request (ninja_report_params.go). Every other walk stages its stamped pages in a 0600 file
// beside the store (one page in memory at a time; a report can be tens of thousands of rows)
// and writes nothing to the partition while it walks. Only a walk that proved it was the whole
// report (complete, from page one, only row-preserving parameters, no since window or tenant
// scope, every page report rows) is committed, by store.ReplaceReportRows in ONE transaction:
// the staged rows are stored and every other row of the partition, shipped-key rows included,
// is removed. Any other walk (capped, failed, stuck, timed out, a non-JSON or single-object
// page) writes nothing: the stored rows stay exactly as they were, reconcile_skipped names the
// reason and report_not_stored says so. A position-keyed row has no identity to keep, so
// --no-prune does not apply to these reports: a whole walk replaces them anyway and says so
// (no_prune_not_applicable), because keeping a row at a position the report no longer reaches
// would show a phantom or a second copy of a moved row. Every other report keeps --no-prune.

func ninjaSnapshotPartition(resource string) bool {
	if resourceIsFlatReconcilable(resourceReconcileMode(resource)) {
		return false
	}
	p, err := syncResourcePath(resource)
	return err == nil && strings.HasPrefix(p, "/v2/queries/")
}

// ninjaReportResumeCursor is the cursor a walk of resource resumes from. A report keyed by row
// ordinal always walks from page one: a walk resumed mid-report would number each device's rows
// from the resume point and store them over that device's earlier rows.
func ninjaReportResumeCursor(resource, cursor string) string {
	if store.ReportRowsAreOrdinal(resource) {
		return ""
	}
	return cursor
}

type ninjaReportWalk struct {
	resource    string
	snapshot    bool
	ordinal     bool
	fromPageOne bool
	ordinals    *store.ReportRowOrdinals
	seen        []string
	spool       *os.File
	nonReport   bool
}

// newNinjaReportWalk starts the bookkeeping of one walk of resource that resumes from cursor
// ("" = page one).
func newNinjaReportWalk(resource, cursor string) *ninjaReportWalk {
	w := &ninjaReportWalk{resource: resource, snapshot: ninjaSnapshotPartition(resource), fromPageOne: cursor == ""}
	if w.snapshot {
		w.ordinals = store.NewReportRowOrdinals(resource)
		w.ordinal = w.ordinals != nil
	}
	return w
}

// stamp numbers an ordinal report's rows before the page is stored.
func (w *ninjaReportWalk) stamp(items []json.RawMessage) []json.RawMessage {
	return w.ordinals.Stamp(items)
}

// upsert stores one page of the walk, with upsertResourceBatch's counts. An ordinal report's
// page is staged instead (commit stores it): a stamped row is a row the walk stores, and a row
// without a stamp has no device to key it on, so it counts as a primary-key miss and fails the
// walk as one.
func (w *ninjaReportWalk) upsert(db *store.Store, items []json.RawMessage) (int, int, int, error) {
	if !w.ordinal {
		return upsertResourceBatch(db, w.resource, items)
	}
	rows := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		if o, err := store.DecodeJSONObject(it); err == nil && o[store.ReportRowOrdinalField] != nil && store.StorageIDFor(w.resource, o) != "" {
			rows = append(rows, it)
		}
	}
	if len(rows) == 0 {
		return 0, len(items), 0, nil
	}
	if w.spool == nil {
		dir := filepath.Dir(db.Path())
		removeStaleReportSpools(dir)
		f, err := os.CreateTemp(dir, ninjaReportSpoolPattern)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("spooling the %s walk: %w", w.resource, err)
		}
		w.spool = f
	}
	line, err := json.Marshal(rows)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("spooling the %s walk: %w", w.resource, err)
	}
	if _, err := w.spool.Write(append(line, '\n')); err != nil {
		return 0, 0, 0, fmt.Errorf("spooling the %s walk: %w", w.resource, err)
	}
	return len(rows), len(items) - len(rows), 0, nil
}

// upsertSingleObject stores a page that carried no report rows. An ordinal report stores
// nothing from it: the walk is not the report, so commit stores none of it.
func (w *ninjaReportWalk) upsertSingleObject(db *store.Store, data json.RawMessage) error {
	if w.ordinal {
		w.nonReport = true
		return nil
	}
	return upsertSingleObject(db, w.resource, data)
}

// see records the storage ids of one stored page.
func (w *ninjaReportWalk) see(items []json.RawMessage) {
	if !w.snapshot {
		return
	}
	for _, it := range items {
		if o, err := store.DecodeJSONObject(it); err == nil {
			if id := store.StorageIDFor(w.resource, o); id != "" {
				w.seen = append(w.seen, id)
			}
		}
	}
}

// close removes the spool; syncResource defers it, so a walk that returns early leaves nothing.
func (w *ninjaReportWalk) close() {
	if w.spool == nil {
		return
	}
	name := w.spool.Name()
	_ = w.spool.Close()
	_ = os.Remove(name)
	w.spool = nil
}

// ninjaReportSpoolPattern names a walk's staging file (os.CreateTemp: mode 0600).
const ninjaReportSpoolPattern = ".ninjaone-report-walk-*.jsonl"

// removeStaleReportSpools removes staging files a killed walk left beside the store. A live
// walk writes its file on every page, so only a file untouched for a day is removed.
func removeStaleReportSpools(dir string) {
	matches, _ := filepath.Glob(filepath.Join(dir, ninjaReportSpoolPattern))
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			_ = os.Remove(m)
		}
	}
}

// commit replaces the stored report with the staged rows of a whole ordinal report walk, in
// one transaction (store.ReplaceReportRows), streaming the staging file a page at a time.
func (w *ninjaReportWalk) commit(db *store.Store) (int, int, error) {
	var r *bufio.Reader
	if w.spool != nil {
		if _, err := w.spool.Seek(0, io.SeekStart); err != nil {
			return 0, 0, fmt.Errorf("reading the staged %s walk: %w", w.resource, err)
		}
		r = bufio.NewReader(w.spool)
	}
	next := func() ([]json.RawMessage, bool, error) {
		for r != nil {
			line, err := r.ReadBytes('\n')
			if err != nil && err != io.EOF {
				return nil, false, fmt.Errorf("reading the staged %s walk: %w", w.resource, err)
			}
			if err == io.EOF {
				r = nil
			}
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var rows []json.RawMessage
			if uerr := json.Unmarshal(line, &rows); uerr != nil {
				return nil, false, fmt.Errorf("reading the staged %s walk: %w", w.resource, uerr)
			}
			return rows, true, nil
		}
		return nil, false, nil
	}
	return db.ReplaceReportRows(w.resource, reconcileTypedTable(w.resource), next)
}

// reconcile ends the walk of a report snapshot partition and reports the decision on the sync
// event stream. An ordinal report is replaced by a whole walk (commit, one transaction) and
// otherwise left exactly as it was (warn says so); prune does not apply to it. Any other report
// is replaced, when prune (every sync unless --no-prune), by a complete, unfiltered walk from
// page one. stored is every row the walk stored or staged; a row no report page carried means
// the walk was not the report. err is a failed commit: the transaction rolled back.
func (w *ninjaReportWalk) reconcile(db *store.Store, prune, complete bool, incompleteReason, skipReason string, stored int, syncEvents io.Writer) (warn, err error) {
	reason := skipReason
	switch {
	case reason != "":
	case !w.fromPageOne:
		reason = "resumed_walk"
	case !complete:
		reason = incompleteReason
		if reason == "" {
			reason = "incomplete_walk"
		}
	case stored != len(w.seen) || w.nonReport:
		reason = "non_report_page"
	}
	if w.ordinal {
		if reason != "" {
			fmt.Fprintf(syncEvents, `{"event":"reconcile_skipped","resource":"%s","scope":"snapshot","reason":%q}`+"\n", w.resource, reason)
			warn = fmt.Errorf("%s was not stored: the walk was not the whole report (%s), and its rows are keyed by their position in the whole report; the stored report is unchanged", w.resource, reason)
			if !humanFriendly {
				fmt.Fprintln(syncEvents, syncWarningJSON(w.resource, "", 0, "report_not_stored", warn.Error()))
			}
			return warn, nil
		}
		if !prune {
			msg := fmt.Sprintf("--no-prune does not apply to %s: its rows have no id of their own, only a position in the whole report, so this whole walk replaced the stored report", w.resource)
			if humanFriendly {
				fmt.Fprintf(os.Stderr, "  %s: note: %s\n", w.resource, msg)
			} else {
				fmt.Fprintln(syncEvents, syncWarningJSON(w.resource, "", 0, "no_prune_not_applicable", msg))
			}
		}
		_, deleted, cerr := w.commit(db)
		if cerr != nil {
			fmt.Fprintf(syncEvents, `{"event":"reconcile_error","resource":"%s","scope":"snapshot","error":%q}`+"\n", w.resource, cerr.Error())
			return nil, fmt.Errorf("replacing %s (rolled back; the stored report is unchanged): %w", w.resource, cerr)
		}
		fmt.Fprintf(syncEvents, `{"event":"reconcile","resource":"%s","scope":"snapshot","deleted":%d}`+"\n", w.resource, deleted)
		return nil, nil
	}
	if !prune {
		return nil, nil
	}
	if reason != "" {
		fmt.Fprintf(syncEvents, `{"event":"reconcile_skipped","resource":"%s","scope":"snapshot","reason":%q}`+"\n", w.resource, reason)
		return nil, nil
	}
	deleted, rerr := db.ReconcileSnapshot(w.resource, w.seen, reconcileTypedTable(w.resource))
	if rerr != nil {
		fmt.Fprintf(syncEvents, `{"event":"reconcile_error","resource":"%s","scope":"snapshot","error":%q}`+"\n", w.resource, rerr.Error())
		return fmt.Errorf("reconciling %s: %w", w.resource, rerr), nil
	}
	fmt.Fprintf(syncEvents, `{"event":"reconcile","resource":"%s","scope":"snapshot","deleted":%d}`+"\n", w.resource, deleted)
	return nil, nil
}

// ninjaArchiveNoPrune is the noPrune `workflow archive` walks resource with. Archive never
// prunes, and an ordinal report is replaced by a whole walk whatever noPrune says (reconcile);
// archive passes false for one so its run does not report a --no-prune it was never given.
func ninjaArchiveNoPrune(resource string) bool {
	return !store.ReportRowsAreOrdinal(resource)
}
