// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// Hand-fix (handfixes.json mcp-search-desc-resources, issue #629):
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/search_desc_test.go.tmpl after a reprint.

import (
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestCodeOrchSearchResourcesNamesFamilies(t *testing.T) {
	eps := func(ids ...string) []codeOrchEndpoint {
		out := make([]codeOrchEndpoint, 0, len(ids))
		for _, id := range ids {
			out = append(out, codeOrchEndpoint{ID: id})
		}
		return out
	}
	cases := map[string]struct {
		api  string
		eps  []codeOrchEndpoint
		want string
	}{
		"none": {"acme", nil, ""},
		"by count then name": {"acme", eps("users.get", "tickets.list", "tickets.get", "kb.answer-create", "users.list", "tickets.update"),
			" Resources: tickets, users, kb."},
		"api name and a shared tag dropped": {"acme", eps("acme-owners-crm.get", "acme-deals-crm.list", "acme-deals-crm.get",
			"acme-line-items-crm.list", "crm.batch", "acme.version"),
			" Resources: deals, acme, crm, line items, owners."},
		"a shared first word dropped": {"acme", eps("v2-users.get", "v2-tickets.get", "v2-tickets.list"),
			" Resources: tickets, users."},
		"a minority word kept": {"acme", eps("ticket-notes.get", "company-notes.get", "tickets.get", "companies.get", "users.get"),
			" Resources: companies, company notes, ticket notes, tickets, users."},
		"snake and kebab api name": {"acme-psa", eps("acme_psa_contracts.get", "acme-psa-tickets.get"),
			" Resources: contracts, tickets."},
	}
	for name, c := range cases {
		if got := codeOrchSearchResources(c.api, c.eps); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestCodeOrchSearchResourcesStopsAtCap(t *testing.T) {
	var many []codeOrchEndpoint
	for i := 0; i < 200; i++ {
		many = append(many, codeOrchEndpoint{ID: "resource-family-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".get"})
	}
	got := codeOrchSearchResources("acme", many)
	list := strings.TrimSuffix(strings.TrimPrefix(got, " Resources: "), ".")
	if !strings.HasSuffix(got, ", and more.") || len(list) > codeOrchSearchResourceCap || len(list) < codeOrchSearchResourceCap-40 {
		t.Fatalf("a long list must stop just under %d characters and say so; got %d: %q", codeOrchSearchResourceCap, len(list), got)
	}
	edge := codeOrchSearchResources("acme", []codeOrchEndpoint{{ID: strings.Repeat("a", 149) + ".get"}, {ID: strings.Repeat("b", 149) + ".get"}, {ID: "z.get"}})
	if want := " Resources: " + strings.Repeat("a", 149) + ", and more."; edge != want {
		t.Fatalf("the closing \", and more\" must fit the cap too; got %d characters: %q", len(edge), edge)
	}
	if again := codeOrchSearchResources("acme", many); again != got {
		t.Fatalf("the description is not deterministic:\n%q\n%q", got, again)
	}
}

// TestSearchToolFoundByResourceWords ranks the registered tools the way a
// host that picks tools by words does (BM25 over each tool's name and
// description) for "<api> <resource>", for every resource the <api>_search
// description names. Without the list a host ranks other tools above
// <api>_search for such a query (issue #629).
//
// A tool whose description names <api>_search (its _get, _read and _execute
// siblings) leads an agent there, so it is not counted above it. For a
// resource only the list names (no other tool and not the description's own
// lead carries one of its words), <api>_search must be in the top
// searchToolTopN. A resource another tool also names is served by that tool
// too, so it is only logged with its rank before and after.
func TestSearchToolFoundByResourceWords(t *testing.T) {
	const searchToolTopN = 3
	s := server.NewMCPServer("search-desc-test", "test")
	RegisterTools(s)
	with := map[string][]string{}
	descs := map[string]string{}
	var searchTool, api, lead, resources string
	for name, st := range s.ListTools() {
		with[name] = bm25Words(name + " " + st.Tool.Description)
		descs[name] = st.Tool.Description
		if strings.HasSuffix(name, "_search") && strings.HasPrefix(st.Tool.Description, "Search the ") {
			searchTool = name
			api, _, _ = strings.Cut(strings.TrimPrefix(st.Tool.Description, "Search the "), " API ")
			lead, resources, _ = strings.Cut(st.Tool.Description, " Resources: ")
		}
	}
	if searchTool == "" {
		t.Fatal("no <api>_search tool is registered")
	}
	resources = strings.TrimSuffix(strings.TrimSuffix(resources, "."), ", and more")
	if resources == "" {
		t.Fatalf("%s names no resources, so a host that ranks tools by words cannot find it", searchTool)
	}
	pointers := map[string]bool{}
	others := map[string]bool{}
	for name, desc := range descs {
		if name == searchTool {
			continue
		}
		if strings.Contains(desc, searchTool) {
			pointers[name] = true
		}
		for _, w := range with[name] {
			others[w] = true
		}
	}
	for _, w := range bm25Words(searchTool + " " + lead) {
		others[w] = true
	}
	without := map[string][]string{}
	for name, words := range with {
		without[name] = words
	}
	without[searchTool] = bm25Words(searchTool + " " + lead)
	for _, family := range strings.Split(resources, ", ") {
		query := bm25Words(api + " " + family)
		after, before := bm25Rank(with, query, searchTool, pointers), bm25Rank(without, query, searchTool, pointers)
		t.Logf("%q: %s ranks %d (%d without the resource list)", api+" "+family, searchTool, after, before)
		if after <= searchToolTopN {
			continue
		}
		onlyHere := false
		for _, w := range bm25Words(family) {
			onlyHere = onlyHere || !others[w]
		}
		if onlyHere {
			t.Errorf("query %q ranks %s at %d, want top %d: only its description names %q", api+" "+family, searchTool, after, searchToolTopN, family)
		}
	}
}

func bm25Words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// bm25Rank is the 1-based rank of want among docs for query (Okapi BM25,
// k1 1.2, b 0.75), not counting the docs in skip; ties rank by name.
func bm25Rank(docs map[string][]string, query []string, want string, skip map[string]bool) int {
	df := map[string]int{}
	total := 0
	for _, words := range docs {
		total += len(words)
		seen := map[string]bool{}
		for _, w := range words {
			if !seen[w] {
				seen[w] = true
				df[w]++
			}
		}
	}
	n := float64(len(docs))
	avg := float64(total) / n
	score := map[string]float64{}
	names := make([]string, 0, len(docs))
	for name, words := range docs {
		names = append(names, name)
		tf := map[string]int{}
		for _, w := range words {
			tf[w]++
		}
		for _, q := range query {
			if tf[q] == 0 {
				continue
			}
			idf := math.Log((n-float64(df[q])+0.5)/(float64(df[q])+0.5) + 1)
			f := float64(tf[q])
			score[name] += idf * f * 2.2 / (f + 1.2*(0.25+0.75*float64(len(words))/avg))
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if score[names[i]] != score[names[j]] {
			return score[names[i]] > score[names[j]]
		}
		return names[i] < names[j]
	})
	rank := 1
	for _, name := range names {
		if name == want {
			return rank
		}
		if !skip[name] {
			rank++
		}
	}
	return rank
}
