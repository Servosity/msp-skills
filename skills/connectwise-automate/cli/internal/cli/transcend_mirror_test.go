// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"connectwise-automate-pp-cli/internal/store"
)

func openSeeded(t *testing.T) *store.Store {
	t.Helper()
	seedStore(t)
	s, err := store.Open(defaultDBPath("connectwise-automate-cli"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

// An alert the latest finished walk of /Alerts no longer returned (resolved
// upstream; a plain sync never prunes it) is not reported as open.
func TestFleetReportsDropRowsTheLastWalkDidNotReturn(t *testing.T) {
	s := openSeeded(t)
	put := `{"Id":7,"ComputerId":10,"Priority":5,"Message":"resolved yesterday","ClientName":"Acme Corp"}`
	if err := s.Upsert("alerts", "7", []byte(put)); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	if _, err := s.DB().Exec(`UPDATE resources SET synced_at = ? WHERE resource_type = 'alerts' AND id = '7'`, old); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, out := range []string{run(t, newNovelAlertTriageCmd), run(t, newNovelFleetHealthCmd), run(t, newNovelClientRollupCmd)} {
		if strings.Contains(out, "resolved yesterday") || strings.Contains(nospace(out), `"open_alerts":2`) {
			t.Fatalf("a row the last walk did not return was reported as current: %s", out)
		}
	}
}

// A resource whose last sync did not finish its walk (--max-pages, an
// interrupted run) or was never synced while rows exist (live write-through)
// fails the report instead of reading as the whole fleet, unless the operator
// passes --allow-partial-failure.
func TestFleetReportsRefuseAnIncompleteMirror(t *testing.T) {
	for name, mark := range map[string]func(*store.Store) error{
		"walk in progress": func(s *store.Store) error { return cwaWalkBegin(s, "computers", true, "") },
		"interrupted walk": func(s *store.Store) error { return cwaWalkBegin(s, "computers", true, "page=7") },
		"never synced": func(s *store.Store) error {
			_, err := s.DB().Exec(`DELETE FROM cwa_walk WHERE resource_type = 'computers'`)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := openSeeded(t)
			if err := mark(s); err != nil {
				t.Fatal(err)
			}
			s.Close()
			flags := &rootFlags{asJSON: true}
			cmd := newNovelFleetHealthCmd(flags)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(nil)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "computers") {
				t.Fatalf("fleet-health over an incomplete computers mirror: err=%v out=%s", err, out.String())
			}
			flags = &rootFlags{asJSON: true, allowPartialFailure: true}
			cmd = newNovelFleetHealthCmd(flags)
			out.Reset()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("--allow-partial-failure should report anyway: %v", err)
			}
			if !strings.Contains(out.String(), "incomplete") {
				t.Fatalf("--allow-partial-failure must still say the mirror is incomplete: %s", out.String())
			}
		})
	}
}
