// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

// Review class "aggregation on display label": every report groups by the
// client's Id and renders a unique label only at output.

import (
	"testing"
	"time"
)

func clientLabelsOf[T any](rows []T, label func(T) string) map[string]int {
	m := map[string]int{}
	for _, r := range rows {
		m[label(r)]++
	}
	return m
}

// Computers synced without their clients: two ClientIds that carry the same
// ClientName stay two clients in every report.
func TestClientIdWithoutIndexStaysApart(t *testing.T) {
	computers := []map[string]any{
		{"Id": float64(10), "ClientId": float64(1), "ClientName": "Acme", "Status": "Online", "RemoteAgentVersion": "1"},
		{"Id": float64(20), "ClientId": float64(2), "ClientName": "Acme", "Status": "Offline", "RemoteAgentVersion": "1"},
	}
	now := time.Now()
	if got := ClientRollups(computers, nil, nil, nil, ""); len(got) != 2 {
		t.Fatalf("client-rollup merged two ClientIds into %d row(s): %+v", len(got), got)
	}
	if got := FleetHealth(computers, nil, nil, 30, now); len(got) != 2 {
		t.Fatalf("fleet-health merged two ClientIds into %d row(s): %+v", len(got), got)
	}
	if got := AgentVersions(computers, nil, false); len(got.Versions) != 1 || len(got.Versions[0].Clients) != 2 {
		t.Fatalf("agent-versions merged two ClientIds: %+v", got)
	}
	stale := StaleAgents(computers, nil, -1, now)
	if labels := clientLabelsOf(stale, func(s StaleAgent) string { return s.Client }); len(labels) != 2 {
		t.Fatalf("stale-agents gave two ClientIds one label: %+v", stale)
	}
}

// A client really named "Acme (#1)" never shares a label with client 1 when
// two clients are named "Acme".
func TestDisambiguatedLabelCannotCollide(t *testing.T) {
	clients := []map[string]any{{"Id": float64(1), "Name": "Acme"}, {"Id": float64(2), "Name": "Acme"}, {"Id": float64(3), "Name": "Acme (#1)"}}
	computers := []map[string]any{{"Id": float64(10), "ClientId": float64(1)}, {"Id": float64(30), "ClientId": float64(3)}}
	got := ClientRollups(computers, clients, nil, nil, "")
	if len(got) != 3 {
		t.Fatalf("clients 1 and 3 share a row: %+v", got)
	}
	if labels := clientLabelsOf(got, func(r ClientRollup) string { return r.Client }); len(labels) != 3 {
		t.Fatalf("labels collide: %+v", got)
	}
}

// An alert that names only its client resolves to that client when the name
// is unambiguous, so it is counted on the client's own row.
func TestNameOnlyAlertJoinsItsClient(t *testing.T) {
	clients := []map[string]any{{"Id": float64(1), "Name": "Acme"}}
	computers := []map[string]any{{"Id": float64(10), "ClientId": float64(1), "Status": "Online"}}
	alerts := []map[string]any{{"Id": float64(7), "ClientName": "Acme", "Priority": float64(3), "Message": "x"}}
	got := ClientRollups(computers, clients, nil, alerts, "")
	if len(got) != 1 || got[0].OpenAlerts != 1 {
		t.Fatalf("a name-only alert did not land on its client: %+v", got)
	}
}

// A Closed alert (documented Alert.Status) is not open in any report.
func TestClosedAlertIsNotOpen(t *testing.T) {
	clients := []map[string]any{{"Id": float64(1), "Name": "Acme"}}
	computers := []map[string]any{{"Id": float64(10), "ClientId": float64(1), "Status": "Online"}}
	alerts := []map[string]any{{"Id": float64(7), "ComputerId": float64(10), "Status": "Closed", "Priority": float64(4), "Message": "resolved"}}
	if got := AlertTriage(alerts, computers, clients, 0); len(got) != 0 {
		t.Fatalf("alert-triage listed a Closed alert: %+v", got)
	}
	if got := FleetHealth(computers, clients, alerts, 30, time.Now()); got[0].OpenAlerts != 0 {
		t.Fatalf("fleet-health counted a Closed alert: %+v", got)
	}
	if got := ClientRollups(computers, clients, nil, alerts, ""); got[0].OpenAlerts != 0 {
		t.Fatalf("client-rollup counted a Closed alert: %+v", got)
	}
}
