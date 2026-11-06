// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): handfixes.json no-credential-to-placeholder-host.

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectwise-control-pp-cli/internal/client"
	"connectwise-control-pp-cli/internal/cliutil"
	"connectwise-control-pp-cli/internal/cliutil/testenv"
)

// Fixture credentials: random, and sharing no 8-character piece with the
// connector's program text.
const (
	placeholderCLIUser = "M8ulUDUpJUtPpcJnwl"
	placeholderCLIPass = "7QxNq7PWgKblc6ln3v"
)

// wireListener is the only place a client built in these tests can reach: a
// loopback listener that records the first line of every connection (a
// proxy CONNECT names the host the client meant) and answers 403.
type wireListener struct {
	ln    net.Listener
	mu    sync.Mutex
	lines []string
}

func newWireListener(t *testing.T) *wireListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	w := &wireListener{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				line, _ := bufio.NewReader(conn).ReadString('\n')
				w.mu.Lock()
				w.lines = append(w.lines, strings.TrimSpace(line))
				w.mu.Unlock()
				_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return w
}

func (w *wireListener) seen() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

// routeClientsTo adds a client hook, after the connector's own, that sends
// every dial and every proxied request of each client built during the test
// to w. Nothing a test client sends can leave the loopback interface, and no
// host name is ever resolved.
func routeClientsTo(t *testing.T, w *wireListener) {
	t.Helper()
	prev := clientHooks
	addr := w.ln.Addr().String()
	clientHooks = append(append([]func(*client.Client) error{}, prev...), func(c *client.Client) error {
		if c.HTTPClient == nil {
			return nil
		}
		pin, ok := c.HTTPClient.Transport.(originPinTripper)
		if !ok {
			return fmt.Errorf("test hook: the outermost transport is %T, not the origin pin", c.HTTPClient.Transport)
		}
		tr, ok := pin.next.(*http.Transport)
		if !ok {
			if _, dry := pin.next.(dryRunTripper); dry {
				return nil
			}
			return fmt.Errorf("test hook: the pin sends through %T", pin.next)
		}
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: addr})
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}
		tr.DialTLSContext = nil
		return nil
	})
	t.Cleanup(func() { clientHooks = prev })
}

// placeholderSource puts the shipped placeholder base URL in place one way.
type placeholderSource struct {
	name  string
	apply func(t *testing.T, home string) (configPath string)
}

func writeSandboxConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

var placeholderSources = []placeholderSource{
	{"built-in default", func(t *testing.T, home string) string { return "" }},
	{"env CONNECTWISE_CONTROL_BASE_URL", func(t *testing.T, home string) string {
		t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "https://your-instance.screenconnect.com")
		return ""
	}},
	{"env, another spelling", func(t *testing.T, home string) string {
		t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "HTTPS://Your-Instance.ScreenConnect.com.:443/")
		return ""
	}},
	{"default config file", func(t *testing.T, home string) string {
		dir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		writeSandboxConfig(t, filepath.Join(dir, "config.toml"), "base_url = 'https://your-instance.screenconnect.com'\n")
		return ""
	}},
	{"explicit --config file", func(t *testing.T, home string) string {
		p := filepath.Join(home, "explicit", "cwc.toml")
		writeSandboxConfig(t, p, "base_url = 'https://ops@your-instance.screenconnect.com:443'\n")
		return p
	}},
}

func requireNamesTheFix(t *testing.T, what, text string) {
	t.Helper()
	for _, want := range []string{"CONNECTWISE_CONTROL_BASE_URL", "base_url"} {
		if !strings.Contains(text, want) {
			t.Errorf("%s: the refusal does not name %s:\n%s", what, want, text)
		}
	}
	if strings.Contains(text, placeholderCLIPass) {
		t.Errorf("%s: the refusal carries the password:\n%s", what, text)
	}
}

// Whatever supplies the placeholder, a client the CLI or a novel builds
// refuses the request before it is sent: the listener every dial is routed to
// never sees a connection.
func TestPlaceholderBaseURLRefusedFromEverySource(t *testing.T) {
	for _, src := range placeholderSources {
		t.Run(src.name, func(t *testing.T) {
			home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir)
			t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "")
			t.Setenv("CONNECTWISE_CONTROL_USERNAME", placeholderCLIUser)
			t.Setenv("CONNECTWISE_CONTROL_PASSWORD", placeholderCLIPass)
			cfgPath := src.apply(t, home)
			w := newWireListener(t)
			routeClientsTo(t, w)
			flags := rootFlags{timeout: 5 * time.Second, configPath: cfgPath}
			c, err := flags.newClient()
			if err != nil {
				t.Fatalf("newClient: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			_, err = c.Get(ctx, "/Services/SessionGroupService.ashx/session-groups", nil)
			if err == nil {
				t.Fatal("GET: no error")
			}
			requireNamesTheFix(t, "GET", err.Error())
			_, _, err = c.PostQueryWithParams(ctx, "/Services/PageService.ashx/GetLiveData", map[string]string{}, cwcLiveDataBody(2, ""))
			if err == nil {
				t.Fatal("novel POST: no error")
			}
			requireNamesTheFix(t, "novel POST", err.Error())
			if got := w.seen(); len(got) != 0 {
				t.Fatalf("the listener saw %d connection(s): %q", len(got), got)
			}
		})
	}
}

// The commands themselves (a generated endpoint, both novels) exit with the
// refusal and send nothing, with no base URL configured.
func TestPlaceholderCommandsSendNothing(t *testing.T) {
	for _, args := range [][]string{
		{"session-groups"},
		{"audit", "get-info"},
		{"sessions", "stale"},
		{"security", "access-review"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir)
			t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "")
			t.Setenv("CONNECTWISE_CONTROL_USERNAME", placeholderCLIUser)
			t.Setenv("CONNECTWISE_CONTROL_PASSWORD", placeholderCLIPass)
			w := newWireListener(t)
			routeClientsTo(t, w)
			cmd := RootCmd()
			cmd.SetArgs(append(args, "--json", "--timeout", "4s"))
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			done := make(chan error, 1)
			go func() { done <- cmd.Execute() }()
			var err error
			select {
			case err = <-done:
			case <-time.After(60 * time.Second):
				t.Fatal("the command did not return within 60s")
			}
			if err == nil {
				t.Fatalf("no error; stdout:\n%s", out.String())
			}
			requireNamesTheFix(t, "command", err.Error()+"\n"+out.String()+"\n"+errOut.String())
			if got := w.seen(); len(got) != 0 {
				t.Fatalf("the listener saw %d connection(s): %q", len(got), got)
			}
		})
	}
}

// doctor keeps telling the truth: any spelling of the shipped placeholder is
// "not configured", names CONNECTWISE_CONTROL_BASE_URL, and is never dialled.
func TestDoctorReportsEverySpellingOfThePlaceholder(t *testing.T) {
	for _, base := range []string{
		"https://your-instance.screenconnect.com",
		"HTTPS://Your-Instance.ScreenConnect.com.:443/",
		"https://ops@your-instance.screenconnect.com",
	} {
		t.Run(base, func(t *testing.T) {
			testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir)
			t.Setenv("CONNECTWISE_CONTROL_BASE_URL", base)
			t.Setenv("CONNECTWISE_CONTROL_USERNAME", placeholderCLIUser)
			t.Setenv("CONNECTWISE_CONTROL_PASSWORD", placeholderCLIPass)
			w := newWireListener(t)
			routeClientsTo(t, w)
			cmd := RootCmd()
			cmd.SetArgs([]string{"doctor", "--json", "--timeout", "4s"})
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			_ = cmd.Execute()
			var report map[string]any
			text := out.String()
			if i := strings.Index(text, "{"); i >= 0 {
				_ = json.Unmarshal([]byte(text[i:]), &report)
			}
			api, _ := report["api"].(string)
			if !strings.Contains(api, "not configured (base_url is a placeholder)") || !strings.Contains(api, "CONNECTWISE_CONTROL_BASE_URL") {
				t.Errorf("doctor api = %q, want the placeholder verdict naming CONNECTWISE_CONTROL_BASE_URL\n%s", api, text)
			}
			if got := w.seen(); len(got) != 0 {
				t.Fatalf("doctor dialled: %q", got)
			}
		})
	}
}
