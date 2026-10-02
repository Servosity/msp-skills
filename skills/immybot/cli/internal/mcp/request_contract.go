// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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

func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		properties, _ := requestContracts[ep.ID]["properties"].(map[string]any)
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			property, _ := properties[name].(map[string]any)
			if property["x-location"] != "query" {
				continue
			}
			wire, _ := property["x-wire-name"].(string)
			if wire == "" {
				wire = name
			}
			other, _ := properties[wire].(map[string]any)
			collision := name != wire && other != nil && other["x-location"] != "query"
			found := false
			for i := range ep.QueryParams {
				binding := &ep.QueryParams[i]
				if binding.WireName == wire {
					if collision {
						binding.PublicName = name
						binding.PublicOnly = true
					}
					found = true
					break
				}
			}
			if !found {
				ep.QueryParams = append(ep.QueryParams, codeOrchParamBinding{PublicName: name, WireName: wire, PublicOnly: collision})
			}
		}
	}
}

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

func indexRequestContracts() map[string]map[string]bool {
	result := make(map[string]map[string]bool, len(requestContracts))
	for id, schema := range requestContracts {
		terms := map[string]bool{}
		var visit func(map[string]any)
		add := func(value string) {
			exact := strings.ToLower(strings.TrimSpace(value))
			for _, term := range requestSearchTerms(value) {
				if (len(term) < 3 || codeOrchStopwords[term]) && term != exact {
					continue
				}
				terms[term] = true
			}
		}
		visit = func(node map[string]any) {
			if wire, ok := node["x-wire-name"].(string); ok {
				add(wire)
			}
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
			} else if len(word) >= 3 && strings.Contains(word, term) {
				score++
			}
		}
	}
	return score
}

// Request contracts describe executor inputs, not flattened CLI flag names.
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
		if location != "path" && location != "template" && location != "header" && location != "query" {
			continue
		}
		wire, _ := property["x-wire-name"].(string)
		if wire == "" {
			wire = name
		}
		value, present := params[name]
		consumed := name
		if !present && (name == wire || properties[wire] == nil) {
			value, present = params[wire]
			consumed = wire
		}
		if !present {
			continue
		}
		if location == "query" {
			other, _ := properties[wire].(map[string]any)
			if name != wire && other != nil && other["x-location"] != "query" {
				continue
			}
			params[wire] = value
			if name != wire {
				delete(params, name)
			}
			continue
		}
		if location == "header" {
			headers[wire] = formatMCPParamValue(value)
		} else {
			path = strings.ReplaceAll(path, "{"+wire+"}", url.PathEscape(formatMCPParamValue(value)))
		}
		delete(params, consumed)
	}
	return path, headers
}

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

func codeOrchQueryKeys(binding codeOrchParamBinding) []string {
	if binding.PublicOnly {
		return []string{binding.PublicName}
	}
	return []string{binding.PublicName, binding.WireName}
}

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
		wire, _ := property["x-wire-name"].(string)
		if wire != "" && properties[wire] == nil {
			if value, present := params[wire]; present && value != nil && formatMCPParamValue(value) != "" {
				continue
			}
		}
		return fmt.Errorf("required path parameter %q is missing (see params_schema)", name)
	}
	return nil
}
