// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"atera-pp-cli/internal/cliutil"
)

// patchAgentStatus is one device's missing-patch rollup.
type patchAgentStatus struct {
	DeviceGuid     string `json:"DeviceGuid"`
	MachineName    string `json:"MachineName"`
	CustomerName   string `json:"CustomerName"`
	MissingPatches int    `json:"MissingPatches"`
	CriticalKBs    int    `json:"CriticalKBs"` // updates whose Class marks them critical/security
}

type patchFetchFailure struct {
	DeviceGuid  string `json:"DeviceGuid"`
	MachineName string `json:"MachineName"`
	Error       string `json:"Error"`
}

type patchStatusReport struct {
	Agents        []patchAgentStatus  `json:"Agents"`
	ScannedAgents int                 `json:"scanned_agents"`
	MaxScanAgents int                 `json:"max_scan_agents"`
	TotalMissing  int                 `json:"TotalMissing"`
	ByCustomer    map[string]int      `json:"ByCustomer"`
	FetchFailures []patchFetchFailure `json:"fetch_failures"`
	Note          string              `json:"note,omitempty"`
}

// patchFetchResult carries each fan-out fetch with its error so failed devices
// are excluded from totals instead of becoming phantom zero rows.
type patchFetchResult struct {
	guid     string
	machine  string
	customer string
	missing  int
	critical int
	err      error
}

// agentAvailableUpdates mirrors the Atera AgentAvailableUpdates DTO.
type agentAvailableUpdates struct {
	DeviceGuid       string `json:"DeviceGuid"`
	AvailableUpdates []struct {
		Name   string `json:"Name"`
		Class  string `json:"Class"`
		KBId   string `json:"KBId"`
		Status string `json:"Status"`
	} `json:"AvailableUpdates"`
}

// patchRollup aggregates fan-out results into the report, excluding failed
// fetches from every total and denominator (parallel-fetch principle).
func patchRollup(results []patchFetchResult, limit, maxScan int) patchStatusReport {
	rep := patchStatusReport{
		Agents:        make([]patchAgentStatus, 0, len(results)),
		ScannedAgents: len(results),
		MaxScanAgents: maxScan,
		ByCustomer:    map[string]int{},
		FetchFailures: make([]patchFetchFailure, 0),
	}
	for _, r := range results {
		if r.err != nil {
			rep.FetchFailures = append(rep.FetchFailures, patchFetchFailure{
				DeviceGuid:  r.guid,
				MachineName: r.machine,
				Error:       r.err.Error(),
			})
			continue
		}
		rep.TotalMissing += r.missing
		if r.customer != "" {
			rep.ByCustomer[r.customer] += r.missing
		}
		if r.missing == 0 {
			continue // fully patched devices stay out of the ranked list
		}
		rep.Agents = append(rep.Agents, patchAgentStatus{
			DeviceGuid:     r.guid,
			MachineName:    r.machine,
			CustomerName:   r.customer,
			MissingPatches: r.missing,
			CriticalKBs:    r.critical,
		})
	}
	sort.SliceStable(rep.Agents, func(i, j int) bool {
		if rep.Agents[i].MissingPatches != rep.Agents[j].MissingPatches {
			return rep.Agents[i].MissingPatches > rep.Agents[j].MissingPatches
		}
		return rep.Agents[i].CriticalKBs > rep.Agents[j].CriticalKBs
	})
	if limit > 0 && len(rep.Agents) > limit {
		rep.Agents = rep.Agents[:limit]
	}
	return rep
}

// pp:data-source live
func newNovelAgentsPatchStatusCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var maxScanAgents int

	cmd := &cobra.Command{
		Use:   "patch-status",
		Short: "Roll up missing-patch counts across the estate, ranked by agent and customer, from the per-device patch endpoints.",
		Long: "Fans out the per-device available-patches endpoint across synced agents (the\n" +
			"API has no fleet-wide patch view) and rolls up missing-patch counts per machine\n" +
			"and customer. The agent list comes from the local store — run `atera-cli\n" +
			"sync` first — but the patch data is fetched live, paced under Atera's 700\n" +
			"req/min limit. --max-scan-agents bounds how many devices are queried;\n" +
			"--limit bounds how many ranked rows are returned.",
		Example:     "  atera-cli agents patch-status --limit 20 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				scope := fmt.Sprintf("up to %d", maxScanAgents)
				if maxScanAgents <= 0 {
					scope = "all"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "would fan out available-patches across %s synced agents\n", scope)
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
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

			if !hintIfUnsynced(cmd, s, "agents") {
				hintIfStale(cmd, s, "agents", flags.maxAge)
			}

			agents, err := nvLoad(s, "agents")
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			// 0 (or negative) means no cap, matching --limit semantics.
			if maxScanAgents < 0 {
				maxScanAgents = 0
			}
			// Curtail fan-out under the live-dogfood matrix's per-command timeout.
			if cliutil.IsDogfoodEnv() && (maxScanAgents == 0 || maxScanAgents > 2) {
				maxScanAgents = 2
			}

			// Bound the scan before any network IO.
			scan := make([]nvObj, 0)
			for _, a := range agents {
				if maxScanAgents > 0 && len(scan) >= maxScanAgents {
					break
				}
				if nvStr(a, "DeviceGuid") == "" {
					continue
				}
				scan = append(scan, a)
			}

			if len(scan) == 0 {
				rep := patchRollup(nil, limit, maxScanAgents)
				rep.Note = "no synced agents with a DeviceGuid to scan; run 'atera-cli sync --resources agents' first"
				return nvEmit(cmd, flags, rep, func() {
					fmt.Fprintln(cmd.OutOrStdout(), rep.Note)
				})
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Fan out with a small worker pool; the client's adaptive limiter
			// paces requests under the 700/min account limit.
			const workers = 4
			jobs := make(chan nvObj)
			resCh := make(chan patchFetchResult, len(scan))
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for a := range jobs {
						guid := nvStr(a, "DeviceGuid")
						r := patchFetchResult{
							guid:     guid,
							machine:  nvStr(a, "MachineName"),
							customer: nvStr(a, "CustomerName"),
						}
						data, err := c.Get(cmd.Context(), "/agents/"+url.PathEscape(guid)+"/available-patches", nil)
						if err != nil {
							r.err = err
							resCh <- r
							continue
						}
						var upd agentAvailableUpdates
						if err := json.Unmarshal(data, &upd); err != nil {
							r.err = fmt.Errorf("parsing available-patches: %w", err)
							resCh <- r
							continue
						}
						r.missing = len(upd.AvailableUpdates)
						for _, u := range upd.AvailableUpdates {
							cls := strings.ToLower(u.Class)
							if strings.Contains(cls, "critical") || strings.Contains(cls, "security") {
								r.critical++
							}
						}
						resCh <- r
					}
				}()
			}
			for _, a := range scan {
				jobs <- a
			}
			close(jobs)
			wg.Wait()
			close(resCh)

			results := make([]patchFetchResult, 0, len(scan))
			for r := range resCh {
				results = append(results, r)
			}

			rep := patchRollup(results, limit, maxScanAgents)
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d patch fetches failed; totals computed over the remaining %d devices\n",
					len(rep.FetchFailures), rep.ScannedAgents, rep.ScannedAgents-len(rep.FetchFailures))
			}
			if len(rep.Agents) == 0 && len(rep.FetchFailures) == 0 && len(agents) > len(scan) {
				rep.Note = fmt.Sprintf("scanned %d of %d synced agents without finding missing patches; raise --max-scan-agents to widen the sweep", len(scan), len(agents))
			}

			return nvEmit(cmd, flags, rep, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Scanned %d agents: %d missing patches total\n\n", rep.ScannedAgents, rep.TotalMissing)
				if len(rep.Agents) == 0 {
					fmt.Fprintln(w, "No devices with missing patches in the scanned set.")
					return
				}
				rows := make([][]string, 0, len(rep.Agents))
				for _, a := range rep.Agents {
					rows = append(rows, []string{
						a.MachineName, a.CustomerName,
						fmt.Sprintf("%d", a.MissingPatches),
						fmt.Sprintf("%d", a.CriticalKBs),
					})
				}
				nvTable(w, []string{"MACHINE", "CUSTOMER", "MISSING", "CRITICAL"}, rows)
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum ranked devices to return")
	cmd.Flags().IntVar(&maxScanAgents, "max-scan-agents", 50, "Maximum synced agents to query before returning partial results (0 = no cap)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
