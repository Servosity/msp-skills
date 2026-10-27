// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: fleet-wide data-source coverage.
// pp:data-source live
package cli

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

type sourceItem struct {
	AccountID       int64    `json:"account_id"`
	DeviceName      string   `json:"device_name"`
	Customer        string   `json:"customer"`
	OSType          string   `json:"os_type,omitempty"`
	DataSources     []string `json:"data_sources"`
	DataSourceCodes []string `json:"data_source_codes"`
	UnknownCodes    []string `json:"unknown_codes,omitempty"`
}

type sourceTally struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Devices int    `json:"devices"`
}

type sourcesView struct {
	PartnerID         int64         `json:"partner_id"`
	Without           string        `json:"without,omitempty"`
	OS                string        `json:"os,omitempty"`
	Items             []sourceItem  `json:"items"`
	TotalDevices      int           `json:"total_devices"`
	Sources           []sourceTally `json:"sources"`
	UnreportedDevices int           `json:"unreported_devices"`
	OSUnknownExcluded int           `json:"os_unknown_excluded,omitempty"`
	scanCapFields
}

// dataSourceTotal (D9) is Cove's synthetic "Total" data source: it is never
// an active source, so "devices without Total" would list every device.
const dataSourceTotal = "D9"

// osTypeNames decodes the I32 "OS type" column.
var osTypeNames = map[int64]string{1: "workstation", 2: "server"}

// resolveDataSource maps a --without value (a D-code such as D2, or a data
// source name such as "System State", case-insensitive) to its D-code. An
// unrecognised value is refused: silently matching nothing would read as
// "every device protects it".
func resolveDataSource(raw string) (string, error) {
	want := strings.TrimSpace(raw)
	if code, ok := normalizeSourceCode(want); ok && code != dataSourceTotal {
		if _, known := coverpc.DataSourceNames[code]; known {
			return code, nil
		}
	}
	for code, name := range coverpc.DataSourceNames {
		if code != dataSourceTotal && strings.EqualFold(name, want) {
			return code, nil
		}
	}
	names := make([]string, 0, len(coverpc.DataSourceNames))
	for _, code := range sortedKeys(coverpc.DataSourceNames) {
		if code == dataSourceTotal {
			continue
		}
		names = append(names, code+" ("+coverpc.DataSourceNames[code]+")")
	}
	return "", fmt.Errorf("unknown data source %q; use a D-code or name: %s", raw, strings.Join(names, ", "))
}

// normalizeSourceCode turns "d02" or " D2 " into "D2". ok is false when the
// value is not a D-code at all.
func normalizeSourceCode(raw string) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if len(s) < 2 || s[0] != 'D' {
		return "", false
	}
	n, err := strconv.Atoi(s[1:])
	if err != nil || n < 0 {
		return "", false
	}
	return "D" + strconv.Itoa(n), true
}

// parseActiveSources splits the I78 value ("D1,D2,D19") into known codes and
// codes this build cannot name. Unknown codes are kept, never dropped.
func parseActiveSources(raw string) (codes, unknown []string) {
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		code, ok := normalizeSourceCode(part)
		if !ok {
			code = strings.TrimSpace(part)
		}
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		if _, known := coverpc.DataSourceNames[code]; known {
			codes = append(codes, code)
		} else {
			unknown = append(unknown, code)
		}
	}
	sort.Slice(codes, func(i, j int) bool { return sourceCodeLess(codes[i], codes[j]) })
	sort.Strings(unknown)
	return codes, unknown
}

func sourceCodeLess(a, b string) bool {
	na, _ := strconv.Atoi(strings.TrimPrefix(a, "D"))
	nb, _ := strconv.Atoi(strings.TrimPrefix(b, "D"))
	return na < nb
}

func newNovelDevicesSourcesCmd(flags *rootFlags) *cobra.Command {
	var without string
	var osFilter string
	var partnerID int64
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Which data sources each device protects, fleet-wide, with a filter for devices missing one",
		Long: strings.Trim(`
Decodes Cove's active data sources column (I78, e.g. "D1,D2") for every device
across the whole partner tree: Files and Folders, System State, MS SQL,
Hyper-V, VMware, Microsoft 365 Exchange/OneDrive/SharePoint/Teams and the rest.
Prints each device's protected sources plus a fleet tally per source.

--without keeps only devices whose reported sources lack the named source.
A device whose sources column was not reported is never listed as missing a
source; it is counted in unreported_devices instead.

Use this command to prove what each device protects. Do NOT use it for backup
success or failure; use 'devices failures' or 'devices stale' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli devices sources --agent
  cove-cli devices sources --without "System State" --os server --json
  cove-cli devices sources --without D3 --csv`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:data-source":         "live",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			wantCode := ""
			if strings.TrimSpace(without) != "" {
				code, err := resolveDataSource(without)
				if err != nil {
					return usageErr(err)
				}
				wantCode = code
			}
			osWant := strings.ToLower(strings.TrimSpace(osFilter))
			switch osWant {
			case "", "any":
				osWant = ""
			case "server", "workstation":
			default:
				return usageErr(fmt.Errorf("--os must be server, workstation or any, not %q", osFilter))
			}
			if dryRunOK(flags) {
				return coveDryRun(cmd, flags, "would decode the active data sources of every device in the partner tree")
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			root, err := resolvePartnerID(cmd.Context(), c, partnerID)
			if err != nil {
				return authErr(err)
			}
			devices, scanned, capHit, err := fetchFleetStats(cmd.Context(), c, fleetStatsQuery{
				PartnerID: root,
				Columns: []string{
					coverpc.ColDeviceID, coverpc.ColDeviceName, coverpc.ColCustomer,
					coverpc.ColOSType, coverpc.ColActiveSources,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			items := make([]sourceItem, 0)
			tally := map[string]int{}
			unreported, osUnknown := 0, 0
			for _, d := range devices {
				s := d.Settings
				osType := ""
				if v, ok := coverpc.SettingInt(s, coverpc.ColOSType); ok {
					osType = osTypeNames[v]
				}
				if osWant != "" && osType != osWant {
					if osType == "" {
						osUnknown++
					}
					continue
				}
				raw, reported := s[coverpc.ColActiveSources]
				if !reported {
					unreported++
					continue
				}
				codes, unknown := parseActiveSources(raw)
				for _, code := range codes {
					tally[code]++
				}
				if wantCode != "" && slices.Contains(codes, wantCode) {
					continue
				}
				names := make([]string, 0, len(codes))
				for _, code := range codes {
					names = append(names, coverpc.DataSourceNames[code])
				}
				items = append(items, sourceItem{
					AccountID:       d.AccountID,
					DeviceName:      s[coverpc.ColDeviceName],
					Customer:        s[coverpc.ColCustomer],
					OSType:          osType,
					DataSources:     names,
					DataSourceCodes: append([]string{}, codes...),
					UnknownCodes:    unknown,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Customer != items[j].Customer {
					return items[i].Customer < items[j].Customer
				}
				return items[i].DeviceName < items[j].DeviceName
			})
			sources := make([]sourceTally, 0, len(tally))
			for code, n := range tally {
				sources = append(sources, sourceTally{Code: code, Name: coverpc.DataSourceNames[code], Devices: n})
			}
			sort.Slice(sources, func(i, j int) bool { return sourceCodeLess(sources[i].Code, sources[j].Code) })
			view := sourcesView{
				PartnerID:         root,
				OS:                osWant,
				Items:             items,
				TotalDevices:      len(items),
				Sources:           sources,
				UnreportedDevices: unreported,
				OSUnknownExcluded: osUnknown,
				scanCapFields: scanCapFields{
					ScannedDevices: scanned,
					MaxScanPages:   effectiveMaxPages(maxScanPages),
					Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "data-source"),
				},
			}
			if wantCode != "" {
				view.Without = coverpc.DataSourceNames[wantCode] + " (" + wantCode + ")"
			}
			if unreported > 0 && view.Note == "" {
				view.Note = fmt.Sprintf("%d devices did not report their active data sources and are not listed", unreported)
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().StringVar(&without, "without", "", "Only devices whose reported sources lack this data source (D-code such as D2, or a name such as \"System State\")")
	cmd.Flags().StringVar(&osFilter, "os", "", "Only server or workstation devices (OS type column I32); default any")
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to sweep (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	return cmd
}
