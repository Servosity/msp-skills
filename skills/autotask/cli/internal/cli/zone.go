// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"autotask-pp-cli/internal/cliutil"
	"autotask-pp-cli/internal/config"
	"autotask-pp-cli/internal/credkit"
	"github.com/spf13/cobra"
)

// zoneBootstrapHost is Autotask's zone-discovery host. It is intentionally the
// un-numbered host: every tenant's API lives on a numbered zone
// (webservicesN.autotask.net), and this endpoint tells you which N. It is hit
// directly (not through the configured client) because the configured BaseURL
// is exactly the value we're trying to discover.
//
// autotask handfix zone-discovery-pinned: the request carries the API user's
// UserName (an identifier, not a held credential: credwire Declarations), and
// the client is pinned to this one origin (scheme, host, port; every
// off-origin redirect refused) exactly as the API client is pinned to the
// configured base URL, and the zone it returns is saved only when it is a real
// Autotask zone host (config.ValidateZoneURL): the saved base URL is where
// every later credential goes.
var zoneBootstrapHost = "https://webservices.autotask.net/atservicesrest/V1.0"

// zoneInfoResponse is the zoneInformation envelope:
//
//	{"url":"https://webservicesN.autotask.net/atservicesrest/","dataBaseType":...,"ciLevel":...}
type zoneInfoResponse struct {
	URL          string `json:"url"`
	DataBaseType string `json:"dataBaseType,omitempty"`
	WebURL       string `json:"webUrl,omitempty"`
	CILevel      int    `json:"ciLevel,omitempty"`
}

func newZoneCmd(flags *rootFlags) *cobra.Command {
	var user string
	var save bool
	var noSave bool
	cmd := &cobra.Command{
		Use:   "zone",
		Short: "Discover and cache your Autotask tenant's API zone (webservicesN base URL).",
		Long: `Autotask's REST API lives on a tenant-specific numbered zone
(https://webservicesN.autotask.net/atservicesrest/V1.0/). This command calls
Autotask's zoneInformation endpoint with your API UserName, prints the resolved
zone base URL, and (by default) caches it to the config file so every other
command targets the correct host.

The UserName comes from --user, AUTOTASK_USERNAME, or AUTOTASK_PSA_USER_NAME.
You can also skip discovery entirely by setting AUTOTASK_BASE_URL, or pass
--no-save to preview the resolved zone without writing it to config. Only a
webservicesN.autotask.net zone is cached.`,
		Example: strings.Trim(`
  autotask-cli zone --user api-user@example.com
  autotask-cli zone --user api-user@example.com --json
  autotask-cli zone --user api-user@example.com --no-save`, "\n"),
		// Not read-only: it caches the zone to the config file by default. Over
		// MCP it previews with --no-save until confirm is true (the
		// mcp-mirror-write-confirm gate reads autotask:preview-flag).
		Annotations: map[string]string{"autotask:preview-flag": "no-save"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if noSave {
				save = false
			}
			if user == "" {
				user = firstNonEmptyEnv("AUTOTASK_USERNAME", "AUTOTASK_PSA_USER_NAME")
			}
			if user == "" {
				return usageErr(fmt.Errorf("a UserName is required: pass --user <api-user-email>, or set AUTOTASK_USERNAME / AUTOTASK_PSA_USER_NAME"))
			}
			if dryRunOK(flags) {
				return nil
			}
			if cliutil.IsVerifyEnv() {
				// Avoid dialing the real Autotask bootstrap host under verify.
				fmt.Fprintln(cmd.OutOrStdout(), `{"status":"noop","reason":"verify_short_circuit","command":"zone"}`)
				return nil
			}

			zoneURL, err := discoverZone(cmd.Context(), user, flags.timeout)
			if err != nil {
				return apiErr(err)
			}
			if err := config.ValidateZoneURL(zoneURL); err != nil {
				return apiErr(err)
			}
			normalized := config.NormalizeZoneBaseURL(zoneURL)

			saved := false
			if save {
				cfg, cerr := config.Load(flags.configPath)
				if cerr != nil {
					return configErr(cerr)
				}
				if serr := cfg.SaveBaseURL(normalized); serr != nil {
					return configErr(fmt.Errorf("caching zone to config: %w", serr))
				}
				saved = true
			}

			out := map[string]any{
				"zone_url": zoneURL,
				"base_url": normalized,
				"cached":   saved,
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "Autotask API UserName (email) used for zone discovery; defaults to AUTOTASK_USERNAME / AUTOTASK_PSA_USER_NAME")
	cmd.Flags().BoolVar(&save, "save", true, "Cache the resolved zone base URL to the config file")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Preview the resolved zone without writing it to config")
	return cmd
}

// discoverZone calls Autotask's zoneInformation endpoint and returns the raw
// tenant zone URL. It uses a fresh stdlib client against the bootstrap host
// because the configured client's BaseURL is the unknown we are resolving.
func discoverZone(ctx context.Context, user string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	endpoint := zoneBootstrapHost + "/zoneInformation?user=" + url.QueryEscape(user)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "autotask-cli/v1")

	hc := &http.Client{Timeout: timeout, Transport: credkit.Transport(nil)} // Hand-fix (handfixes.json credkit-wiring)
	pin := ""
	if u, perr := url.Parse(zoneBootstrapHost); perr == nil {
		pin = normalizedOrigin(u)
	}
	installPin(hc, pin, false)
	resp, err := hc.Do(req)
	if err != nil {
		// The *url.Error repeats the lookup URL with the user in its query; the
		// cause alone is what the operator needs.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return "", fmt.Errorf("zone discovery request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("zoneInformation returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var zi zoneInfoResponse
	if err := json.Unmarshal(body, &zi); err != nil {
		return "", fmt.Errorf("decoding zoneInformation response: %w", err)
	}
	if strings.TrimSpace(zi.URL) == "" {
		return "", fmt.Errorf("zoneInformation returned no url for that UserName")
	}
	return zi.URL, nil
}

// describeZone returns a human/agent-readable verdict about the resolved zone
// derived from the configured base URL. Used by doctor.
func describeZone(baseURL string) string {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	if b == "" {
		return "not resolved (run `autotask-cli zone --user <api-user>` or set AUTOTASK_BASE_URL)"
	}
	host := b
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if j := strings.IndexByte(host, '/'); j >= 0 {
		host = host[:j]
	}
	if strings.HasPrefix(host, "webservices") && host != "webservices.autotask.net" {
		return "resolved: " + host
	}
	if strings.Contains(host, "127.0.0.1") || strings.Contains(host, "localhost") {
		return "test/override base (" + host + ")"
	}
	return "not zone-resolved (base host " + host + "); run `autotask-cli zone --user <api-user>` or set AUTOTASK_BASE_URL"
}

// firstNonEmptyEnv returns the value of the first set, non-empty env var.
func firstNonEmptyEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}
