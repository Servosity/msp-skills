// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"strings"
	"testing"
)

// RedactText changes only the values the one rule replaces: key order,
// number spelling and layout stay as printed, and applying it twice changes
// nothing more (an opaque document's digest is kept, not re-digested).
func TestRedactTextSplicesOnlyRedactedValues(t *testing.T) {
	in := "{\n  \"zeta\": 12345678901234567890,\n  \"pin\": 864209,\n  \"alpha\": \"keep\",\n  \"extensionAttributes\": [\n    {\"name\": \"Device PIN\", \"values\": [\"864209\"]}\n  ],\n  \"scriptContents\": \"password=ScriptOnly-7abc\"\n}\n"
	out := RedactText(in)
	for _, gone := range []string{"864209", "ScriptOnly-7abc"} {
		if strings.Contains(out, gone) {
			t.Fatalf("RedactText kept %q:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"\"zeta\": 12345678901234567890,\n  \"pin\": \"[redacted]\",\n  \"alpha\": \"keep\"", "{\"name\": \"Device PIN\", \"values\": \"[redacted]\"}", "\"scriptContents\": \"[redacted sha256:"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("RedactText changed more than the redacted values; want %q in:\n%s", kept, out)
		}
	}
	if again := RedactText(out); again != out {
		t.Fatalf("RedactText is not idempotent:\n%s\n---\n%s", out, again)
	}
}

func TestRedactTextFindsEveryShape(t *testing.T) {
	cases := map[string]string{
		"embedded fragment":   `request failed: {"name":"API Token","value":"FragSecret-1a2b"} (retrying)`,
		"json lines":          "{\"id\":1,\"name\":\"a\"}\n{\"id\":2,\"lockPin\":\"FragSecret-1a2b\"}\n",
		"repeated key":        `{"name":"x","pin":"FragSecret-1a2b","pin":"[redacted]"}`,
		"xml element":         `<account><name>admin</name><password>FragSecret-1a2b</password></account>`,
		"xml attribute":       `<account name="admin" password="FragMarker-1a2b"/>`,
		"xml in a json value": `{"notes":"<account><password>FragSecret-1a2b</password></account>"}`,
		"encoded json body":   `{"body":"eyJwYXNzd29yZCI6IkZyYWdTZWNyZXQtMWEyYiJ9"}`,
	}
	for name, in := range cases {
		if out := RedactText(in); strings.Contains(out, "FragSecret-1a2b") || strings.Contains(out, "eyJwYXNzd29yZCI6IkZyYWdTZWNyZXQtMWEyYiJ9") {
			t.Errorf("%s: RedactText kept the credential:\n%s", name, out)
		}
	}
	for _, clean := range []string{`{"id":1,"name":"HQ","count":3}`, "plain text with <b>markup</b> and [brackets]", `<general><name>HQ</name></general>`} {
		if out := RedactText(clean); out != clean {
			t.Errorf("RedactText changed text with no credential:\n%s\n---\n%s", clean, out)
		}
	}
}
