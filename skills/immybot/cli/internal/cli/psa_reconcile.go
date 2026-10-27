// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: PSA/RMM provider-link reconciliation.
// pp:data-source local

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// psaReconcileView is the reconcile result. LocalComputers counts the distinct
// computers in the mirrored roster (immyComputerRosterTypes).
// UnlinkedComputerCount and OrphanedAssetCount are the totals before --limit
// caps their lists; UnlinkedComputerCount is null, like linked_computers is 0,
// when a matched link carries no computer roster and the split was not computed.
type psaReconcileView struct {
	Provider              string             `json:"provider,omitempty"`
	DataSource            string             `json:"data_source"`
	ProviderLinksSeen     int                `json:"provider_links_seen"`
	LocalComputers        int                `json:"local_computers"`
	LinkedComputers       int                `json:"linked_computers"`
	UnlinkedComputerCount *int               `json:"unlinked_computer_count"`
	UnlinkedComputers     []unlinkedComputer `json:"unlinked_computers"`
	OrphanedAssetCount    int                `json:"orphaned_provider_asset_count"`
	OrphanedAssets        []linkedAsset      `json:"orphaned_provider_assets"`
	UnmappedClients       []oneSidedClient   `json:"unmapped_provider_clients"`
	TenantsWithNoLink     []string           `json:"tenants_with_no_provider_client"`
	LiveRefreshSkipped    string             `json:"live_refresh_skipped,omitempty"`
	Note                  string             `json:"note,omitempty"`
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

			// The reconcile reads the local mirror only; it has no live path,
			// so it never reports another data source.
			view := psaReconcileView{
				Provider:          strings.TrimSpace(flagProvider),
				DataSource:        "local",
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
			db, err := immyOpenMirror(ctx, dbPath)
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

			// Side B: the local roster, each resource read with its own
			// schema's field names (immyComputerShapes).
			roster, err := immyLoadComputers(ctx, db, immyComputerRosterTypes)
			if err != nil {
				return err
			}
			localComputers := map[string]unlinkedComputer{}
			for _, c := range roster.list() {
				localComputers[c.ID] = unlinkedComputer{
					ComputerID: c.ID, ComputerName: c.Name,
					TenantID: c.TenantID, TenantName: c.TenantName,
				}
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
			// A discrepancy is only reported against a roster that was read:
			// a matched link whose computers (or providerClients) are null
			// says nothing about which local computers (or tenants) it links.
			// Every matched link must carry the roster: a computer that a
			// roster-less link names is indistinguishable from an unlinked one.
			var noComputerRoster, noClientRoster []string

			for _, l := range links {
				if provFilter != "" && l.id != provFilter && !strings.EqualFold(l.name, provFilter) {
					continue
				}
				view.ProviderLinksSeen++
				label := firstNonEmpty(l.name, l.id)

				var assets []map[string]any
				if l.computers == "" || json.Unmarshal([]byte(l.computers), &assets) != nil || assets == nil {
					noComputerRoster = append(noComputerRoster, label)
				}
				for _, a := range assets {
					asset := immyComputerFromObject(immyProviderLinkComputerFields, a)
					cid := asset.ID
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
							ComputerName: asset.Name,
							TenantID:     asset.TenantID,
							TenantName:   asset.TenantName,
							ProviderLink: label,
						})
					}
				}

				var clients []map[string]any
				if l.clients == "" || json.Unmarshal([]byte(l.clients), &clients) != nil || clients == nil {
					noClientRoster = append(noClientRoster, label)
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

			// linked_computers counts LOCAL computers a matched link names, so
			// linked + unlinked always equals local_computers; a linked id
			// with no local computer is an orphaned asset, never "linked".
			if view.ProviderLinksSeen > 0 && len(noComputerRoster) == 0 {
				for id, c := range localComputers {
					if _, ok := linkedIDs[id]; ok {
						view.LinkedComputers++
						continue
					}
					view.UnlinkedComputers = append(view.UnlinkedComputers, c)
				}
				n := len(view.UnlinkedComputers)
				view.UnlinkedComputerCount = &n
			}
			view.OrphanedAssetCount = len(view.OrphanedAssets)
			tenantsComputed := view.ProviderLinksSeen > 0 && len(noClientRoster) == 0
			if tenantsComputed {
				for id, name := range tenantNames {
					if _, ok := mappedTenants[id]; !ok {
						view.TenantsWithNoLink = append(view.TenantsWithNoLink, firstNonEmpty(name, id))
					}
				}
			}

			sort.Slice(view.UnlinkedComputers, func(i, j int) bool {
				a, b := view.UnlinkedComputers[i], view.UnlinkedComputers[j]
				if a.ComputerName != b.ComputerName {
					return a.ComputerName < b.ComputerName
				}
				return a.ComputerID < b.ComputerID
			})
			sort.Slice(view.OrphanedAssets, func(i, j int) bool {
				a, b := view.OrphanedAssets[i], view.OrphanedAssets[j]
				if a.ComputerName != b.ComputerName {
					return a.ComputerName < b.ComputerName
				}
				if a.ComputerID != b.ComputerID {
					return a.ComputerID < b.ComputerID
				}
				return a.ProviderLink < b.ProviderLink
			})
			sort.Slice(view.UnmappedClients, func(i, j int) bool {
				return view.UnmappedClients[i].ExternalName < view.UnmappedClients[j].ExternalName
			})
			sort.Strings(view.TenantsWithNoLink)
			if flagLimit > 0 {
				view.UnlinkedComputers = capUnlinked(view.UnlinkedComputers, flagLimit)
				view.OrphanedAssets = capAssets(view.OrphanedAssets, flagLimit)
			}

			notes := make([]string, 0, 2)
			switch {
			case view.ProviderLinksSeen == 0:
				notes = append(notes, "no provider links matched, so nothing was reconciled; run 'immybot-cli sync --resources provider-links' or check --provider")
			default:
				if len(noComputerRoster) > 0 {
					sort.Strings(noComputerRoster)
					notes = append(notes, fmt.Sprintf("unlinked computers were not computed: matched provider link(s) %s carry no computer roster "+
						"(pass --provider to reconcile one link that does)", strings.Join(noComputerRoster, ", ")))
				}
				if len(noClientRoster) > 0 {
					sort.Strings(noClientRoster)
					notes = append(notes, fmt.Sprintf("tenants with no provider client were not computed: matched provider link(s) %s carry no provider-client roster",
						strings.Join(noClientRoster, ", ")))
				}
			}
			view.Note = strings.Join(notes, "; ")

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.ProviderLinksSeen == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			// The totals, not the --limit-capped list lengths; a split the
			// rosters could not support says so instead of printing 0.
			linked, unlinked := "not computed", "not computed"
			if view.UnlinkedComputerCount != nil {
				linked, unlinked = fmt.Sprint(view.LinkedComputers), fmt.Sprint(*view.UnlinkedComputerCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provider links reconciled: %d\n", view.ProviderLinksSeen)
			fmt.Fprintf(cmd.OutOrStdout(), "  local computers          %d\n", view.LocalComputers)
			fmt.Fprintf(cmd.OutOrStdout(), "  linked to a provider     %s\n", linked)
			fmt.Fprintf(cmd.OutOrStdout(), "  unlinked computers       %s\n", unlinked)
			fmt.Fprintf(cmd.OutOrStdout(), "  orphaned provider assets %d\n", view.OrphanedAssetCount)
			fmt.Fprintf(cmd.OutOrStdout(), "  unmapped provider clients %d\n", len(view.UnmappedClients))
			tenants := "not computed"
			if tenantsComputed {
				tenants = fmt.Sprint(len(view.TenantsWithNoLink))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  tenants with no provider client %s\n", tenants)
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
