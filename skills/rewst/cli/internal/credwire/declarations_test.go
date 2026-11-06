// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"encoding/json"
	"os"
	"testing"
)

// The kit holds a response field by its name alone, so a declared name that is
// also a Rewst field would hold that product data in every response. testdata
// pins every name spec.graphql defines (fields, arguments, types).
func TestDeclaredResponseFieldsAreNoRewstProductName(t *testing.T) {
	data, err := os.ReadFile("testdata/rewst-spec-names.json")
	if err != nil {
		t.Fatal(err)
	}
	var pinned struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(data, &pinned); err != nil {
		t.Fatal(err)
	}
	sdl := map[string]bool{}
	for _, n := range pinned.Names {
		sdl[n] = true
	}
	// The credentials the SDL itself names: each must be an SDL field.
	documented := map[string]bool{"auth0ClientSecret": true, "auth0ClientId": true,
		"getCannyToken": true, "getHaloLiveChatToken": true, "getSkilljarLoginToken": true}
	for name := range documented {
		if !sdl[name] {
			t.Errorf("%s is declared as a Rewst credential field but spec.graphql has no such name", name)
		}
	}
	seen := map[string]bool{}
	for _, name := range Declarations.ResponseFields {
		seen[name] = true
		if !documented[name] && sdl[name] {
			t.Errorf("ResponseFields %q is a Rewst product name; holding it would withhold that data in every answer", name)
		}
	}
	for name := range documented {
		if !seen[name] {
			t.Errorf("Rewst answers with the credential %s, but ResponseFields does not declare it", name)
		}
	}
	for _, name := range Declarations.CredentialFields {
		if sdl[name] {
			t.Errorf("CredentialFields %q is a Rewst product name; holding it would withhold that data in every request", name)
		}
	}
}
