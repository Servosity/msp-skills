// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type exposureDevice struct {
	Name   string `json:"name"`
	IP     string `json:"ip,omitempty"`
	MAC    string `json:"mac,omitempty"`
	Source string `json:"source"`
}

type exposureSubnet struct {
	Subnet       string           `json:"subnet"`
	Unmanaged    int              `json:"unmanaged"`
	ManagedPeers int              `json:"managed_peers"`
	Devices      []exposureDevice `json:"devices,omitempty"`
}

// subnet24 returns the /24 of an IPv4 address, or "(unknown)".
func subnet24(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".0/24"
	}
	return "(unknown)"
}

// newNovelRangerExposureCmd surfaces unmanaged/rogue endpoints on each subnet
// by cross-referencing Ranger-discovered devices (and rogues) against managed
// agents, ranked by how many managed peers sit beside them. The console never
// joins the discovery inventory to the agent inventory.
// pp:data-source local
func newNovelRangerExposureCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Find unmanaged/rogue devices per subnet by joining Ranger discovery to managed agents",
		Long: `Cross-reference SentinelOne Ranger-discovered devices and rogues against the
managed agent inventory to surface unmanaged endpoints, grouped by /24 subnet
and ranked by how many managed peers sit beside them (high managed density +
an unmanaged device = a likely blind spot worth investigating).

Requires 'ranger' and/or 'rogues' to be synced. A device is treated as
unmanaged when it is a rogue, when Ranger marks it unmanaged, or when its IP
does not match any managed agent.`,
		Example: `  # Unmanaged exposure by subnet
  sentinelone-cli ranger exposure

  # JSON
  sentinelone-cli ranger exposure --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}
			rogues, err := loadResourceObjects(cmd.Context(), db, "rogues")
			if err != nil {
				return fmt.Errorf("loading rogues: %w", err)
			}
			ranger, err := loadResourceObjects(cmd.Context(), db, "ranger")
			if err != nil {
				return fmt.Errorf("loading ranger inventory: %w", err)
			}
			if len(rogues) == 0 && len(ranger) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No Ranger or rogues data in the local store. Run 'sentinelone-cli sync --resources ranger,rogues' first.", nil)
			}

			// Managed agent IPs + per-subnet managed counts.
			managedIPs := map[string]bool{}
			managedSubnet := map[string]int{}
			for _, a := range agents {
				ip := gstrFirst(a, "lastIpToMgmt", "externalIp")
				if ip == "" {
					continue
				}
				managedIPs[ip] = true
				managedSubnet[subnet24(ip)]++
			}

			deviceIP := func(d map[string]any) string {
				return gstrFirst(d, "ip", "ipAddress", "lastSeenIp", "networkInterfaceIp", "srcIp", "deviceIp")
			}
			deviceName := func(d map[string]any) string {
				return gstrFirst(d, "deviceName", "name", "hostname", "dvcHostname", "networkName")
			}
			deviceMAC := func(d map[string]any) string {
				return gstrFirst(d, "macAddress", "mac", "networkInterfacePhysical")
			}

			seen := map[string]bool{}
			subnets := map[string]*exposureSubnet{}
			addUnmanaged := func(d map[string]any, source string) {
				ip := deviceIP(d)
				mac := deviceMAC(d)
				key := ip + "|" + mac + "|" + deviceName(d)
				if seen[key] {
					return
				}
				seen[key] = true
				sn := subnet24(ip)
				es := subnets[sn]
				if es == nil {
					es = &exposureSubnet{Subnet: sn, ManagedPeers: managedSubnet[sn]}
					subnets[sn] = es
				}
				es.Unmanaged++
				es.Devices = append(es.Devices, exposureDevice{
					Name:   orUnknown(deviceName(d)),
					IP:     ip,
					MAC:    mac,
					Source: source,
				})
			}

			for _, d := range rogues {
				addUnmanaged(d, "rogue")
			}
			for _, d := range ranger {
				state := strings.ToLower(gstr(d, "managedState"))
				switch {
				case strings.Contains(state, "unmanaged"):
					addUnmanaged(d, "ranger:unmanaged")
				case state == "":
					if ip := deviceIP(d); ip != "" && !managedIPs[ip] {
						addUnmanaged(d, "ranger:no-agent-ip")
					}
				}
			}

			if len(subnets) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No unmanaged devices found — every discovered device maps to a managed agent.", nil)
			}

			var list []*exposureSubnet
			for _, es := range subnets {
				if limit > 0 && len(es.Devices) > limit {
					es.Devices = es.Devices[:limit]
				}
				list = append(list, es)
			}
			sort.SliceStable(list, func(i, j int) bool {
				if list[i].Unmanaged != list[j].Unmanaged {
					return list[i].Unmanaged > list[j].Unmanaged
				}
				return list[i].ManagedPeers > list[j].ManagedPeers
			})

			totalUnmanaged := 0
			for _, es := range list {
				totalUnmanaged += es.Unmanaged
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"total_unmanaged": totalUnmanaged,
					"subnets":         list,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Unmanaged exposure: %d devices across %d subnet(s):\n\n", totalUnmanaged, len(list))
			fmt.Fprintf(w, "%-20s %10s %14s\n", "SUBNET", "UNMANAGED", "MANAGED-PEERS")
			for _, es := range list {
				fmt.Fprintf(w, "%-20s %10d %14d\n", es.Subnet, es.Unmanaged, es.ManagedPeers)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum devices to list per subnet in JSON (0 = all)")
	return cmd
}
