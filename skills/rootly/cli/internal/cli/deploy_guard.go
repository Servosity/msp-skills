// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): pre-deploy gate for a service.
// Joins open + recent incidents to the service and current on-call from the local
// mirror, returning a non-zero exit (8) when it is unsafe to ship — a gate the
// web UI cannot provide to a CI pipeline.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelDeployGuardCmd(flags *rootFlags) *cobra.Command {
	var within string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "deploy-guard <service>",
		Short: "Pre-deploy gate: block when a service has an open incident or no on-call.",
		Long: `Decide whether it is safe to deploy a service. Checks the local mirror for
open incidents on the service, recent incidents within --within, and whether
anyone is currently on call for it. Exits 0 when safe (or when there is not
enough data to judge), and exits 8 when unsafe (an open incident on the service),
so it can gate a CI pipeline. Offline — wire it into a deploy script.`,
		Example: `  rootly-cli deploy-guard checkout-api --within 7d
  rootly-cli deploy-guard checkout-api --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:typed-exit-codes": "0,8"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			service := args[0]
			svcLower := strings.ToLower(strings.TrimSpace(service))

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}

			recentWindow := 7 * 24 * time.Hour
			if d, ok := parseWindowDuration(within); ok {
				recentWindow = d
			}
			recentCutoff := time.Now().Add(-recentWindow)

			type incRef struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Severity string `json:"severity,omitempty"`
				Status   string `json:"status,omitempty"`
			}
			var openInc, recentInc []incRef
			matchedService := false
			for _, r := range incidents {
				onService := false
				for _, n := range incidentServiceNames(r) {
					if strings.ToLower(n) == svcLower {
						onService = true
						break
					}
				}
				if !onService {
					continue
				}
				matchedService = true
				ref := incRef{ID: r.ID, Title: incidentTitle(r), Severity: incidentSeverity(r), Status: recStr(r.Attrs, "status")}
				if incidentOpen(r) {
					openInc = append(openInc, ref)
				}
				if start, ok := incidentStart(r); ok && start.After(recentCutoff) {
					recentInc = append(recentInc, ref)
				}
			}

			oncall := oncallForServices(db, []string{service})

			// Decide.
			var reasons []string
			safe := true
			dataKnown := len(incidents) > 0
			if len(openInc) > 0 {
				safe = false
				reasons = append(reasons, fmt.Sprintf("%d open incident(s) on %s", len(openInc), service))
			}
			if dataKnown && len(oncall) == 0 {
				// A warning, not a hard block on its own unless there is also no service match.
				reasons = append(reasons, "no current on-call found for this service")
			}
			if len(recentInc) > 0 {
				reasons = append(reasons, fmt.Sprintf("%d incident(s) on %s in the last %s", len(recentInc), service, humanDuration(recentWindow)))
			}
			if !dataKnown {
				reasons = append(reasons, "no incidents synced — cannot evaluate; run 'rootly-cli sync'")
			} else if !matchedService {
				reasons = append(reasons, fmt.Sprintf("no incidents reference service %q (treating as low-risk)", service))
			}
			if len(reasons) == 0 {
				reasons = append(reasons, "no open incidents, on-call present")
			}

			out := struct {
				Service       string   `json:"service"`
				Safe          bool     `json:"safe"`
				Reasons       []string `json:"reasons"`
				OpenIncidents []incRef `json:"open_incidents"`
				RecentCount   int      `json:"recent_incidents"`
				OnCall        []string `json:"current_oncall"`
			}{
				Service:       service,
				Safe:          safe,
				Reasons:       reasons,
				OpenIncidents: nonNilRefs(openInc),
				RecentCount:   len(recentInc),
				OnCall:        nonNilStrings(oncall),
			}

			renderErr := novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				verdict := "SAFE TO DEPLOY ✓"
				if !safe {
					verdict = "DO NOT DEPLOY ✗"
				}
				fmt.Fprintf(w, "deploy-guard %s: %s\n", service, verdict)
				for _, r := range reasons {
					fmt.Fprintf(w, "  - %s\n", r)
				}
				if len(out.OnCall) > 0 {
					fmt.Fprintf(w, "  on-call: %s\n", strings.Join(out.OnCall, ", "))
				}
			})
			if renderErr != nil {
				return renderErr
			}
			if !safe {
				// Typed exit 8 so a CI step fails. Message already rendered above.
				return &cliError{code: 8, err: fmt.Errorf("deploy-guard: unsafe to deploy %s (%s)", service, strings.Join(reasons, "; "))}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "7d", "Window for 'recent incident' checks, e.g. 7d, 48h")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
