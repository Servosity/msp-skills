// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: per-cluster connectivity check. doctor reads the PROXMOX_*
// environment only; this walks the clusters the fleet commands actually visit.

// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type clusterCheckRow struct {
	Cluster string `json:"cluster"`
	Tenant  string `json:"tenant"`
	Service string `json:"service"` // config | pve | pbs
	Status  string `json:"status"`  // ok | failed | not-configured
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		// tail polls forever by default (--follow), so as an MCP tool it never
		// returns and its child process outlives the server.
		if c, _, err := root.Find([]string{"tail"}); err == nil && c != root {
			if c.Annotations == nil {
				c.Annotations = map[string]string{}
			}
			c.Annotations["mcp:hidden"] = "true"
		}
		// `--cluster X doctor` would check the environment, not cluster X:
		// answer it with the cluster check instead.
		for _, c := range root.Commands() {
			if c.Name() != "doctor" || c.RunE == nil {
				continue
			}
			inner := c.RunE
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if strings.TrimSpace(selectedCluster) == "" {
					return inner(cmd, args)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "doctor checks the PROXMOX_* environment; cluster %q comes from clusters.toml, so this is 'clusters check' for it\n", selectedCluster)
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "check configured clusters")
				}
				return runClustersCheck(cmd, flags)
			}
		}
	})
}

func newClustersCheckCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Probe every configured cluster's PVE and PBS: reachable, certificate pin accepted, token accepted",
		Long: "Use this command to confirm each clusters.toml entry (or the PROXMOX_* environment when there is no clusters.toml) " +
			"answers with its pinned certificate and accepts its token, for PVE and for PBS separately. " +
			"'--cluster <name> doctor' runs this check for that cluster. Exits 1 when any check fails. " +
			"It does not check privileges; run 'posture' and read fetch_failures for what a token cannot see.",
		Example: strings.Trim(`
  proxmox-cli clusters check
  proxmox-cli --cluster acme clusters check --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "check configured clusters")
			}
			return runClustersCheck(cmd, flags)
		},
	}
}

func runClustersCheck(cmd *cobra.Command, flags *rootFlags) error {
	targets, err := fleetTargets(flags, "")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	rows := make([]clusterCheckRow, 0, 2*len(targets))
	failed := 0
	add := func(t fleetTarget, service, version string, err error) {
		if err == nil && version == "" {
			// A 200 without a version is not Proxmox answering.
			err = fmt.Errorf("answered without a %s version; is this the Proxmox API?", strings.ToUpper(service))
		}
		r := clusterCheckRow{Cluster: t.Name, Tenant: t.Tenant, Service: service, Status: "ok", Version: version}
		if err != nil {
			r.Status, r.Detail = "failed", err.Error()
			failed++
		}
		rows = append(rows, r)
	}
	// ponytail: one cluster at a time; use runFleet if fleets grow past a few dozen.
	for _, t := range targets {
		if !t.usable() {
			add(t, "config", "", t.Err)
			continue
		}
		var v struct {
			Version string `json:"version"`
		}
		if !t.hasPVE() {
			rows = append(rows, clusterCheckRow{Cluster: t.Name, Tenant: t.Tenant, Service: "pve", Status: "not-configured"})
		} else {
			// PVE answers /version only to an authenticated caller.
			err := getJSON(ctx, t.C, "/version", nil, &v)
			add(t, "pve", v.Version, err)
		}
		if !t.C.PBSConfigured() {
			rows = append(rows, clusterCheckRow{Cluster: t.Name, Tenant: t.Tenant, Service: "pbs", Status: "not-configured"})
			continue
		}
		v.Version = ""
		err := getJSON(ctx, t.C, pbsPath("/version"), nil, &v)
		if err == nil {
			// PBS /version is public; /access/permissions proves the token.
			var perms map[string]any
			err = getJSON(ctx, t.C, pbsPath("/access/permissions"), nil, &perms)
		}
		add(t, "pbs", v.Version, err)
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
			return err
		}
	} else {
		table := make([][]string, 0, len(rows))
		for _, r := range rows {
			table = append(table, []string{r.Cluster, r.Service, r.Status, r.Version, r.Detail})
		}
		if err := flags.printTable(cmd, []string{"CLUSTER", "SERVICE", "STATUS", "VERSION", "DETAIL"}, table); err != nil {
			return err
		}
	}
	if failed > 0 {
		return &cliError{code: 1, err: fmt.Errorf("%d cluster check(s) failed", failed)}
	}
	return nil
}
