// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"sync"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// checkClock is a sweep clock the fakes move: each check sets the time it
// finished, so a stamp taken after a later check is visibly wrong.
type checkClock struct {
	mu          sync.Mutex
	cur         time.Time
	tlsReturned bool
	stamped     chan struct{}
	once        sync.Once
}

func (c *checkClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tlsReturned {
		c.once.Do(func() { close(c.stamped) })
	}
	return c.cur
}

func (c *checkClock) set(t time.Time, tls bool) {
	c.mu.Lock()
	c.cur = t
	if tls {
		c.tlsReturned = true
	}
	c.mu.Unlock()
}

type clockedTLS struct {
	inner *fakeTLS
	clk   *checkClock
	at    time.Time
}

func (f *clockedTLS) Probe(ctx context.Context, host string, port int) (TLSObs, error) {
	o, err := f.inner.Probe(ctx, host, port)
	f.clk.set(f.at, true)
	return o, err
}

type clockedCT struct {
	*fakeCT
	clk *checkClock
	at  time.Time
}

func (f *clockedCT) Search(ctx context.Context, d string) ([]CTCert, error) {
	// Finish after the TLS check has been stamped (or after a bound, so a
	// sweep that stamps nothing per check still completes).
	select {
	case <-f.clk.stamped:
	case <-time.After(2 * time.Second):
	}
	f.clk.set(f.at, false)
	return f.fakeCT.Search(ctx, d)
}

// Each observation carries the time its own check finished. A TLS probe that
// saw the old certificate 10 s inside the deploy grace is not deploy_stalled
// because a slower CT check in the same sweep finished after the grace ran
// out (review: TLS 11:59:50, CT 12:00:10, successor first seen 12:00 the day
// before).
func TestSweepStampsEachObservationWithItsOwnCheckTime(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	old := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	succ := ctCert(2, le, "2026-10-26T00:00:00Z", "2027-01-24T00:00:00Z", "bb", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 4, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": {old, succ}}}
	day1 := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if _, err := RunSweep(ctx, st, Sources{CT: ct, Guard: netguard.New()}, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return day1 }}); err != nil {
		t.Fatal(err)
	}

	clk := &checkClock{cur: day1.Add(23*time.Hour + 59*time.Minute + 40*time.Second), stamped: make(chan struct{})}
	tlsAt := day1.Add(24*time.Hour - 10*time.Second)
	ctAt := day1.Add(24*time.Hour + 10*time.Second)
	src := Sources{
		CT:    &clockedCT{fakeCT: ct, clk: clk, at: ctAt},
		TLS:   &clockedTLS{inner: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, clk: clk, at: tlsAt},
		Guard: netguard.New(),
	}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Concurrency: 2, Now: clk.now}); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, ctAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no renewal findings")
	}
	if got[0].Status == RenewalDeployStalled {
		t.Fatalf("status deploy_stalled (%s): the TLS observation was stamped with a later check's time", got[0].Detail)
	}
}
