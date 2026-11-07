// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelChecksWorst(t *testing.T) {
	t.Run("empty store returns empty envelope without network", func(t *testing.T) {
		novelTestEnv(t)
		stub := newCheckStub(t)
		out, _, err := runNovel(t, "checks", "worst")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		w := decodeWorst(t, out)
		if len(w.Items) != 0 || w.ScannedAgents != 0 || w.CandidateAgents != 0 {
			t.Errorf("empty store should yield no items and no scan, got %+v", w)
		}
		if len(stub.seen) != 0 {
			t.Errorf("empty store reached the server: %v", stub.seen)
		}
	})

	t.Run("a status on a synced check row is not trusted", func(t *testing.T) {
		// GET /checks/ never carries a status; a row that claims one (and no
		// synced agent reporting failing checks) must not produce a ranking.
		db := novelTestEnv(t)
		stub := newCheckStub(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"h1","checks":{"total":2,"failing":0}}`)
		seedResource(t, db, "checks", "c1", `{"id":"c1","readable_desc":"Disk Space","check_type":"diskspace","status":"failing"}`)
		seedResource(t, db, "checks", "c2", `{"id":"c2","readable_desc":"Disk Space","check_type":"diskspace","status":"failing"}`)
		out, _, err := runNovel(t, "checks", "worst")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		w := decodeWorst(t, out)
		if len(w.Items) != 0 || w.ScannedAgents != 0 {
			t.Errorf("synced check-row status produced %+v", w)
		}
		if len(stub.seen) != 0 {
			t.Errorf("no agent reports failing checks, yet the server saw %v", stub.seen)
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		db := novelTestEnv(t)
		stub := newCheckStub(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"h1","checks":{"total":2,"failing":2}}`)
		stub.set("a1", 0, `[{"id":1,"readable_desc":"A","check_type":"ping","check_result":{"status":"failing"}},{"id":2,"readable_desc":"B","check_type":"ping","check_result":{"status":"failing"}}]`)
		out, _, err := runNovel(t, "checks", "worst", "--limit", "1")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		w := decodeWorst(t, out)
		if len(w.Items) != 1 || w.Items[0].Check != "A" {
			t.Errorf("--limit 1 should yield only A, got %+v", w.Items)
		}
	})
}
