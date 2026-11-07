// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet app finds clients with an installed app below a version floor.
// pp:data-source live

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"encoding/json"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetAppCmd(flags *rootFlags) *cobra.Command {
	var below string
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "app <application>",
		Short: "Which clients and computers have an installed application below a version floor",
		Long: `Which clients and computers have an installed application below a version floor.

Use this command to find which clients and computers have an installed application below a version floor, whether or not it is a patch-management title. Do NOT use this command for configured patch-title compliance; use 'fleet patch' instead.

Reads the Classic API computer application inventory
(/JSSResource/computerapplications/application/<name>) on every instance,
compares each installed version with --below, and lists the computers on every
older version. Jamf inventories Mac apps by bundle name (e.g. TextWrangler.app);
".app" is appended when the name does not already end with it. Versions compare
numerically part by part (129.0.6668.100 > 129.0.6668.9).`,
		Example: strings.Trim(`
  jamf-pro-cli fleet app "Google Chrome" --below 129.0.6668.100 --agent
  jamf-pro-cli fleet app Firefox --below 131.0 --tag gold --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "application=Firefox;--below=131.0",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet app")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one application name is required, e.g. \"Google Chrome\""))
			}
			if strings.TrimSpace(below) == "" || fleet.CompareVersions(below, "0") <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--below is required: the lowest acceptable version, e.g. 129.0.6668.100"))
			}
			app := appBundleName(args[0])
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			path := "/JSSResource/computerapplications/application/" + url.PathEscape(app)
			rows, failed := collect(cmd, &scope, flags, instances, []fleet.Request{jamfGet(path)},
				func(p fleet.Instance, bodies []json.RawMessage) ([]fleet.AppRow, error) {
					versions, err := fleet.AppVersions(bodies[0])
					if err != nil {
						return nil, err
					}
					var out []fleet.AppRow
					for _, v := range fleet.BelowFloor(versions, below) {
						version := v.Number
						if version == "" {
							version = "unknown" // no version reported: cannot be judged current
						}
						out = append(out, fleet.AppRow{Instance: p.Name, App: app, Version: version, Computers: len(v.Computers), Names: strings.Join(v.Computers, ", ")})
					}
					return out, nil
				},
				func(p fleet.Instance, err error) fleet.AppRow {
					return fleet.AppRow{Instance: p.Name, App: app, Error: err.Error()}
				})
			// Oldest (most exposed) version first within each instance.
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Instance != rows[j].Instance {
					return rows[i].Instance < rows[j].Instance
				}
				return fleet.CompareVersions(rows[i].Version, rows[j].Version) < 0
			})
			if err := render(cmd, flags, rows, []string{"instance", "version", "error"}, fmt.Sprintf("No computer has %s below %s.", app, below),
				[]string{"instance", "app", "version", "computers", "computer_names", "error"}, func(r fleet.AppRow) map[string]any {
					return map[string]any{"instance": r.Instance, "app": r.App, "version": r.Version, "computers": r.Computers, "computer_names": r.Names, "error": r.Error}
				}); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().StringVar(&below, "below", "", "Lowest acceptable version; older installs, and installs reporting no version (as unknown), are listed (required)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

// appBundleName appends ".app" unless the name already ends with it.
func appBundleName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(strings.ToLower(name), ".app") {
		return name
	}
	return name + ".app"
}
