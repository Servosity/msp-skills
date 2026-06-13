// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// eolMarkers are case-insensitive substrings of OS strings considered
// end-of-life (no longer receiving vendor security updates).
var eolMarkers = []string{
	"windows xp", "windows vista", "windows 7", "windows 8 ", "windows 8.0",
	"server 2003", "server 2008", "server 2012", "windows 2000",
	"el capitan", "yosemite", "mavericks", "mountain lion", "sierra", "high sierra",
	"mojave", "catalina",
}

func isEOL(os string) bool {
	l := strings.ToLower(os)
	for _, m := range eolMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

type eolAgent struct {
	AgentID      int64  `json:"AgentID"`
	MachineName  string `json:"MachineName"`
	CustomerName string `json:"CustomerName"`
	OS           string `json:"OS"`
	Online       bool   `json:"Online"`
}

type inventoryReport struct {
	TotalAgents int            `json:"TotalAgents"`
	ByOS        map[string]int `json:"ByOS"`
	ByOSType    map[string]int `json:"ByOSType"`
	EOLCount    int            `json:"EOLCount"`
	EOLAgents   []eolAgent     `json:"EOLAgents"`
}

// pp:data-source local
func newNovelAgentsInventoryCmd(flags *rootFlags) *cobra.Command {
	var eolOnly bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Roll up OS and OS-type across the whole estate and flag end-of-life operating systems.",
		Long: "Aggregates every synced agent's OS into counts and flags machines running an\n" +
			"end-of-life OS (Windows 7/8, Server 2003/2008/2012, older macOS, etc.).\n" +
			"Reads the local store; run `atera-cli sync` first. Use --eol to list only EOL machines.",
		Example:     "  atera-cli agents inventory --eol --agent",
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

			if !hintIfUnsynced(cmd, s, "agents") {
				hintIfStale(cmd, s, "agents", flags.maxAge)
			}

			agents, err := nvLoad(s, "agents")
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			rep := inventoryReport{
				ByOS:      map[string]int{},
				ByOSType:  map[string]int{},
				EOLAgents: make([]eolAgent, 0),
			}
			for _, o := range agents {
				rep.TotalAgents++
				os := nvStr(o, "OS")
				if os == "" {
					os = "(unknown)"
				}
				rep.ByOS[os]++
				ost := nvStr(o, "OSType")
				if ost == "" {
					ost = "(unknown)"
				}
				rep.ByOSType[ost]++
				if isEOL(os) {
					rep.EOLCount++
					id, _ := nvInt(o, "AgentID")
					rep.EOLAgents = append(rep.EOLAgents, eolAgent{
						AgentID:      id,
						MachineName:  nvStr(o, "MachineName"),
						CustomerName: nvStr(o, "CustomerName"),
						OS:           os,
						Online:       nvBool(o, "Online"),
					})
				}
			}
			sort.SliceStable(rep.EOLAgents, func(i, j int) bool {
				return rep.EOLAgents[i].CustomerName < rep.EOLAgents[j].CustomerName
			})

			// --eol narrows the payload to just the EOL machines + count.
			if eolOnly {
				out := struct {
					EOLCount  int        `json:"EOLCount"`
					EOLAgents []eolAgent `json:"EOLAgents"`
				}{rep.EOLCount, rep.EOLAgents}
				return nvEmit(cmd, flags, out, func() {
					w := cmd.OutOrStdout()
					if rep.EOLCount == 0 {
						fmt.Fprintln(w, "No end-of-life agents found.")
						return
					}
					rows := make([][]string, 0, len(rep.EOLAgents))
					for _, a := range rep.EOLAgents {
						rows = append(rows, []string{a.MachineName, a.CustomerName, a.OS})
					}
					nvTable(w, []string{"MACHINE", "CUSTOMER", "OS (EOL)"}, rows)
				})
			}

			return nvEmit(cmd, flags, rep, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Total agents: %d   EOL: %d\n\n", rep.TotalAgents, rep.EOLCount)
				type kv struct {
					k string
					v int
				}
				osRows := make([]kv, 0, len(rep.ByOS))
				for k, v := range rep.ByOS {
					osRows = append(osRows, kv{k, v})
				}
				sort.Slice(osRows, func(i, j int) bool { return osRows[i].v > osRows[j].v })
				rows := make([][]string, 0, len(osRows))
				for _, r := range osRows {
					rows = append(rows, []string{r.k, fmt.Sprintf("%d", r.v)})
				}
				nvTable(w, []string{"OS", "COUNT"}, rows)
			})
		},
	}
	cmd.Flags().BoolVar(&eolOnly, "eol", false, "List only end-of-life machines instead of the full rollup")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
