// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/config"
)

// zoom-phone hand-fix: zoom-phone-offboard-write-sim. offboard --apply writes
// into a live tenant, so it ships only with this random simulation: a fake
// Zoom that refuses, drops the connection, answers late or acts and then
// fails, crashed runs (cancelled mid-plan) and parallel runs. Properties:
// nothing is removed twice, and a run never reports success (no failed
// receipt, so exit 0) while any planned membership is still present.
type offboardSimServer struct {
	mu      sync.Mutex
	present map[string]bool
	removed map[string]int
	rng     *rand.Rand
	faults  bool
}

func (s *offboardSimServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	mode := 0
	if s.faults {
		mode = s.rng.Intn(7)
	}
	act := func() {
		if s.present[r.URL.Path] {
			s.present[r.URL.Path] = false
			s.removed[r.URL.Path]++
		}
	}
	exists := s.present[r.URL.Path]
	switch mode {
	case 2, 3, 5:
		act()
	case 0, 6:
		if exists {
			act()
		}
	}
	s.mu.Unlock()
	switch mode {
	case 0, 6: // normal answer
		if !exists {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":404,"message":"Member does not exist."}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case 1: // refuse without acting
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":500,"message":"internal error"}`)
	case 2: // act, then fail
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"code":502,"message":"bad gateway"}`)
	case 3: // act, answer too late
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	case 4, 5: // drop the connection (5 after acting)
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *offboardSimServer) remaining(steps []localOffboardStep) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var left []string
	for _, st := range steps {
		if s.present["/v2"+st.Path] || s.present[st.Path] {
			left = append(left, st.Path)
		}
	}
	return left
}

func TestOffboardApplySimulation(t *testing.T) {
	localTestEnv(t)
	for seed := int64(1); seed <= 12; seed++ {
		rng := rand.New(rand.NewSource(seed))
		sim := &offboardSimServer{present: map[string]bool{}, removed: map[string]int{}, rng: rng, faults: true}
		srv := httptest.NewServer(sim)
		user := &localFootprintUser{ID: "u1", Email: "jane@example.com", ExtensionNumber: "101"}
		var steps []localOffboardStep
		for i := 0; i < 3+rng.Intn(6); i++ {
			p := fmt.Sprintf("/phone/call_queues/q%d/members/u1", i)
			steps = append(steps, localOffboardStep{Step: i + 1, Method: "DELETE", Path: p})
			sim.present[p] = true
		}
		newClient := func() *client.Client {
			c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "sim-token-000001"}, 250*time.Millisecond, 0)
			c.NoCache = true
			if err := ApplyClientHooks(c); err != nil {
				t.Fatal(err)
			}
			return c
		}
		run := func(crash bool) []localOffboardReceipt {
			ctx := context.Background()
			if crash {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(rng.Intn(300))*time.Millisecond)
				defer cancel()
			}
			return localApplyOffboard(ctx, newClient(), user, steps)
		}
		check := func(label string, receipts []localOffboardReceipt) {
			failed := 0
			for _, r := range receipts {
				if r.Status != "removed" && r.Status != "already_removed" {
					failed++
				}
			}
			if failed == 0 {
				if left := sim.remaining(steps); len(left) > 0 {
					t.Fatalf("seed %d %s: run reported every step done (exit 0) but %v still present", seed, label, left)
				}
			}
		}
		// crashed and faulty runs, two at a time in parallel
		for round := 0; round < 3; round++ {
			var wg sync.WaitGroup
			results := make([][]localOffboardReceipt, 2)
			for k := 0; k < 2; k++ {
				crash := rng.Intn(2) == 0
				wg.Add(1)
				go func(k int, crash bool) {
					defer wg.Done()
					results[k] = run(crash)
				}(k, crash)
			}
			wg.Wait()
			for k, res := range results {
				check(fmt.Sprintf("round %d run %d", round, k), res)
			}
		}
		// the operator re-runs after the faults clear: it must converge
		sim.mu.Lock()
		sim.faults = false
		sim.mu.Unlock()
		final := run(false)
		check("final", final)
		for _, r := range final {
			if r.Status != "removed" && r.Status != "already_removed" {
				t.Fatalf("seed %d: re-run with a healthy API did not converge: %+v", seed, r)
			}
		}
		for p, n := range sim.removed {
			if n > 1 {
				t.Fatalf("seed %d: %s removed %d times", seed, p, n)
			}
		}
		srv.Close()
	}
}
