// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"sort"
	"strings"
)

// Hand-fix (handfixes.json mcp-search-desc-resources, issue #629):
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/search_desc.go.tmpl after a reprint, and wires the one
// WithDescription call of the <api>_search tool in code_orch.go to it. The
// same function is proposed for the press template (fleet_handfixes.json
// names the upstream PR), so a reprint from a press that carries it drops
// this file.

// codeOrchSearchResourceCap bounds the resource list in the <api>_search
// description, so a large API does not flood the host's tool list.
const codeOrchSearchResourceCap = 300

// codeOrchSearchResources returns a sentence naming the API's resource
// families, appended to the <api>_search description ("" when the registry
// names none). Hosts that pick tools by matching words (BM25 over each tool's
// name and description) find the search tool for "<api> <resource>" only when
// its description names the resource.
//
// A family is an endpoint_id's part before the first dot. Its words drop the
// API name when the family starts with it ("<api>-owners" is "owners"), then a
// first or last word that more than half of the families share as their first
// or last word (a spec-wide tag such as "crm"). Families are listed by
// endpoint count, most first, then by name, and the list, with its closing
// ", and more" when families are left out, stays within
// codeOrchSearchResourceCap characters.
func codeOrchSearchResources(apiName string, endpoints []codeOrchEndpoint) string {
	words := func(s string) []string {
		return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		})
	}
	hasPrefix := func(w, p []string) bool {
		if len(p) == 0 || len(w) <= len(p) {
			return false
		}
		for i := range p {
			if w[i] != p[i] {
				return false
			}
		}
		return true
	}
	name := words(apiName)
	counts := map[string]int{}
	for _, ep := range endpoints {
		prefix, _, _ := strings.Cut(ep.ID, ".")
		counts[prefix]++
	}
	type family struct {
		words []string
		count int
	}
	families := make([]family, 0, len(counts))
	for prefix, n := range counts {
		w := words(prefix)
		if hasPrefix(w, name) {
			w = w[len(name):]
		}
		if len(w) > 0 {
			families = append(families, family{w, n})
		}
	}
	// A first or last word more than half of the families share is a tag
	// the spec put on every resource, not a resource.
	for _, last := range []bool{false, true} {
		at := func(f family) string {
			if last {
				return f.words[len(f.words)-1]
			}
			return f.words[0]
		}
		shared := map[string]int{}
		for _, f := range families {
			if len(f.words) > 1 {
				shared[at(f)]++
			}
		}
		for word, n := range shared {
			if 2*n <= len(families) {
				continue
			}
			for i, f := range families {
				if len(f.words) > 1 && at(f) == word {
					if last {
						families[i].words = f.words[:len(f.words)-1]
					} else {
						families[i].words = f.words[1:]
					}
				}
			}
		}
	}
	merged := map[string]int{}
	for _, f := range families {
		merged[strings.Join(f.words, " ")] += f.count
	}
	names := make([]string, 0, len(merged))
	for n := range merged {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if merged[names[i]] != merged[names[j]] {
			return merged[names[i]] > merged[names[j]]
		}
		return names[i] < names[j]
	})
	var list []string
	size := 0
	for _, n := range names {
		add := len(n)
		if len(list) > 0 {
			add += len(", ")
		}
		if size+add > codeOrchSearchResourceCap {
			break
		}
		list = append(list, n)
		size += add
	}
	more := ""
	if len(list) < len(names) {
		more = ", and more"
		for len(list) > 0 && size+len(more) > codeOrchSearchResourceCap {
			size -= len(list[len(list)-1])
			if len(list) > 1 {
				size -= len(", ")
			}
			list = list[:len(list)-1]
		}
	}
	if len(list) == 0 {
		return ""
	}
	return " Resources: " + strings.Join(list, ", ") + more + "."
}
