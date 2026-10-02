// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import "testing"

func TestRequestSearchKeepsPluralMatchesWithoutSubstringNoise(t *testing.T) {
	for _, pair := range [][2]string{{"ticket", "tickets"}, {"address", "addresses"}, {"status", "statuses"}, {"policy", "policies"}, {"company", "companies"}, {"patch", "patches"}, {"switch", "switches"}} {
		for _, forms := range [][2]string{pair, {pair[1], pair[0]}} {
			ep := &codeOrchEndpoint{ID: "review-only", keywords: []string{forms[0]}}
			if codeOrchRequestScore(ep, forms[1]) == 0 {
				t.Errorf("%q lost keyword %q", forms[1], forms[0])
			}
			if codeOrchRequestScore(ep, "zzzz"+forms[0]+"zzzz") != 0 {
				t.Fatal("arbitrary reverse substring matched")
			}
		}
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
