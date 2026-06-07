// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNovelSoftwareFind(t *testing.T) {
	t.Run("empty store returns empty envelope echoing the query", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "software", "find", "openssl")
		if err != nil {
			t.Fatalf("software find: %v", err)
		}
		var env map[string]any
		assertJSON(t, out, &env)
		if env["query"] != "openssl" {
			t.Errorf("envelope should echo query, got %v", env["query"])
		}
		items, ok := env["items"].([]any)
		if !ok || len(items) != 0 {
			t.Errorf("empty store should yield empty items, got %v", env["items"])
		}
		if env["total"] != float64(0) {
			t.Errorf("total should be 0, got %v", env["total"])
		}
		if note, _ := env["note"].(string); note == "" {
			t.Errorf("empty result should carry an explanatory note")
		}
	})

	t.Run("matches installed package case-insensitively", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "software", "a1", `{"agent":"a1","software":[{"name":"OpenSSL","version":"3.0","publisher":"OpenSSL"},{"name":"Chrome","version":"120"}]}`)
		out, _, err := runNovel(t, "software", "find", "openssl")
		if err != nil {
			t.Fatalf("software find: %v", err)
		}
		var env struct {
			Query string           `json:"query"`
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		assertJSON(t, out, &env)
		if env.Total != 1 || len(env.Items) != 1 || env.Items[0]["name"] != "OpenSSL" {
			t.Fatalf("want one OpenSSL match, got %+v", env)
		}
	})

	t.Run("bare invocation shows help", func(t *testing.T) {
		novelTestEnv(t)
		var flags rootFlags
		root := newRootCmd(&flags)
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs([]string{"software", "find"})
		if err := root.Execute(); err != nil {
			t.Fatalf("bare invocation should exit 0 with help: %v", err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("bare invocation should print help, got %q", out.String())
		}
	})

	t.Run("blank --name with flags set is a usage error", func(t *testing.T) {
		novelTestEnv(t)
		_, _, err := runNovel(t, "software", "find", "--name", "")
		if err == nil {
			t.Fatalf("blank query in real mode should be a usage error")
		}
	})
}
