// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by handfixes.json "share-identity-follows-host-addresses".

package cli

import (
	"strings"
	"testing"
)

// Codex 2026-11-02 r2: a share host the MSP machine cannot resolve (NXDOMAIN or
// the deadline) has unknown identity, so it must overlap every peer share of the
// same type and path, never look like a distinct share.
func TestUnresolvedShareHostIsNotProofOfDistinctShares(t *testing.T) {
	a := netStorageID("nfs", "nas.client.lan", "", "/backups", "", "", "", "", "")
	b := netStorageID("nfs", "10.0.0.9", "", "/backups", "", "", "", "", "")
	if !overlaps(a, b) {
		t.Fatalf("unresolved name treated as a different share than an address: %v %v", a, b)
	}
	if overlaps(a, netStorageID("nfs", "10.0.0.9", "", "/other", "", "", "", "", "")) {
		t.Fatal("an unresolved host overlapped a share with another export path")
	}
	if overlaps(a, netStorageID("cifs", "10.0.0.9", "", "", "/backups", "", "", "", "")) {
		t.Fatal("an unresolved host overlapped a share of another type")
	}
	// The cluster-level check names the peer, with the unresolved host as the reason.
	own := &pbsOwnership{
		net:   map[string]map[string]string{},
		netOf: map[string]map[string][]string{"pveA": {"nas": a}, "pveB": {"nas": b}},
	}
	for id, m := range own.netOf {
		for _, tok := range m["nas"] {
			if own.net[tok] == nil {
				own.net[tok] = map[string]string{}
			}
			own.net[tok][id] = map[string]string{"pveA": "acme", "pveB": "beta"}[id]
		}
	}
	for _, c := range []struct{ me, id, peer string }{{"acme", "pveA", "beta"}, {"beta", "pveB", "acme"}} {
		got := (fleetTarget{Name: c.me, pveID: c.id, own: own}).netSharers(storageInfo{Storage: "nas", Type: "nfs"})
		if len(got) != 1 || !strings.HasPrefix(got[0], c.peer) || !strings.Contains(got[0], "nas.client.lan") {
			t.Errorf("%s: sharers %v, want %s with the unresolved host named", c.me, got, c.peer)
		}
	}
}

// The same class for a PBS endpoint with no certificate pin: an unresolved name
// may be any PBS on that port, so it joins them.
func TestUnresolvedUnpinnedPBSIsNotProofOfDistinctServices(t *testing.T) {
	s := pbsServices{}
	s.link(pbsEndpointFromHost("pbs.client.lan"))
	s.link(pbsEndpointFromHost("10.0.0.30"))
	s.link(pbsEndpointFromHost("10.0.0.31:8008"))
	s.joinUnknown()
	if !overlaps(s.of(pbsEndpointFromHost("pbs.client.lan")), s.of(pbsEndpointFromHost("10.0.0.30"))) {
		t.Fatal("an unresolved, unpinned PBS name was treated as a different service than an address")
	}
	if overlaps(s.of(pbsEndpointFromHost("pbs.client.lan")), s.of(pbsEndpointFromHost("10.0.0.31:8008"))) {
		t.Fatal("another port was merged into the unresolved PBS service")
	}
	// A pin identifies the service: a pinned unresolved name stays its own.
	p := pbsServices{}
	p.link(pbsEndpointFromHost("pbs.client.lan"), "aa:bb")
	p.link(pbsEndpointFromHost("10.0.0.30"), "cc:dd")
	p.joinUnknown()
	if overlaps(p.of(pbsEndpointFromHost("pbs.client.lan")), p.of(pbsEndpointFromHost("10.0.0.30"))) {
		t.Fatal("a pinned PBS name was merged into a service with another pin")
	}
}
