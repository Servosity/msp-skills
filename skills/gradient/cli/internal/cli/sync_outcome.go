// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"gradient-pp-cli/internal/store"
)

// syncForbiddenKey returns the sync_state key to record as skipped when the
// vendor refused this token outright: HTTP 403 before any page was read. A
// refusal part way through a walk, or a 400 that only looks like one, is not
// a skip, so that resource stays partial.
// Hand-fix (handfixes.json sync-forbidden-not-partial);
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/sync_outcome.go.tmpl after a reprint.
func syncForbiddenKey(w *accessWarning, pagesFetched int, stateKey string) string {
	if w != nil && w.Status == http.StatusForbidden && pagesFetched == 0 {
		return stateKey
	}
	return ""
}

// syncOutcomes names, for one run, the resources the token may not read and
// the ones that failed, so a sync that exits 0 still says which is which.
type syncOutcomes struct {
	skippedForbidden []string
	failed           []string
}

// note records one finished resource. Only the results loop calls it.
func (o *syncOutcomes) note(db *store.Store, res syncResult, dryRun bool) {
	if res.Err != nil {
		o.failed = append(o.failed, res.Resource)
		return
	}
	if res.ForbiddenKey == "" {
		return
	}
	o.skippedForbidden = append(o.skippedForbidden, res.Resource)
	if dryRun {
		return
	}
	if err := db.MarkSyncForbidden(res.ForbiddenKey); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s stays partial in the local store: recording its forbidden skip failed: %v\n", res.Resource, err)
	}
}

// fields returns the summary members this run has, sorted because workers
// finish in any order.
func (o *syncOutcomes) fields() map[string][]string {
	out := map[string][]string{}
	if len(o.skippedForbidden) > 0 {
		sort.Strings(o.skippedForbidden)
		out["skipped_forbidden"] = o.skippedForbidden
	}
	if len(o.failed) > 0 {
		sort.Strings(o.failed)
		out["failed"] = o.failed
	}
	return out
}

// summaryJSON is the sync_summary event's added members, or "".
func (o *syncOutcomes) summaryJSON() string {
	var b strings.Builder
	fields := o.fields()
	for _, key := range []string{"skipped_forbidden", "failed"} {
		if names, ok := fields[key]; ok {
			encoded, _ := json.Marshal(names)
			fmt.Fprintf(&b, `,"%s":%s`, key, encoded)
		}
	}
	return b.String()
}

func (o *syncOutcomes) addTo(summary map[string]any) {
	for key, names := range o.fields() {
		summary[key] = names
	}
}

func (o *syncOutcomes) printHuman() {
	fields := o.fields()
	if names, ok := fields["skipped_forbidden"]; ok {
		fmt.Fprintf(os.Stderr, "Skipped (this token is forbidden to read them): %s\n", strings.Join(names, ", "))
	}
	if names, ok := fields["failed"]; ok {
		fmt.Fprintf(os.Stderr, "Failed: %s\n", strings.Join(names, ", "))
	}
}
