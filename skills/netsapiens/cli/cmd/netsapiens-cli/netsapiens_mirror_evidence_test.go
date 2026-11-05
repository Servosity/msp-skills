// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (mirror-reconciliation), the built CLI: what the mirror holds is what
// the last sync walk returned. A read answered from the HTTP cache cannot write over it.

package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func domainsVendor(t *testing.T, first string) (*atomic.Value, []string) {
	t.Helper()
	body := &atomic.Value{}
	body.Store(first)
	srv, trust := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/domains") {
			_, _ = io.WriteString(w, body.Load().(string))
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	return body, append(trust, "NETSAPIENS_BASE_URL="+srv.URL, "NETSAPIENS_API_KEY="+apiKey, "NETSAPIENS_RETRY_MAX=0")
}

func TestClassCachedReadNeverOverwritesAWalk(t *testing.T) {
	cliBinary(t)
	body, env := domainsVendor(t, `[{"domain":"a.example","reseller":"r","description":"Old"}]`)
	testenv.Isolate(t)
	runCLI(t, env, "sync", "--resources", "domains")
	runCLI(t, env, "fleet", "changes", "--rebaseline", "--json")
	runCLI(t, env, "domains", "list", "--json") // warms the HTTP cache with Old
	body.Store(`[{"domain":"a.example","reseller":"r","description":"New"}]`)
	runCLI(t, env, "sync", "--resources", "domains")
	if r := runCLI(t, env, "domains", "list", "--json"); !strings.Contains(r.stdout, "Old") {
		t.Fatalf("this read was to come from the cache, so the test proved nothing:\n%s", r.stdout)
	}
	r := runCLI(t, env, "fleet", "changes", "--json")
	if !strings.Contains(r.stdout, "New") {
		t.Fatalf("the walk saw New, but a cached read wrote Old over it:\n%s\n%s", r.stdout, r.stderr)
	}
}
