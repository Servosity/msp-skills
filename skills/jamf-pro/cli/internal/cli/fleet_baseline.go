// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet baseline compares every client instance with a golden one.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetBaselineCmd(flags *rootFlags) *cobra.Command {
	var golden string
	var hideExtra bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Compare every client instance against a golden instance: scripts and extension attributes by content; packages, policies, profiles and smart groups by name",
		Long: `Compare every client instance against a golden instance: scripts and extension attributes by content; packages, policies, profiles and smart groups by name.

Use this command to compare client instances against a golden instance: scripts (by content), packages, policies, configuration profiles, extension attributes, and smart groups that are missing, different, or extra. Do NOT use this command for defects inside a single instance's policies; use 'fleet policy-hygiene' instead.

Objects are matched by name. Scripts and computer extension attributes also
compare a hash of their script contents, so an outdated copy shows as
"differs". Nothing is written: to bring a client in line, run the per-instance
command yourself (for example 'jamf-pro-cli --instance acme scripts update ...').
Sources: /api/v1/scripts, /api/v1/packages, /api/v1/computer-extension-attributes,
/api/v3/computer-groups/smart-groups, Classic /JSSResource/policies/createdBy/jss
(UI- and API-created policies, without one-off Jamf Remote policies) and
/JSSResource/osxconfigurationprofiles. Script contents are compared by digest
and never printed.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet baseline --golden msp-template --agent
  jamf-pro-cli fleet baseline --golden msp-template --only acme --hide-extra --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--golden=msp-template",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet baseline")
			}
			if strings.TrimSpace(golden) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--golden is required: the registered instance holding your standard configuration"))
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(scope.reg.Instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			if len(scope.reg.Instances) == 0 {
				return printJSONFiltered(cmd.OutOrStdout(), []fleet.DriftRow{}, flags)
			}
			g, ok := scope.reg.Get(golden)
			if !ok {
				return usageErr(fmt.Errorf("unknown golden instance %q (registered: %s)", golden, strings.Join(scope.reg.Names(), ", ")))
			}
			if pinnedInstance != "" && pinnedInstance != golden {
				return usageErr(fmt.Errorf("--instance %s scrubbed every other instance's secret, so the golden instance %q cannot be read; drop --instance and use --only", pinnedInstance, golden))
			}
			targets := []fleet.Instance{g}
			for _, p := range instances {
				if p.Name != golden {
					targets = append(targets, p)
				}
			}
			if len(targets) == 1 {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: only the golden instance is in scope; nothing to compare")
			}
			reqs := []fleet.Request{
				jamfGet("/api/v1/scripts", "page-size", "100"),
				jamfGet("/api/v1/packages", "page-size", "100"),
				jamfGet("/api/v1/computer-extension-attributes", "page-size", "100"),
				jamfGet("/api/v3/computer-groups/smart-groups", "page-size", "100"),
				// createdBy/jss: policies made in the UI or API, not one-off
				// Jamf Remote (Casper Remote) policies.
				jamfGet("/JSSResource/policies/createdBy/jss"),
				jamfGet("/JSSResource/osxconfigurationprofiles"),
			}
			type snap struct {
				name string
				snap fleet.Snapshot
				err  string
			}
			snaps, failed := collect(cmd, &scope, flags, targets, reqs,
				func(p fleet.Instance, bodies []json.RawMessage) ([]snap, error) {
					s, err := baselineSnapshot(bodies)
					if err != nil {
						return nil, err
					}
					return []snap{{name: p.Name, snap: s}}, nil
				},
				func(p fleet.Instance, err error) snap { return snap{name: p.Name, err: err.Error()} })
			if snaps[0].err != "" {
				// Every unread instance keeps its row, the golden one first.
				var rows []fleet.DriftRow
				for _, s := range snaps {
					if s.err != "" {
						rows = append(rows, fleet.DriftRow{Instance: s.name, Error: s.err})
					}
				}
				if err := render(cmd, flags, rows, []string{"instance", "error"}, "No rows.", driftColumns, driftItem); err != nil {
					return err
				}
				return apiErr(fmt.Errorf("golden instance %s could not be read: %s", golden, snaps[0].err))
			}
			var rows []fleet.DriftRow
			targetFailed := 0
			for _, s := range snaps[1:] {
				if s.err != "" {
					targetFailed++
					rows = append(rows, fleet.DriftRow{Instance: s.name, Error: s.err})
					continue
				}
				rows = append(rows, fleet.Diff(s.name, snaps[0].snap, s.snap, !hideExtra)...)
			}
			if err := render(cmd, flags, rows, []string{"instance", "kind", "status", "error"}, "No differences from the golden instance: scripts and extension attributes compared by content, the rest by name.",
				driftColumns, driftItem); err != nil {
				return err
			}
			_ = failed // the golden instance's own read is checked above
			return allFailed(targetFailed, len(targets)-1)
		},
	}
	cmd.Flags().StringVar(&golden, "golden", "", "Registered instance holding the standard configuration (required)")
	cmd.Flags().BoolVar(&hideExtra, "hide-extra", false, "Do not list objects a client has that the golden instance lacks")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// baselineSnapshot turns the six list bodies into name -> fingerprint maps.
func baselineSnapshot(bodies []json.RawMessage) (fleet.Snapshot, error) {
	s := fleet.Snapshot{}
	scripts, err := fleet.Results[fleet.Script](bodies[0], "name")
	if err != nil {
		return nil, fmt.Errorf("scripts: %w", err)
	}
	s[fleet.KindScript] = map[string]string{}
	for _, x := range scripts {
		s[fleet.KindScript][x.Name] = fleet.Fingerprint(x.ScriptContents)
	}
	packages, err := fleet.Results[fleet.Package](bodies[1], "packageName")
	if err != nil {
		return nil, fmt.Errorf("packages: %w", err)
	}
	s[fleet.KindPackage] = map[string]string{}
	for _, x := range packages {
		s[fleet.KindPackage][x.PackageName] = ""
	}
	eas, err := fleet.Results[fleet.ExtensionAttribute](bodies[2], "name")
	if err != nil {
		return nil, fmt.Errorf("computer-extension-attributes: %w", err)
	}
	s[fleet.KindEA] = map[string]string{}
	for _, x := range eas {
		s[fleet.KindEA][x.Name] = fleet.Fingerprint(x.ScriptContents)
	}
	groups, err := fleet.Results[fleet.Named](bodies[3], "name")
	if err != nil {
		return nil, fmt.Errorf("smart-groups: %w", err)
	}
	s[fleet.KindSmartGrp] = map[string]string{}
	for _, x := range groups {
		s[fleet.KindSmartGrp][x.Name] = ""
	}
	for i, k := range []struct{ kind, root, singular string }{
		{fleet.KindPolicy, "policies", "policy"},
		{fleet.KindProfile, "os_x_configuration_profiles", "os_x_configuration_profile"},
	} {
		items, err := fleet.ClassicList(bodies[4+i], k.root, k.singular, "name")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k.root, err)
		}
		s[k.kind] = map[string]string{}
		for _, o := range items {
			if name, _ := o["name"].(string); name != "" {
				s[k.kind][name] = ""
			}
		}
	}
	return s, nil
}

var driftColumns = []string{"instance", "kind", "name", "status", "detail", "error"}

func driftItem(r fleet.DriftRow) map[string]any {
	return map[string]any{"instance": r.Instance, "kind": r.Kind, "name": r.Name, "status": r.Status, "detail": r.Detail, "error": r.Error}
}
