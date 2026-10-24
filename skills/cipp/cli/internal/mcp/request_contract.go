// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix mcp-executor-request-contracts (issue #351): search results carry a
// params_schema per endpoint (executor input names, x-location, types,
// required fields, known enum values) and the executor routes inputs by it.
// request_contracts.json is generated from the OpenAPI spec and the
// code_orch.go endpoint table; regenerate it after a reprint.
package mcp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

//go:embed request_contracts.json
var requestContractJSON []byte

var requestContracts = loadRequestContracts()
var requestContractTerms = indexRequestContracts()

func loadRequestContracts() map[string]map[string]any {
	var contracts map[string]map[string]any
	if err := json.Unmarshal(requestContractJSON, &contracts); err != nil {
		panic(fmt.Sprintf("invalid embedded request contracts: %v", err))
	}
	return contracts
}

func requestSearchTerms(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	terms := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if text != "" {
		terms = append(terms, text)
	}
	return terms
}

// indexRequestContracts collects each endpoint's input names and enum values
// so a search for a body field ("ConvertToShared") finds its endpoint.
func indexRequestContracts() map[string]map[string]bool {
	result := make(map[string]map[string]bool, len(requestContracts))
	for id, schema := range requestContracts {
		terms := map[string]bool{}
		add := func(value string) {
			exact := strings.ToLower(strings.TrimSpace(value))
			for _, term := range requestSearchTerms(value) {
				if (len(term) < 3 || codeOrchStopwords[term]) && term != exact {
					continue
				}
				terms[term] = true
			}
		}
		var visit func(map[string]any)
		visit = func(node map[string]any) {
			if values, ok := node["enum"].([]any); ok {
				for _, value := range values {
					add(fmt.Sprint(value))
				}
			}
			if properties, ok := node["properties"].(map[string]any); ok {
				for name, value := range properties {
					add(name)
					if child, ok := value.(map[string]any); ok {
						visit(child)
					}
				}
			}
			if items, ok := node["items"].(map[string]any); ok {
				visit(items)
			}
			for _, key := range []string{"allOf", "anyOf", "oneOf"} {
				if children, ok := node[key].([]any); ok {
					for _, child := range children {
						if child, ok := child.(map[string]any); ok {
							visit(child)
						}
					}
				}
			}
		}
		visit(schema)
		result[id] = terms
	}
	return result
}

// codeOrchPluralMatch matches ordinary plural inflections without matching
// arbitrary surrounding text.
func codeOrchPluralMatch(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		singular, plural := pair[0], pair[1]
		if plural == singular+"s" {
			return true
		}
		if strings.HasSuffix(singular, "y") && plural == strings.TrimSuffix(singular, "y")+"ies" {
			return true
		}
		for _, ending := range []string{"s", "x", "z", "ch", "sh"} {
			if strings.HasSuffix(singular, ending) && plural == singular+"es" {
				return true
			}
		}
	}
	return false
}

// codeOrchRequestScore ranks an endpoint for a search query: an exact input
// name or enum value scores 4, an exact keyword 2, a substring or plural 1.
func codeOrchRequestScore(ep *codeOrchEndpoint, query string) int {
	score := 0
	seen := map[string]bool{}
	for _, term := range requestSearchTerms(query) {
		if seen[term] {
			continue
		}
		seen[term] = true
		if requestContractTerms[ep.ID][term] {
			score += 4
		}
		if len(term) < 3 || codeOrchStopwords[term] {
			continue
		}
		for _, word := range ep.keywords {
			if word == term {
				score += 2
			} else if len(word) >= 3 && (strings.Contains(word, term) || codeOrchPluralMatch(term, word)) {
				score++
			}
		}
	}
	return score
}

// codeOrchResolveContract fills path placeholders and collects header inputs
// the contract declares, removing them from params so they do not also land
// in the query string or body. Query and body inputs stay in params.
func codeOrchResolveContract(ep *codeOrchEndpoint, path string, params map[string]any) (string, map[string]string) {
	headers := map[string]string{}
	properties, _ := requestContracts[ep.ID]["properties"].(map[string]any)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		property, _ := properties[name].(map[string]any)
		location, _ := property["x-location"].(string)
		value, present := params[name]
		if !present {
			continue
		}
		switch location {
		case "header":
			headers[name] = formatMCPParamValue(value)
		case "path":
			path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(formatMCPParamValue(value)))
		default:
			continue
		}
		delete(params, name)
	}
	return path, headers
}

// codeOrchHasBody reports whether the endpoint takes a request body. CIPP has
// DELETE endpoints with bodies; sending their inputs as a query string drops
// the body the API reads.
func codeOrchHasBody(ep *codeOrchEndpoint) bool {
	properties, _ := requestContracts[ep.ID]["properties"].(map[string]any)
	for _, value := range properties {
		property, _ := value.(map[string]any)
		if property["x-location"] == "body" {
			return true
		}
	}
	return false
}

// codeOrchContractBody is the request body: params["body"] as-is for an
// endpoint whose body root is not an object, else the params object.
func codeOrchContractBody(ep *codeOrchEndpoint, params map[string]any) any {
	properties, _ := requestContracts[ep.ID]["properties"].(map[string]any)
	body, _ := properties["body"].(map[string]any)
	if raw, _ := body["x-raw-body"].(bool); raw {
		if value, ok := params["body"]; ok {
			return value
		}
	}
	if ep.BodyIsArray {
		return codeOrchArrayBody(params)
	}
	return codeOrchWriteBody(params)
}

// codeOrchValidatePathInputs refuses a call missing a required path input,
// which would otherwise reach the API with a literal {placeholder}.
func codeOrchValidatePathInputs(ep *codeOrchEndpoint, params map[string]any) error {
	schema := requestContracts[ep.ID]
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	for _, field := range required {
		name, _ := field.(string)
		property, _ := properties[name].(map[string]any)
		if property["x-location"] != "path" {
			continue
		}
		if value, present := params[name]; present && value != nil && formatMCPParamValue(value) != "" {
			continue
		}
		return fmt.Errorf("required path parameter %q is missing (see params_schema)", name)
	}
	return nil
}
