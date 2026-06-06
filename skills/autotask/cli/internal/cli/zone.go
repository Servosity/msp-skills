// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"autotask-pp-cli/internal/cliutil"
	"autotask-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

// zoneBootstrapHost is Autotask's zone-discovery host. It is intentionally the
// un-numbered host: every tenant's API lives on a numbered zone
// (webservicesN.autotask.net), and this endpoint tells you which N. It is hit
// directly (not through the configured client) because the configured BaseURL
// is exactly the value we're trying to discover.
const zoneBootstrapHost = "https://webservices.autotask.net/atservicesrest/V1.0"

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
You can also skip discovery entirely by setting AUTOTASK_ZONE_URL, or pass
--no-save to preview the resolved zone without writing it to config.`,
		Example: strings.Trim(`
  autotask-cli zone --user api-user@example.com
  autotask-cli zone --user api-user@example.com --json
  autotask-cli zone --user api-user@example.com --no-save`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
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
				"user":     user,
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

	hc := &http.Client{Timeout: timeout}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("zone discovery request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("zoneInformation returned HTTP %d for user %q: %s", resp.StatusCode, user, truncate(string(body), 300))
	}
	var zi zoneInfoResponse
	if err := json.Unmarshal(body, &zi); err != nil {
		return "", fmt.Errorf("decoding zoneInformation response: %w", err)
	}
	if strings.TrimSpace(zi.URL) == "" {
		return "", fmt.Errorf("zoneInformation returned no url for user %q", user)
	}
	return zi.URL, nil
}

// describeZone returns a human/agent-readable verdict about the resolved zone
// derived from the configured base URL. Used by doctor.
func describeZone(baseURL string) string {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	if b == "" {
		return "not resolved (run `autotask-cli zone --user <api-user>` or set AUTOTASK_ZONE_URL)"
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
	return "not zone-resolved (base host " + host + "); run `autotask-cli zone --user <api-user>` or set AUTOTASK_ZONE_URL"
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
