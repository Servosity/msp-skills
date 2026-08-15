// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// corkClientDevice is a device Cork attributes to a client. associated_endpoints
// carries the per-connector identity, which is the exact join key against a
// connector's own device list — no name heuristics required.
type corkClientDevice struct {
	UUID                string                   `json:"uuid"`
	Name                string                   `json:"name"`
	DeviceType          string                   `json:"device_type"`
	CanInstallSoftware  bool                     `json:"can_install_software"`
	AssociatedEndpoints []corkAssociatedEndpoint `json:"associated_endpoints"`
}

// corkAssociatedEndpoint is one connector's view of a device. As with
// associated_tenants, `integration` is a nested object rather than an
// identifier string; typing it as a string makes the whole device fail to
// decode, which silently empties the client's device set and turns every
// connector device into a false "coverage gap".
type corkAssociatedEndpoint struct {
	Name                  string `json:"name"`
	IntegrationIdentifier string `json:"integration_identifier"`
	Integration           struct {
		UUID   string `json:"uuid"`
		Vendor struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"vendor"`
	} `json:"integration"`
}

type corkIntegrationDevice struct {
	UUID                  string `json:"uuid"`
	Name                  string `json:"name"`
	IntegrationIdentifier string `json:"integration_identifier"`
	TenantUUID            string `json:"tenant_uuid"`
}

type coverageGapRow struct {
	Device      string   `json:"device"`
	DeviceUUID  string   `json:"device_uuid,omitempty"`
	Identifier  string   `json:"integration_identifier,omitempty"`
	SeenBy      []string `json:"seen_by"`
	MissingFrom []string `json:"missing_from"`
	Gap         string   `json:"gap"`
}

type coverageGapsView struct {
	ClientUUID          string           `json:"client_uuid"`
	Client              string           `json:"client,omitempty"`
	Gaps                []coverageGapRow `json:"gaps"`
	ClientDevices       int              `json:"client_devices"`
	ConnectorsWalked    int              `json:"connectors_walked"`
	ConnectorsSkipped   int              `json:"connectors_skipped"`
	ConnectorDevices    int              `json:"connector_devices"`
	MatchKey            string           `json:"match_key"`
	TenantFilterApplied bool             `json:"tenant_filter_applied"`
	ScanCapHit          bool             `json:"scan_cap_hit"`
	Note                string           `json:"note,omitempty"`
}

// normIdent normalizes a connector identity for comparison. Identifiers are
// compared case-insensitively; names are only ever a fallback.
func normIdent(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var flagClient string
	var flagMaxConnectors int
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Diff connector-reported devices against client-attributed devices to expose coverage blind spots",
		Long: "Diff the devices a connector reports against the devices Cork attributes to a\n" +
			"client, exposing endpoints one tool sees and another is missing.\n\n" +
			"This is the concrete driver behind coverage_impact in the score decomposition,\n" +
			"and no endpoint compares the two sets. Devices are matched on\n" +
			"associated_endpoints[].integration_identifier, an exact per-connector key, so\n" +
			"the diff does not rely on name guessing; devices with no identifier on either\n" +
			"side are reported as unmatched rather than silently assumed covered.\n\n" +
			"Use this command to find device-level coverage blind spots inside a client. Do\n" +
			"NOT use this command to check whether a connector is down or stale; use\n" +
			"'integrations health' instead.",
		Example: "  cork-cli coverage gaps --client 00000000-0000-0000-0000-000000000000 --agent",
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:happy-args":       "--client=00000000-0000-0000-0000-000000000000",
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "coverage gaps")
			}
			if strings.TrimSpace(flagClient) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--client is required (a client uuid)"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// 1. What Cork attributes to this client.
			scanCapHit := false
			rawDevices, _, devCap, err := corkFetchPages(ctx, c, "/clients/"+corkPathSeg(flagClient)+"/devices", nil, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			// A truncated baseline makes every unseen device look like a gap, so
			// the cap must travel with the result rather than being discarded.
			scanCapHit = scanCapHit || devCap
			clientDevices := make([]corkClientDevice, 0, len(rawDevices))
			undecodable := 0
			for _, r := range rawDevices {
				var d corkClientDevice
				if json.Unmarshal(r, &d) != nil {
					undecodable++
					continue
				}
				clientDevices = append(clientDevices, d)
			}
			// A device set that failed to decode is NOT an empty device set.
			// Reporting gaps against a silently-empty baseline would tell the
			// operator every connector device is unmonitored, so refuse instead.
			if undecodable > 0 && len(clientDevices) == 0 {
				return apiErr(fmt.Errorf("could not decode any of the %d device(s) Cork attributes to this client; refusing to report coverage gaps against an empty baseline", undecodable))
			}
			if undecodable > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d client device(s) could not be decoded; gap results may overstate coverage loss\n", undecodable, len(rawDevices))
			}

			// Index the identifiers this client's devices are already known by,
			// per integration.
			type known struct {
				device string
				uuid   string
			}
			byIdent := map[string]known{}
			identsPerIntegration := map[string]map[string]struct{}{}
			for _, d := range clientDevices {
				for _, e := range d.AssociatedEndpoints {
					id := normIdent(e.IntegrationIdentifier)
					if id == "" {
						continue
					}
					byIdent[id] = known{device: d.Name, uuid: d.UUID}
					igKey := e.Integration.UUID
					if igKey == "" {
						igKey = e.Integration.Vendor.Key
					}
					if igKey == "" {
						continue
					}
					if identsPerIntegration[igKey] == nil {
						identsPerIntegration[igKey] = map[string]struct{}{}
					}
					identsPerIntegration[igKey][id] = struct{}{}
				}
			}

			// 2. What each connected integration reports.
			rawIntegrations, _, intCap, err := corkFetchPages(ctx, c, "/integrations/connected", nil, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			scanCapHit = scanCapHit || intCap
			integrations := make([]corkIntegration, 0, len(rawIntegrations))
			for _, r := range rawIntegrations {
				var in corkIntegration
				if json.Unmarshal(r, &in) != nil || in.UUID == "" {
					continue
				}
				integrations = append(integrations, in)
			}
			connectorsSkipped := 0
			if flagMaxConnectors > 0 && len(integrations) > flagMaxConnectors {
				connectorsSkipped = len(integrations) - flagMaxConnectors
				integrations = integrations[:flagMaxConnectors]
				scanCapHit = true
			}

			// Which tenants belong to this client, so a connector's device list
			// can be narrowed to the right tenant.
			// Which tenants belong to this client. Without this mapping a
			// connector's device list spans every tenant of every client, and
			// diffing that against one client's devices reports the entire rest
			// of the book of business as this client's coverage gaps. The
			// mapping is therefore required, not best-effort: prefer the local
			// mirror, fall back to a live roster scan, and refuse if neither
			// resolves the client.
			clientTenants := map[string]struct{}{}
			clientName := ""
			clientFound := false
			absorbClient := func(cl corkClient) {
				if cl.UUID != flagClient {
					return
				}
				clientFound = true
				clientName = cl.Name
				for _, t := range cl.Tenants {
					if t.UUID != "" {
						clientTenants[t.UUID] = struct{}{}
					}
				}
			}
			if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients"); openErr == nil && ok {
				defer db.Close()
				clients, loadErr := corkLoadClients(ctx, db)
				if loadErr != nil {
					return loadErr
				}
				for _, cl := range clients {
					absorbClient(cl)
				}
			}
			if !clientFound {
				rawRoster, _, _, rErr := corkFetchPages(ctx, c, "/clients", nil, flagMaxScanPages)
				if rErr != nil {
					return classifyAPIError(rErr, flags)
				}
				roster, _ := corkDecodeClients(rawRoster)
				for _, cl := range roster {
					absorbClient(cl)
				}
			}
			if !clientFound {
				return notFoundErr(fmt.Errorf("client %s was not found in the local mirror or the first %d page(s) of the live roster; without its tenant list every connector device would be misreported as a coverage gap", flagClient, flagMaxScanPages))
			}
			if len(clientTenants) == 0 {
				return apiErr(fmt.Errorf("client %s has no associated integration tenants, so connector devices cannot be attributed to it; refusing to report coverage gaps that would include other clients' devices", flagClient))
			}

			gaps := make([]coverageGapRow, 0)
			connectorDevices := 0
			walked := 0
			connectorsFailed := 0
			undecodableConnectorDevices := 0
			for _, in := range integrations {
				rawID, _, devCapHit, idErr := corkFetchPages(ctx, c, "/integrations/"+corkPathSeg(in.UUID)+"/devices", nil, flagMaxScanPages)
				if idErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: devices for integration %s failed: %v\n", in.DisplayName, idErr)
					connectorsFailed++
					scanCapHit = true
					continue
				}
				scanCapHit = scanCapHit || devCapHit
				walked++
				vendorLabel := in.Vendor.Name
				if vendorLabel == "" {
					vendorLabel = in.DisplayName
				}
				for _, r := range rawID {
					var d corkIntegrationDevice
					if json.Unmarshal(r, &d) != nil {
						undecodableConnectorDevices++
						continue
					}
					// The tenant set is guaranteed non-empty by the check above.
					// A device with no tenant cannot be attributed to this
					// client, so it is skipped rather than assumed to belong.
					if d.TenantUUID == "" {
						continue
					}
					if _, ok := clientTenants[d.TenantUUID]; !ok {
						continue
					}
					connectorDevices++
					id := normIdent(d.IntegrationIdentifier)
					if id == "" {
						gaps = append(gaps, coverageGapRow{
							Device:      d.Name,
							SeenBy:      []string{vendorLabel},
							MissingFrom: make([]string, 0),
							Gap:         "unmatchable: connector device has no integration_identifier",
							DeviceUUID:  d.UUID,
						})
						continue
					}
					if _, ok := byIdent[id]; ok {
						continue // already attributed to the client
					}
					gaps = append(gaps, coverageGapRow{
						Device:      d.Name,
						DeviceUUID:  d.UUID,
						Identifier:  d.IntegrationIdentifier,
						SeenBy:      []string{vendorLabel},
						MissingFrom: []string{"cork client device inventory"},
						Gap:         "reported by connector, not attributed to this client",
					})
				}
			}

			corkSortStable(gaps, func(a, b coverageGapRow) bool { return a.Device < b.Device })

			if undecodableConnectorDevices > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d connector device(s) could not be decoded and were not diffed\n", undecodableConnectorDevices)
				scanCapHit = true
			}

			view := coverageGapsView{
				ClientUUID:          flagClient,
				Client:              clientName,
				Gaps:                gaps,
				ClientDevices:       len(clientDevices),
				ConnectorsWalked:    walked,
				ConnectorsSkipped:   connectorsSkipped + connectorsFailed,
				ConnectorDevices:    connectorDevices,
				MatchKey:            "associated_endpoints[].integration_identifier",
				TenantFilterApplied: true,
				ScanCapHit:          scanCapHit,
			}

			if len(gaps) == 0 {
				// Never claim a clean result off a truncated or partly-failed
				// sweep; an unqualified "no coverage gaps" is exactly the
				// sentence an operator would act on.
				if scanCapHit {
					view.Note = fmt.Sprintf("no coverage gaps among the %d connector device(s) examined across %d connector(s), but the sweep was truncated (%d connector(s) skipped or failed); this is not a clean bill of health — raise --max-connectors or --max-scan-pages",
						connectorDevices, walked, view.ConnectorsSkipped)
				} else {
					view.Note = fmt.Sprintf("no coverage gaps: %d connector device(s) across %d connector(s) all map to the client's %d attributed device(s)",
						connectorDevices, walked, len(clientDevices))
				}
				if connectorDevices == 0 {
					view.Note = fmt.Sprintf("no connector devices found for this client's tenants across %d connector(s); nothing to diff", walked)
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
						return err
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				if connectorDevices == 0 {
					// Nothing to compare is a distinct, non-failing outcome.
					return notFoundErr(fmt.Errorf("no connector devices to diff for client %s", flagClient))
				}
				return nil
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "DEVICE\tSEEN BY\tIDENTIFIER\tGAP")
			for _, g := range gaps {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
					truncate(g.Device, 30), truncate(strings.Join(g.SeenBy, ","), 20), truncate(g.Identifier, 24), truncate(g.Gap, 46))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d gap(s): %d connector device(s) across %d connector(s) vs %d attributed device(s)\n",
				len(gaps), connectorDevices, walked, len(clientDevices))
			// A truncated sweep must say so even when it DID find gaps. The gap
			// list is a floor, not a total: the skipped connectors were never
			// read, so a device only they can see is missing from this table.
			if scanCapHit {
				// ConnectorsSkipped only counts connectors; a page cap or an
				// undecodable-device warning also truncates the sweep, so do not
				// print "(0 connector(s) skipped)" and read as self-contradictory.
				detail := "a page limit was reached"
				if view.ConnectorsSkipped > 0 {
					detail = fmt.Sprintf("%d connector(s) skipped or failed", view.ConnectorsSkipped)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "the sweep was truncated (%s); this gap list is a floor, not a total - raise --max-connectors or --max-scan-pages\n",
					detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClient, "client", "", "Client uuid to audit (required)")
	cmd.Flags().IntVar(&flagMaxConnectors, "max-connectors", 10, "Maximum connectors to walk before returning partial results")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum device pages to read per source")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to map the client's integration tenants")
	return cmd
}
