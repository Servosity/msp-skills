// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: Broadcom per-core licence exposure.
// pp:data-source live
//
// Counting rule: Broadcom KB 313548 ("Counting Cores for VMware Cloud
// Foundation and vSphere Foundation"): license at least 16 physical cores for
// each CPU, even when a CPU has fewer.

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/cliutil"
)

type licenceHostRow struct {
	Host            string `json:"host"`
	Name            string `json:"name"`
	Cluster         string `json:"cluster_name,omitempty"`
	ConnectionState string `json:"connection_state"`
	Sockets         int64  `json:"sockets"`
	Cores           int64  `json:"cores"`
	CoresPerCPU     int64  `json:"cores_per_cpu"`
	LicensableCores int64  `json:"licensable_cores"`
	FloorUplift     int64  `json:"floor_uplift_cores"`
	ESXiVersion     string `json:"esxi_version,omitempty"`
	ESXiBuild       string `json:"esxi_build,omitempty"`
	CPUModel        string `json:"cpu_model,omitempty"`
}

// licenceTotals covers one cluster (or the vCenter). hosts is every host in
// scope; hosts_counted is how many had readable CPU counts. The sums cover only
// counted hosts and are null when none was counted; estimated_cost is given
// only when every host in scope was counted (a partial cost would understate).
type licenceTotals struct {
	Scope           string   `json:"scope"`
	Cluster         string   `json:"cluster,omitempty"`
	Hosts           int      `json:"hosts"`
	HostsCounted    int      `json:"hosts_counted"`
	Sockets         *int64   `json:"sockets"`
	Cores           *int64   `json:"cores"`
	LicensableCores *int64   `json:"licensable_cores"`
	EstimatedCost   *float64 `json:"estimated_cost,omitempty"`
}

type licAcc struct {
	scope, cluster             string
	hosts, counted             int
	sockets, cores, licensable int64
}

func (a *licAcc) totals(price float64) licenceTotals {
	t := licenceTotals{Scope: a.scope, Cluster: a.cluster, Hosts: a.hosts, HostsCounted: a.counted}
	if a.counted > 0 || a.hosts == 0 {
		s, c, l := a.sockets, a.cores, a.licensable
		t.Sockets, t.Cores, t.LicensableCores = &s, &c, &l
	}
	if price > 0 && a.counted == a.hosts && a.hosts > 0 {
		cost := float64(a.licensable) * price
		t.EstimatedCost = &cost
	}
	return t
}

type licenceEntitlement struct {
	Name      string `json:"name"`
	Edition   string `json:"edition_key"`
	KeySuffix string `json:"key_masked"`
	CostUnit  string `json:"cost_unit"`
	Total     int64  `json:"total"`
	Used      *int64 `json:"used"` // optional in the vendor schema: null when not reported
}

type licenceExposureView struct {
	Rule          string               `json:"rule"`
	MinCoresCPU   int64                `json:"min_cores_per_cpu"`
	PricePerCore  float64              `json:"price_per_core,omitempty"`
	VIJSON        viStatus             `json:"vi_json"`
	Hosts         []licenceHostRow     `json:"hosts"`
	Clusters      []licenceTotals      `json:"clusters"`
	Total         licenceTotals        `json:"total"`
	Licences      []licenceEntitlement `json:"licences"`
	FetchFailures []fetchFailure       `json:"fetch_failures"`
}

func newNovelLicenceExposureCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	var price float64
	var minPerCPU int64
	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Licensable cores per host, cluster and vCenter under the 16-cores-per-CPU minimum, with licences masked",
		Long: `Use this command for Broadcom per-core subscription exposure (sockets x cores with the 16-core-per-CPU floor) and current licence editions and usage with keys masked.
Do NOT use this command for target hardware sizing; use 'migration capacity' instead.

Per host: sockets x max(--min-cores-per-cpu, cores per CPU), from the VI/JSON
host hardware summary (Broadcom KB 313548). Hosts whose hardware cannot be read
are listed in fetch_failures and left out of every total. --price-per-core is
your own per-core price; the result is an estimate in that unit, not a quote.
Needs vCenter 8.0 Update 1 or later (VI/JSON API).`,
		Example: strings.Trim(`
  vsphere-cli licence exposure --agent
  vsphere-cli licence exposure --price-per-core 350 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "licence exposure")
			}
			if price < 0 || minPerCPU < 0 {
				return usageErr(fmt.Errorf("--price-per-core and --min-cores-per-cpu must be zero or more"))
			}
			if err := sf.validate(); err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := vcenterClient(cmd, flags)
			if err != nil {
				return err
			}
			vi := probeVI(ctx, c)
			if !vi.Available {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("licence exposure needs the VI/JSON API for host CPU counts: %s", vi.Reason), flags)
			}
			inv, err := walkInventory(ctx, c, sf.scope, sf.concurrency, false)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := licenceExposureView{Rule: "Broadcom KB 313548: license at least 16 physical cores per CPU (override with --min-cores-per-cpu)",
				MinCoresCPU: minPerCPU, PricePerCore: price, VIJSON: vi, Hosts: make([]licenceHostRow, 0),
				Clusters: make([]licenceTotals, 0), Licences: make([]licenceEntitlement, 0), FetchFailures: nonNilFailures(inv.Failures)}

			res, errs := cliutil.FanoutRun(ctx, inv.Hosts, func(h invHost) string { return h.ID },
				func(ctx context.Context, h invHost) (hostSummary, error) {
					var s hostSummary
					err := getJSON(ctx, c, viRoot+"/HostSystem/"+esc(h.ID)+"/summary", nil, &s)
					return s, err
				}, cliutil.WithConcurrency(sf.concurrency))
			for _, e := range errs {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: e.Source, Path: viRoot + "/HostSystem/" + e.Source + "/summary", Error: errText(e.Err)})
			}
			summaries := map[string]hostSummary{}
			for _, r := range res {
				summaries[r.Source] = r.Value
			}
			byCluster := map[string]*licAcc{}
			total := &licAcc{scope: "vcenter"}
			for _, h := range inv.Hosts {
				// Keyed by cluster id: two clusters may share a name.
				key, label := h.Cluster, h.ClusterName
				if key == "" {
					label = "(standalone hosts)"
				}
				acc := byCluster[key]
				if acc == nil {
					acc = &licAcc{scope: label, cluster: key}
					byCluster[key] = acc
				}
				acc.hosts++
				total.hosts++
				s, ok := summaries[h.ID]
				if !ok {
					continue // listed in fetch_failures
				}
				hw := s.Hardware
				if hw == nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: h.ID, Path: viRoot + "/HostSystem/" + h.ID + "/summary", Error: "host summary has no hardware section (host disconnected?)"})
					continue
				}
				lic, valid := licensableCores(hw.NumCPUPkgs, hw.NumCPUCores, minPerCPU)
				if !valid {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: h.ID, Path: viRoot + "/HostSystem/" + h.ID + "/summary", Error: "host summary has no CPU package/core counts (host disconnected?)"})
					continue
				}
				row := licenceHostRow{Host: h.ID, Name: h.Name, Cluster: h.ClusterName, ConnectionState: h.ConnectionState,
					Sockets: hw.NumCPUPkgs, Cores: hw.NumCPUCores, CoresPerCPU: (hw.NumCPUCores + hw.NumCPUPkgs - 1) / hw.NumCPUPkgs,
					LicensableCores: lic, FloorUplift: lic - hw.NumCPUCores, ESXiVersion: s.Config.Product.Version,
					ESXiBuild: s.Config.Product.Build, CPUModel: hw.CPUModel}
				view.Hosts = append(view.Hosts, row)
				for _, a := range []*licAcc{acc, total} {
					a.counted++
					a.sockets += row.Sockets
					a.cores += row.Cores
					a.licensable += row.LicensableCores
				}
			}
			for _, k := range sortedKeys(byCluster) {
				view.Clusters = append(view.Clusters, byCluster[k].totals(price))
			}
			view.Total = total.totals(price)

			var licences []struct {
				LicenseKey string `json:"licenseKey"`
				EditionKey string `json:"editionKey"`
				Name       string `json:"name"`
				Total      int64  `json:"total"`
				Used       *int64 `json:"used"`
				CostUnit   string `json:"costUnit"`
			}
			licPath := viRoot + "/LicenseManager/" + esc(vi.LicenseManager) + "/licenses"
			if err := getJSON(ctx, c, licPath, nil, &licences); err != nil {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "licences", Path: licPath, Error: errText(err)})
			}
			for _, l := range licences {
				view.Licences = append(view.Licences, licenceEntitlement{Name: l.Name, Edition: l.EditionKey,
					KeySuffix: maskLicenceKey(l.LicenseKey), CostUnit: l.CostUnit, Total: l.Total, Used: l.Used})
			}
			sort.Slice(view.Hosts, func(i, j int) bool { return view.Hosts[i].Name < view.Hosts[j].Name })
			warnFailures(cmd.ErrOrStderr(), "host", view.FetchFailures, len(inv.Hosts))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			rows := make([]map[string]any, 0, len(view.Hosts))
			for _, h := range view.Hosts {
				rows = append(rows, map[string]any{"host": h.Name, "cluster": h.Cluster, "sockets": h.Sockets, "cores": h.Cores, "licensable": h.LicensableCores, "esxi": h.ESXiVersion})
			}
			if len(rows) > 0 {
				if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
					return err
				}
			}
			if view.Total.LicensableCores == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "\nno host CPU counts could be read (%d hosts in scope); see fetch_failures\n", view.Total.Hosts)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d of %d hosts counted: %d sockets, %d physical cores -> %d licensable cores (min %d per CPU)\n",
				view.Total.HostsCounted, view.Total.Hosts, *view.Total.Sockets, *view.Total.Cores, *view.Total.LicensableCores, minPerCPU)
			if view.Total.EstimatedCost != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "estimate at %.2f per core: %.2f\n", price, *view.Total.EstimatedCost)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().Float64Var(&price, "price-per-core", 0, "Your per-core price; adds an estimated_cost in that unit")
	cmd.Flags().Int64Var(&minPerCPU, "min-cores-per-cpu", 16, "Minimum cores counted per CPU (Broadcom KB 313548 says 16)")
	return cmd
}
