// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the topology edge/degree summarizer.

package cli

import "testing"

func TestSummarizeTopology(t *testing.T) {
	// Device 1 is a hub connected to 2, 3, 4; 2-3 also linked.
	doc := topologyDoc{Edges: []topologyEdge{
		{From: 1, To: 2},
		{From: 1, To: 3},
		{From: 1, To: 4},
		{From: 2, To: 3},
	}}

	got := summarizeTopology("100", "live", doc, 10)

	if got.Edges != 4 {
		t.Fatalf("edges = %d, want 4", got.Edges)
	}
	if got.Nodes != 4 {
		t.Fatalf("nodes = %d, want 4 (devices 1-4)", got.Nodes)
	}
	if got.AgentID != "100" || got.Source != "live" {
		t.Fatalf("agent/source = %q/%q, want 100/live", got.AgentID, got.Source)
	}
	if len(got.TopNodes) == 0 || got.TopNodes[0].DeviceID != 1 || got.TopNodes[0].Degree != 3 {
		t.Fatalf("top node = %+v, want device 1 with degree 3", got.TopNodes[0])
	}
}

func TestSummarizeTopologyTopN(t *testing.T) {
	doc := topologyDoc{Edges: []topologyEdge{{From: 1, To: 2}, {From: 3, To: 4}, {From: 5, To: 6}}}
	got := summarizeTopology("1", "cached", doc, 2)
	if len(got.TopNodes) != 2 {
		t.Fatalf("top nodes = %d, want capped at 2", len(got.TopNodes))
	}
}

func TestSummarizeTopologyEmpty(t *testing.T) {
	got := summarizeTopology("1", "live", topologyDoc{}, 10)
	if got.Edges != 0 || got.Nodes != 0 || len(got.TopNodes) != 0 {
		t.Fatalf("empty topology should summarize to zeros, got %+v", got)
	}
}
