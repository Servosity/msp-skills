// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: PSA/RMM provider-link reconciliation.
// pp:data-source auto

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type linkedAsset struct {
	ComputerID   string `json:"computer_id"`
	ComputerName string `json:"computer_name,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	TenantName   string `json:"tenant_name,omitempty"`
	ProviderLink string `json:"provider_link"`
}

type unlinkedComputer struct {
	ComputerID   string `json:"computer_id"`
	ComputerName string `json:"computer_name,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	TenantName   string `json:"tenant_name,omitempty"`
}

type oneSidedClient struct {
	ProviderLink    string `json:"provider_link"`
	ExternalClient  string `json:"external_client_id"`
	ExternalName    string `json:"external_client_name,omitempty"`
	LinkedToTenant  string `json:"linked_to_tenant_id,omitempty"`
	Status          string `json:"status,omitempty"`
	ReconcileReason string `json:"reason"`
}

type psaReconcileView struct {
	Provider           string             `json:"provider,omitempty"`
	DataSource         string             `json:"data_source"`
	ProviderLinksSeen  int                `json:"provider_links_seen"`
	LocalComputers     int                `json:"local_computers"`
	LinkedComputers    int                `json:"linked_computers"`
	UnlinkedComputers  []unlinkedComputer `json:"unlinked_computers"`
	OrphanedAssets     []linkedAsset      `json:"orphaned_provider_assets"`
	UnmappedClients    []oneSidedClient   `json:"unmapped_provider_clients"`
	TenantsWithNoLink  []string           `json:"tenants_with_no_provider_client"`
	LiveRefreshSkipped string             `json:"live_refresh_skipped,omitempty"`
	Note               string             `json:"note,omitempty"`
}

func newNovelPsaReconcileCmd(flags *rootFlags) *cobra.Command {
	var (
		flagProvider string
		flagLimit    int
		dbPath       string
	)

	cmd := &cobra.Command{
		Use:   "psa-reconcile",
		Short: "Diff the ImmyBot roster against a linked PSA or RMM roster",
		Long: "Diff the ImmyBot roster against a linked PSA or RMM asset roster to find unlinked " +
			"computers, orphaned provider assets, and one-sided tenants.",
		Example: strings.Trim(`
  immybot-cli psa-reconcile
  immybot-cli psa-reconcile --provider 7 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=25",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "psa-reconcile")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			strategy := strings.ToLower(strings.TrimSpace(flags.dataSource))
			if strategy == "" {
				strategy = "auto"
			}
			view := psaReconcileView{
				Provider:          strings.TrimSpace(flagProvider),
				DataSource:        strategy,
				UnlinkedComputers: make([]unlinkedComputer, 0),
				OrphanedAssets:    make([]linkedAsset, 0),
				UnmappedClients:   make([]oneSidedClient, 0),
				TenantsWithNoLink: make([]string, 0),
			}

			dbPath = immyMirrorPath(dbPath)
			if immyMirrorMissing(cmd, dbPath, "provider-links,computers,tenants") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "provider-links") {
				hintIfStale(cmd, db, "provider-links", flags.maxAge)
			}

			// Side A: the provider-link rosters. Drain before touching computers.
			prows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.computers'), json_extract(data,'$.providerClients')
				FROM resources WHERE resource_type = 'provider-links'`)
			if err != nil {
				return fmt.Errorf("querying provider links: %w", err)
			}
			type linkRow struct{ id, name, computers, clients string }
			links := make([]linkRow, 0)
			for prows.Next() {
				var id, name, computers, clients sql.NullString
				if err := prows.Scan(&id, &name, &computers, &clients); err != nil {
					_ = prows.Close()
					return fmt.Errorf("scanning provider link: %w", err)
				}
				links = append(links, linkRow{nullStr(id), nullStr(name), nullStr(computers), nullStr(clients)})
			}
			if err := prows.Err(); err != nil {
				_ = prows.Close()
				return fmt.Errorf("iterating provider links: %w", err)
			}
			if err := prows.Close(); err != nil {
				return fmt.Errorf("closing provider links: %w", err)
			}

			// Side B: the local roster.
			crows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.tenantId'), json_extract(data,'$.tenant')
				FROM resources WHERE resource_type IN ('computers','computers-paged','computers-dx')`)
			if err != nil {
				return fmt.Errorf("querying computers: %w", err)
			}
			localComputers := map[string]unlinkedComputer{}
			for crows.Next() {
				var id, name, tenantID, tenant sql.NullString
				if err := crows.Scan(&id, &name, &tenantID, &tenant); err != nil {
					_ = crows.Close()
					return fmt.Errorf("scanning computer: %w", err)
				}
				if nullStr(id) == "" {
					continue
				}
				localComputers[nullStr(id)] = unlinkedComputer{
					ComputerID: nullStr(id), ComputerName: nullStr(name),
					TenantID: nullStr(tenantID), TenantName: nullStr(tenant),
				}
			}
			if err := crows.Err(); err != nil {
				_ = crows.Close()
				return fmt.Errorf("iterating computers: %w", err)
			}
			if err := crows.Close(); err != nil {
				return fmt.Errorf("closing computers: %w", err)
			}
			view.LocalComputers = len(localComputers)

			trows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name') FROM resources WHERE resource_type = 'tenants'`)
			if err != nil {
				return fmt.Errorf("querying tenants: %w", err)
			}
			tenantNames := map[string]string{}
			for trows.Next() {
				var id, name sql.NullString
				if err := trows.Scan(&id, &name); err != nil {
					_ = trows.Close()
					return fmt.Errorf("scanning tenant: %w", err)
				}
				tenantNames[nullStr(id)] = nullStr(name)
			}
			if err := trows.Err(); err != nil {
				_ = trows.Close()
				return fmt.Errorf("iterating tenants: %w", err)
			}
			if err := trows.Close(); err != nil {
				return fmt.Errorf("closing tenants: %w", err)
			}

			linkedIDs := map[string]struct{}{}
			mappedTenants := map[string]struct{}{}
			provFilter := strings.TrimSpace(flagProvider)

			for _, l := range links {
				if provFilter != "" && l.id != provFilter && !strings.EqualFold(l.name, provFilter) {
					continue
				}
				view.ProviderLinksSeen++
				label := firstNonEmpty(l.name, l.id)

				var assets []map[string]any
				if l.computers != "" {
					_ = json.Unmarshal([]byte(l.computers), &assets)
				}
				for _, a := range assets {
					cid := anyToString(a["id"])
					if cid == "" {
						cid = anyToString(a["deviceId"])
					}
					if cid == "" {
						continue
					}
					linkedIDs[cid] = struct{}{}
					if _, ok := localComputers[cid]; !ok {
						view.OrphanedAssets = append(view.OrphanedAssets, linkedAsset{
							ComputerID:   cid,
							ComputerName: anyToString(a["computerName"]),
							TenantID:     anyToString(a["tenantId"]),
							TenantName:   anyToString(a["tenantName"]),
							ProviderLink: label,
						})
					}
				}

				var clients []map[string]any
				if l.clients != "" {
					_ = json.Unmarshal([]byte(l.clients), &clients)
				}
				for _, c := range clients {
					linked := anyToString(c["linkedToTenantId"])
					if linked != "" {
						mappedTenants[linked] = struct{}{}
						continue
					}
					view.UnmappedClients = append(view.UnmappedClients, oneSidedClient{
						ProviderLink:    label,
						ExternalClient:  anyToString(c["externalClientId"]),
						ExternalName:    anyToString(c["externalClientName"]),
						LinkedToTenant:  linked,
						Status:          anyToString(c["status"]),
						ReconcileReason: "provider client is not linked to an ImmyBot tenant",
					})
				}
			}

			for id, c := range localComputers {
				if _, ok := linkedIDs[id]; !ok {
					view.UnlinkedComputers = append(view.UnlinkedComputers, c)
				}
			}
			view.LinkedComputers = len(linkedIDs)

			for id, name := range tenantNames {
				if _, ok := mappedTenants[id]; !ok {
					view.TenantsWithNoLink = append(view.TenantsWithNoLink, firstNonEmpty(name, id))
				}
			}

			sort.Slice(view.UnlinkedComputers, func(i, j int) bool {
				return view.UnlinkedComputers[i].ComputerName < view.UnlinkedComputers[j].ComputerName
			})
			sort.Slice(view.OrphanedAssets, func(i, j int) bool {
				return view.OrphanedAssets[i].ComputerName < view.OrphanedAssets[j].ComputerName
			})
			sort.Slice(view.UnmappedClients, func(i, j int) bool {
				return view.UnmappedClients[i].ExternalName < view.UnmappedClients[j].ExternalName
			})
			sort.Strings(view.TenantsWithNoLink)
			if flagLimit > 0 {
				view.UnlinkedComputers = capUnlinked(view.UnlinkedComputers, flagLimit)
				view.OrphanedAssets = capAssets(view.OrphanedAssets, flagLimit)
			}

			if view.ProviderLinksSeen == 0 {
				view.Note = "no provider links matched; run 'immybot-cli sync --resources provider-links' or check --provider"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.ProviderLinksSeen == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provider links reconciled: %d\n", view.ProviderLinksSeen)
			fmt.Fprintf(cmd.OutOrStdout(), "  local computers          %d\n", view.LocalComputers)
			fmt.Fprintf(cmd.OutOrStdout(), "  linked to a provider     %d\n", view.LinkedComputers)
			fmt.Fprintf(cmd.OutOrStdout(), "  unlinked computers       %d\n", len(view.UnlinkedComputers))
			fmt.Fprintf(cmd.OutOrStdout(), "  orphaned provider assets %d\n", len(view.OrphanedAssets))
			fmt.Fprintf(cmd.OutOrStdout(), "  unmapped provider clients %d\n", len(view.UnmappedClients))
			fmt.Fprintf(cmd.OutOrStdout(), "  tenants with no provider client %d\n", len(view.TenantsWithNoLink))
			return nil
		},
	}
	cmd.Flags().StringVar(&flagProvider, "provider", "", "Reconcile only this provider link (id or name)")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum rows per discrepancy list (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func capUnlinked(in []unlinkedComputer, n int) []unlinkedComputer {
	if n > 0 && len(in) > n {
		return in[:n]
	}
	return in
}

func capAssets(in []linkedAsset, n int) []linkedAsset {
	if n > 0 && len(in) > n {
		return in[:n]
	}
	return in
}

// anyToString renders a decoded JSON scalar without turning a missing key into
// the string "<nil>".
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
