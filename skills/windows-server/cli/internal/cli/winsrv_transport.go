// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the virtual REST surface. Recorded in
// handfixes.json (winrm-transport).
//
// The generated commands and MCP tools call https://winrm.invalid/<path>.
// That host does not exist: this RoundTripper answers every request in
// process by running the fixed script for the path on the configured server
// over WinRM. Nothing generated ever dials the network. Under the Printing
// Press verifier, requests to its loopback mock are passed to the mock with
// the Authorization header removed; every other host is refused.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"windows-server-pp-cli/internal/client"
	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/credkit"
	"windows-server-pp-cli/internal/winrm"
	"windows-server-pp-cli/internal/winsrv"
)

// VirtualHost is the base URL host the spec names.
const VirtualHost = "winrm.invalid"

type virtualTransport struct {
	cfg  *config.Config
	mock http.RoundTripper // set only under the verifier, for its loopback mock
}

func installVirtualTransport(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	c.NoCache = true // server data is never written to the response cache
	t := &virtualTransport{cfg: c.Config}
	if cliutil.IsVerifyEnv() && loopbackBase(c.BaseURL) {
		t.mock = &http.Transport{Proxy: nil}
	}
	// Hand-fix (handfixes.json credkit-wiring): the request to the virtual
	// host carries the shared credential, so it passes the kit's outbound
	// check (base_url only) before the in-process answer runs.
	c.HTTPClient.Transport = credkit.Transport(t)
	// Each script run carries its own budget (up to 5 minutes for the
	// Windows Update search); the generated whole-call timeout would cut it.
	c.HTTPClient.Timeout = 0
	c.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("redirects are refused")
	}
	return nil
}

func loopbackBase(base string) bool {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
}

func (t *virtualTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer req.Body.Close()
	}
	if t.mock != nil && loopbackBase(req.URL.String()) {
		out := req.Clone(req.Context())
		out.Header.Del("Authorization")
		return t.mock.RoundTrip(out)
	}
	if req.URL.Scheme != "https" || req.URL.Host != VirtualHost {
		return nil, fmt.Errorf("refused: this CLI only talks to servers named in its config file, never to %s", req.URL.Redacted())
	}
	if req.Method != http.MethodGet {
		return respond(req, http.StatusMethodNotAllowed, map[string]string{"error": "the virtual API is read-only; use the services restart or start commands"}), nil
	}
	profiles, credErrs, err := serversFromConfig(t.cfg, winsrvServer, false)
	if err != nil {
		return respond(req, http.StatusBadRequest, map[string]string{"error": err.Error()}), nil
	}
	p := profiles[0]
	if err := credErrs[p.Name]; err != nil {
		return respond(req, http.StatusUnauthorized, map[string]string{"error": err.Error()}), nil
	}
	runner := newWinsrvRunner(winsrvFlags)
	body, err := winsrv.Read(req.Context(), runner, p, req.URL.Path, req.URL.Query(), winsrvNow())
	if err != nil {
		var he *winsrv.HTTPError
		switch {
		case errors.As(err, &he):
			return respond(req, he.Status, map[string]string{"error": he.Message}), nil
		case isWinRMAuth(err):
			return respond(req, http.StatusUnauthorized, map[string]string{"error": err.Error()}), nil
		}
		return respond(req, http.StatusFailedDependency, map[string]string{"error": err.Error()}), nil
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}, "Date": {time.Now().UTC().Format(http.TimeFormat)}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func respond(req *http.Request, status int, v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(b)),
		ContentLength: int64(len(b)),
		Request:       req,
	}
}

func isWinRMAuth(err error) bool {
	return errors.Is(err, winrm.ErrAuth)
}
