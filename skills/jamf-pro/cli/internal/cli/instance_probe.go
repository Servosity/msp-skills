// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: instance test proves every registered API client works.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

type instanceTestRow struct {
	Instance  string `json:"instance"`
	Server    string `json:"server"`
	SecretSet bool   `json:"secret_set"`
	OK        bool   `json:"ok"`
	Version   string `json:"version"`
	Error     string `json:"error,omitempty"`
}

func newInstanceTestCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Prove each registered instance's API client can get a token and read the Jamf Pro version",
		Long: `Prove each registered instance's API client can get a token and read the Jamf Pro version.

Use 'instance test' to prove one registration's credentials, reachability, and Jamf Pro version. Do NOT use it for token or certificate expiry across clients; use 'fleet health' instead.

Each instance is read in its own process with its own credential: a new token
is always requested from https://<server>/api/v1/oauth/token (a cached token is
never reused, so a changed or revoked secret fails here), then
GET /api/v1/jamf-pro-version is read with it. A row with ok=false carries the reason (secret env var unset,
401 from a wrong secret, unreachable host).`,
		Example: strings.Trim(`
  jamf-pro-cli instance test --agent
  jamf-pro-cli instance test --only acme --json`, "\n"),
		// pp:data-source is live: this reaches every server.
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "instance test")
			}
			scope.freshToken = true
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			rows, failed := collect(cmd, &scope, flags, instances, []fleet.Request{jamfGet("/api/v1/jamf-pro-version")},
				func(p fleet.Instance, bodies []json.RawMessage) ([]instanceTestRow, error) {
					var v struct {
						Version string `json:"version"`
					}
					if err := fleet.DecodeObject(bodies[0], &v, "version"); err != nil {
						return nil, fmt.Errorf("unexpected version response: %w", err)
					}
					return []instanceTestRow{{Instance: p.Name, Server: p.Server, SecretSet: fleet.HasSecret(p.Name), OK: true, Version: v.Version}}, nil
				},
				func(p fleet.Instance, err error) instanceTestRow {
					return instanceTestRow{Instance: p.Name, Server: p.Server, SecretSet: fleet.HasSecret(p.Name), Error: err.Error()}
				})
			if err := render(cmd, flags, rows, []string{"instance", "ok", "error"}, "No instances.", []string{"instance", "server", "ok", "version", "error"}, func(r instanceTestRow) map[string]any {
				return map[string]any{"instance": r.Instance, "server": r.Server, "ok": r.OK, "version": r.Version, "error": r.Error}
			}); err != nil {
				return err
			}
			if failed > 0 {
				return apiErr(fmt.Errorf("%d of %d instance(s) failed the test; see the error field on each row", failed, len(instances)))
			}
			return nil
		},
	}
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
