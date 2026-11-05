// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizeDestination(t *testing.T) {
	cases := []struct{ in, value, kind string }{
		{"Example.COM.", "example.com", "domain"},
		{"*.example.com", "example.com", "domain"},
		{"https://Example.com/", "example.com", "domain"},
		{"https://example.com/Login?x=1", "example.com/Login?x=1", "url"},
		{"example.com/path/", "example.com/path/", "url"},
		{"198.51.100.7", "198.51.100.7", "ipv4"},
		{"  evil.test  # from feed", "evil.test", "domain"},
	}
	for _, c := range cases {
		v, k, ok := normalizeDestination(c.in)
		if !ok || v != c.value || k != c.kind {
			t.Errorf("normalizeDestination(%q) = %q,%q,%v; want %q,%q", c.in, v, k, ok, c.value, c.kind)
		}
	}
	for _, bad := range []string{"", "   ", "# only a comment", "two words"} {
		if _, _, ok := normalizeDestination(bad); ok {
			t.Errorf("normalizeDestination(%q) accepted junk", bad)
		}
	}
}

func TestDomainCovers(t *testing.T) {
	for _, c := range []struct {
		entry, domain string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "login.example.com", true},
		{"example.com", "a.b.example.com", true},
		{"example.com", "notexample.com", false}, // lookalike must not match
		{"login.example.com", "example.com", false},
		{"*.example.com", "x.example.com", true},
		{"", "example.com", false},
	} {
		if got := domainCovers(c.entry, c.domain); got != c.want {
			t.Errorf("domainCovers(%q,%q)=%v want %v", c.entry, c.domain, got, c.want)
		}
	}
}

func TestParseCategorizationShapes(t *testing.T) {
	keyed, _ := parseCategorization(json.RawMessage(`{"amazon.com":{"status":1,"security_categories":["150"],"content_categories":["8"]}}`), "")
	if keyed["amazon.com"].Status != 1 || keyed["amazon.com"].ContentCategories[0] != "8" {
		t.Fatalf("keyed shape: %+v", keyed)
	}
	// The bulk example in Cisco's doc flattens each entry and names it by a null key.
	flat, _ := parseCategorization(json.RawMessage(`[{"google.com":null,"status":1,"security_categories":[],"content_categories":["23"]},{"bad.test":null,"status":-1,"security_categories":["66"]}]`), "")
	if flat["google.com"].Status != 1 || flat["bad.test"].Status != -1 {
		t.Fatalf("flat array shape: %+v", flat)
	}
	single, _ := parseCategorization(json.RawMessage(`{"status":-1,"security_categories":["Phishing"]}`), "x.test")
	if single["x.test"].Status != -1 {
		t.Fatalf("single flat object needs the fallback domain: %+v", single)
	}
}

func TestPlanDestinationDiff(t *testing.T) {
	want, skipped := normalizeAll([]string{"a.test", "B.test.", "junk line", "a.test"})
	if len(want) != 2 || len(skipped) != 1 {
		t.Fatalf("normalizeAll: %v %v", want, skipped)
	}
	current := []umbrellaDestination{{ID: "1", Destination: "a.test"}, {ID: "2", Destination: "a.test"}, {ID: "3", Destination: "c.test"}}
	plan := planDestinationDiff(want, current)
	if len(plan.ToAdd) != 1 || plan.ToAdd[0] != "b.test" || plan.Unchanged != 1 {
		t.Fatalf("plan adds/unchanged wrong: %+v", plan)
	}
	if len(plan.ToRemove) != 1 || plan.ToRemove[0].ID != "3" {
		t.Fatalf("a duplicate of a wanted entry must stay; only c.test goes: %+v", plan.ToRemove)
	}
}

func TestConflictsAndCreatedAt(t *testing.T) {
	if !conflicts("login.example.com", "domain", "example.com", "domain") || conflicts("example.org", "domain", "example.com", "domain") {
		t.Fatal("conflicts parent/sibling logic wrong")
	}
	if conflicts("10.0.0.1", "ipv4", "10.0.0.2", "ipv4") || !conflicts("10.0.0.1", "ipv4", "10.0.0.1", "ipv4") {
		t.Fatal("conflicts IP logic wrong")
	}
	for _, s := range []string{"2024-05-01T10:00:00Z", "2024-05-01 10:00:00", "1714557600", "1714557600000"} {
		if got, ok := parseCreatedAt(s); !ok || got.UTC().Format("2006-01-02") != "2024-05-01" {
			t.Errorf("parseCreatedAt(%q) = %v,%v", s, got, ok)
		}
	}
}

func TestParseReportTime(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"now":           now,
		"-30days":       now.Add(-30 * 24 * time.Hour),
		"-12hours":      now.Add(-12 * time.Hour),
		"-2weeks":       now.Add(-14 * 24 * time.Hour),
		"1760443200000": time.UnixMilli(1760443200000),
	} {
		got, err := parseReportTime(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseReportTime(%q) = %v,%v want %v", in, got, err, want)
		}
	}
	if _, err := parseReportTime("yesterday", now); err == nil {
		t.Error("parseReportTime accepted an unknown form")
	}
}

func TestNormalizeDestinationDropsPortsFromDomains(t *testing.T) {
	for _, in := range []string{"https://example.com:8443/", "example.com:8443", "HTTPS://Example.com:443"} {
		if v, k, ok := normalizeDestination(in); !ok || v != "example.com" || k != "domain" {
			t.Errorf("normalizeDestination(%q) = %q,%q,%v; want example.com domain", in, v, k, ok)
		}
	}
	if !destinationMatches(umbrellaDestination{Destination: "example.com"}, "example.com:8443/login", "url") {
		t.Error("a domain entry must cover a port-bearing URL on that host")
	}
}

func TestParseCategorizationSkipsEmptyRecordsAndRejectsMalformed(t *testing.T) {
	got, err := parseCategorization(json.RawMessage(`{"a.test":null,"b.test":{},"c.test":{"status":1}}`), "")
	if err != nil || len(got) != 1 || got["c.test"].Status != 1 {
		t.Fatalf("null and status-less records must be left out as missing: %v %v", got, err)
	}
	if got, err := parseCategorization(json.RawMessage(`[{"x.test":null,"status":"bad"}]`), ""); err != nil || len(got) != 0 {
		t.Fatalf("a record without a documented status is no answer (missing), got %v %v", got, err)
	}
	if _, err := parseCategorization(json.RawMessage(`"maintenance"`), ""); err == nil {
		t.Fatal("a body that is neither an object nor an array must be an error")
	}
}

func TestRound7Classes(t *testing.T) {
	// One matcher, both directions: an IPv4 entry covers a URL on that address.
	if !conflicts("198.51.100.7/path", "url", "198.51.100.7", "ipv4") || !conflicts("198.51.100.7", "ipv4", "198.51.100.7/path", "url") {
		t.Error("an IPv4 block must conflict with an allowed URL on that IP, in both directions")
	}
	if !conflicts("login.example.com", "domain", "example.com", "domain") || conflicts("example.org", "domain", "example.com", "domain") {
		t.Error("parent/sibling domain conflicts wrong")
	}
	if conflicts("198.51.100.8/path", "url", "198.51.100.7", "ipv4") {
		t.Error("a different IP must not conflict")
	}
	// Query-only scheme-less input is a URL on its host.
	v, k, ok := normalizeDestination("example.com?x=1")
	if !ok || k != "url" || v != "example.com?x=1" {
		t.Errorf("example.com?x=1 = %q,%q,%v; want url", v, k, ok)
	}
	if !destinationMatches(umbrellaDestination{Destination: "example.com"}, v, k) {
		t.Error("a domain entry must cover a query-only URL on that host")
	}
	for _, bad := range []string{"example.com;x", "exa$mple.com", "https://bad host/x", ".example.com", "a..b.com"} {
		if got, _, ok := normalizeDestination(bad); ok {
			t.Errorf("normalizeDestination(%q) = %q; invalid hostnames must be rejected", bad, got)
		}
	}
}

func TestURLWithURLInQuery(t *testing.T) {
	v, k, ok := normalizeDestination("redirect.test/go?next=https://target.test")
	if !ok || k != "url" || v != "redirect.test/go?next=https://target.test" {
		t.Fatalf("a URL whose query holds a URL must normalize as a url on its own host: %q %q %v", v, k, ok)
	}
	if v, k, ok := normalizeDestination("example.com:8443/x"); !ok || k != "url" || hostOf(v) != "example.com" {
		t.Fatalf("host:port/path must parse as a url on the host: %q %q %v", v, k, ok)
	}
}

// Normalizing a normalized value must be a no-op: diff compares the file against
// the list's read-back entries, so any drift fakes adds, removals, or not_added.
func TestNormalizeDestinationIsIdempotent(t *testing.T) {
	inputs := []string{
		"example.com/path//", "example.com/download/?token=1", "Example.COM..", "https://example.com.:8443/a//", "example.com/a%20b/", "*.example.com.", "https://example.com/?q=1", "redirect.test/go?next=https://target.test/", "198.51.100.7",
		"http://198.51.100.7:80/x/", "example.com:8443", "EXAMPLE.com/Path/", "a.b.example.com#frag", "example.com?x=1",
	}
	for _, in := range inputs {
		v1, k1, ok1 := normalizeDestination(in)
		if !ok1 {
			t.Errorf("normalize(%q) rejected an input that is valid", in)
			continue
		}
		v2, k2, ok2 := normalizeDestination(v1)
		if !ok2 || v2 != v1 || k2 != k1 {
			t.Errorf("normalize(%q) = %q/%s, but normalize(%q) = %q/%s ok=%v", in, v1, k1, v1, v2, k2, ok2)
		}
	}
}

func TestURLPathsKeepTheirSlashes(t *testing.T) {
	for in, want := range map[string]string{
		"example.com/download/?token=1": "example.com/download/?token=1",
		"https://Example.com/a/":        "example.com/a/",
		"example.com/":                  "example.com",
	} {
		if v, _, ok := normalizeDestination(in); !ok || v != want {
			t.Errorf("normalize(%q) = %q, want %q", in, v, want)
		}
	}
}

func TestParseReportTimeRejectsOverflow(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{"-213504days", "-99999999999minutes", "-0days"} {
		if got, err := parseReportTime(in, now); err == nil {
			t.Errorf("parseReportTime(%q) = %v; want an out-of-range error", in, got)
		}
	}
	if got, err := parseReportTime("-36500days", now); err != nil || !got.Before(now) {
		t.Errorf("100 years back must still parse: %v %v", got, err)
	}
}
