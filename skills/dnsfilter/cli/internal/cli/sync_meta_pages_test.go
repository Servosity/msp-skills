// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: sync-meta-total-pages-continues).

package cli

import (
	"net/url"
	"path/filepath"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// meta.current_page below meta.total_pages is a continuation
// (spec.json#/components/schemas/UnblockRequestsMetaSpec, GET
// /v2/unblock_requests). Codex review round 1: a short first page carrying it
// ended the walk, saved the mirror complete, and --full pruned the rest.
func TestSyncMetaTotalPagesContinues(t *testing.T) {
	testenv.Isolate(t)
	req := func(id string) string {
		return `{"id":"` + id + `","type":"unblock_requests","attributes":{"domain":"` + id + `.example"}}`
	}
	lan := func(id string) string {
		return `{"id":"` + id + `","type":"lan_ips","attributes":{"ip":"10.0.0.` + id + `"}}`
	}
	meta := func(page int) string {
		return `"meta":{"current_page":` + string(rune('0'+page)) + `,"total_pages":2,"total_count":2}`
	}
	api := syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		page := syncR17Page(q)
		switch path {
		case "/v2/unblock_requests":
			switch page {
			case 1:
				return `{"data":[` + req("11111111-1111-4111-8111-111111111111") + `],` + meta(1) + `}`, true
			case 2:
				return `{"data":[` + req("22222222-2222-4222-8222-222222222222") + `],` + meta(2) + `}`, true
			}
			return `{"data":[]}`, true
		case "/v1/networks/all":
			if page != 1 {
				return `{"data":[]}`, true
			}
			return `{"data":[{"id":"2","type":"networks","attributes":{"name":"HQ"}}]}`, true
		case "/v1/networks/2/lan_ips":
			switch page {
			case 1:
				return `{"data":[` + lan("1") + `],` + meta(1) + `}`, true
			case 2:
				return `{"data":[` + lan("2") + `],` + meta(2) + `}`, true
			}
			return `{"data":[]}`, true
		}
		return "", false
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out, err := syncHFRun("sync", "--full", "--resources", "unblock-requests,networks,lan_ips", "--db", dbPath)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := api.last("/v2/unblock_requests").Get("page[number]"); got != "2" {
		t.Fatalf("meta.total_pages on the short first page was not followed (last page requested %q):\n%s", got, out)
	}
	if n := syncR17Count(t, dbPath, "unblock-requests"); n != 2 {
		t.Fatalf("flat walk stored %d unblock-requests, want 2:\n%s", n, out)
	}
	if n := syncR17Count(t, dbPath, "lan_ips"); n != 2 {
		t.Fatalf("per-parent walk stored %d lan_ips, want 2:\n%s", n, out)
	}
	syncHFWantComplete(t, dbPath, "unblock-requests", 1, "walk that followed meta.total_pages")
	capped := filepath.Join(t.TempDir(), "data.db")
	_, _ = syncHFRun("sync", "--resources", "unblock-requests", "--max-pages", "1", "--db", capped)
	syncHFWantComplete(t, capped, "unblock-requests", 0, "--max-pages 1 on a short page with meta pages remaining")
}
