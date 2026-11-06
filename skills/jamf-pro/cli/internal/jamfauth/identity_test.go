// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfauth

import (
	"net/url"
	"testing"
)

// TestIdentityIsEstablishedOnce: a process acts as one identity; a second,
// different one is refused, the same one is a no-op, and an encoded
// identity round-trips unchanged.
func TestIdentityIsEstablishedOnce(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	a, _ := New("https://a.example", "c1")
	b, _ := New("https://b.example", "c1")
	if err := Establish(a); err != nil {
		t.Fatal(err)
	}
	if err := Establish(a); err != nil {
		t.Fatalf("re-establishing the same identity must be a no-op: %v", err)
	}
	if err := Establish(b); err == nil {
		t.Fatal("a second, different identity must be refused")
	}
	if got, _ := Established(); got != a {
		t.Fatalf("established identity changed to %+v", got)
	}
	inst, err := ForInstance("acme", "acme.jamfcloud.com", "c")
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(inst.Encode())
	if err != nil || back != inst {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	for _, raw := range []string{"https://a.example/api/v1/oauth/token/", "https://a.example/api/v1/oauth/token?x=1", "https://b.example/api/v1/oauth/token"} {
		u, _ := url.Parse(raw)
		if a.CheckMint(u, "c1") == nil {
			t.Fatalf("%s must be refused", raw)
		}
	}
	u, _ := url.Parse("https://a.example/api/v1/oauth/token")
	if a.CheckMint(u, "c1") != nil || a.CheckMint(u, "c2") == nil {
		t.Fatal("exactly the token URL for the same client must pass, another client must not")
	}
}
