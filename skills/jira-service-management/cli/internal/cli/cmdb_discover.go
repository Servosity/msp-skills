// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"jira-service-management-pp-cli/internal/config"
)

var discoverIDRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

type discoverView struct {
	Site        string   `json:"site,omitempty"`
	CloudID     string   `json:"cloud_id"`
	WorkspaceID string   `json:"workspace_id"`
	Exports     []string `json:"exports"`
	Saved       string   `json:"saved_to,omitempty"`
}

func newNovelCmdbDiscoverCmd(flags *rootFlags) *cobra.Command {
	var save bool

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find your site's cloudId and Assets workspaceId so Assets (CMDB) commands work.",
		Long: strings.Trim(`
Assets lives on api.atlassian.com and needs your site's cloudId and Assets
workspaceId in every URL. This reads the cloudId from
https://<site>.atlassian.net/_edge/tenant_info and the workspaceId from the service
desk Assets workspace endpoint, then prints the export lines. --save writes both
into the config file instead, bound to this site: a saved id never applies to
another site, so run it once per client site.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli cmdb discover --agent
  jira-service-management-cli cmdb discover --save`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only-unless-flags": "save"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cmdb discover")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			view := discoverView{Site: cfg.TemplateVars["site"], Exports: make([]string, 0)}

			raw, err := jsmGet(ctx, c, "https://{site}.atlassian.net/_edge/tenant_info", nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var tenant struct {
				CloudID string `json:"cloudId"`
			}
			if err := json.Unmarshal(raw, &tenant); err != nil || tenant.CloudID == "" {
				return fmt.Errorf("tenant_info returned no cloudId (is JIRA_SERVICE_MANAGEMENT_SITE your atlassian.net subdomain?)")
			}
			view.CloudID = tenant.CloudID

			ws, _, err := jsmWalk(ctx, c, "/assets/workspace", nil, 1, nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if len(ws) > 0 {
				view.WorkspaceID = jsonKey("workspaceId")(ws[0])
			}
			if view.WorkspaceID == "" {
				return fmt.Errorf("no Assets workspace found: Assets requires a JSM Premium or Enterprise site, and your account needs Assets access")
			}
			for _, id := range []string{view.CloudID, view.WorkspaceID} {
				if !discoverIDRe.MatchString(id) {
					return fmt.Errorf("unexpected id %q from Atlassian; refusing to print or save it", id)
				}
			}
			view.Exports = append(view.Exports,
				envAssignLine("JIRA_SERVICE_MANAGEMENT_CLOUD_ID", view.CloudID),
				envAssignLine("JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID", view.WorkspaceID))
			if save {
				site := strings.TrimSpace(view.Site)
				if site == "" {
					return fmt.Errorf("set JIRA_SERVICE_MANAGEMENT_SITE before --save: saved Assets ids apply only to the site they were found for")
				}
				if err := cfg.SaveTemplateVars(map[string]string{config.JSMSiteBoundKey("cloudId", site): view.CloudID, config.JSMSiteBoundKey("workspaceId", site): view.WorkspaceID}); err != nil {
					return fmt.Errorf("saving to config: %w; set them with:\n  %s", err, strings.Join(view.Exports, "\n  "))
				}
				view.Saved = cfg.Path
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "cloudId:     %s\nworkspaceId: %s\n\n", view.CloudID, view.WorkspaceID)
			for _, e := range view.Exports {
				fmt.Fprintln(w, e)
			}
			if view.Saved != "" {
				fmt.Fprintf(w, "\nsaved to %s\n", view.Saved)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "Write cloudId and workspaceId into the config file")
	return cmd
}

// envAssignLine prints a variable assignment in the current OS's shell syntax
// (PowerShell on Windows, POSIX sh elsewhere). The id is already checked
// against discoverIDRe, so it needs no quoting beyond PowerShell's string.
func envAssignLine(name, value string) string {
	if runtime.GOOS == "windows" {
		return "$env:" + name + " = \"" + value + "\""
	}
	return "export " + name + "=" + value
}
