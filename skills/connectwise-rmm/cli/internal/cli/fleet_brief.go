// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
	"connectwise-rmm-pp-cli/internal/store"
)

type briefTicket struct {
	ID        string `json:"id"`
	Number    string `json:"number,omitempty"`
	Summary   string `json:"summary"`
	Status    string `json:"status,omitempty"`
	Priority  string `json:"priority,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Linked    string `json:"linked_by"` // asset | site
}

type briefView struct {
	Query       string            `json:"query"`
	Endpoint    *fleet.Endpoint   `json:"endpoint,omitempty"`
	Heartbeat   any               `json:"heartbeat,omitempty"`
	SystemState any               `json:"system_state,omitempty"`
	Patches     any               `json:"patches,omitempty"`
	Tickets     []briefTicket     `json:"tickets"`
	Detail      json.RawMessage   `json:"detail,omitempty"`
	Unavailable map[string]string `json:"unavailable,omitempty"`
}

func newNovelFleetBriefCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "brief [endpoint]",
		Short: "Show one endpoint's card by ID or cached name: online state, logged-on users, patch counts, tickets",
		Long: strings.Trim(`
One agent-shaped card for one endpoint, by endpoint ID or machine name:
availability and last seen (heartbeat), last boot and logged-on users (system
state), patch summary, synced tickets linked to the endpoint or its site, and
the raw endpoint detail under "detail". A section the API key cannot read is
listed under "unavailable" instead of failing the whole card.

Names resolve from the local store; machine names are cached by 'fleet whois'
and earlier briefs, so pass the endpoint ID the first time.

Use this command for one endpoint. Do NOT use this command to find which
machine a user is on; use 'fleet whois' instead. Do NOT use this command for a
company-wide view; use 'fleet scorecard' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet brief ACME-DC01 --agent
  connectwise-rmm-cli fleet brief 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --select heartbeat,patches`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "endpoint=ACME-DC01"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet brief")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an endpoint ID or name is required, e.g. fleet brief ACME-DC01"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := briefView{Query: args[0], Tickets: make([]briefTicket, 0), Unavailable: map[string]string{}}
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			var eps []fleet.Endpoint
			if !missing {
				defer db.Close()
				if !hintIfUnsynced(cmd, db, "endpoints") {
					hintIfStale(cmd, db, "endpoints", flags.maxAge)
				}
				if eps, err = loadFleet(db); err != nil {
					return err
				}
			}
			e, err := resolveEndpoint(eps, args[0])
			if err != nil {
				// An endpoint ID can be read live even when it is not synced.
				if !uuidLike.MatchString(strings.TrimSpace(args[0])) {
					return err
				}
				e = fleet.Endpoint{ID: strings.TrimSpace(args[0])}
			}
			view.Endpoint = &e
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, detailErr := c.GetNoCache(ctx, "/api/platform/v1/device/endpoints/"+url.PathEscape(e.ID), nil)
			if detailErr != nil {
				view.Unavailable["detail"] = detailErr.Error()
			} else {
				view.Detail = raw
				enrichEndpoint(view.Endpoint, fleet.ParseEndpoint(fleet.Object(raw)), db)
			}
			hb, hbFail := fetchHeartbeats(ctx, c, []string{e.ID})
			if h, ok := hb[e.ID]; ok {
				view.Heartbeat = h
			} else if len(hbFail) > 0 {
				view.Unavailable["heartbeat"] = hbFail[0].Error
			} else {
				view.Unavailable["heartbeat"] = "no heartbeat record returned for this endpoint"
			}
			states, stFail := liveEndpointData(ctx, cmd, db, c, cacheSystemState, e.ID, systemStatePath)
			if raw, ok := states[e.ID]; ok {
				if st, valid := fleet.ParseSystemState(raw); valid {
					view.SystemState = st
				} else {
					view.Unavailable["system_state"] = "unreadable system state payload"
				}
			} else if len(stFail) > 0 {
				view.Unavailable["system_state"] = stFail[0].Error
			}
			patches, pFail := liveEndpointData(ctx, cmd, db, c, cachePatchSummary, e.ID, patchPath)
			if raw, ok := patches[e.ID]; ok {
				if s, ok := fleet.ParsePatchSummary(raw); ok {
					view.Patches = s
				} else {
					view.Unavailable["patches"] = "no readable patch summary"
				}
			} else if len(pFail) > 0 {
				view.Unavailable["patches"] = pFail[0].Error
			}
			if db == nil {
				view.Unavailable["tickets"] = "no local store; sync tickets to include them"
			} else if tickets, err := endpointTickets(db, e); err != nil {
				view.Unavailable["tickets"] = err.Error()
			} else {
				view.Tickets = tickets
			}
			// Every live read came back empty and the endpoint read itself
			// failed: return that error (exit code and auth hint) instead of an
			// empty card. Partial failures stay listed under "unavailable".
			if detailErr != nil && view.Heartbeat == nil && view.SystemState == nil && view.Patches == nil {
				return classifyAPIErrorOnly(detailErr)
			}
			if len(view.Unavailable) == 0 {
				view.Unavailable = nil
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			return printBrief(cmd, view)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}

// endpointTickets returns synced tickets that list the endpoint as an asset
// or belong to its site, asset-linked first.
func endpointTickets(db *store.Store, e fleet.Endpoint) ([]briefTicket, error) {
	rows, err := scan(db, "tickets")
	if err != nil {
		return nil, err
	}
	asset, site := make([]briefTicket, 0), make([]briefTicket, 0)
	for _, t := range rows {
		linked := ""
		for _, a := range listOf(t["assets"]) {
			if am, ok := a.(map[string]any); ok && strings.EqualFold(fleet.Str(am, "id"), e.ID) {
				linked = "asset"
			}
		}
		if linked == "" && e.SiteID != "" && strings.EqualFold(nestedID(t, "site"), e.SiteID) {
			linked = "site"
		}
		if linked == "" {
			continue
		}
		bt := briefTicket{
			ID: fleet.Str(t, "id"), Number: fleet.Str(t, "number"), Summary: fleet.Str(t, "summary"),
			Status: nestedName(t, "status"), Priority: nestedName(t, "priority"), UpdatedAt: fleet.Str(t, "updatedAt"), Linked: linked,
		}
		if linked == "asset" {
			asset = append(asset, bt)
		} else {
			site = append(site, bt)
		}
	}
	return append(asset, site...), nil
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

func nestedID(m map[string]any, key string) string {
	if o, ok := m[key].(map[string]any); ok {
		return fleet.Str(o, "id")
	}
	return ""
}

func nestedName(m map[string]any, key string) string {
	if o, ok := m[key].(map[string]any); ok {
		return firstNonEmpty(fleet.Str(o, "name"), fleet.Str(o, "id"))
	}
	return ""
}

func printBrief(cmd *cobra.Command, v briefView) error {
	w := cmd.OutOrStdout()
	if v.Endpoint == nil {
		fmt.Fprintln(w, "No endpoint resolved.")
		return nil
	}
	e := v.Endpoint
	fmt.Fprintf(w, "%s  (%s)\n  company: %s  site: %s  type: %s  os: %s\n", firstNonEmpty(e.Name, e.ID), e.ID, e.CompanyName, e.SiteName, e.Type, e.OS)
	if h, ok := v.Heartbeat.(fleet.Heartbeat); ok {
		fmt.Fprintf(w, "  online: %v  last seen: %s\n", h.Available, h.LastSeen.Format("2006-01-02 15:04 MST"))
	}
	if s, ok := v.SystemState.(fleet.SystemState); ok {
		users := make([]string, 0, len(s.LoggedOn))
		for _, u := range s.LoggedOn {
			users = append(users, u.Username)
		}
		fmt.Fprintf(w, "  last boot: %s  logged on: %s\n", s.LastBoot, strings.Join(users, ", "))
	}
	if p, ok := v.Patches.(fleet.PatchSummary); ok {
		comp := "n/a"
		if p.Compliance != nil {
			comp = fmt.Sprintf("%.1f", *p.Compliance)
		}
		fmt.Fprintf(w, "  patches: compliance %s, missing %d, pending reboot %d, policy %s\n", comp, p.Missing, p.PendingReboot, p.Policy)
	}
	fmt.Fprintf(w, "  tickets: %d\n", len(v.Tickets))
	for _, t := range v.Tickets {
		fmt.Fprintf(w, "    [%s] %s  %s (%s)\n", t.Linked, firstNonEmpty(t.Number, t.ID), t.Summary, t.Status)
	}
	for k, msg := range v.Unavailable {
		fmt.Fprintf(w, "  %s unavailable: %s\n", k, msg)
	}
	return nil
}

// enrichEndpoint fills fields the local store did not have (an unsynced
// endpoint ID) from the live detail payload, then joins company and site names,
// so ticket matching and the card see the real company and site.
func enrichEndpoint(e *fleet.Endpoint, d fleet.Endpoint, db *store.Store) {
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&e.Name, d.Name}, {&e.CompanyID, d.CompanyID}, {&e.SiteID, d.SiteID}, {&e.Type, d.Type}, {&e.OS, d.OS},
	} {
		if *f.dst == "" {
			*f.dst = f.src
		}
	}
	if db == nil || (e.CompanyName != "" && e.SiteName != "") {
		return
	}
	if companies, sites, err := fleetNames(db); err == nil {
		if e.CompanyName == "" {
			e.CompanyName = companies[e.CompanyID]
		}
		if e.SiteName == "" {
			e.SiteName = sites[e.SiteID]
		}
	}
}
