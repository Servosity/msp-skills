// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestSummarizeLoginsCSV(t *testing.T) {
	csvData := "Timestamp,User Principal Name,User Display Name,Status,IP Address,City,State,Country or Region\n" +
		"2026-06-01T10:00:00Z,vip@example.com,VIP User,Success,198.51.100.1,Springfield,IL,United States\n" +
		"2026-06-03T11:00:00Z,vip@example.com,VIP User,Failure,203.0.113.9,Lagos,LA,Nigeria\n" +
		"2026-06-02T09:00:00Z,vip@example.com,VIP User,Success,198.51.100.1,Springfield,IL,United States\n"
	sum := summarizeLoginsCSV([]byte(csvData))
	if sum.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", sum.TotalEvents)
	}
	if sum.FailedEvents != 1 {
		t.Errorf("FailedEvents = %d, want 1", sum.FailedEvents)
	}
	if sum.DistinctIPs != 2 {
		t.Errorf("DistinctIPs = %d, want 2", sum.DistinctIPs)
	}
	if len(sum.Countries) != 2 {
		t.Errorf("Countries = %v, want 2 entries", sum.Countries)
	}
	if sum.LastLogin != "2026-06-03T11:00:00Z" {
		t.Errorf("LastLogin = %q, want the newest timestamp", sum.LastLogin)
	}

	// Garbage input degrades gracefully instead of erroring.
	bad := summarizeLoginsCSV([]byte("\x00\xff not a csv \"unterminated"))
	if bad.TotalEvents != 0 || bad.ParseLimitedNote == "" {
		t.Errorf("garbage CSV should produce zero events and a note, got %+v", bad)
	}

	// Empty input also degrades gracefully.
	empty := summarizeLoginsCSV([]byte(""))
	if empty.TotalEvents != 0 {
		t.Errorf("empty CSV should produce zero events, got %+v", empty)
	}
}

func TestCaseMatchesEmployee(t *testing.T) {
	tests := []struct {
		name     string
		affected string
		email    string
		want     bool
	}{
		{"full email match", "vip@example.com", "vip@example.com", true},
		{"email inside text", "Case affecting vip@example.com today", "vip@example.com", true},
		{"first.last vs display name", "First Last", "first.last@example.com", true},
		{"underscore local part", "First Last", "first_last@example.com", true},
		{"case insensitive", "FIRST LAST", "first.last@example.com", true},
		{"different person", "Other Person", "first.last@example.com", false},
		{"empty affected", "", "vip@example.com", false},
		{"empty email", "First Last", "", false},
		{"short local part must not substring-match", "Albert Jones", "al@corp.com", false},
		{"short single token ambiguous", "Jo Smith", "jo@corp.com", false},
		{"partial token set does not match", "First Other", "first.last@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caseMatchesEmployee(tt.affected, tt.email); got != tt.want {
				t.Errorf("caseMatchesEmployee(%q, %q) = %v, want %v", tt.affected, tt.email, got, tt.want)
			}
		})
	}
}
