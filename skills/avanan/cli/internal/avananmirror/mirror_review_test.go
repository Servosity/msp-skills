// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package avananmirror

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"avanan-pp-cli/internal/store"
)

// TestExceptionSourcesCoverTheDocumentedExcTypeEnum: GET
// /v1.0/exceptions/{exc_type} documents an enum (spec.json), and
// spam_blacklist was never requested, so `exceptions find` answered "not
// excepted anywhere" for a spam-blacklisted sender. The source list is checked
// against the documented enum so a new list type cannot be silently skipped.
func TestExceptionSourcesCoverTheDocumentedExcTypeEnum(t *testing.T) {
	// The exc_type enum documented for GET /v1.0/exceptions/{exc_type} in the
	// press spec (not vendored here, so the list is carried in the test).
	enum := []string{"whitelist", "blacklist", "spam_whitelist", "spam_blacklist"}
	paths := map[string]bool{}
	for _, s := range ExceptionSources() {
		paths[s.Path] = true
	}
	for _, v := range enum {
		if !paths["/v1.0/exceptions/"+v] {
			t.Errorf("documented exception list %q is never mirrored", v)
		}
	}
}

func envelope(code int, data string) string {
	b, _ := json.Marshal(code)
	return `{"responseEnvelope":{"responseCode":` + string(b) + `,"scrollId":""},"responseData":` + data + `}`
}

// TestEntitiesReportsAPartialFanOut: one platform failing with a documented
// failure envelope while another returned rows was discarded, and the caller
// stamped the entity mirror complete. Only a positively identified unlicensed
// platform may be skipped.
func TestEntitiesReportsAPartialFanOut(t *testing.T) {
	responses := make([]string, len(EntitySaaS))
	for i := range responses {
		responses[i] = envelope(200, `[]`)
	}
	responses[0] = envelope(200, `[{"entityInfo":{"entityId":"e1"}}]`)
	responses[1] = envelope(500, `[]`)
	p := &fakePoster{responses: responses}
	if _, err := Entities(context.Background(), p, openTestStore(t), Options{}); err == nil {
		t.Error("Entities() returned nil with one platform failing on a 500 envelope; the mirror would be stamped complete")
	}

	// An unlicensed platform (403, ledger mirror-fails-when-nothing-was-mirrored)
	// is still tolerated.
	responses[1] = envelope(403, `[]`)
	p = &fakePoster{responses: responses}
	if _, err := Entities(context.Background(), p, openTestStore(t), Options{}); err != nil {
		t.Errorf("Entities() failed on an unlicensed (403) platform: %v", err)
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// listPoster serves a fixed body per exception path.
type listPoster struct{ byPath map[string]string }

func (l *listPoster) Post(context.Context, string, any) (json.RawMessage, int, error) {
	return nil, 0, nil
}

func (l *listPoster) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	if b, ok := l.byPath[path]; ok {
		return json.RawMessage(b), nil
	}
	return json.RawMessage(envelope(200, `[]`)), nil
}

// TestExceptionsDropEntriesTheAPINoLongerLists: ingestion only upserted, so an
// exception deleted upstream stayed in the local inventory after a fresh,
// complete mirror and `exceptions find` kept reporting it.
func TestExceptionsDropEntriesTheAPINoLongerLists(t *testing.T) {
	db := openTestStore(t)
	p := &listPoster{byPath: map[string]string{
		"/v1.0/exceptions/whitelist": envelope(200, `[{"entityId":"x1","senderDomain":"bad.example"}]`),
	}}
	opts := Options{Scopes: []string{"farm:a"}}
	if _, problems := Exceptions(context.Background(), p, db, opts); len(problems) != 0 {
		t.Fatalf("first ingest: %v", problems)
	}
	if n, _ := db.Count(ResourceExceptions); n != 1 {
		t.Fatalf("after first ingest: %d rows, want 1", n)
	}
	p.byPath["/v1.0/exceptions/whitelist"] = envelope(200, `[]`)
	if _, problems := Exceptions(context.Background(), p, db, opts); len(problems) != 0 {
		t.Fatalf("second ingest: %v", problems)
	}
	if n, _ := db.Count(ResourceExceptions); n != 0 {
		t.Errorf("after the API stopped listing it: %d rows remain, want 0", n)
	}

	// A partition that FAILED must keep its rows: absence of an answer is not
	// absence of the exception.
	p.byPath["/v1.0/exceptions/whitelist"] = envelope(200, `[{"entityId":"x1","senderDomain":"bad.example"}]`)
	if _, problems := Exceptions(context.Background(), p, db, opts); len(problems) != 0 {
		t.Fatalf("third ingest: %v", problems)
	}
	p.byPath["/v1.0/exceptions/whitelist"] = envelope(500, `[]`)
	_, _ = Exceptions(context.Background(), p, db, opts)
	if n, _ := db.Count(ResourceExceptions); n != 1 {
		t.Errorf("a failed listing removed %d row(s); want the row kept", 1-n)
	}
}
