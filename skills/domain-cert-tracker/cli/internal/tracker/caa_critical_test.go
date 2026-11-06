package tracker

import (
	"context"
	"strings"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// RFC 8659 section 4.1: a CA that does not understand a property marked
// critical must not issue. The audit cannot tell whether the issuer
// understood it, so a CAA set carrying one is never an affirmative verdict.
func TestIssuerAuditUnknownCriticalCAAPropertyIsNotRoutine(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	c := ctCert(7, "C=US, O=Let's Encrypt, CN=R11", "2026-10-20T00:00:00Z", "2027-01-18T00:00:00Z", "07", "example.com")
	dns := &fakeDNS{raw: map[string][][]byte{"example.com|CAA": {caaRData(128, "foobar", "x"), caaRData(0, "issue", "letsencrypt.org")}}}
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"example.com": {c}}, autoDER: true}, DNS: dns, Guard: netguard.New()}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil {
		t.Fatal(err)
	}
	if res.Routine != 0 || len(res.Findings) != 1 || res.Findings[0].Status != IssuerCAAUnverified || !strings.Contains(res.Findings[0].Detail, "foobar") {
		t.Fatalf("unknown critical CAA property judged as permission: %+v routine %d", res.Findings, res.Routine)
	}
	// A non-critical unknown property is ignored (RFC 8659 4.1).
	dns.raw["example.com|CAA"] = [][]byte{caaRData(0, "foobar", "x"), caaRData(0, "issue", "letsencrypt.org")}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14)); err != nil {
		t.Fatal(err)
	}
	if res.Routine != 1 || len(res.Findings) != 0 {
		t.Fatalf("non-critical unknown property: %+v routine %d", res.Findings, res.Routine)
	}
}
