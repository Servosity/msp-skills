// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import "testing"

func TestAgentVersionsNewestAndBehind(t *testing.T) {
	clients := []map[string]any{{"Id": float64(1), "Name": "Acme"}, {"Id": float64(2), "Name": "Globex"}}
	computers := []map[string]any{
		{"Id": float64(10), "ClientId": float64(1), "RemoteAgentVersion": "2023.9.110"},
		{"Id": float64(11), "ClientId": float64(1), "RemoteAgentVersion": "2023.10.1"},
		{"Id": float64(12), "ClientId": float64(2), "RemoteAgentVersion": "2023.10.1"},
		{"Id": float64(13), "ClientId": float64(2), "RemoteAgentVersion": "2023.9.2"},
		{"Id": float64(14), "ClientId": float64(2)},
	}
	rep := AgentVersions(computers, clients, false)
	// 2023.10.1 > 2023.9.110 numerically, though "2023.9" sorts after "2023.1" as text.
	if rep.Newest != "2023.10.1" || rep.Total != 5 || rep.Behind != 2 || rep.Unknown != 1 {
		t.Fatalf("report = %+v", rep)
	}
	want := []string{"2023.10.1", "2023.9.110", "2023.9.2", "(unknown)"}
	if len(rep.Versions) != len(want) {
		t.Fatalf("versions = %+v", rep.Versions)
	}
	for i, v := range want {
		if rep.Versions[i].Version != v {
			t.Fatalf("versions[%d] = %s, want %s", i, rep.Versions[i].Version, v)
		}
	}
	if !rep.Versions[0].Newest || rep.Versions[0].Clients["Acme"] != 1 || rep.Versions[0].Clients["Globex"] != 1 {
		t.Fatalf("newest group = %+v", rep.Versions[0])
	}
	behind := AgentVersions(computers, clients, true)
	if len(behind.Versions) != 3 || behind.Behind != 2 || behind.Total != 5 {
		t.Fatalf("behind-only = %+v", behind)
	}
}

func TestAgentVersionsEmptyStore(t *testing.T) {
	rep := AgentVersions(nil, nil, false)
	if rep.Total != 0 || rep.Newest != "" || rep.Versions == nil {
		t.Fatalf("empty = %+v", rep)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"2020.11", "2020.11.1", -1}, {"10.0", "9.9", 1}, {"1.2", "1.2", 0}, {"1.b", "1.a", 1}} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
