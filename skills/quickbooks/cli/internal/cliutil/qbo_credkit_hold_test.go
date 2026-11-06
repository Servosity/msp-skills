// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"slices"
	"testing"
)

func TestQBOURLCredentials(t *testing.T) {
	for raw, want := range map[string][]string{
		"https://u:Secret9q@host/":               {"Secret9q"},
		"https://u:Sec%2Fret9q@host/":            {"Sec%2Fret9q", "Sec/ret9q"},
		"https://TokenAsUser9q@host/":            {"TokenAsUser9q"},
		"https://u:Secret9q@host:bad/x":          {"Secret9q"},
		"https://host/x?api_key=Secret9q&page=2": {"Secret9q"},
		"https://host/?%74oken=Secret9q":         {"Secret9q"},
		"https://host/?x=%zz&%74oken=Secret9q":   {"Secret9q"},
		"https://host/?token=!!!!!!!!":           {"!!!!!!!!"},
		"https://host/?token=Secret9q#frag":      {"Secret9q"},
		// A value a setting must send, and no credential at all.
		"https://host/hook?v=2&channel=ops": nil,
		"https://developer.intuit.com/docs": nil,
		"https://host/a@b/c":                nil,
		"":                                  nil,
	} {
		if got := qboURLCredentials(raw); !slices.Equal(got, want) {
			t.Errorf("qboURLCredentials(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestQBOCredentialProfileFlag(t *testing.T) {
	for name, want := range map[string]bool{"refresh-token": true, "Client_Secret": true, "api-key": true, "limit": false, "base-url": false, "select": false} {
		if got := QBOCredentialProfileFlag(name); got != want {
			t.Errorf("QBOCredentialProfileFlag(%q) = %v", name, got)
		}
	}
}
