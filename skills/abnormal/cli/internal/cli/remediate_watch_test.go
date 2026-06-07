// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestParseActionReceipt(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
		wantErr bool
	}{
		{"threat snake_case", `{"action_id":"a33a212a-89ff-461f-be34-ea52aff44a67","status_url":"https://api.example/v1/threats/x/actions/a33a212a"}`, "a33a212a-89ff-461f-be34-ea52aff44a67", false},
		{"case camelCase", `{"actionId":"61e76395-40d3-4d78-b6a8-8b17634d0f5b","statusUrl":"https://api.example/v1/cases/1/actions/61e76395"}`, "61e76395-40d3-4d78-b6a8-8b17634d0f5b", false},
		{"snake wins when both present", `{"action_id":"snake","actionId":"camel"}`, "snake", false},
		{"missing id errors", `{"status_url":"x"}`, "", true},
		{"invalid json errors", `not-json`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseActionReceipt([]byte(tt.payload))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseActionReceipt() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseActionReceipt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemediateWatchActionValidation(t *testing.T) {
	for action := range threatActions {
		if !threatActions[action] {
			t.Errorf("threat action %q should be valid", action)
		}
	}
	if threatActions["acknowledge_resolved"] {
		t.Errorf("case action leaked into threat actions")
	}
	if caseActions["remediate"] {
		t.Errorf("threat action leaked into case actions")
	}
	for _, a := range []string{"action_required", "acknowledge_resolved", "acknowledge_in_progress", "acknowledge_not_an_attack"} {
		if !caseActions[a] {
			t.Errorf("case action %q should be valid", a)
		}
	}
}
