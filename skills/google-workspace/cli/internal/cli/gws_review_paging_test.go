// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/store"
)

// --- class: incomplete paging read as complete ----------------------------

// Directory users.list (directory_v1.json, schema Users) pages with
// nextPageToken; a page may carry no users (the array omitted or empty)
// while nextPageToken says more follow.
var googleEmptyFirstPages = map[string]string{
	"omitted": `{"kind":"admin#directory#users","etag":"\"e\"","nextPageToken":"p2"}`,
	"empty":   `{"kind":"admin#directory#users","etag":"\"e\"","users":[],"nextPageToken":"p2"}`,
	"null":    `{"kind":"admin#directory#users","etag":"\"e\"","users":null,"nextPageToken":"p2"}`,
	// Schema Users requires no field, so kind is not a continuation signal:
	// a page without it still pages on nextPageToken (r2 finding).
	"omitted-no-kind": `{"nextPageToken":"p2"}`,
	"empty-no-kind":   `{"users":[],"nextPageToken":"p2"}`,
	"null-no-kind":    `{"users":null,"etag":"\"e\"","nextPageToken":"p2"}`,
}

// A configured Google list page with no items and no nextPageToken is the
// last page, kind or not: never a record to emit or store.
var googleEmptyLastPages = map[string]string{
	"kind":    `{"kind":"admin#directory#users","etag":"\"e\""}`,
	"no-kind": `{"etag":"\"e\""}`,
	"bare":    `{}`,
	"null":    `{"users":null}`,
}

func TestGoogleEmptyLastPageIsNotARecord(t *testing.T) {
	for name, page := range googleEmptyLastPages {
		items, next, more := extractResourcePage(json.RawMessage(page), resourceReadConfigFor("users"))
		if items == nil || len(items) != 0 || next != "" || more {
			t.Errorf("%s: extract: items=%s next=%q more=%v", name, items, next, more)
		}
		g := &fakePageGetter{first: page}
		var out bytes.Buffer
		if err := fetchAndEmit(context.Background(), g, "/admin/directory/v1/users", nil, resourceReadConfigFor("users"), json.NewEncoder(&out)); err != nil {
			t.Fatalf("%s: tail: %v", name, err)
		}
		if strings.TrimSpace(out.String()) != "" {
			t.Errorf("%s: tail emitted the empty envelope as a record: %s", name, out.String())
		}
		db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		res := syncResource(context.Background(), &fakePageGetter{first: page}, db, "users", "", true, 0, false, false, nil, io.Discard)
		_ = db.Close()
		if res.Err != nil || res.Count != 0 {
			t.Errorf("%s: sync stored the empty envelope: count=%d err=%v", name, res.Count, res.Err)
		}
	}
}

const googleSecondPage = `{"kind":"admin#directory#users","etag":"\"e\"","users":[{"id":"u1","primaryEmail":"second-page@acme.example"}]}`

func googlePageFor(first string, params map[string]string) string {
	if params["pageToken"] == "p2" {
		return googleSecondPage
	}
	return first
}

func TestExtractResourcePageKeepsNextTokenOnEmptyGooglePage(t *testing.T) {
	for name, first := range googleEmptyFirstPages {
		items, next, more := extractResourcePage(json.RawMessage(first), resourceReadConfigFor("users"))
		if items == nil || len(items) != 0 || next != "p2" || !more {
			t.Errorf("%s: an empty Google page must keep its nextPageToken: items=%v next=%q more=%v", name, items, next, more)
		}
	}
}

func TestExportFollowsNextTokenPastEmptyGooglePage(t *testing.T) {
	for name, first := range googleEmptyFirstPages {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			restore, err := cliutil.SetHomeOverride(home)
			if err != nil {
				t.Fatal(err)
			}
			defer restore()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, googlePageFor(first, map[string]string{"pageToken": r.URL.Query().Get("pageToken")}))
			}))
			defer srv.Close()
			t.Setenv("GOOGLE_WORKSPACE_BASE_URL", srv.URL)
			t.Setenv("GOOGLE_WORKSPACE_TOKEN", "test-token")
			root := RootCmd()
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			root.SetArgs([]string{"export", "users", "--format", "json", "--no-cache", "--home", home})
			if err := root.Execute(); err != nil {
				t.Fatalf("export: %v %s", err, errOut.String())
			}
			if !strings.Contains(out.String(), "second-page@acme.example") {
				t.Fatalf("export stopped at an empty page that had a nextPageToken: %s", out.String())
			}
		})
	}
}

type fakePageGetter struct {
	first string
	calls []map[string]string
}

func (f *fakePageGetter) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	cp := map[string]string{}
	for k, v := range params {
		cp[k] = v
	}
	f.calls = append(f.calls, cp)
	return json.RawMessage(googlePageFor(f.first, params)), nil
}

func (f *fakePageGetter) RateLimit() float64 { return 0 }

func TestTailFollowsNextTokenPastEmptyGooglePage(t *testing.T) {
	for name, first := range googleEmptyFirstPages {
		g := &fakePageGetter{first: first}
		var out bytes.Buffer
		if err := fetchAndEmit(context.Background(), g, "/admin/directory/v1/users", withSyncParamDefaults("users", nil), resourceReadConfigFor("users"), json.NewEncoder(&out)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(out.String(), "second-page@acme.example") {
			t.Errorf("%s: tail stopped at an empty page that had a nextPageToken: %s", name, out.String())
		}
	}
}

func TestSyncFollowsNextTokenPastEmptyGooglePage(t *testing.T) {
	for name, first := range googleEmptyFirstPages {
		t.Run(name, func(t *testing.T) {
			db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			g := &fakePageGetter{first: first}
			res := syncResource(context.Background(), g, db, "users", "", true, 0, false, false, nil, io.Discard)
			if res.Err != nil {
				t.Fatalf("sync: %v", res.Err)
			}
			if res.Count != 1 || len(g.calls) < 2 {
				t.Fatalf("sync recorded users complete after an empty page that had a nextPageToken: count=%d calls=%v", res.Count, g.calls)
			}
		})
	}
}
