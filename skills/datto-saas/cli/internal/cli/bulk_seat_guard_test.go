// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCanonicalBulkSeatBody(t *testing.T) {
	ids := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf("%q", fmt.Sprintf("id-%d", i))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	cases := []struct {
		name, body, want, wantErr string
	}{
		{"valid", `{"seat_type":"User","action_type":"License","ids":["a"]}`, `{"seat_type":"User","action_type":"License","ids":["a"]}`, ""},
		{"csv ids from MCP", `{"action_type":"License","ids":"a1, b2","seat_type":"User"}`, `{"seat_type":"User","action_type":"License","ids":["a1","b2"]}`, ""},
		{"valid 100", `{"seat_type":"SharedDrive","action_type":"Pause","ids":` + ids(100) + `}`, "", ""},
		{"lowercase seat type", `{"seat_type":"user","action_type":"License","ids":["a"]}`, "", `use "User"`},
		{"lowercase action", `{"seat_type":"User","action_type":"unlicense","ids":["a"]}`, "", `use "Unlicense"`},
		{"unknown action", `{"seat_type":"User","action_type":"Delete","ids":["a"]}`, "", "not one of"},
		{"missing seat type", `{"action_type":"License","ids":["a"]}`, "", "seat_type is required"},
		{"no ids", `{"seat_type":"User","action_type":"License","ids":[]}`, "", "at least one"},
		{"101 ids", `{"seat_type":"User","action_type":"License","ids":` + ids(101) + `}`, "", "exceeds"},
		{"blank id", `{"seat_type":"User","action_type":"License","ids":["a"," "]}`, "", "ids[1] is empty"},
		{"trailing comma csv", `{"seat_type":"User","action_type":"License","ids":"a1,"}`, `{"seat_type":"User","action_type":"License","ids":["a1"]}`, ""},
		{"json array string from MCP", `{"seat_type":"User","action_type":"License","ids":"[\"a,b\"]"}`, `{"seat_type":"User","action_type":"License","ids":["a,b"]}`, ""},
		{"empty in json array string", `{"seat_type":"User","action_type":"License","ids":"[\"\"]"}`, "", "ids[0] is empty"},
		{"unicode alias key", `{"seat_type":"User","action_type":"License","ids":` + ids(101) + `,"idſ":["a"]}`, "", `unexpected key "idſ"`},
		{"case alias key", `{"Seat_Type":"User","seat_type":"User","action_type":"License","ids":["a"]}`, "", "unexpected key"},
		{"duplicate key", `{"seat_type":"User","seat_type":"Site","action_type":"License","ids":["a"]}`, "", "duplicate key"},
		{"trailing data", `{"seat_type":"User","action_type":"License","ids":["a"]} {}`, "", "trailing data"},
		{"not json", `seat_type=User`, "", "JSON object"},
	}
	for _, tc := range cases {
		got, err := canonicalBulkSeatBody([]byte(tc.body))
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			} else if tc.want != "" && string(got) != tc.want {
				t.Errorf("%s: canonical = %s, want %s", tc.name, got, tc.want)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}
}

type captureRT struct{ body string }

func (c *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		c.body = string(b)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]")), Header: http.Header{}}, nil
}

func TestBulkSeatGuardTransportForwardsOnlyCanonicalBody(t *testing.T) {
	next := &captureRT{}
	guard := bulkSeatGuard{next: next}
	put := func(body string) error {
		req, _ := http.NewRequest(http.MethodPut, "https://api.datto.com/v1/saas/1/Classic:Office365:2/bulkSeatChange", strings.NewReader(body))
		_, err := guard.RoundTrip(req)
		return err
	}
	if err := put(`{"seat_type":"User","action_type":"License","ids":"a1,b2"}`); err != nil {
		t.Fatal(err)
	}
	if next.body != `{"seat_type":"User","action_type":"License","ids":["a1","b2"]}` {
		t.Fatalf("forwarded body = %s", next.body)
	}
	next.body = ""
	if err := put(`{"seat_type":"User","action_type":"License","ids":["a"],"idſ":["b"]}`); err == nil || next.body != "" {
		t.Fatalf("alias key must be refused before sending; err=%v sent=%q", err, next.body)
	}
	// Non-bulkSeatChange requests pass through untouched.
	req, _ := http.NewRequest(http.MethodGet, "https://api.datto.com/v1/saas/domains", nil)
	if _, err := guard.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
}

func TestBulkChangeHelpCarriesWriteWarning(t *testing.T) {
	bc, _, err := RootCmd().Find([]string{"seats", "bulk-change"})
	if err != nil || bc == nil || bc.Name() != "bulk-change" {
		t.Fatalf("seats bulk-change not found: %v", err)
	}
	for _, want := range []string{"only write", "no confirmation prompt", "--dry-run", "never retried"} {
		if !strings.Contains(bc.Long, want) {
			t.Errorf("Long help lacks %q", want)
		}
	}
	for flag, want := range map[string]string{"action-type": "Unlicense", "ids": "At most 100"} {
		f := bc.Flags().Lookup(flag)
		if f == nil || !strings.Contains(f.Usage, want) {
			t.Errorf("--%s help lacks %q", flag, want)
		}
	}
}
