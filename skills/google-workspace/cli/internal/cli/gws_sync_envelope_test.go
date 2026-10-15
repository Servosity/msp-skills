// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestGoogleListEnvelopeRules(t *testing.T) {
	empty := json.RawMessage(`{"kind":"admin#datatransfer#DataTransfersList","etag":"\"e\""}`)
	if !googleListPageIsEmpty("transfers", empty) {
		t.Fatal("a {kind, etag} transfers page is an empty list, not a record")
	}
	if googleListPageIsEmpty("transfers", json.RawMessage(`{"kind":"k","dataTransfers":[{"id":"1"}]}`)) {
		t.Fatal("a page with items is not empty")
	}
	if googleListPageIsEmpty("customer", empty) || googleListPageIsEmpty("transfers", json.RawMessage(`{"etag":"x"}`)) {
		t.Fatal("only synced Google list resources with a kind qualify")
	}
	if !googleListEnvelopeEnded(json.RawMessage(`{"kind":"admin#directory#users","users":[{"id":"1"}]}`)) {
		t.Fatal("no nextPageToken is the last page")
	}
	if googleListEnvelopeEnded(json.RawMessage(`{"kind":"admin#directory#users","nextPageToken":"p2"}`)) {
		t.Fatal("a nextPageToken means more pages")
	}
	if googleListEnvelopeEnded(json.RawMessage(`[{"id":"1"}]`)) || googleListEnvelopeEnded(json.RawMessage(`{"users":[]}`)) {
		t.Fatal("a bare array or kind-less body proves nothing")
	}
}
