// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "single-read-keeps-the-object" entry.

package cli

import (
	"encoding/json"
	"testing"
)

func TestApplyEntityResponsePath(t *testing.T) {
	for _, tc := range []struct{ name, body, path, want string }{
		{"alert keeps the object", `{"id":"a1","severity":"high","allowedActions":["acknowledge"]}`, "allowedActions", `{"id":"a1","severity":"high","allowedActions":["acknowledge"]}`},
		{"tamper keeps current password", `{"enabled":true,"password":"cur","previousPasswords":[{"password":"old"}]}`, "previousPasswords", `{"enabled":true,"password":"cur","previousPasswords":[{"password":"old"}]}`},
		{"items+pages unwraps", `{"items":[{"id":"e1"}],"pages":{"nextKey":"k"}}`, "items", `[{"id":"e1"}]`},
		{"bare wrapper unwraps", `{"bands":[1,2]}`, "bands", `[1,2]`},
		{"siem cursor unwraps", `{"items":[1],"has_more":false,"next_cursor":"c"}`, "items", `[1]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applyEntityResponsePath(json.RawMessage(tc.body), tc.path)
			var g, w any
			_ = json.Unmarshal(got, &g)
			_ = json.Unmarshal([]byte(tc.want), &w)
			gb, _ := json.Marshal(g)
			wb, _ := json.Marshal(w)
			if string(gb) != string(wb) {
				t.Fatalf("got %s, want %s", gb, wb)
			}
		})
	}
}
