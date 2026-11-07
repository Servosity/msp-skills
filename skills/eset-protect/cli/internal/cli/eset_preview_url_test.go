// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// previewRT answers ESET Connect in process for any host and records every
// request URL, so a run against a real ESET host name never leaves the test.
type previewRT struct {
	mu   sync.Mutex
	sent []string
}

func (p *previewRT) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.sent = append(p.sent, req.Method+" "+req.URL.Scheme+"://"+req.URL.Host+req.URL.Path)
	p.mu.Unlock()
	body := `{}`
	switch {
	case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/v2/policies/"):
		body = `{"policy":{"uuid":"0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f","displayName":"Workstations","features":[{"name":"FEATURE_X","configuration":{}}]}}`
	case req.Method == http.MethodPost && req.URL.Path == "/v2/policies":
		body = `{"policy":{"uuid":"11111111-2222-4333-8444-555555555555","displayName":"copy"}}`
	case req.Method == http.MethodPost && req.URL.Path == "/v1/device_tasks":
		body = `{"task":{"uuid":"9c1d2e3f-0000-4000-8000-000000000001","displayName":"x"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}, nil
}

func (p *previewRT) posts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, s := range p.sent {
		if strings.HasPrefix(s, "POST ") {
			out = append(out, strings.TrimPrefix(s, "POST "))
		}
	}
	return out
}

// TestPreviewURLIsTheSentURL (adversarial round 4, P2): the respond and
// policies clone previews printed request.url with "{region}" unresolved,
// whatever ESET_PROTECT_REGION said, and with the ESET host even when
// ESET_PROTECT_BASE_URL sends --yes elsewhere, so the data center a human
// approves was never shown. The preview now prints the client's own
// resolution, and the URL --yes then posts to is exactly that one.
func TestPreviewURLIsTheSentURL(t *testing.T) {
	override := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(override.Close)
	for _, tc := range []struct {
		name, region, base, wantHost string
	}{
		{"region us", "us", "", "https://us.%s.eset.systems"},
		{"region jpn", "jpn", "", "https://jpn.%s.eset.systems"},
		{"default eu", "", "", "https://eu.%s.eset.systems"},
		{"base URL override", "us", override.URL, override.URL},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			respondClearHarness(t)
			db := respondSeedMirror(t)
			t.Setenv("ESET_PROTECT_REGION", tc.region)
			t.Setenv("ESET_PROTECT_BASE_URL", tc.base)
			t.Setenv("ESET_PROTECT_TOKEN", "test-token")
			t.Setenv("ESET_PROTECT_USERNAME", "")
			t.Setenv("ESET_PROTECT_PASSWORD", "")
			rt := &previewRT{}
			saved := clientHooks
			t.Cleanup(func() { clientHooks = saved })
			clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
				if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
					tr.Base = rt
				}
				return nil
			})
			host := func(service string) string {
				if strings.Contains(tc.wantHost, "%s") {
					return strings.Replace(tc.wantHost, "%s", service, 1)
				}
				return tc.wantHost
			}

			for _, run := range []struct {
				args    []string
				service string
				path    string
			}{
				{[]string{"respond", "isolate", "FRONTDESK-01", "--db", db}, "automation", "/v1/device_tasks"},
				{[]string{"policies", "clone", "0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f", "--name", "copy"}, "policy-management", "/v2/policies"},
			} {
				want := host(run.service) + run.path
				got, text, err := respondRunRoot(t, append(append([]string(nil), run.args...), "--json")...)
				if err != nil {
					t.Fatalf("%v preview: %v\n%s", run.args, err, text)
				}
				req, _ := got["request"].(map[string]any)
				if req["url"] != want {
					t.Fatalf("%v preview request.url = %v, want %s", run.args, req["url"], want)
				}
				if before := len(rt.posts()); before != 0 {
					t.Fatalf("%v: a preview sent %d POST(s)", run.args, before)
				}
				if run.args[0] == "respond" {
					_, text, err = respondRunRoot(t, append(append([]string(nil), run.args...), "--yes", "--wait", "0", "--json")...)
				} else {
					_, text, err = respondRunRoot(t, append(append([]string(nil), run.args...), "--yes", "--json")...)
				}
				if err != nil {
					t.Fatalf("%v --yes: %v\n%s", run.args, err, text)
				}
				posts := rt.posts()
				if len(posts) != 1 || posts[0] != want {
					t.Fatalf("%v --yes posted %v; the preview showed %s", run.args, posts, want)
				}
				rt.mu.Lock()
				rt.sent = nil
				rt.mu.Unlock()
			}
		})
	}
}

// TestPreviewURLMasksCredentials: a base URL carrying userinfo is shown
// masked in the preview, as error text shows it.
func TestPreviewURLMasksCredentials(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)
	t.Setenv("ESET_PROTECT_BASE_URL", "https://proxyuser:proxy-secret-pw@proxy.example")
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	got, text, err := respondRunRoot(t, "respond", "isolate", "FRONTDESK-01", "--db", db, "--json")
	if err != nil {
		t.Fatalf("preview: %v\n%s", err, text)
	}
	b, _ := json.Marshal(got["request"])
	if strings.Contains(string(b), "proxy-secret-pw") || !strings.Contains(string(b), "proxy.example/v1/device_tasks") {
		t.Fatalf("preview request = %s", b)
	}
}
