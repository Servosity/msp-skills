// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-execute-writes-refuse-unknown-args" entry.

package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	cliWriteAnnotationRE = regexp.MustCompile(`"pp:method": "(POST|PUT|PATCH)", "pp:path": "([^"]+)"`)
	cliWrappedBodyRE     = regexp.MustCompile(`body = map\[string\]any\{"([^"]+)": bodyMap\}`)
	cliFlatBodyKeyRE     = regexp.MustCompile(`bodyMap\["([^"]+)"\]`)
)

// TestCodeOrchWriteBodyKeysMatchCLI re-derives every write endpoint's
// top-level body fields from the generated companion CLI and fails when
// codeOrchWriteBodyKeys has drifted, and when a registry write endpoint has
// no entry (it would then accept no body field at all).
func TestCodeOrchWriteBodyKeysMatchCLI(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "cli", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no CLI sources found: %v", err)
	}
	derived := map[string][]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		m := cliWriteAnnotationRE.FindStringSubmatch(src)
		if m == nil {
			continue
		}
		seen := map[string]bool{}
		if wrapped := cliWrappedBodyRE.FindAllStringSubmatch(src, -1); len(wrapped) > 0 {
			for _, w := range wrapped {
				seen[w[1]] = true
			}
		} else if strings.Contains(src, "body = bodyMap") {
			for _, k := range cliFlatBodyKeyRE.FindAllStringSubmatch(src, -1) {
				seen[k[1]] = true
			}
		}
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		derived[m[1]+" "+m[2]] = keys
	}
	for k, want := range derived {
		got := append([]string{}, codeOrchWriteBodyKeys[k]...)
		sort.Strings(got)
		if len(got) == 0 && len(want) == 0 {
			if _, ok := codeOrchWriteBodyKeys[k]; !ok {
				t.Errorf("%s: missing from codeOrchWriteBodyKeys", k)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: codeOrchWriteBodyKeys = %v, CLI builds %v", k, got, want)
		}
	}
	for k := range codeOrchWriteBodyKeys {
		if _, ok := derived[k]; !ok {
			t.Errorf("%s: in codeOrchWriteBodyKeys but no CLI command builds it", k)
		}
	}
	for _, ep := range codeOrchEndpoints {
		switch ep.Method {
		case "POST", "PUT", "PATCH":
			if _, ok := codeOrchWriteBodyKeys[ep.Method+" "+ep.Path]; !ok {
				t.Errorf("registry write %s (%s %s) has no body-key entry", ep.ID, ep.Method, ep.Path)
			}
		}
	}
}

// TestCodeOrchRefuseUnknownWriteParamsUnit covers the rule without the API.
func TestCodeOrchRefuseUnknownWriteParamsUnit(t *testing.T) {
	create := findCodeOrchEndpoint("networks.create")
	destroy := findCodeOrchEndpoint("networks.destroy")
	index := findCodeOrchEndpoint("networks.index")
	suggest := findCodeOrchEndpoint("domains.suggest-threat")
	if create == nil || destroy == nil || index == nil || suggest == nil {
		t.Fatal("registry entries missing")
	}
	if err := codeOrchRefuseUnknownWriteParams(create, map[string]any{"network": map[string]any{}}); err != nil {
		t.Fatalf("wrapper body refused: %v", err)
	}
	if err := codeOrchRefuseUnknownWriteParams(create, map[string]any{"dry_run": true}); err == nil {
		t.Fatal("dry_run accepted on a POST")
	}
	if err := codeOrchRefuseUnknownWriteParams(destroy, map[string]any{"id": 7}); err != nil {
		t.Fatalf("path placeholder refused on DELETE: %v", err)
	}
	if err := codeOrchRefuseUnknownWriteParams(destroy, map[string]any{"id": 7, "force": true}); err == nil {
		t.Fatal("unknown DELETE param accepted")
	}
	if err := codeOrchRefuseUnknownWriteParams(index, map[string]any{"anything": 1}); err != nil {
		t.Fatalf("GET extras must stay query params: %v", err)
	}
	// domains.suggest-threat declares query params on a POST: both the public
	// and wire names are accepted.
	if err := codeOrchRefuseUnknownWriteParams(suggest, map[string]any{"fqdn": "a.example", "notes": "x"}); err != nil {
		t.Fatalf("declared query params on a POST refused: %v", err)
	}
}
