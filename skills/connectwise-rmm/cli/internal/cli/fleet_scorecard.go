// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type siteCard struct {
	SiteID              string         `json:"site_id"`
	Site                string         `json:"site,omitempty"`
	Endpoints           int            `json:"endpoints"`
	ByType              map[string]int `json:"by_type"`
	ByOS                map[string]int `json:"by_os"`
	Offline             int            `json:"offline"`
	AvailabilityUnknown int            `json:"availability_unknown"`
	Assessed            int            `json:"patch_assessed"`
	MissingPatches      int            `json:"missing_patches"`
	PendingReboot       int            `json:"pending_reboot_endpoints"`
	OutOfSupport        int            `json:"out_of_support"`
	TicketsByStatus     map[string]int `json:"tickets_by_status"`
	TicketsByPriority   map[string]int `json:"tickets_by_priority"`
}

type scorecardView struct {
	Query         string         `json:"query"`
	CompanyID     string         `json:"company_id,omitempty"`
	Company       string         `json:"company,omitempty"`
	Endpoints     int            `json:"endpoints"`
	CapHit        bool           `json:"scan_cap_hit,omitempty"`
	Sites         []siteCard     `json:"sites"`
	FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
}

func newNovelFleetScorecardCmd(flags *rootFlags) *cobra.Command {
	var cacheTTL, dbPath string
	var maxEndpoints int
	var refresh bool
	cmd := &cobra.Command{
		Use:   "scorecard [company]",
		Short: "Show one company's per-site health card: endpoint mix, offline agents, patch risk, tickets by status",
		Long: strings.Trim(`
A one-page health card for one company (ID or name), one row per site:
endpoint counts by type and OS, offline agents (live heartbeat), patch risk
from cached patch summaries (missing patches, pending-reboot endpoints, out of
support), and synced tickets grouped by their raw status and priority names.

Use this command for one company's combined health card. Do NOT use this
command for cross-company patch ranking; use 'fleet patch-posture' instead. Do
NOT use this command for fleet-wide offline agents; use 'fleet stale' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet scorecard "Acme Corp" --agent
  connectwise-rmm-cli fleet scorecard 2c4e8f1a-13b2-4f6d-bf2d-fd2324300001 --refresh`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "company=Acme Corp"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet scorecard")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a company ID or name is required, e.g. fleet scorecard \"Acme Corp\""))
			}
			ttl, err := parseDurationFlag("cache-ttl", cacheTTL)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := scorecardView{Query: args[0], Sites: make([]siteCard, 0)}
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				return notFoundErr(fmt.Errorf("no local store; run 'connectwise-rmm-cli sync --resources companies,sites,endpoints,tickets' before 'fleet scorecard'"))
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "endpoints") {
				hintIfStale(cmd, db, "endpoints", flags.maxAge)
			}
			ps, err := loadPatchScope(ctx, cmd, flags, db, args[0], maxEndpoints, ttl, refresh)
			if err != nil {
				return err
			}
			if len(ps.eps) == 0 {
				return notFoundErr(fmt.Errorf("no synced endpoints for company %q; check the name with 'companies list' or run sync", args[0]))
			}
			view.CompanyID, view.Company = ps.eps[0].CompanyID, ps.eps[0].CompanyName
			view.Endpoints, view.CapHit, view.FetchFailures = len(ps.eps), ps.capHit, ps.failures
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			hb, hbFail := fetchHeartbeats(ctx, c, endpointIDs(ps.eps))
			if err := allHeartbeatsFailed(hb, hbFail); err != nil {
				return err
			}
			warnFailures(cmd, "heartbeat", hbFail, len(ps.eps))
			unknown := unknownIDs(hbFail)
			view.FetchFailures = append(view.FetchFailures, hbFail...)
			cards := map[string]*siteCard{}
			card := func(id, name string) *siteCard {
				if sc, ok := cards[id]; ok {
					return sc
				}
				sc := &siteCard{SiteID: id, Site: name, ByType: map[string]int{}, ByOS: map[string]int{}, TicketsByStatus: map[string]int{}, TicketsByPriority: map[string]int{}}
				cards[id] = sc
				return sc
			}
			for _, e := range ps.eps {
				sc := card(e.SiteID, e.SiteName)
				sc.Endpoints++
				sc.ByType[firstNonEmpty(e.Type, "unknown")]++
				sc.ByOS[firstNonEmpty(e.OS, "unknown")]++
				switch h, ok := hb[e.ID]; {
				case unknown[e.ID]:
					sc.AvailabilityUnknown++
				case !ok || !h.Available: // missing from a successful heartbeat counts as offline, as in stale and reboot-due
					sc.Offline++
				}
				if s, ok := ps.sums[e.ID]; ok {
					sc.Assessed++
					sc.MissingPatches += s.Missing
					if s.PendingReboot > 0 {
						sc.PendingReboot++
					}
					if s.OutOfSupport {
						sc.OutOfSupport++
					}
				}
			}
			tickets, err := scan(db, "tickets")
			if err != nil {
				return err
			}
			for _, t := range tickets {
				if !strings.EqualFold(nestedID(t, "company"), view.CompanyID) {
					continue
				}
				sc := card(nestedID(t, "site"), "")
				sc.TicketsByStatus[firstNonEmpty(nestedName(t, "status"), "unknown")]++
				sc.TicketsByPriority[firstNonEmpty(nestedName(t, "priority"), "unknown")]++
			}
			for _, sc := range cards {
				view.Sites = append(view.Sites, *sc)
			}
			sort.Slice(view.Sites, func(i, j int) bool { return view.Sites[i].Endpoints > view.Sites[j].Endpoints })
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d endpoints across %d sites\n", firstNonEmpty(view.Company, view.CompanyID), view.Endpoints, len(view.Sites))
			rows := make([]map[string]any, 0, len(view.Sites))
			for _, s := range view.Sites {
				ticketCount := 0
				for _, n := range s.TicketsByStatus {
					ticketCount += n
				}
				rows = append(rows, map[string]any{"site": firstNonEmpty(s.Site, s.SiteID), "endpoints": s.Endpoints, "offline": s.Offline, "avail_unknown": s.AvailabilityUnknown, "missing": s.MissingPatches, "reboot": s.PendingReboot, "unsupported": s.OutOfSupport, "tickets": ticketCount})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&cacheTTL, "cache-ttl", "12h", "Reuse cached patch summaries younger than this")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ignore cached patch summaries and re-read every endpoint")
	cmd.Flags().IntVar(&maxEndpoints, "max-endpoints", 500, "Maximum endpoints to read patch summaries for (0 = no cap)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
