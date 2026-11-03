// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by handfixes.json "share-identity-follows-host-addresses".

package cli

import "testing"

// Package tests never touch DNS: share and PBS host names resolve from this table.
var testHostAddrs = map[string][]string{"nas.example": {"10.0.0.9"}, "nas1.example": {"10.0.0.11"}, "pbs.example": {"10.0.0.20", "fd00::20"}}

func init() {
	resolveHostAddrs = func(host string) []string { return testHostAddrs[host] }
}

// Codex 2026-11-02 r1: a NAS named by hostname in one cluster and by IP in
// another is one share, so two clusters' vzdump archives there must collide.
func TestShareIdentityFollowsHostAddresses(t *testing.T) {
	cases := []struct{ typ, a, b string }{
		{"nfs", "nas.example", "10.0.0.9"},
		{"cifs", "NAS.example.", "10.0.0.9"},
		{"glusterfs", "nas.example", "10.0.0.9"},
	}
	for _, c := range cases {
		a := netStorageID(c.typ, c.a, "", "/backups", "/backups", "", "", "", "/backups")
		b := netStorageID(c.typ, c.b, "", "/backups", "/backups", "", "", "", "/backups")
		if !overlaps(a, b) {
			t.Errorf("%s: %s and %s name one server but share no token: %v %v", c.typ, c.a, c.b, a, b)
		}
	}
	a := netStorageID("cephfs", "", "", "", "", "", "nas.example:6789", "cephfs", "")
	b := netStorageID("cephfs", "", "", "", "", "", "10.0.0.9", "cephfs", "")
	if !overlaps(a, b) {
		t.Errorf("cephfs monitors by name and address share no token: %v %v", a, b)
	}
	if overlaps(netStorageID("nfs", "nas.example", "", "/backups", "", "", "", "", ""), netStorageID("nfs", "10.0.0.10", "", "/backups", "", "", "", "", "")) {
		t.Error("a different address was treated as the same share")
	}
}

// The same class for PBS services linked without a certificate pin.
func TestPBSServiceFollowsHostAddresses(t *testing.T) {
	s := pbsServices{}
	s.link(pbsEndpointFromHost("pbs.example"))
	s.link(pbsEndpointFromHost("[fd00::20]:8007"))
	a, b := s.of(pbsEndpointFromHost("pbs.example")), s.of(pbsEndpointFromHost("10.0.0.20"))
	if !overlaps(a, b) {
		t.Fatalf("a PBS named by host and by address is two services: %v %v", a, b)
	}
	if overlaps(s.of(pbsEndpointFromHost("pbs.example")), s.of(pbsEndpointFromHost("10.0.0.20:8008"))) {
		t.Fatal("another port on the same address was merged into the PBS service")
	}
}
