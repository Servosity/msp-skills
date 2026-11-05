// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ledger

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gradient-pp-cli/internal/credkit"
)

// gradient handfix: ledger-files-withheld, now through the shared credential
// kit (handfixes.json credkit-wiring). The push and alert ledgers record API
// error text and operator-typed titles; neither file may hold a credential
// the process holds, in any form. A record that would is withheld whole: an
// append lands the kit's notice in its place (or fails), the rewrite fails; a
// clean record is written.
func TestLedgerFilesHoldNoCredential(t *testing.T) {
	const held = "ldgrheldqzvkxjwm-7c2e"
	credkit.Hold("test:ledger", held)
	dir := t.TempDir()
	b64 := base64.StdEncoding.EncodeToString([]byte(held))
	if err := AppendPushes(dir, []PushRecord{{RunID: "r0", At: time.Now(), ServiceID: "svc", AccountID: "acct", Status: "sent"}}); err != nil {
		t.Fatal(err)
	}
	if err := AppendAlert(dir, AlertRecord{At: time.Now(), AccountID: "acct", MessageID: "m1", Title: "backup failed", TicketStatus: "pending"}); err != nil {
		t.Fatal(err)
	}
	pushErr := AppendPushes(dir, []PushRecord{{RunID: "r1", At: time.Now(), ServiceID: "svc", AccountID: "acct", Status: "failed", Error: "HTTP 401: bad token " + held + " (" + b64 + ")"}})
	alertErr := AppendAlert(dir, AlertRecord{At: time.Now(), AccountID: "acct", MessageID: "m2", Title: "rotated " + held, TicketStatus: "pending"})
	if ok, err := UpdateAlert(dir, AlertRecord{At: time.Now(), AccountID: "acct", MessageID: "m1", Title: "still " + b64, TicketStatus: "created"}); err == nil || ok {
		t.Errorf("UpdateAlert rewrote the ledger with the held credential: ok %v err %v", ok, err)
	}
	for _, name := range []string{"pushes.jsonl", "alerts.jsonl"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), held) || strings.Contains(string(b), b64) {
			t.Errorf("%s holds the credential:\n%s", name, b)
		}
		if !strings.Contains(string(b), "acct") {
			t.Errorf("%s lost the clean record:\n%s", name, b)
		}
	}
	// The ledgers stay readable past a withheld record.
	if got, err := ReadPushes(dir); err != nil || len(got) != 1 || got[0].RunID != "r0" {
		t.Errorf("ReadPushes after a withheld record = %+v, %v; want the one clean record", got, err)
	}
	if got, err := ReadAlerts(dir); err != nil || len(got) != 1 || got[0].MessageID != "m1" {
		t.Errorf("ReadAlerts after a withheld record = %+v, %v; want the one clean record", got, err)
	}
	// The held record was refused or replaced by the notice, never dropped silently.
	for name, err := range map[string]error{"pushes.jsonl": pushErr, "alerts.jsonl": alertErr} {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if err == nil && !strings.Contains(string(b), credkit.Notice(credkit.SinkFile)) {
			t.Errorf("%s: the append carrying the held credential reported success with no notice in its place:\n%s", name, b)
		}
	}
}
