// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type scriptedRT struct {
	statuses []int
	setCook  []string
	seen     []string
}

func (s *scriptedRT) RoundTrip(req *http.Request) (*http.Response, error) {
	i := len(s.seen)
	s.seen = append(s.seen, req.Header.Get("Cookie"))
	h := http.Header{}
	if s.setCook[i] != "" {
		h.Add("Set-Cookie", s.setCook[i])
	}
	return &http.Response{StatusCode: s.statuses[i], Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

func get(t *testing.T, rt http.RoundTripper) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", "https://riverside.com/api/v4/projects/x", nil)
	req.Header.Set("Cookie", "riverside_auth_access=old.old.old; riverside_auth_refresh=r.r.r")
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestSessionRetryUsesRotatedCookieOnce(t *testing.T) {
	base := &scriptedRT{statuses: []int{403, 200}, setCook: []string{"riverside_auth_access=new.new.new; Path=/", ""}}
	if resp := get(t, &sessionRetryTransport{base: base}); resp.StatusCode != 200 {
		t.Fatalf("status = %d, want the retried 200", resp.StatusCode)
	}
	if len(base.seen) != 2 || !strings.Contains(base.seen[1], "riverside_auth_access=new.new.new") || strings.Contains(base.seen[1], "old.old.old") {
		t.Fatalf("retry did not carry the rotated cookie: %q", base.seen)
	}
}

func TestSessionRetryNeverLoops(t *testing.T) {
	base := &scriptedRT{statuses: []int{401, 401, 401}, setCook: []string{"riverside_auth_access=a.a.a", "riverside_auth_access=b.b.b", ""}}
	if resp := get(t, &sessionRetryTransport{base: base}); resp.StatusCode != 401 || len(base.seen) != 2 {
		t.Fatalf("status=%d calls=%d, want the second 401 after exactly one retry", resp.StatusCode, len(base.seen))
	}
}

func TestSessionRetrySkipsWithoutRotation(t *testing.T) {
	for _, set := range []string{"", "riverside_auth_access=deleted", "other=x.y.z", "riverside_auth_access=notajwt"} {
		base := &scriptedRT{statuses: []int{403, 200}, setCook: []string{set, ""}}
		if resp := get(t, &sessionRetryTransport{base: base}); resp.StatusCode != 403 || len(base.seen) != 1 {
			t.Fatalf("Set-Cookie %q: status=%d calls=%d, want the 403 with no retry", set, resp.StatusCode, len(base.seen))
		}
	}
}
