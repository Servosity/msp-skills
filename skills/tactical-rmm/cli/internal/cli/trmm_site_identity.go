// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix site-identity-client-and-site.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Tactical RMM site names are unique only within a client (vendor-src/clients/views.py
// GetAddSites; client names are unique fleet-wide), so two clients each with a
// site called "Main" is ordinary MSP data. A site is identified by its id when
// the agent row carries one, otherwise by (client name, site name), never by
// the site name alone. 'fleet health' counts sites from the synced site list
// for the same reason (handfix entity-inventory-from-own-list).
//
// Agent rows come in two shapes. GET /agents/ (sync) carries client_name and
// site_name. GET /agents/{agent_id}/ (write-through cache) carries the site id
// in "site" and the client name in "client". trmmAgentSiteFrom joins an agent
// row (alias a) to the synced site list (alias s) on that id, so the site list's
// names win when the id is known.
const trmmAgentSiteFrom = "resources a LEFT JOIN resources s ON s.resource_type='clients-sites' AND json_type(a.data,'$.site')='integer' AND json_extract(s.data,'$.id')=json_extract(a.data,'$.site')"

// trmmAgentClientSQL is the agent row's client name (over trmmAgentSiteFrom).
const trmmAgentClientSQL = "COALESCE(json_extract(s.data,'$.client_name'),json_extract(a.data,'$.client_name'),CASE WHEN json_type(a.data,'$.client')='text' THEN json_extract(a.data,'$.client') END,'(unknown)')"

// trmmAgentSiteNameSQL is the agent row's site name (over trmmAgentSiteFrom).
const trmmAgentSiteNameSQL = "COALESCE(json_extract(s.data,'$.name'),json_extract(a.data,'$.site_name'),'(unknown)')"

// trmmClientsHoldingSiteNameSQL lists every client that has a site with the
// bound name, from the agents and from the synced site list (a site with no
// agents still exists).
const trmmClientsHoldingSiteNameSQL = "SELECT " + trmmAgentClientSQL + " c FROM " + trmmAgentSiteFrom +
	" WHERE a.resource_type='agents' AND json_extract(a.data,'$.site_name')=?" +
	" UNION SELECT COALESCE(json_extract(data,'$.client_name'),'(unknown)') FROM resources WHERE resource_type='clients-sites' AND json_extract(data,'$.name')=?"

// novelRefuseAmbiguousSite refuses a cohort --filter site=<name> that names a
// site in more than one client unless a client= entry picks the client: the
// cohort selector matches site_name, so the bare name would select agents in
// every client's site of that name. Runs before any request (preview included).
func novelRefuseAmbiguousSite(cmd *cobra.Command, flags *rootFlags, filter []string) error {
	sites := []string{}
	for _, kv := range filter {
		p := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(p) != 2 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(p[0])) {
		case "client":
			return nil
		case "site":
			sites = append(sites, strings.TrimSpace(p[1]))
		}
	}
	if len(sites) == 0 {
		return nil
	}
	s := novelLocalRead(cmd, flags, "agents")
	if s == nil {
		return nil
	}
	defer s.Close()
	for _, site := range sites {
		rows, err := s.DB().QueryContext(cmd.Context(), trmmClientsHoldingSiteNameSQL, site, site)
		if err != nil {
			return fmt.Errorf("checking which clients have a site named %q: %w", site, err)
		}
		clients := []string{}
		for rows.Next() {
			var c sql.NullString
			if rows.Scan(&c) == nil {
				clients = append(clients, c.String)
			}
		}
		rerr := rows.Err()
		_ = rows.Close()
		if rerr != nil {
			return fmt.Errorf("checking which clients have a site named %q: %w", site, rerr)
		}
		if len(clients) > 1 {
			sort.Strings(clients)
			return usageErr(fmt.Errorf("--filter site=%s names a site in %d clients (%s); site names are unique only within a client, so add client=<name> to pick one", site, len(clients), strings.Join(clients, ", ")))
		}
	}
	return nil
}
