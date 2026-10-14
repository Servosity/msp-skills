// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: sharing-drift — external-sharing changes across
// client tenants from the Reports audit log (admin + drive applications).

// pp:data-source live
// pp:client-call reports activities.list through listActivities -> listAll -> getWithQuotaRetry -> client.GetNoCache (gws_fanout.go)

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

const activitiesPath = "/admin/reports/v1/activity/users/all/applications/"

type auditActivity struct {
	ID struct {
		Time string `json:"time"`
	} `json:"id"`
	Actor struct {
		Email string `json:"email"`
	} `json:"actor"`
	Events []struct {
		Type       string `json:"type"`
		Name       string `json:"name"`
		Parameters []struct {
			Name       string   `json:"name"`
			Value      string   `json:"value"`
			MultiValue []string `json:"multiValue"`
			BoolValue  *bool    `json:"boolValue"`
		} `json:"parameters"`
	} `json:"events"`
}

func (a auditActivity) param(ev int, name string) string {
	for _, p := range a.Events[ev].Parameters {
		if p.Name == name {
			switch {
			case p.Value != "":
				return p.Value
			case len(p.MultiValue) > 0:
				return strings.Join(p.MultiValue, ",")
			case p.BoolValue != nil:
				return fmt.Sprint(*p.BoolValue)
			}
		}
	}
	return ""
}

type sharingFinding struct {
	Tenant  string   `json:"tenant"`
	Kind    string   `json:"kind"` // setting_change | external_share | public_link
	Time    string   `json:"time,omitempty"`
	Actor   string   `json:"actor,omitempty"`
	Setting string   `json:"setting,omitempty"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	OrgUnit string   `json:"org_unit,omitempty"`
	Count   int      `json:"count,omitempty"`
	Targets []string `json:"targets,omitempty"`
	Files   []string `json:"files,omitempty"`
}

// settingChangeEvents are the admin audit events settingChanges reads (shared
// by sharing-drift and posture).
var settingChangeEvents = []string{"CHANGE_DOCS_SETTING", "CHANGE_CALENDAR_SETTING", "CHANGE_APPLICATION_SETTING", "CREATE_APPLICATION_SETTING", "DELETE_APPLICATION_SETTING"}

// sharingSettingRe keeps CHANGE_APPLICATION_SETTING rows that touch sharing.
var sharingSettingRe = regexp.MustCompile(`(?i)shar|external|outside|publish|visib`)

func settingChanges(tenant string, acts []auditActivity) []sharingFinding {
	out := []sharingFinding{}
	for _, a := range acts {
		for i, ev := range a.Events {
			setting := a.param(i, "SETTING_NAME")
			switch ev.Name {
			case "CHANGE_DOCS_SETTING", "CHANGE_CALENDAR_SETTING":
			case "CHANGE_APPLICATION_SETTING", "CREATE_APPLICATION_SETTING", "DELETE_APPLICATION_SETTING":
				if !sharingSettingRe.MatchString(setting) {
					continue
				}
				if app := a.param(i, "APPLICATION_NAME"); app != "" {
					setting = app + ": " + setting
				}
			default:
				continue
			}
			out = append(out, sharingFinding{
				Tenant: tenant, Kind: "setting_change", Time: a.ID.Time, Actor: a.Actor.Email,
				Setting: setting, From: a.param(i, "OLD_VALUE"), To: a.param(i, "NEW_VALUE"), OrgUnit: a.param(i, "ORG_UNIT_NAME"),
			})
		}
	}
	return out
}

// externalShares aggregates change_user_access(visibility_change=external)
// per actor, with the distinct external targets and a few file titles.
func externalShares(tenant string, acts []auditActivity, sample int) []sharingFinding {
	type agg struct {
		docs    map[string]bool // distinct files (doc_id); re-sharing one file counts once
		last    string
		targets map[string]bool
		files   []string
	}
	by := map[string]*agg{}
	for j, a := range acts {
		for i, ev := range a.Events {
			if ev.Name != "change_user_access" || a.param(i, "visibility_change") != "external" {
				continue
			}
			g := by[a.Actor.Email]
			if g == nil {
				g = &agg{docs: map[string]bool{}, targets: map[string]bool{}}
				by[a.Actor.Email] = g
			}
			doc := a.param(i, "doc_id")
			if doc == "" {
				doc = fmt.Sprintf("event:%d/%d", j, i) // no doc_id: count the event
			}
			newDoc := !g.docs[doc]
			g.docs[doc] = true
			if g.last == "" || laterTS(a.ID.Time, g.last) {
				g.last = a.ID.Time
			}
			if tgt := a.param(i, "target_user"); tgt != "" {
				if _, dom, ok := strings.Cut(tgt, "@"); ok {
					tgt = dom
				}
				g.targets[strings.ToLower(tgt)] = true
			}
			if title := a.param(i, "doc_title"); title != "" && newDoc && len(g.files) < sample {
				g.files = append(g.files, title)
			}
		}
	}
	out := []sharingFinding{}
	for actor, g := range by {
		targets := make([]string, 0, len(g.targets))
		for t := range g.targets {
			targets = append(targets, t)
		}
		sort.Strings(targets)
		out = append(out, sharingFinding{Tenant: tenant, Kind: "external_share", Time: g.last, Actor: actor, Count: len(g.docs), Targets: targets, Files: g.files})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

var publicVisibility = map[string]bool{"people_with_link": true, "public_on_the_web": true}

func publicLinks(tenant string, acts []auditActivity) []sharingFinding {
	out := []sharingFinding{}
	for _, a := range acts {
		for i, ev := range a.Events {
			if ev.Name != "change_document_visibility" {
				continue
			}
			vis := a.param(i, "visibility")
			if !publicVisibility[vis] {
				continue
			}
			out = append(out, sharingFinding{
				Tenant: tenant, Kind: "public_link", Time: a.ID.Time, Actor: a.Actor.Email,
				From: a.param(i, "old_visibility"), To: vis, Files: []string{a.param(i, "doc_title")},
			})
		}
	}
	return out
}

func listActivities(ctx context.Context, c *client.Client, app, event, filters string, start time.Time, max int) ([]auditActivity, error) {
	params := map[string]string{"startTime": start.Format(time.RFC3339), "maxResults": "1000", "eventName": event}
	if filters != "" {
		params["filters"] = filters
	}
	raw, err := listAll(ctx, c, activitiesPath+app, params, "items", max)
	if err != nil {
		return nil, err
	}
	out := make([]auditActivity, 0, len(raw))
	for _, r := range raw {
		var a auditActivity
		if err := json.Unmarshal(r, &a); err != nil {
			return nil, fmt.Errorf("decoding activity: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

func newNovelSharingDriftCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var since string
	var maxEvents int
	var settingsOnly bool
	cmd := &cobra.Command{
		Use:   "sharing-drift",
		Short: "External-sharing changes across client tenants: Drive, Calendar and app-sharing setting changes, new external shares, public links",
		Long: "Reads each tenant's audit log for the --since window and reports three kinds of drift:\n" +
			"  setting_change  any Drive (Docs) or Calendar setting changed in the Admin console\n" +
			"                  (CHANGE_DOCS_SETTING, CHANGE_CALENDAR_SETTING, and sharing-related\n" +
			"                  CHANGE_, CREATE_ and DELETE_APPLICATION_SETTING events), with old ->\n" +
			"                  new and who changed it\n" +
			"  external_share  files that newly became visible outside the domain (Google's\n" +
			"                  visibility_change=external), per user with target domains; extra grants on\n" +
			"                  a file that was already external are not counted\n" +
			"  public_link     files switched to anyone-with-the-link or public on the web\n" +
			"Drive audit data needs Business Standard or higher; for tenants without it the Drive\n" +
			"queries fail and the tenant lands in fetch_failures, so run those with --settings-only.\n" +
			"Use this command for changes to external sharing in a recent window: who changed a Drive or\n" +
			"Calendar setting, and how many files newly went external or public (up to 5 sample\n" +
			"names per user). Do NOT use this command for a tenant's current sharing setting;\n" +
			"use 'posture' instead.",
		Example: "  google-workspace-cli sharing-drift --all-tenants --since 7d\n" +
			"  google-workspace-cli sharing-drift --all-tenants --settings-only --agent --select results.tenant,results.setting,results.from,results.to,results.actor",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "report external-sharing drift per tenant"); done {
				return err
			}
			start, err := parseSince(since, 180)
			if err != nil {
				return err
			}
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]sharingFinding, error) {
				var out []sharingFinding
				// Hand-fix (handfixes.json sharing-drift-reads-created-deleted-settings):
				// settingChanges handles CREATE_/DELETE_APPLICATION_SETTING too,
				// but they were never fetched, so a new or removed sharing policy
				// read as "no findings".
				for _, ev := range settingChangeEvents {
					acts, err := listActivities(ctx, c, "admin", ev, "", start, maxEvents)
					if err != nil {
						return nil, fmt.Errorf("admin audit %s: %w", ev, err)
					}
					out = append(out, settingChanges(t.Name, acts)...)
				}
				if settingsOnly {
					return out, nil
				}
				shares, err := listActivities(ctx, c, "drive", "change_user_access", "visibility_change==external", start, maxEvents)
				if err != nil {
					return nil, fmt.Errorf("drive audit (needs Business Standard or higher; use --settings-only for tenants without it): %w", err)
				}
				out = append(out, externalShares(t.Name, shares, 5)...)
				// filters only apply with an eventName and AND together, so one
				// call per public visibility value.
				for _, vis := range []string{"people_with_link", "public_on_the_web"} {
					acts, err := listActivities(ctx, c, "drive", "change_document_visibility", "visibility=="+vis, start, maxEvents)
					if err != nil {
						return nil, fmt.Errorf("drive audit visibility %s: %w", vis, err)
					}
					out = append(out, publicLinks(t.Name, acts)...)
				}
				return out, nil
			})
			if err != nil {
				return err
			}
			return emitFanout(cmd, flags, res, func(rs []sharingFinding) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					what := r.Setting
					switch r.Kind {
					case "external_share":
						what = fmt.Sprintf("%d file(s) -> %s", r.Count, strings.Join(r.Targets, ","))
					case "public_link":
						what = strings.Join(r.Files, ",") + " -> " + r.To
					case "setting_change":
						what = r.Setting + ": " + r.From + " -> " + r.To
					}
					body = append(body, []string{r.Tenant, r.Kind, r.Actor, r.Time, what})
				}
				return []string{"TENANT", "KIND", "ACTOR", "TIME", "DETAIL"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().StringVar(&since, "since", "30d", "Look-back window, e.g. 7d, 24h, 2w (max 180d)")
	cmd.Flags().IntVar(&maxEvents, "max-events", 5000, "Cap audit events read per query per tenant")
	cmd.Flags().BoolVar(&settingsOnly, "settings-only", false, "Only Admin console Drive, Calendar and app-sharing setting changes (no Drive activity scan)")
	return cmd
}
