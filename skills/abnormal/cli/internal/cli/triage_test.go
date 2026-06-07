// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestAttackTypeWeight(t *testing.T) {
	tests := []struct {
		name       string
		attackType string
		want       int
	}{
		{"account takeover ranks highest", "Internal-to-Internal Attacks (Email Account Takeover)", 40},
		{"extortion", "Extortion", 36},
		{"malware", "Malware", 34},
		{"bec invoice fraud", "Invoice/Payment Fraud (BEC)", 32},
		{"credential phishing", "Phishing: Credential", 30},
		{"social engineering", "Social Engineering (BEC)", 28},
		{"sensitive data phishing", "Phishing: Sensitive Data", 26},
		{"scam", "Scam", 22},
		{"reconnaissance", "Reconnaissance", 14},
		{"spam ranks low", "Spam", 6},
		{"graymail ranks lowest", "Graymail", 4},
		{"empty gets mid-low default", "", 10},
		{"unknown gets mid default", "Quantum Hijack", 18},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := attackTypeWeight(tt.attackType); got != tt.want {
				t.Errorf("attackTypeWeight(%q) = %d, want %d", tt.attackType, got, tt.want)
			}
		})
	}
}

func TestRecencyWeight(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		age  time.Duration
		want int
	}{
		{"under an hour", 30 * time.Minute, 40},
		{"under six hours", 3 * time.Hour, 35},
		{"under a day", 20 * time.Hour, 30},
		{"under three days", 48 * time.Hour, 20},
		{"under a week", 5 * 24 * time.Hour, 10},
		{"older than a week", 30 * 24 * time.Hour, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recencyWeight(now.Add(-tt.age), now); got != tt.want {
				t.Errorf("recencyWeight(age=%s) = %d, want %d", tt.age, got, tt.want)
			}
		})
	}
	if got := recencyWeight(time.Time{}, now); got != 5 {
		t.Errorf("recencyWeight(zero) = %d, want 5", got)
	}
}

func TestMessageRemediated(t *testing.T) {
	tests := []struct {
		name string
		msg  threatDocMessage
		want bool
	}{
		{"auto remediated bool", threatDocMessage{AutoRemediated: true}, true},
		{"auto remediated string", threatDocMessage{AutoRemediated: "true"}, true},
		{"post remediated", threatDocMessage{PostRemediated: true}, true},
		{"remediation timestamp", threatDocMessage{RemediationTimestamp: "2026-06-01T00:00:00Z"}, true},
		{"status remediated", threatDocMessage{RemediationStatus: "Auto-Remediated"}, true},
		{"status not remediated", threatDocMessage{RemediationStatus: "Not remediated"}, false},
		{"empty status", threatDocMessage{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageRemediated(tt.msg); got != tt.want {
				t.Errorf("messageRemediated(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestScoreThreat(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	doc := threatDoc{
		ThreatID:       "t-1",
		RecipientCount: 7,
		Messages: []threatDocMessage{
			{
				Subject:           "Urgent wire",
				FromAddress:       "ceo@evil.test",
				ReceivedTime:      now.Add(-30 * time.Minute).Format(time.RFC3339),
				AttackType:        "Invoice/Payment Fraud (BEC)",
				ImpersonatedParty: "VIP",
			},
		},
	}
	item := scoreThreat(doc, now)
	// 32 (BEC) + 40 (fresh) + 10 (impersonation) = 82
	if item.Score != 82 {
		t.Errorf("score = %d, want 82", item.Score)
	}
	if item.Remediated {
		t.Errorf("unremediated message marked remediated")
	}
	if item.Subject != "Urgent wire" || item.MessageCount != 1 || item.RecipientCount != 7 {
		t.Errorf("representative fields wrong: %+v", item)
	}

	// All messages remediated -> Remediated true.
	doc.Messages[0].AutoRemediated = true
	if got := scoreThreat(doc, now); !got.Remediated {
		t.Errorf("fully remediated threat not marked remediated")
	}

	// Newest message wins representative fields.
	multi := threatDoc{ThreatID: "t-2", Messages: []threatDocMessage{
		{Subject: "old", ReceivedTime: now.Add(-48 * time.Hour).Format(time.RFC3339), AttackType: "Spam"},
		{Subject: "new", ReceivedTime: now.Add(-1 * time.Hour).Format(time.RFC3339), AttackType: "Malware"},
	}}
	got := scoreThreat(multi, now)
	if got.Subject != "new" || got.AttackType != "Malware" {
		t.Errorf("newest message did not win: %+v", got)
	}
	if got.Remediated {
		t.Errorf("unremediated multi-message threat marked remediated")
	}
}
