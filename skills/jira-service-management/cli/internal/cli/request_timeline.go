// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type timelineEvent struct {
	At         string `json:"at"`
	EpochMS    int64  `json:"epoch_ms"`
	Kind       string `json:"kind"`
	Visibility string `json:"visibility,omitempty"`
	Author     string `json:"author,omitempty"`
	Text       string `json:"text"`
}

type timelineView struct {
	Key          string          `json:"key"`
	Summary      string          `json:"summary"`
	Status       string          `json:"status"`
	Reporter     string          `json:"reporter,omitempty"`
	Participants []string        `json:"participants"`
	RequestType  string          `json:"request_type,omitempty"`
	SLA          []slaRisk       `json:"sla"`
	Events       []timelineEvent `json:"events"`
	Internal     int             `json:"internal_comments"`
	Public       int             `json:"public_comments"`
	Warnings     []string        `json:"warnings,omitempty"`
}

type jsmRequestDetail struct {
	IssueKey string `json:"issueKey"`
	// JSM has no top-level summary: it is the requestFieldValues entry with
	// fieldId "summary".
	FieldValues []struct {
		FieldID string          `json:"fieldId"`
		Value   json.RawMessage `json:"value"`
	} `json:"requestFieldValues"`
	CreatedDate   jsmDate `json:"createdDate"`
	Reporter      jsmUser `json:"reporter"`
	CurrentStatus struct {
		Status string `json:"status"`
	} `json:"currentStatus"`
	RequestType struct {
		Name string `json:"name"`
	} `json:"requestType"`
}

func newNovelRequestTimelineCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline <issueIdOrKey>",
		Short: "One ticket's whole story in order: public and internal comments, status changes, approvals, SLA state.",
		Long: strings.Trim(`
Merges the request, every comment (labeled PUBLIC or INTERNAL), status history,
approvals, participants and SLA state into one chronological timeline, so an agent
reads internal notes before replying and never mistakes one for a customer reply.`, "\n"),
		Example:     "  jira-service-management-cli request timeline SD-123 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "issueIdOrKey=SD-1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "request timeline")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a request key or id is required, e.g. SD-123"))
			}
			key := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view, err := buildTimeline(ctx, c, key)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// --agent compaction drops row keys most events lack; the
				// comment label and author must survive it.
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "visibility", "author")
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s  %s\nstatus: %s  reporter: %s  comments: %d public / %d internal\n", termSafe(view.Key), termSafe(view.Summary), termSafe(view.Status), termSafe(view.Reporter), view.Public, view.Internal)
			for _, s := range view.SLA {
				fmt.Fprintf(w, "sla: %s remaining %s breached=%t\n", termSafe(s.Metric), termSafe(s.Remaining), s.Breached)
			}
			fmt.Fprintln(w)
			for _, e := range view.Events {
				tag := strings.ToUpper(e.Kind)
				if e.Visibility != "" {
					tag += " " + e.Visibility
				}
				fmt.Fprintf(w, "%s  [%s] %s %s\n", termSafe(e.At), tag, termSafe(e.Author), truncateText(strings.ReplaceAll(e.Text, "\n", " "), 120))
			}
			for _, warn := range view.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", termSafe(warn))
			}
			return nil
		},
	}
	return cmd
}

func buildTimeline(ctx context.Context, c jsmGetter, key string) (timelineView, error) {
	seg, err := jsmPathSeg(key)
	if err != nil {
		return timelineView{}, err
	}
	raw, err := jsmGet(ctx, c, "/request/"+seg, map[string]string{"expand": "requestType"})
	if err != nil {
		return timelineView{}, err
	}
	var req jsmRequestDetail
	if err := json.Unmarshal(raw, &req); err != nil {
		return timelineView{}, fmt.Errorf("decoding request %s: %w", key, err)
	}
	view := timelineView{Key: req.IssueKey, Summary: req.summary(), Status: req.CurrentStatus.Status, Reporter: req.Reporter.DisplayName,
		RequestType: req.RequestType.Name, Participants: make([]string, 0), SLA: make([]slaRisk, 0), Events: make([]timelineEvent, 0)}
	if view.Key == "" {
		view.Key = key
	}
	// Participants and SLA come from their own paged endpoints: the request
	// expansions are PagedDTOs too and would silently cap long lists.
	if parts, _, err := jsmWalk(ctx, c, "/request/"+seg+"/participant", nil, 0, jsonKey("accountId")); err != nil {
		view.Warnings = append(view.Warnings, "participants: "+err.Error())
	} else {
		for _, raw := range parts {
			var u jsmUser
			if json.Unmarshal(raw, &u) == nil {
				view.Participants = append(view.Participants, u.DisplayName)
			}
		}
	}
	if slas, err := fetchSLAs(ctx, c, key); err != nil {
		view.Warnings = append(view.Warnings, "sla: "+err.Error())
	} else {
		for _, s := range slas {
			if r, ok := riskOf(s); ok {
				view.SLA = append(view.SLA, r)
			}
		}
	}
	if req.CreatedDate.EpochMillis > 0 {
		view.Events = append(view.Events, timelineEvent{At: req.CreatedDate.ISO8601, EpochMS: req.CreatedDate.EpochMillis, Kind: "created", Author: req.Reporter.DisplayName, Text: view.Summary})
	}

	comments, _, err := jsmWalk(ctx, c, "/request/"+seg+"/comment", map[string]string{"public": "true", "internal": "true"}, 0, jsonKey("id"))
	if err != nil {
		return view, err
	}
	for _, rc := range comments {
		var cm struct {
			Body    string  `json:"body"`
			Public  bool    `json:"public"`
			Created jsmDate `json:"created"`
			Author  jsmUser `json:"author"`
		}
		if json.Unmarshal(rc, &cm) != nil {
			continue
		}
		vis := "INTERNAL"
		if cm.Public {
			vis = "PUBLIC"
			view.Public++
		} else {
			view.Internal++
		}
		view.Events = append(view.Events, timelineEvent{At: cm.Created.ISO8601, EpochMS: cm.Created.EpochMillis, Kind: "comment", Visibility: vis, Author: cm.Author.DisplayName, Text: cm.Body})
	}

	// Status history and approvals are context; a failure there should not
	// hide the comment thread, so record it and keep going.
	if statuses, _, err := jsmWalk(ctx, c, "/request/"+seg+"/status", nil, 0, nil); err != nil {
		view.Warnings = append(view.Warnings, "status history: "+err.Error())
	} else {
		for _, rs := range statuses {
			var st struct {
				Status     string  `json:"status"`
				StatusDate jsmDate `json:"statusDate"`
			}
			if json.Unmarshal(rs, &st) == nil {
				view.Events = append(view.Events, timelineEvent{At: st.StatusDate.ISO8601, EpochMS: st.StatusDate.EpochMillis, Kind: "status", Text: st.Status})
			}
		}
	}
	if approvals, _, err := jsmWalk(ctx, c, "/request/"+seg+"/approval", nil, 0, jsonKey("id")); err != nil {
		view.Warnings = append(view.Warnings, "approvals: "+err.Error())
	} else {
		for _, ra := range approvals {
			var ap struct {
				Name          string   `json:"name"`
				FinalDecision string   `json:"finalDecision"`
				CreatedDate   jsmDate  `json:"createdDate"`
				CompletedDate *jsmDate `json:"completedDate"`
			}
			if json.Unmarshal(ra, &ap) != nil {
				continue
			}
			view.Events = append(view.Events, timelineEvent{At: ap.CreatedDate.ISO8601, EpochMS: ap.CreatedDate.EpochMillis, Kind: "approval", Text: "requested: " + ap.Name})
			if ap.CompletedDate != nil {
				view.Events = append(view.Events, timelineEvent{At: ap.CompletedDate.ISO8601, EpochMS: ap.CompletedDate.EpochMillis, Kind: "approval", Text: ap.Name + ": " + ap.FinalDecision})
			}
		}
	}
	sort.SliceStable(view.Events, func(i, j int) bool { return view.Events[i].EpochMS < view.Events[j].EpochMS })
	return view, nil
}

func (r jsmRequestDetail) summary() string {
	for _, f := range r.FieldValues {
		var v string
		if f.FieldID == "summary" && json.Unmarshal(f.Value, &v) == nil {
			return v
		}
	}
	return ""
}
