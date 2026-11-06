// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/comet"
	"comet-backup-pp-cli/internal/config"
)

func TestCleanCellAndMaskSecrets(t *testing.T) {
	if got := cleanCell("ok\x1b[2J\nFORGED\tROW\x9b31m\u009b2J"); got != "ok[2J FORGED ROW31m2J" {
		t.Fatalf("cleanCell = %q", got)
	}
	c := client.New(&config.Config{CometBackupPassword: "pw-123", CometBackupSessionKey: "sk-456"}, time.Second, 0)
	if got := maskSecrets(c, "bad pw-123 and sk-456"); got != "bad **** and ****" {
		t.Fatalf("maskSecrets = %q", got)
	}
	if got := maskSecrets(nil, "x"); got != "x" {
		t.Fatal("nil client must pass text through")
	}
}

func TestSafeTextCleansBeforeMasking(t *testing.T) {
	c := client.New(&config.Config{CometBackupPassword: "p@ss+word"}, time.Second, 0)
	for _, in := range []string{"denied p@\x1bss+word", "denied p@\nss+word", "x p%40\x07ss%2Bword"} {
		if out := safeText(c, in); strings.Contains(out, "p@ss") || strings.Contains(out, "\x1b") {
			t.Fatalf("%q -> %q", in, out)
		}
	}
	num := client.New(&config.Config{CometBackupPassword: "123456789012345678901234567890"}, time.Second, 0)
	err := safeErr(num, errors.New("json: cannot unmarshal number 123456789012345678901234567890 into Go struct field"))
	if strings.Contains(err.Error(), "123456789012345678901234567890") {
		t.Fatalf("decode error leaked a numeric password: %v", err)
	}
}

// TestAgentKeepsSparseFleetKeys: --agent compaction must keep the sparse,
// decision-relevant row keys (a suspended tenant, a connection id) even when
// fewer than 80% of rows carry them.
func TestAgentKeepsSparseFleetKeys(t *testing.T) {
	rows := make([]comet.HealthRow, 0, 10)
	for i := 0; i < 9; i++ {
		rows = append(rows, comet.HealthRow{Tenant: "t", ItemID: fmt.Sprint(i), State: "ok"})
	}
	rows = append(rows, comet.HealthRow{Tenant: "s", ItemID: "x", State: "failed", Suspended: true, ConnectionID: "c1", ScheduleUnresolved: true})
	flags := &rootFlags{asJSON: true, compact: true, agent: true}
	var buf bytes.Buffer
	if err := printJSONFilteredKeep(&buf, rows, flags, healthAgentKeep...); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"suspended"`, `"connection_id"`, `"schedule_unresolved"`} {
		if !strings.Contains(buf.String(), k) {
			t.Errorf("--agent output lost %s:\n%s", k, buf.String())
		}
	}
	// Control: the generic cut drops the key, so the floor is what keeps it.
	buf.Reset()
	if err := printJSONFiltered(&buf, rows, flags); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"suspended"`) {
		t.Fatal("control: generic compaction kept suspended; this test no longer exercises the keep floor")
	}
}
