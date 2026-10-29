// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The child's argument list is fixed literals plus validated values: a
// request path that is not a plain XAPI path never reaches argv.
func TestChildArgsAreFixedLiteralsAndValidatedPaths(t *testing.T) {
	got, err := childArgs(30*time.Second, []Request{{Path: "/Users", Params: map[string]string{"$select": "Number"}}, {Path: "/SystemStatus"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fleet", "exec", "--raw", "--timeout", "30s", "--req", "/Users?%24select=Number", "--req", "/SystemStatus", "--json", "--no-input"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	for _, bad := range []string{"--deliver=file:/tmp/x", "Users", "/a/../b", "https://evil/x", "/a?b=1", "/a b", "/a\nb", "/a%2e", `/a\b`, "/" + strings.Repeat("x", 600)} {
		if _, err := childArgs(0, []Request{{Path: bad}}); err == nil {
			t.Errorf("path %q reached argv", bad)
		}
	}
	if _, err := childArgs(0, nil); err == nil {
		t.Error("an empty request list started a child")
	}
	if _, err := (Runner{}).Fetch(t.Context(), PBX{Name: "../x", FQDN: "pbx.example.com", ClientID: "1", EnvPrefix: "THREECX_X"}, Request{Path: "/Users"}); err == nil {
		t.Error("an invalid PBX entry started a child")
	}
}
