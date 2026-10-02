// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import "testing"

func TestRequestSearchKeepsPluralMatchesWithoutSubstringNoise(t *testing.T) {
	ep := &codeOrchEndpoint{ID: "review-only", keywords: []string{"ticket"}}
	if codeOrchRequestScore(ep, "tickets") == 0 {
		t.Fatal("plural query lost ticket keyword")
	}
	if codeOrchRequestScore(ep, "zzzzticketzzzz") != 0 {
		t.Fatal("arbitrary reverse substring matched")
	}
}

func TestAllCatalogPathInputsAreRequired(t *testing.T) {
	for id, contract := range requestContracts {
		required := map[string]bool{}
		fields, _ := contract["required"].([]any)
		for _, value := range fields {
			required[value.(string)] = true
		}
		props, _ := contract["properties"].(map[string]any)
		for name, value := range props {
			prop := value.(map[string]any)
			if (prop["x-location"] == "path" || prop["x-location"] == "template") && !required[name] {
				t.Errorf("%s.%s path input must be required", id, name)
			}
		}
	}
}
