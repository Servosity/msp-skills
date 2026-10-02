// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"fmt"
	"regexp"
	"testing"
)

func TestRequestContractCatalogCoverage(t *testing.T) {
	if len(requestContracts) != len(codeOrchEndpoints) {
		t.Fatalf("contracts=%d endpoints=%d", len(requestContracts), len(codeOrchEndpoints))
	}
	for _, ep := range codeOrchEndpoints {
		t.Run(ep.ID, func(t *testing.T) {
			schema, ok := requestContracts[ep.ID]
			if !ok {
				t.Fatal("missing executor contract")
			}
			if schema["type"] != "object" {
				t.Fatal("executor params must be an object")
			}
			properties := schema["properties"].(map[string]any)
			for _, placeholder := range regexp.MustCompile(`\{([^{}]+)\}`).FindAllStringSubmatch(ep.Path, -1) {
				found := false
				for name, value := range properties {
					property := value.(map[string]any)
					wire, _ := property["x-wire-name"].(string)
					if wire == "" {
						wire = name
					}
					if wire == placeholder[1] && (property["x-location"] == "path" || property["x-location"] == "template") {
						found = true
					}
				}
				if !found {
					t.Errorf("undiscoverable path input %s", placeholder[1])
				}
			}
			if ep.Method == "GET" {
				for name, value := range properties {
					if property := value.(map[string]any); property["x-location"] == "body" {
						t.Errorf("GET advertises an unsent body input %s", name)
					}
				}
			} else if ep.BodyIsArray && (ep.Method == "POST" || ep.Method == "PUT" || ep.Method == "PATCH" || ep.Method == "DELETE") {
				body, _ := properties["body"].(map[string]any)
				if body["type"] != "array" || body["x-location"] != "body" {
					t.Error("array-body endpoint must describe its executable body input")
				}
			}
			validateContractNode(t, schema, ep.ID, true)
		})
	}
}

func validateContractNode(t *testing.T, schema map[string]any, path string, root bool) {
	t.Helper()
	props, _ := schema["properties"].(map[string]any)
	if required, ok := schema["required"].([]any); ok {
		for _, name := range required {
			if _, ok := props[fmt.Sprint(name)]; !ok {
				t.Errorf("%s requires undiscoverable field %v", path, name)
			}
		}
	}
	for name, value := range props {
		child, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s.%s is not a schema", path, name)
		}
		if root {
			switch child["x-location"] {
			case "path", "template", "query", "header", "body":
			default:
				t.Errorf("%s.%s missing request location", path, name)
			}
		}
		validateContractNode(t, child, path+"."+name, false)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		validateContractNode(t, items, path+"[]", false)
	}
}
