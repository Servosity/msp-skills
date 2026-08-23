// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestNovelRemediateHelpWires smoke-tests that the remediate command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRemediateHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"remediate", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remediate --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "remediate"} {
		if !strings.Contains(help, want) {
			t.Fatalf("remediate --help missing %q in output:\n%s", want, help)
		}
	}
}

// entityTypeClient serves canned /v1.0/search/entity/{id} responses and records
// the scope it was asked with.
type entityTypeClient struct {
	byID       map[string]string // entityId -> saasEntityType
	paths      []string
	lastParams map[string]string
	fail       bool
}

func (c *entityTypeClient) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	c.paths = append(c.paths, path)
	c.lastParams = params
	if c.fail {
		return nil, fmt.Errorf("live lookup unavailable")
	}
	id := path[strings.LastIndex(path, "/")+1:]
	t, ok := c.byID[id]
	if !ok {
		return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`), nil
	}
	return json.RawMessage(fmt.Sprintf(
		`{"responseEnvelope":{"responseCode":200},"responseData":[{"entityInfo":{"entityId":%q,"saasEntityType":%q}}]}`,
		id, t)), nil
}

// TestResolveEntityTypeUsesTheEntitysOwnType is the regression guard for the
// defect that made both remediation commands unusable.
//
// The action endpoint looks entityType up per SaaS and throws server-side on a
// miss, so a hardcoded "email" returned HTTP 500 internal_error[KeyError] on
// every tenant. The value is a property of the entity, so it has to be read
// from the entity.
func TestResolveEntityTypeUsesTheEntitysOwnType(t *testing.T) {
	c := &entityTypeClient{byID: map[string]string{
		"aaa": "office365_emails_email",
	}}
	got, err := resolveEntityType(context.Background(), c, "", []string{"aaa"}, "farm:tenant", t.TempDir()+"/none.db")
	if err != nil {
		t.Fatalf("resolveEntityType() error = %v", err)
	}
	if got != "office365_emails_email" {
		t.Errorf("resolveEntityType() = %q, want office365_emails_email", got)
	}
	if got == "email" {
		t.Error("resolved the hardcoded value that caused the 500")
	}
	if c.lastParams["scopes"] != "farm:tenant" {
		t.Errorf("lookup sent scopes=%q, want the caller's scope", c.lastParams["scopes"])
	}
}

// TestResolveEntityTypeOverrideWins keeps the escape hatch working for an
// entity the CLI cannot look up at all.
func TestResolveEntityTypeOverrideWins(t *testing.T) {
	c := &entityTypeClient{byID: map[string]string{"aaa": "google_mail_email"}}
	got, err := resolveEntityType(context.Background(), c, "  custom_type  ", []string{"aaa"}, "farm:tenant", t.TempDir()+"/none.db")
	if err != nil {
		t.Fatalf("resolveEntityType() error = %v", err)
	}
	if got != "custom_type" {
		t.Errorf("resolveEntityType() = %q, want the override", got)
	}
	if len(c.paths) != 0 {
		t.Error("override still performed a lookup; it should short-circuit")
	}
}

// TestResolveEntityTypeRejectsAMixedBatch: the endpoint takes one entityType
// per call, so a batch spanning platforms would silently act on some targets
// with the wrong type.
func TestResolveEntityTypeRejectsAMixedBatch(t *testing.T) {
	c := &entityTypeClient{byID: map[string]string{
		"aaa": "office365_emails_email",
		"bbb": "google_mail_email",
	}}
	_, err := resolveEntityType(context.Background(), c, "", []string{"aaa", "bbb"}, "farm:tenant", t.TempDir()+"/none.db")
	if err == nil {
		t.Fatal("mixed batch resolved without error")
	}
	for _, want := range []string{"google_mail_email", "office365_emails_email", "--entity-type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestResolveEntityTypeFailsClosed: guessing "email" is what produced an
// unexplained 500, so an unresolvable target must stop with instructions.
func TestResolveEntityTypeFailsClosed(t *testing.T) {
	c := &entityTypeClient{fail: true}
	_, err := resolveEntityType(context.Background(), c, "", []string{"unknown-id"}, "farm:tenant", t.TempDir()+"/none.db")
	if err == nil {
		t.Fatal("unresolvable target resolved without error")
	}
	if !strings.Contains(err.Error(), "--entity-type") {
		t.Errorf("error %q does not tell the operator how to proceed", err)
	}
}

// cacheAwareClient implements both getters so the test can tell which one a
// poll loop reached for.
type cacheAwareClient struct{ cached, fresh int }

func (c *cacheAwareClient) Get(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	c.cached++
	return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`), nil
}

func (c *cacheAwareClient) GetNoCache(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	c.fresh++
	return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`), nil
}

// TestGetFreshBypassesTheResponseCache is the regression guard for a poll that
// could never observe the thing it was polling for.
//
// The response cache is keyed on path plus params, so a watcher reading through
// it sees the pre-action state on every iteration. Measured 2026-08-22: an
// entity that flipped to quarantined three seconds after submission was still
// reported unchanged after a two-minute poll, because every read after the
// first was served from cache. The command then reported a successful action as
// unfinished.
func TestGetFreshBypassesTheResponseCache(t *testing.T) {
	c := &cacheAwareClient{}
	if _, err := getFresh(context.Background(), c, "/v1.0/task/1", nil); err != nil {
		t.Fatalf("getFresh() error = %v", err)
	}
	if c.fresh != 1 {
		t.Errorf("GetNoCache called %d times, want 1", c.fresh)
	}
	if c.cached != 0 {
		t.Errorf("cached Get called %d times, want 0; a poll cannot read through the cache", c.cached)
	}
}

// TestGetFreshFallsBackToPlainGet keeps the helper usable with any client that
// has no cache to bypass.
func TestGetFreshFallsBackToPlainGet(t *testing.T) {
	c := &entityTypeClient{byID: map[string]string{}}
	if _, err := getFresh(context.Background(), c, "/v1.0/search/entity/x", nil); err != nil {
		t.Fatalf("getFresh() error = %v", err)
	}
	if len(c.paths) != 1 {
		t.Errorf("plain Get called %d times, want 1", len(c.paths))
	}
}

// TestEntityFlagsReflect pins what counts as the action having landed. This is
// the only honest confirmation available when the API returns no task id: a 200
// means accepted, not applied.
func TestEntityFlagsReflect(t *testing.T) {
	tests := []struct {
		name   string
		flags  entityFlags
		action string
		want   bool
	}{
		{"quarantine landed", entityFlags{Quarantined: true}, "quarantine", true},
		{"quarantine not yet visible", entityFlags{Quarantined: false}, "quarantine", false},
		{"restore landed, flag set", entityFlags{Restored: true}, "restore", true},
		{"restore landed, simply no longer quarantined", entityFlags{Quarantined: false}, "restore", true},
		{"restore not yet visible", entityFlags{Quarantined: true, Restored: false}, "restore", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.flags.reflects(tt.action); got != tt.want {
				t.Errorf("reflects(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}
