// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Open P1s from the 2026-10-24 factory Codex review round 2.

import (
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

// [P1 batch correlation] a JSONL correlation handle is not a record id.
func TestR2UpsertReceiptNamesTheUpsertedRecord(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"status":"COMPLETE","results":[{"id":"101","properties":{"email":"a@example.com"}},{"id":"202","properties":{"email":"b@example.com"}}]}`)
	s, path := r1Store(t)
	for _, p := range []string{"firstname", "email"} {
		r1Upsert(t, s.UpsertHubspotPropertiesCrm, map[string]any{"name": p, "type": "string", "objectType": "contacts"})
	}
	s.Close()
	in := `{"id":"a","email":"a@example.com","firstname":"A"}` + "\n" + `{"id":"101","email":"b@example.com","firstname":"B"}` + "\n"
	out, err := r1Run(t, in, "contacts", "bulk-update", "--jsonl", "--id-property", "email", "--yes", "--db", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, `{"id":"101"`) && !strings.Contains(line, `"id":"202"`) {
			t.Fatalf("row keyed b@example.com got another contact's receipt: %s", line)
		}
	}
}

// [P1 batch correlation] one result never confirms two rows: rows sharing a
// key in one batch cannot be told apart, so none of them is reported updated.
func TestR2DuplicateKeysInOneBatchAreNotConfirmed(t *testing.T) {
	for name, tc := range map[string]struct{ body, in string }{
		"update": {`{"status":"COMPLETE","results":[{"id":"1","properties":{"firstname":"X"}}]}`,
			`{"id":"1","firstname":"X"}` + "\n" + `{"id":"1","firstname":"Y"}` + "\n"},
		"upsert": {`{"status":"COMPLETE","results":[{"id":"101","properties":{"email":"a@example.com"}}]}`,
			`{"id":"a","email":"a@example.com","firstname":"X"}` + "\n" + `{"id":"b","email":"A@example.com","firstname":"Y"}` + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			newR1Server(t, 200, tc.body)
			s, path := r1Store(t)
			for _, p := range []string{"firstname", "email"} {
				r1Upsert(t, s.UpsertHubspotPropertiesCrm, map[string]any{"name": p, "type": "string", "objectType": "contacts"})
			}
			s.Close()
			args := []string{"contacts", "bulk-update", "--jsonl", "--yes", "--db", path}
			if name == "upsert" {
				args = append(args, "--id-property", "email")
			}
			out, err := r1Run(t, tc.in, args...)
			if err == nil || strings.Contains(out, `"ok":true`) {
				t.Fatalf("err=%v: a shared key must fail every row it names; out=%s", err, out)
			}
		})
	}
}

// [P2 batch accounting] a JSONL row rejected while reading is not an update:
// one bad row and one good row fail the run without --allow-partial-failure.
func TestR2RejectedInputRowsFailTheRun(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"status":"COMPLETE","results":[{"id":"1","properties":{"firstname":"X"}}]}`)
	path := r1BulkStore(t)
	in := `{"id":"1","firstname":"X"}` + "\n" + `{"id":"2","nosuchproperty":"Y"}` + "\n"
	if out, err := r1Run(t, in, "contacts", "bulk-update", "--jsonl", "--yes", "--db", path); err == nil {
		t.Fatalf("a rejected input row must fail the run; out=%s", out)
	}
	if _, err := r1Run(t, in, "contacts", "bulk-update", "--jsonl", "--yes", "--allow-partial-failure", "--db", path); err != nil {
		t.Fatalf("--allow-partial-failure accepts a partial run: %v", err)
	}
}
