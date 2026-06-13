// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type triageAlert struct {
	AlertID      int64  `json:"AlertID"`
	Severity     string `json:"Severity"`
	Title        string `json:"Title"`
	CustomerName string `json:"CustomerName"`
	DeviceName   string `json:"DeviceName"`
	Created      string `json:"Created"`
	AgeHours     int64  `json:"AgeHours"` // -1 when Created is missing/unparseable
}

// severityRank orders alert severities loudest-first.
func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "information", "info":
		return 1
	default:
		return 0
	}
}

// pp:data-source local
func newNovelAlertsTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includeArchived bool

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Rank open alerts by severity then age so the loudest, oldest problems surface first.",
		Long: "Use this command to triage the CURRENT open-alert queue by severity and customer. Do NOT use it to find the machines generating the most alerts over time; use 'agents noisy' instead.\n\n" +
			"Loads synced alerts (skipping archived ones unless --include-archived), then ranks\n" +
			"by severity (Critical > Warning > Information) and age. Reads the local store;\n" +
			"run `atera-cli sync` first. Correlating across the alert set is not an API filter.",
		Example:     "  atera-cli alerts triage --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("atera-cli")
			}
			s, nvOK, err := nvOpenRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			if nvOK {
				defer s.Close()
			}

			if !hintIfUnsynced(cmd, s, "alerts") {
				hintIfStale(cmd, s, "alerts", flags.maxAge)
			}

			alerts, err := nvLoad(s, "alerts")
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}

			now := nvNow()
			results := make([]triageAlert, 0)
			for _, o := range alerts {
				if !includeArchived && nvBool(o, "Archived") {
					continue
				}
				age := int64(-1)
				if t, ok := nvTime(o, "Created"); ok {
					age = int64(now.Sub(t).Hours())
				}
				id, _ := nvInt(o, "AlertID")
				results = append(results, triageAlert{
					AlertID:      id,
					Severity:     nvStr(o, "Severity"),
					Title:        nvStr(o, "Title"),
					CustomerName: nvStr(o, "CustomerName"),
					DeviceName:   nvStr(o, "DeviceName"),
					Created:      nvStr(o, "Created"),
					AgeHours:     age,
				})
			}
			sort.SliceStable(results, func(i, j int) bool {
				ri, rj := severityRank(results[i].Severity), severityRank(results[j].Severity)
				if ri != rj {
					return ri > rj
				}
				return results[i].AgeHours > results[j].AgeHours // oldest first within a severity
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintln(w, "No open alerts.")
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					age := "?"
					if r.AgeHours >= 0 {
						age = fmt.Sprintf("%dh", r.AgeHours)
					}
					rows = append(rows, []string{r.Severity, r.CustomerName, r.DeviceName, r.Title, age})
				}
				nvTable(w, []string{"SEVERITY", "CUSTOMER", "DEVICE", "TITLE", "AGE"}, rows)
			})
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived alerts (default: open only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
