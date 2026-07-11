// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"testing"
)

func TestClassifyZammadStatePrefersStateTypeID(t *testing.T) {
	tests := []struct {
		name        string
		stateTypeID sql.NullInt64
		stateName   string
		want        zammadStateKind
	}{
		{name: "new", stateTypeID: sql.NullInt64{Int64: 1, Valid: true}, stateName: "closed", want: zammadStateOpen},
		{name: "open", stateTypeID: sql.NullInt64{Int64: 2, Valid: true}, stateName: "merged", want: zammadStateOpen},
		{name: "pending", stateTypeID: sql.NullInt64{Int64: 3, Valid: true}, stateName: "custom", want: zammadStatePending},
		{name: "pending close", stateTypeID: sql.NullInt64{Int64: 4, Valid: true}, stateName: "custom", want: zammadStateResolved},
		{name: "closed", stateTypeID: sql.NullInt64{Int64: 5, Valid: true}, stateName: "open", want: zammadStateClosed},
		{name: "merged", stateTypeID: sql.NullInt64{Int64: 6, Valid: true}, stateName: "open", want: zammadStateMerged},
		{name: "removed", stateTypeID: sql.NullInt64{Int64: 7, Valid: true}, stateName: "open", want: zammadStateClosed},
		{name: "missing type falls back", stateTypeID: sql.NullInt64{}, stateName: "pending close", want: zammadStateResolved},
		{name: "unknown type falls back", stateTypeID: sql.NullInt64{Int64: 99, Valid: true}, stateName: "merged", want: zammadStateMerged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyZammadState(tt.stateTypeID, tt.stateName); got != tt.want {
				t.Fatalf("classifyZammadState(%v, %q) = %q, want %q", tt.stateTypeID, tt.stateName, got, tt.want)
			}
		})
	}
}

func TestScanLexiconUsesUnicodeWordBoundariesAndLongestOverlap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		terms []string
		want  []string
	}{
		{name: "cost is not in costume", text: "The costume arrived", terms: []string{"cost"}},
		{name: "legal is not in illegal", text: "That is illegal", terms: []string{"legal"}},
		{name: "unicode letter is a boundary blocker", text: "prélegal", terms: []string{"legal"}},
		{name: "punctuation delimits a word", text: "This is LEGAL.", terms: []string{"legal"}, want: []string{"legal"}},
		{name: "longest phrase wins", text: "Feature request: add exports", terms: []string{"feature", "feature request"}, want: []string{"feature request"}},
		{name: "cancel does not overlap cancelling", text: "I am cancelling today", terms: []string{"cancel", "cancelling"}, want: []string{"cancelling"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := scanLexicon(tt.text, tt.terms)
			got := make([]string, 0, len(matches))
			for _, match := range matches {
				got = append(got, match.Term)
			}
			if !equalStrings(got, tt.want) {
				t.Fatalf("scanLexicon(%q, %v) terms = %v, want %v", tt.text, tt.terms, got, tt.want)
			}
		})
	}
}

func TestZammadPriorityWeightCustomSynonyms(t *testing.T) {
	for _, name := range []string{"critical", "urgent response", "Emergency"} {
		t.Run(name, func(t *testing.T) {
			if got := zammadPriorityWeight("", name); got != 3 {
				t.Fatalf("zammadPriorityWeight(%q) = %v, want 3", name, got)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
