// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: Mock-Data smoke test. Hand-authored; preserved across regenerations.

// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"abnormal-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type smokeProbe struct {
	Endpoint  string `json:"endpoint"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type smokeView struct {
	BaseURL string       `json:"base_url"`
	Probes  []smokeProbe `json:"probes"`
	Passed  int          `json:"passed"`
	Failed  int          `json:"failed"`
}

func smokeEndpoints() []string {
	return []string{"/threats", "/cases", "/vendors", "/users", "/aggregations/attack_stopped"}
}

func newNovelSmokeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smoke",
		Short: "Verify token, base URL (US/EU), and IP allowlist using Abnormal's Mock-Data test payloads",
		Long: strings.Trim(`
Probes a set of read endpoints with Abnormal's 'Mock-Data: True' request
header, which returns vendor-supplied test payloads instead of tenant data.
A passing run proves the token, the configured base URL (US or EU), and the
IP allowlist are all working — without touching real threat data.

Run this first when onboarding a tenant or rotating a token. For a config-only
check that needs no token at all, use 'doctor --dry-run'.`, "\n"),
		Example: strings.Trim(`
  abnormal-cli smoke
  abnormal-cli smoke --json
  abnormal-cli smoke --agent --select probes.endpoint,probes.ok`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("smoke probes the live API; no local data source"))
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would probe %d endpoints with the Mock-Data: True header\n", len(smokeEndpoints()))
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			endpoints := smokeEndpoints()
			if cliutil.IsDogfoodEnv() && len(endpoints) > 1 {
				endpoints = endpoints[:1]
			}
			view := smokeView{BaseURL: c.RequestBaseURL(), Probes: make([]smokeProbe, 0, len(endpoints))}
			headers := map[string]string{"Mock-Data": "True"}
			for _, ep := range endpoints {
				start := time.Now()
				_, err := c.GetWithHeadersNoCache(cmd.Context(), ep, nil, headers)
				probe := smokeProbe{Endpoint: ep, OK: err == nil, ElapsedMS: time.Since(start).Milliseconds()}
				if err != nil {
					probe.Error = err.Error()
					view.Failed++
				} else {
					view.Passed++
				}
				view.Probes = append(view.Probes, probe)
			}
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			if view.Failed > 0 {
				if view.Passed == 0 {
					return authErr(fmt.Errorf("all %d smoke probes failed — check ABNORMAL_API_TOKEN, the base URL (US vs EU host), and the portal IP allowlist", view.Failed))
				}
				return apiErr(fmt.Errorf("%d of %d smoke probes failed", view.Failed, len(view.Probes)))
			}
			return nil
		},
	}
	return cmd
}
