// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Review round 1 (P1-B): HeldEchoes builds the funnel's needles once per held
// set (funnelEchoesWith) instead of on every call, and consults the detectors
// the client registers. These tests hold the cached search equal to the
// fleet funnel's own and the detectors fail closed.

import (
	"bytes"
	"strings"
	"testing"
)

func TestFunnelEchoesWithMatchesFunnelEchoes(t *testing.T) {
	secrets := funnelTestSecrets()
	needles := funnelNeedles(secrets)
	var texts []string
	for _, secret := range secrets {
		for _, enc := range funnelEncodings(secret) {
			for _, text := range funnelShapes(enc) {
				texts = append(texts, text)
			}
		}
	}
	texts = append(texts, "", "plain text with nothing held", `{"Data":[{"Id":1,"Name":"Acme"}]}`,
		strings.Repeat("QUJDREVGR0hJSktMTU5PUA== ", 400), strings.Repeat("x", funnelMaxInput+1))
	for _, text := range texts {
		if got, want := funnelEchoesWith(text, needles), FunnelEchoes(text, secrets); got != want {
			t.Errorf("funnelEchoesWith = %v, FunnelEchoes = %v for %.120q", got, want, text)
		}
	}
	// The work cap fails closed the same way.
	saved := funnelWorkPerByte
	funnelWorkPerByte, funnelWorkBase = 1, 1
	defer func() { funnelWorkPerByte, funnelWorkBase = saved, 4<<20 }()
	deep := strings.Repeat("%25", 40) + "41" + strings.Repeat(" QUJDREVGR0hJSktMTU5PUA==", 50)
	if got, want := funnelEchoesWith(deep, needles), FunnelEchoes(deep, secrets); got != want || !got {
		t.Errorf("past the work cap: funnelEchoesWith = %v, FunnelEchoes = %v, want both true", got, want)
	}
}

func TestEchoDetectorsFailClosed(t *testing.T) {
	panicky := func([]string) func(string) bool { panic("builder") }
	if d := buildDetector(panicky, []string{"held-value-0001"}); d == nil || !d("anything") {
		t.Fatal("a builder that panics must yield a check that withholds")
	}
	if !detectorFires(func(string) bool { panic("check") }, "anything") {
		t.Fatal("a check that panics must withhold")
	}
	if detectorFires(func(string) bool { return false }, "anything") {
		t.Fatal("a check that answers false must not withhold")
	}
}

func TestWithholdWriterWithholdsEachRecord(t *testing.T) {
	held := "writer-held-" + "7c1e9a52f0"
	FunnelHold(held)
	var buf bytes.Buffer
	w := WithholdWriter(&buf)
	if n, err := w.Write([]byte(`{"text":"` + held + `","ok":"kept"}` + "\n")); err != nil || n == 0 {
		t.Fatalf("write: %d %v", n, err)
	}
	if strings.Contains(buf.String(), held) || !strings.Contains(buf.String(), `"ok":"kept"`) {
		t.Fatalf("got %q", buf.String())
	}
}

// Review round 2 [P1] (credential provenance): the held set holds only what a
// caller held as a credential. The encodings the chokepoint derives live in
// its own state, so a credential equal to one of them stays a credential.
func TestHeldSetHoldsOnlyCredentials(t *testing.T) {
	v := "provenance-held-" + "51c0de77aa"
	FunnelHold(v)
	HeldEchoes("expand the held set now")
	heldSecrets.Lock()
	defer heldSecrets.Unlock()
	for _, core := range encodedCores(v) {
		if heldSecrets.seen[core] {
			t.Errorf("derived encoding %q was added to the held credentials", core)
		}
	}
}
