// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test: a cleared scalar list must not survive a re-sync.

package store

import (
	"encoding/json"
	"testing"
)

func TestMergeClearedScalarListIsAuthoritative(t *testing.T) {
	existing := json.RawMessage(`{"id":"10","attributes":{"whitelist_domains":["example.com"],"blacklist_categories":[7],"rels":[{"id":"1","name":"kept"}]}}`)
	incoming := json.RawMessage(`{"id":"10","attributes":{"whitelist_domains":[],"blacklist_categories":[],"rels":[]}}`)
	merged, ok := mergeKeepRicherJSON("policies", existing, incoming)
	if !ok {
		t.Fatal("mergeKeepRicherJSON returned !ok")
	}
	var got struct {
		Attributes struct {
			Whitelist  []string         `json:"whitelist_domains"`
			Categories []int            `json:"blacklist_categories"`
			Rels       []map[string]any `json:"rels"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Attributes.Whitelist) != 0 || len(got.Attributes.Categories) != 0 {
		t.Fatalf("cleared scalar lists survived the merge: %s", merged)
	}
	if len(got.Attributes.Rels) != 1 {
		t.Fatalf("object array lost its richer copy: %s", merged)
	}
}

func TestMergeExplicitNullTimestampClears(t *testing.T) {
	existing := json.RawMessage(`{"id":"a1","attributes":{"hostname":"PC-1","uninstalled_at":"2026-01-01","last_sync":"2026-10-01","friendly_name":"Front"}}`)
	incoming := json.RawMessage(`{"id":"a1","attributes":{"hostname":"PC-1","uninstalled_at":null,"last_sync":null,"friendly_name":null}}`)
	merged, ok := mergeKeepRicherJSON("user-agents", existing, incoming)
	if !ok {
		t.Fatal("mergeKeepRicherJSON returned !ok")
	}
	var got struct {
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"uninstalled_at", "last_sync"} {
		if v, present := got.Attributes[k]; !present || v != nil {
			t.Fatalf("%s must be present and null after an explicit clear: %s", k, merged)
		}
	}
	if got.Attributes["friendly_name"] != "Front" {
		t.Fatalf("non-timestamp null should keep the richer value: %s", merged)
	}
}

func TestMergePartialRelationshipsKeepSiblings(t *testing.T) {
	existing := json.RawMessage(`{"id":"a1","relationships":{"policy":{"data":{"id":"10"}},"network":{"data":{"id":"50"}}}}`)
	incoming := json.RawMessage(`{"id":"a1","relationships":{"policy":{"data":null}}}`)
	merged, ok := mergeKeepRicherJSON("user-agents", existing, incoming)
	if !ok {
		t.Fatal("mergeKeepRicherJSON returned !ok")
	}
	var got struct {
		Relationships map[string]struct {
			Data any `json:"data"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got.Relationships["policy"].Data != nil {
		t.Fatalf("supplied policy clear was not applied: %s", merged)
	}
	if got.Relationships["network"].Data == nil {
		t.Fatalf("omitted network relationship was dropped: %s", merged)
	}
}

func TestMergeClearedRelationshipsAreAuthoritative(t *testing.T) {
	existing := json.RawMessage(`{"id":"50","attributes":{"name":"HQ"},"relationships":{"policy":{"data":{"id":"10"}},"policies":{"data":[{"id":"10"},{"id":"11"}]}}}`)
	incoming := json.RawMessage(`{"id":"50","attributes":{"name":"HQ"},"relationships":{"policy":{"data":null},"policies":{"data":[]}}}`)
	merged, ok := mergeKeepRicherJSON("networks", existing, incoming)
	if !ok {
		t.Fatal("mergeKeepRicherJSON returned !ok")
	}
	var got struct {
		Relationships struct {
			Policy   struct{ Data any }   `json:"policy"`
			Policies struct{ Data []any } `json:"policies"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if got.Relationships.Policy.Data != nil || len(got.Relationships.Policies.Data) != 0 {
		t.Fatalf("cleared relationships survived the merge: %s", merged)
	}
}
