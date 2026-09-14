// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"acronis-pp-cli/internal/client"
	"acronis-pp-cli/internal/config"
	"acronis-pp-cli/internal/store"
)

func TestAcronisTaskContract(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Has("tenant_id") || r.URL.Query().Has("result_code") {
			t.Error("invalid wire filter", r.URL)
		}
		if calls == 1 {
			if r.URL.Query().Get("order") != "desc(startedAt)" || r.URL.Query().Get("resultCode") != "error" {
				t.Error("wire mapping", r.URL)
			}
			fmt.Fprint(w, `{"items":[{"id":9007199254740993,"tenant":{"uuid":"other"}}],"paging":{"cursors":{"after":"next+/="}}}`)
		} else {
			if r.URL.Query().Get("after") != "next+/=" || len(r.URL.Query()) != 2 {
				t.Error("continuation", r.URL)
			}
			fmt.Fprint(w, `{"items":[{"id":9007199254740994,"tenant":{"uuid":"target"}}],"paging":{"cursors":{}}}`)
		}
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL, AccessToken: "fixture"}
	c := client.New(cfg, 5*time.Second, 0)
	flags := &rootFlags{dataSource: "live"}
	data, _, err := resolveAcronisTaskRead(context.Background(), c, flags, "task-manager", "/api/task_manager/v2/tasks", map[string]string{"tenant_id": "target", "result_code": "error", "order": "startedAt desc", "limit": "100"}, false, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(data), "9007199254740994") || strings.Contains(string(data), "9007199254740993") {
		t.Fatalf("calls=%d data=%s", calls, data)
	}
}

func TestAcronisTaskPaginationFailsClosed(t *testing.T) {
	for _, mode := range []string{"missing", "repeated", "malformed", "short-missing", "short-next"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "short-missing":
					fmt.Fprint(w, `{"items":[{"id":1}],"has_more":true}`)
				case "malformed":
					fmt.Fprint(w, `{"items":[{"id":1}],"paging":{"cursors":{"after":123}}}`)
				case "missing":
					fmt.Fprint(w, `{"items":[{"id":1},{"id":2}]}`)
				case "repeated":
					fmt.Fprint(w, `{"items":[{"id":1}],"cursors":{"after":"same"}}`)
				case "short-next":
					if calls == 1 {
						fmt.Fprint(w, `{"items":[{"id":1}],"cursors":{"after":"next"}}`)
					} else {
						fmt.Fprint(w, `{"items":[],"cursors":{}}`)
					}
				}
			}))
			defer srv.Close()
			c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "fixture"}, 5*time.Second, 0)
			_, err := acronisTaskPages(context.Background(), c, "/tasks", map[string]string{"limit": "2"}, true, "")
			if mode == "short-next" {
				if err != nil || calls != 2 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			} else if err == nil {
				t.Fatal("partial result reported success")
			}
		})
	}
}

func TestAcronisPartnerSync(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprint(broken), func(t *testing.T) {
			tmp := t.TempDir()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/2/clients/fixture-client":
					fmt.Fprint(w, `{"tenant_id":"root"}`)
				case "/api/2/clients":
					fmt.Fprint(w, `{"items":[{"id":"fixture-client"}]}`)
				case "/api/2/tenants":
					if r.URL.Query().Get("subtree_root_id") != "root" {
						t.Error("root scope missing")
					}
					fmt.Fprint(w, `{"items":[{"id":"root","name":"Partner","kind":"partner"},{"id":"child","name":"Customer","kind":"customer","parent_id":"root"}],"paging":{"cursors":{}}}`)
				case "/api/agent_manager/v2/agents":
					fmt.Fprint(w, `{"items":[{"id":"agent","hostname":"server","online":true,"tenant":{"uuid":"child","id":"42"}}],"paging":{"cursors":{}}}`)
				case "/api/task_manager/v2/tasks", "/api/task_manager/v2/activities":
					if broken {
						http.Error(w, `{"message":"parameter malformed"}`, 400)
						return
					}
					if r.URL.Query().Get("after") == "" {
						fmt.Fprint(w, `{"items":[{"id":9007199254740993,"state":"completed","result":{"code":"error"},"tenant":{"uuid":"child"},"completedAt":"2026-09-11T12:00:00Z"}],"paging":{"cursors":{"after":"page2"}}}`)
					} else {
						fmt.Fprint(w, `{"items":[{"id":9007199254740994,"state":"completed","result":{"code":"ok"},"tenant":{"uuid":"child"}}],"paging":{"cursors":{}}}`)
					}
				default:
					switch {
					case strings.HasSuffix(r.URL.Path, "/users"):
						fmt.Fprint(w, `{"items":[{"id":"user"}],"paging":{"cursors":{}}}`)
					case strings.HasSuffix(r.URL.Path, "/usages"):
						fmt.Fprint(w, `{"items":[{"application_id":"backup","name":"storage","edition":"standard","value":1},{"application_id":"backup","name":"storage","edition":"advanced","value":2}]}`)
					case strings.HasSuffix(r.URL.Path, "/offering_items"):
						if r.URL.Query().Get("edition") != "*" {
							t.Error("not fetching all editions")
						}
						fmt.Fprint(w, `{"items":[{"application_id":"backup","name":"storage","edition":"standard"}],"paging":{"cursors":{}}}`)
					default:
						t.Error("unexpected request", r.URL)
						http.NotFound(w, r)
					}
				}
			}))
			defer srv.Close()
			t.Setenv("ACRONIS_BASE_URL", srv.URL)
			t.Setenv("ACRONIS_CLIENT_ID", "fixture-client")
			t.Setenv("ACRONIS_BEARER_AUTH", "fixture")
			flags := &rootFlags{configPath: filepath.Join(tmp, "config.toml"), dataSource: "live"}
			cmd := newSyncCmd(flags)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			path := filepath.Join(tmp, "mirror.db")
			old, err := store.OpenWithContext(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := old.UpsertTenants([]byte(`{"id":"deleted","name":"Removed"}`)); err != nil {
				t.Fatal(err)
			}
			if err := old.UpsertAgentManager([]byte(`{"id":"removed-agent","tenant_id":"deleted"}`)); err != nil {
				t.Fatal(err)
			}
			old.Close()
			cmd.SetArgs([]string{"--full", "--db", path, "--concurrency", "1"})
			err = cmd.Execute()
			if broken {
				if err == nil {
					t.Fatal("failed sync exited successfully")
				}
			} else if err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			db, err := store.OpenWithContext(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			err = acronisRequireCompleteMirror(db)
			if broken {
				if err == nil {
					t.Fatal("rollup accepted partial data")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for table, want := range map[string]int{"tenants": 2, "agent_manager": 1, "task_manager": 2, "users": 2, "usages": 4, "offering_items": 2} {
				var count int
				if err := db.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
					t.Errorf("%s count=%d want=%d err=%v", table, count, want, err)
				}
			}
			var tenant, result string
			if err := db.DB().QueryRow(`SELECT tenant_id,result_code FROM task_manager WHERE id='9007199254740993'`).Scan(&tenant, &result); err != nil || tenant != "child" || result != "error" {
				t.Errorf("mapped task tenant=%s result=%s err=%v", tenant, result, err)
			}
		})
	}
}

func TestAcronisSearchParameters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("text") != "Customer" || r.URL.Query().Get("tenant") != "root" || r.URL.Query().Has("query") {
			t.Error("search params", r.URL)
		}
		fmt.Fprint(w, `{"items":[]}`)
	}))
	defer srv.Close()
	t.Setenv("ACRONIS_BASE_URL", srv.URL)
	t.Setenv("ACRONIS_BEARER_AUTH", "fixture")
	cmd := newRemoteSearchPromotedCmd(&rootFlags{configPath: filepath.Join(t.TempDir(), "config.toml"), dataSource: "live"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--query", "Customer", "--tenant-id", "root"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestAcronisValidationErrorIsNotAuth(t *testing.T) {
	err := classifyAPIError(fmt.Errorf(`HTTP 400: {"details":{"info":"Key: tenant is required"}}`), &rootFlags{})
	if strings.Contains(err.Error(), "auth") || strings.Contains(err.Error(), "token") {
		t.Fatal(err)
	}
}

type acronisFixtureTransport func(*http.Request) (*http.Response, error)

func (f acronisFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAcronisLoginDatacenterPersistence(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	http.DefaultTransport = acronisFixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Host, ".acronis.com") || r.URL.Path != "/api/2/idp/token" {
			t.Errorf("unexpected login request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"fixture-token","expires_in":7200}`))}, nil
	})
	t.Setenv("ACRONIS_BASE_URL", "")
	for _, initial := range []string{"https://{datacenter}.acronis.com", "https://eu2-cloud.acronis.com", "https://custom.example.test"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(fmt.Sprintf("base_url = %q\n", initial)), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newAuthLoginCmd(&rootFlags{configPath: path})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"--client-id", "fixture", "--client-secret", "fixture-secret", "--datacenter", "eu8-cloud"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		want := "https://eu8-cloud.acronis.com"
		if initial == "https://custom.example.test" {
			want = initial
		}
		if cfg.BaseURL != want {
			t.Errorf("base=%s want=%s", cfg.BaseURL, want)
		}
	}
}

func TestAcronisMirrorLockContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirror.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	unlock, err := acronisLockMirror(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acronisLockMirror(path); err == nil {
		second()
		t.Fatal("overlapping sync acquired mirror")
	}
	unlock()
	third, err := acronisLockMirror(path)
	if err != nil {
		t.Fatal(err)
	}
	third()
}

func TestAcronisDatacenterRejectsURLSyntax(t *testing.T) {
	for _, value := range []string{"attacker.example/", "attacker.example?", "user@host", "host#fragment", "../host", "", "-cloud"} {
		if acronisValidDatacenter(value) {
			t.Errorf("accepted unsafe datacenter %q", value)
		}
	}
	for _, value := range []string{"eu8-cloud", "us-cloud", "dev-cloud"} {
		if !acronisValidDatacenter(value) {
			t.Errorf("rejected datacenter %q", value)
		}
	}
}

func TestAcronisLocalTaskTenantCannotLeakOtherTenants(t *testing.T) {
	_, _, err := resolveAcronisTaskRead(context.Background(), nil, &rootFlags{dataSource: "local"}, "task-manager", "/api/task_manager/v2/tasks", map[string]string{"tenant_id": "target"}, false, io.Discard)
	if err == nil {
		t.Fatal("local tenant request must not fall through to unfiltered cache")
	}
}

type acronisRepeatingPage struct{}

func (acronisRepeatingPage) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"items":[{"id":1}],"paging":{"cursors":{"after":"same"}}}`), nil
}
func (acronisRepeatingPage) RateLimit() float64 { return 0 }

func TestAcronisSyncPageCapDoesNotHideRepeatedCursor(t *testing.T) {
	db := newSyncHintTestStore(t)
	params, err := parseSyncUserParams(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := syncResource(context.Background(), acronisRepeatingPage{}, db, "task-manager", "", true, 2, false, params, io.Discard)
	if result.Err == nil {
		t.Fatal("cap certified a repeated cursor")
	}
}
