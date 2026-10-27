// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "testing"

// A value substituted into the base URL decides which host receives the
// credential, so it must be one DNS label (hand-fix handfixes.json
// host-template-single-label).
func TestBuildURLRefusesAHostTemplateValueThatIsNotOneLabel(t *testing.T) {
	base := "https://{datacenter}.acronis.com"
	for _, bad := range []string{
		"sink.example/", "sink.example", "sink.example#", "sink.example?", "sink.example\\",
		"user@sink.example", "sink:8443", "sink%2eexample", " eu2-cloud", "eu2-cloud ", "-eu2", "eu2-",
		"a/b", "..", "eu2-cloud\n", "еu2-cloud",
	} {
		if got, err := buildURL(base, "/api/2/clients", map[string]string{"datacenter": bad}); err == nil {
			t.Errorf("datacenter %q built %q; want an error", bad, got)
		}
		if got := ResolvedBaseURL(base, map[string]string{"datacenter": bad}); got != base {
			t.Errorf("ResolvedBaseURL with datacenter %q = %q; want the unresolved base", bad, got)
		}
	}
	for _, good := range []string{"eu2-cloud", "us-cloud", "au-cloud", "eu8-cloud", "dev-cloud"} {
		got, err := buildURL(base, "/api/2/clients", map[string]string{"datacenter": good})
		if err != nil || got != "https://"+good+".acronis.com/api/2/clients" {
			t.Errorf("datacenter %q: got %q, %v", good, got, err)
		}
	}
	// Path placeholders are not host labels and keep their own rules.
	if got, err := buildURL("https://eu2-cloud.acronis.com", "/api/{datacenter}/x", map[string]string{"datacenter": "a.b"}); err != nil || got != "https://eu2-cloud.acronis.com/api/a.b/x" {
		t.Errorf("path placeholder: got %q, %v", got, err)
	}
}
