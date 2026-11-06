// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/portal"
)

// TestGuardSimCheckersFire proves each invariant checker of the guard
// simulation both ways on synthetic truth: it reports the violation it
// exists for, and stays quiet (or reports an evidence gap) in the cases the
// contract allows. A checker that cannot fire would let the simulation pass
// for the wrong reason.
func TestGuardSimCheckersFire(t *testing.T) {
	isolateAuthState(t)
	base := time.Now().Add(-time.Minute)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	newState := func() *simState {
		return &simState{t: t, w: &simWorld{seed: 1}, seenViol: map[string]bool{}, labelBatch: map[string]string{}}
	}
	kinds := func(s *simState) string {
		var k []string
		for _, v := range s.violations {
			k = append(k, strings.SplitN(v, ":", 2)[0])
		}
		return strings.Join(k, ",")
	}

	// I1: a restore created twice.
	restore := func(id string, ms, visible int) *simRestore {
		return &simRestore{ID: id, Identity: "task-1|a@contoso.com|20260930042000", Entity: "a@contoso.com", Resolved: 20260930042000,
			Created: true, Answered: true, At: at(ms), VisibleAt: at(visible), FinalAt: at(visible + 10), FinalStatus: "COMPLETED"}
	}
	s := newState()
	s.ops = []*simOp{{idx: 1, kind: "rSubmit", task: simTask, start: at(0), end: at(2000)}}
	s.w.restores = []*simRestore{restore("rt-1", 100, 150), restore("rt-2", 1000, 1050)}
	if s.checkI1(); kinds(s) != "I1" {
		t.Fatalf("I1 must fire on a restore created twice, got %q", kinds(s))
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "rSubmit", task: simTask, start: at(0), end: at(500)},
		{idx: 2, kind: "rNewBatch", task: simTask, newBatch: true, start: at(900), end: at(1100)}}
	s.w.restores = []*simRestore{restore("rt-1", 100, 150), restore("rt-2", 1000, 1050)}
	if s.checkI1(); len(s.violations) != 0 {
		t.Fatalf("a --new-batch re-create is allowed, got %v", s.violations)
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "rRetry", task: simTask, retryUnknown: true, watchAt: at(500), start: at(900), end: at(1100)}}
	s.w.restores = []*simRestore{restore("rt-1", 100, 3000), restore("rt-2", 1000, 1050)}
	if s.checkI1(); len(s.violations) != 0 || len(s.gaps) != 1 {
		t.Fatalf("a --retry-unknown resend of a task not yet listed is an evidence gap, got %v / %v", s.violations, s.gaps)
	}

	// A resume of a batch a --new-batch op made after the first create
	// carries out that --new-batch; a resume of any other batch does not.
	s = newState()
	s.newBatches = map[string]time.Time{"B": at(500)}
	s.ops = []*simOp{{idx: 1, kind: "rResume", task: simTask, restoreBatchID: "B", start: at(900), end: at(1100)}}
	s.w.restores = []*simRestore{restore("rt-1", 100, 150), restore("rt-2", 1000, 1050)}
	if s.checkI1(); len(s.violations) != 0 {
		t.Fatalf("a --new-batch batch's resume re-creating is allowed, got %v", s.violations)
	}
	s = newState()
	s.newBatches = map[string]time.Time{"B": at(50)}
	s.ops = []*simOp{{idx: 1, kind: "rResume", task: simTask, restoreBatchID: "B", start: at(900), end: at(1100)}}
	s.w.restores = []*simRestore{restore("rt-1", 100, 150), restore("rt-2", 1000, 1050)}
	if s.checkI1(); kinds(s) != "I1" {
		t.Fatalf("a --new-batch made before the first create covers nothing, got %q", kinds(s))
	}

	// I2: an export created twice.
	export := func(id string, ms, visible int, final string) *simExport {
		return &simExport{ID: id, Identity: "task-1|a@contoso.com|O365MAIL|20260930042000|zip|del=|hold=", Entity: "a@contoso.com",
			Sub: "O365MAIL", Task: simTask, Snapshot: 20260930042000, Created: true, At: at(ms), VisibleAt: at(visible),
			FinalAt: at(visible + 10), FinalStatus: final}
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "dSubmit", task: simTask, start: at(0), end: at(2000)}}
	s.w.exports = []*simExport{export("job-1", 100, 150, portal.StatusReadyForDownload), export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); kinds(s) != "I2" {
		t.Fatalf("I2 must fire on an archive created twice while the first was listed, got %q", kinds(s))
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "dSubmit", task: simTask, start: at(0), end: at(2000)}}
	s.w.exports = []*simExport{export("job-1", 100, 3000, portal.StatusReadyForDownload), export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); len(s.violations) != 0 || len(s.gaps) != 1 {
		t.Fatalf("a re-create before the first job was listed is an evidence gap, got %v / %v", s.violations, s.gaps)
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "dResume", task: simTask, start: at(0), end: at(2000)}}
	s.w.exports = []*simExport{export("job-1", 100, 150, "FAILED"), export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); len(s.violations) != 0 {
		t.Fatalf("re-creating a job that failed is the documented JOB_FAILED path, got %v", s.violations)
	}
	s = newState()
	s.w.pageCap = 1
	s.ops = []*simOp{{idx: 1, kind: "dSubmit", task: simTask, start: at(0), end: at(2000)}}
	newer := export("job-0", 500, 550, portal.StatusReadyForDownload)
	newer.Identity, newer.Entity = "task-1|b@contoso.com|O365MAIL|20260930042001|zip|del=|hold=", "b@contoso.com"
	s.w.exports = []*simExport{export("job-1", 100, 150, portal.StatusReadyForDownload), newer, export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); kinds(s) != "I2" || !strings.Contains(s.violations[0], "page") {
		t.Fatalf("a first job hidden only past the first page is still evidence, got %v", s.violations)
	}

	// An unconfirmed first job that expired off the listing before the
	// second create is an evidence gap; a confirmed one is a violation.
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "dResume", task: simTask, start: at(0), end: at(3000)}}
	expired := export("job-1", 100, 150, portal.StatusReadyForDownload)
	expired.ExpireAt = at(600)
	s.w.exports = []*simExport{expired, export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); len(s.violations) != 0 || len(s.gaps) != 1 {
		t.Fatalf("a re-create after an unconfirmed job expired is an evidence gap, got %v / %v", s.violations, s.gaps)
	}
	dirC, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dirC, "download-batches"), 0o700); err != nil {
		t.Fatal(err)
	}
	conf, _ := json.Marshal(simLedger{BatchKey: "kc", TaskID: simTask, Rows: []simLedgerRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", Status: "CREATED", JobID: "job-1"}}})
	if err := os.WriteFile(filepath.Join(dirC, "download-batches", "kc.json"), conf, 0o600); err != nil {
		t.Fatal(err)
	}
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "dResume", task: simTask, start: at(0), end: at(3000)}}
	s.w.exports = []*simExport{expired, export("job-2", 1000, 1050, portal.StatusReadyForDownload)}
	if s.checkI2(); kinds(s) != "I2" {
		t.Fatalf("re-creating an archive a batch had confirmed is a violation even after it expired, got %q", kinds(s))
	}
	_ = os.Remove(filepath.Join(dirC, "download-batches", "kc.json"))

	// A create by a --new-batch batch whose own first create of the
	// archive came after the first job is covered; one whose batch had
	// already created it before is not.
	if err := os.WriteFile(filepath.Join(dirC, "download-batches", "kn.json"), mustJSON(t, simLedger{BatchKey: "kn", TaskID: simTask,
		Rows: []simLedgerRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", Status: "REQUESTED", Label: "L-new"}}}), 0o600); err != nil {
		t.Fatal(err)
	}
	labelled := func(id string, ms int, label string) *simExport {
		e := export(id, ms, ms+50, portal.StatusReadyForDownload)
		e.Label = label
		return e
	}
	s = newState()
	s.newBatches = map[string]time.Time{"kn": at(0)}
	s.ops = []*simOp{{idx: 1, kind: "dResume", task: simTask, start: at(2000), end: at(3000)}}
	s.w.exports = []*simExport{labelled("job-1", 100, "L-old"), labelled("job-2", 2500, "L-new")}
	if s.checkI2(); len(s.violations) != 0 {
		t.Fatalf("a --new-batch batch's own first create after the first job is covered, got %v", s.violations)
	}
	s = newState()
	s.newBatches = map[string]time.Time{"kn": at(0)}
	s.ops = []*simOp{{idx: 1, kind: "dResume", task: simTask, start: at(2000), end: at(3000)}}
	s.w.exports = []*simExport{labelled("job-0", 50, "L-new"), labelled("job-1", 100, "L-old"), labelled("job-2", 2500, "L-new")}
	if s.checkI2(); !strings.Contains(strings.Join(s.violations, "\n"), "job-1 (") || !strings.Contains(strings.Join(s.violations, "\n"), " then job-2 (") {
		t.Fatalf("a batch that had created the archive before the other job is not covered by its --new-batch, got %v", s.violations)
	}
	_ = os.Remove(filepath.Join(dirC, "download-batches", "kn.json"))

	// I4: steering to --new-batch.
	s = newState()
	s.checkI4(&simOp{idx: 1, kind: "dSubmit", res: simResult{stderr: "batch X holds these; run again with --new-batch to get them"}})
	if kinds(s) != "I4" {
		t.Fatalf("I4 must fire on a steer to --new-batch, got %q", kinds(s))
	}
	s = newState()
	s.checkI4(&simOp{idx: 1, kind: "rSubmit", res: simResult{errText: "If you have checked and it is a different backup, send it on purpose with: x --new-batch --yes."}})
	if len(s.violations) != 0 {
		t.Fatalf("a person-decision mention is allowed, got %v", s.violations)
	}

	// I3: a watch that exits 0 over a failed restore.
	s = newState()
	s.ops = []*simOp{{idx: 1, kind: "rWatch", start: at(0), end: at(5000)}}
	failed := restore("rt-1", 100, 150)
	failed.FinalStatus = "FAILED"
	s.w.restores = []*simRestore{failed}
	out, _ := json.Marshal(watchOutput{Rows: []restoreJob{{Entity: "a@contoso.com", ResolvedRaw: 20260930042000}}})
	s.checkI3(&simOp{idx: 1, kind: "rWatch", end: at(5000), res: simResult{stdout: string(out)}})
	if kinds(s) != "I3" {
		t.Fatalf("I3 must fire on a watch that exits 0 over a failed restore, got %q", kinds(s))
	}
	s = newState()
	ok := restore("rt-1", 100, 150)
	s.w.restores = []*simRestore{ok}
	s.checkI3(&simOp{idx: 1, kind: "rWatch", end: at(5000), res: simResult{stdout: string(out)}})
	if len(s.violations) != 0 {
		t.Fatalf("a watch that exits 0 over a completed restore is fine, got %v", s.violations)
	}

	// I3: a submit that exits 0 counting a restore the API neither
	// answered nor created; quiet when the restore exists.
	submitOut, _ := json.Marshal(map[string]any{"rows": []map[string]any{{"entity": "a@contoso.com", "resolved_raw": 20260930042000,
		"status": "SUBMITTED", "submit_status": "SUBMITTED"}}})
	s = newState()
	dropped := restore("rt-1", 100, 150)
	dropped.Created, dropped.Answered = false, false
	s.w.restores = []*simRestore{dropped}
	s.checkI3(&simOp{idx: 1, kind: "rResume", res: simResult{stdout: string(submitOut)}})
	if kinds(s) != "I3" {
		t.Fatalf("I3 must fire on an accepted row with no evidence, got %q", kinds(s))
	}
	s = newState()
	unanswered := restore("rt-1", 100, 150)
	unanswered.Answered = false
	s.w.restores = []*simRestore{unanswered}
	s.checkI3(&simOp{idx: 1, kind: "rResume", res: simResult{stdout: string(submitOut)}})
	if len(s.violations) != 0 {
		t.Fatalf("a row watch settled from a created restore is accepted, got %v", s.violations)
	}

	// I5: an EMPTY row whose create was dropped.
	dir, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	ledgers := filepath.Join(dir, "download-batches")
	if err := os.MkdirAll(ledgers, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLedger := func(label string) {
		raw, _ := json.Marshal(simLedger{BatchKey: "k1", TaskID: simTask, Rows: []simLedgerRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", Status: "EMPTY", Label: label}}})
		if err := os.WriteFile(filepath.Join(ledgers, "k1.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s = newState()
	s.w.exports = []*simExport{{ID: "job-1", Label: "cloudally-cli L1", Decision: "acceptDrop", At: at(0)}}
	writeLedger("cloudally-cli L1")
	if s.checkI5(); kinds(s) != "I5" {
		t.Fatalf("I5 must fire on an EMPTY row whose create made nothing, got %q", kinds(s))
	}
	s = newState()
	s.w.exports = []*simExport{{ID: "job-1", Label: "cloudally-cli L1", Decision: "acceptDrop", At: at(0)},
		{ID: "job-2", Label: "cloudally-cli L1", Decision: "acceptEmpty", Created: true, Empty: true, At: at(10)}}
	if s.checkI5(); len(s.violations) != 0 {
		t.Fatalf("an EMPTY row whose resend made an empty job is fine, got %v", s.violations)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
