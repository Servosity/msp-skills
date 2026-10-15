// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"testing"
)

// keyPager serves Sophos key-paged envelopes: three pages linked by pages.nextKey.
type keyPager struct{ sent []string }

func (k *keyPager) GetWithHeaders(_ context.Context, _ string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	from := params["pageFromKey"]
	k.sent = append(k.sent, from)
	pages := map[string]string{
		"":   `{"items":[{"id":"a"},{"id":"b"}],"pages":{"nextKey":"k2","size":2}}`,
		"k2": `{"items":[{"id":"c"},{"id":"d"}],"pages":{"fromKey":"k2","nextKey":"k3","size":2}}`,
		"k3": `{"items":[{"id":"e"}],"pages":{"fromKey":"k3","size":1}}`,
	}
	return json.RawMessage(pages[from]), nil
}

// The key-paging hand-fix passes these exact arguments; the generated default
// ("", "offset", ..., "") stops after the first page.
func TestSophosKeyPagingFetchesEveryPage(t *testing.T) {
	k := &keyPager{}
	out, err := paginatedGet(context.Background(), k, "/endpoint/v1/endpoints", map[string]string{"pageSize": "2"}, nil,
		true, "pageFromKey", "cursor", "pageSize", 50, "pages.nextKey", "")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Items []map[string]string `json:"items"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		var bare []map[string]string
		if json.Unmarshal(out, &bare) != nil {
			t.Fatalf("unexpected output %s", out)
		}
		env.Items = bare
	}
	if len(env.Items) != 5 {
		t.Fatalf("got %d items across key pages, want 5 (sent keys %v)", len(env.Items), k.sent)
	}
	if len(k.sent) != 3 || k.sent[1] != "k2" || k.sent[2] != "k3" {
		t.Fatalf("pageFromKey sequence %v, want [\"\" k2 k3]", k.sent)
	}
}
