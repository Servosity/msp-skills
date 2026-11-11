// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-forbidden-not-partial): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_outcome_test.go.tmpl after a reprint.

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/store"
)

func TestSyncForbiddenKeyOnlyForAnOutrightRefusal(t *testing.T) {
	forbidden := &accessWarning{Status: 403, Reason: "forbidden"}
	if got := syncForbiddenKey(forbidden, 0, "acme"); got != "acme" {
		t.Fatalf("403 on the first page = %q, want the key", got)
	}
	if got := syncForbiddenKey(forbidden, 1, "acme"); got != "" {
		t.Fatalf("403 after a page was read = %q: the rows read so far are a partial walk", got)
	}
	if got := syncForbiddenKey(&accessWarning{Status: 400, Reason: "argument_missing"}, 0, "acme"); got != "" {
		t.Fatalf("400 = %q: the resource is readable with the right argument", got)
	}
}

func TestSyncOutcomesNameWhatWasSkippedAndWhatFailed(t *testing.T) {
	o := &syncOutcomes{skippedForbidden: []string{"tags", "overviews"}, failed: []string{"tickets"}}
	want := `,"skipped_forbidden":["overviews","tags"],"failed":["tickets"]`
	if got := o.summaryJSON(); got != want {
		t.Fatalf("summary fields = %s, want %s", got, want)
	}
	if got := (&syncOutcomes{}).summaryJSON(); got != "" {
		t.Fatalf("a clean run added %s to its summary", got)
	}
}

// syncRefusingFake answers every read with one HTTP status.
type syncRefusingFake struct {
	status int
	calls  int
}

func (f *syncRefusingFake) reply() error {
	f.calls++
	return &client.APIError{StatusCode: f.status, Body: `{"message":"refused for Acme Corp"}`}
}

func (f *syncRefusingFake) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return nil, f.reply()
}

func (f *syncRefusingFake) PostWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error) {
	return nil, f.status, f.reply()
}

func (f *syncRefusingFake) PostQueryWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error) {
	return nil, f.status, f.reply()
}

func (f *syncRefusingFake) RateLimit() float64 { return 0 }

// TestSyncForbiddenResourceIsSkippedThenReattempted walks a real resource: a
// 403 records a skip and leaves nothing incomplete, and a later attempt that
// errors takes the skip back, so an error can never read as a skip.
func TestSyncForbiddenResourceIsSkippedThenReattempted(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	for _, resource := range defaultSyncResources() {
		refused := &syncRefusingFake{status: 403}
		res := syncResource(context.Background(), refused, db, resource, "", false, 0, false, false, &syncUserParams{}, io.Discard)
		if refused.calls == 0 {
			continue // needs parent context or a required argument: never reached the vendor
		}
		if res.Err != nil || res.ForbiddenKey == "" {
			t.Fatalf("%s: a 403 gave Err=%v ForbiddenKey=%q, want a recorded skip", resource, res.Err, res.ForbiddenKey)
		}
		outcomes := &syncOutcomes{}
		outcomes.note(db, res, false)
		got, err := db.SyncCoverage()
		if err != nil {
			t.Fatalf("reading sync coverage: %v", err)
		}
		if want := []string{res.ForbiddenKey}; !reflect.DeepEqual(got.SkippedForbidden, want) || len(got.Incomplete) != 0 {
			t.Fatalf("%s after a 403: coverage = %+v, want skipped %v and nothing incomplete", resource, got, want)
		}
		if !strings.Contains(outcomes.summaryJSON(), resource) {
			t.Fatalf("%s: the summary does not name the skip: %s", resource, outcomes.summaryJSON())
		}

		failing := &syncRefusingFake{status: 500}
		res = syncResource(context.Background(), failing, db, resource, "", false, 0, false, false, &syncUserParams{}, io.Discard)
		if res.Err == nil {
			t.Fatalf("%s: a 500 did not fail the resource", resource)
		}
		outcomes = &syncOutcomes{}
		outcomes.note(db, res, false)
		got, err = db.SyncCoverage()
		if err != nil {
			t.Fatalf("reading sync coverage: %v", err)
		}
		if len(got.SkippedForbidden) != 0 || len(got.Incomplete) != 1 {
			t.Fatalf("%s after a 500: coverage = %+v, want one incomplete resource and no skip", resource, got)
		}
		if !strings.Contains(outcomes.summaryJSON(), `"failed":["`+resource+`"]`) {
			t.Fatalf("%s: the summary does not name the failure: %s", resource, outcomes.summaryJSON())
		}
		return
	}
	t.Skip("no default resource reaches the vendor without parent context")
}
