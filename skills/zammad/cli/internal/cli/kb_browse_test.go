// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestNovelKbBrowseHelpWires smoke-tests that the kb browse command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelKbBrowseHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"kb", "browse", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kb browse --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "browse"} {
		if !strings.Contains(help, want) {
			t.Fatalf("kb browse --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestParseZammadKBBundleChoosesOneDeterministicTranslation(t *testing.T) {
	data := json.RawMessage(`{
		"KnowledgeBaseLocale": {
			"1": {"id": 1, "primary": false},
			"2": {"id": 2, "primary": true}
		},
		"KnowledgeBaseCategory": {
			"10": {"id": 10, "parent_id": null}
		},
		"KnowledgeBaseCategoryTranslation": {
			"101": {"id": 101, "category_id": 10, "kb_locale_id": 1, "title": "English Category"},
			"102": {"id": 102, "category_id": 10, "kb_locale_id": 2, "title": "Primary Category"}
		},
		"KnowledgeBaseAnswer": {
			"20": {"id": 20, "category_id": 10, "published": true},
			"21": {"id": 21, "category_id": 10, "published": true}
		},
		"KnowledgeBaseAnswerTranslation": {
			"201": {"id": 201, "answer_id": 20, "kb_locale_id": 1, "title": "English Answer", "content_attributes": {"body": "English Body"}},
			"202": {"id": 202, "answer_id": 20, "kb_locale_id": 2, "title": "Primary Answer", "content_attributes": {"body": "Primary Body"}},
			"211": {"id": 211, "answer_id": 21, "kb_locale_id": 10, "title": "Locale Ten", "content_attributes": {"body": "Body Ten"}},
			"212": {"id": 212, "answer_id": 21, "kb_locale_id": 3, "title": "Locale Three", "content_attributes": {"body": "Body Three"}}
		}
	}`)

	bundle, err := parseZammadKBBundle(data)
	if err != nil {
		t.Fatalf("parseZammadKBBundle: %v", err)
	}
	if len(bundle.Categories) != 1 || bundle.Categories[0].Title != "Primary Category" {
		t.Fatalf("category translation = %+v, want primary locale title", bundle.Categories)
	}
	for _, tt := range []struct {
		id, title, body string
	}{
		{id: "20", title: "Primary Answer", body: "Primary Body"},
		{id: "21", title: "Locale Three", body: "Body Three"},
	} {
		t.Run(tt.id, func(t *testing.T) {
			answer := bundle.Answers[tt.id]
			if answer.Title != tt.title || answer.Body != tt.body {
				t.Fatalf("answer %s translation = title %q body %q, want title %q body %q", tt.id, answer.Title, answer.Body, tt.title, tt.body)
			}
		})
	}
}
