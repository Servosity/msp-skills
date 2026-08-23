// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// `triage` — the shift-start digest.
//
// pp:data-source local
//
// Avanan has no "what changed since I last looked" endpoint, and its scrollId
// cursor is explicitly not durable across runs. Reading the local mirror is
// the only way to answer the first question an analyst asks every shift.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"avanan-pp-cli/internal/avananmirror"
	"avanan-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelTriageCmd(flags))
	})
}

type triageBucket struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type triageSender struct {
	Domain     string `json:"domain"`
	Count      int    `json:"count"`
	Unresolved int    `json:"unresolved"`
}

type triageReport struct {
	Since         string         `json:"since"`
	TotalEvents   int            `json:"total_events"`
	Unresolved    int            `json:"unresolved"`
	UndatedEvents int            `json:"undated_events"`
	ByType        []triageBucket `json:"by_type"`
	ByState       []triageBucket `json:"by_state"`
	BySeverity    []triageBucket `json:"by_severity"`
	BySaaS        []triageBucket `json:"by_saas"`
	TopSenders    []triageSender `json:"top_sender_domains"`
	Note          string         `json:"note,omitempty"`
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var (
		since  string
		top    int
		dbPath string
	)

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "See everything detected in a window you choose, bucketed by threat type, severity, and state",
		Long: strings.Trim(`
The shift-start view: every detection in the local mirror inside a time
window, bucketed by detection type, state, severity, and SaaS platform, with
the sender domains driving the volume.

Reads the local mirror only. Run 'mirror --since 7d' first, and again whenever
you want fresher numbers.

Use this command for "what is new since my last shift / since yesterday"
triage over mirrored events. The window is whatever you pass to --since; the
command keeps no cursor of its own. Do NOT use this command to run a fresh
filtered query against the live API; use 'event query' instead. Do NOT use
this command to group detections by sender into campaigns; use 'campaign'
instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli triage --since 24h
  avanan-cli triage --since 12h --agent
  avanan-cli triage --since 7d --top 20 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--since=24h",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "triage")
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("triage has no live equivalent: it compares against the local mirror; run 'mirror' then retry without --data-source live"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			window := 24 * time.Hour
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --since %q: %w (try 24h, 7d, or 4w)", since, err))
				}
				window = d
			}
			cutoff := time.Now().Add(-window)

			empty := triageReport{
				Since:      cutoff.UTC().Format(time.RFC3339),
				ByType:     []triageBucket{},
				ByState:    []triageBucket{},
				BySeverity: []triageBucket{},
				BySaaS:     []triageBucket{},
				TopSenders: []triageSender{},
			}

			db, resolvedDB, ok, err := openMirror(cmd, ctx, flags, dbPath, empty)
			if err != nil || !ok {
				return err
			}
			defer db.Close()

			hintMirrorFreshness(cmd, db, avananmirror.ResourceEvents, flags.maxAge)

			events, err := loadResources(ctx, db, avananmirror.ResourceEvents)
			if err != nil {
				return err
			}

			report := empty
			byType := map[string]int{}
			byState := map[string]int{}
			bySeverity := map[string]int{}
			bySaaS := map[string]int{}
			senders := map[string]*triageSender{}

			for _, ev := range events {
				if ev.Obj == nil {
					continue
				}
				ts := avananTime(ev.Obj)
				if ts.IsZero() {
					// Count separately rather than silently including or
					// excluding: an undated record is a data-quality signal,
					// not a member of the window.
					report.UndatedEvents++
					continue
				}
				if ts.Before(cutoff) {
					continue
				}

				report.TotalEvents++
				state := str(ev.Obj, stateKeys...)
				unresolved := isUnresolved(state)
				if unresolved {
					report.Unresolved++
				}

				bump(byType, str(ev.Obj, eventTypeKeys...))
				bump(byState, state)
				bump(bySeverity, str(ev.Obj, severityKeys...))
				bump(bySaaS, str(ev.Obj, saasKeys...))

				if domain := emailDomain(str(ev.Obj, senderKeys...)); domain != "" {
					s, ok := senders[domain]
					if !ok {
						s = &triageSender{Domain: domain}
						senders[domain] = s
					}
					s.Count++
					if unresolved {
						s.Unresolved++
					}
				}
			}

			report.ByType = topBuckets(byType, 0)
			report.ByState = topBuckets(byState, 0)
			report.BySeverity = topBuckets(bySeverity, 0)
			report.BySaaS = topBuckets(bySaaS, 0)

			if top <= 0 {
				top = 10
			}
			senderList := make([]triageSender, 0, len(senders))
			for _, s := range senders {
				senderList = append(senderList, *s)
			}
			sort.Slice(senderList, func(i, j int) bool {
				if senderList[i].Count != senderList[j].Count {
					return senderList[i].Count > senderList[j].Count
				}
				return senderList[i].Domain < senderList[j].Domain
			})
			if len(senderList) > top {
				senderList = senderList[:top]
			}
			report.TopSenders = senderList

			if report.TotalEvents == 0 {
				report.Note = fmt.Sprintf(
					"no events in the local mirror newer than %s; run 'avanan-cli mirror --since %s' to populate it",
					cutoff.UTC().Format(time.RFC3339), strings.TrimSpace(orDefault(since, "24h")))
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			if report.TotalEvents == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), report.Note)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Detections since %s  (mirror: %s)\n",
				cutoff.UTC().Format(time.RFC3339), resolvedDB)
			fmt.Fprintf(cmd.OutOrStdout(), "%d events, %d still unresolved\n", report.TotalEvents, report.Unresolved)
			if report.UndatedEvents > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "(%d mirrored events carry no parseable timestamp and are excluded)\n", report.UndatedEvents)
			}

			printBuckets(cmd, "BY TYPE", report.ByType)
			printBuckets(cmd, "BY STATE", report.ByState)
			printBuckets(cmd, "BY SEVERITY", report.BySeverity)
			printBuckets(cmd, "BY PLATFORM", report.BySaaS)

			if len(report.TopSenders) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nTOP SENDER DOMAINS\n")
				for _, s := range report.TopSenders {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s %5d  (%d unresolved)\n", s.Domain, s.Count, s.Unresolved)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Window to summarize: 24h, 7d, 4w (default 24h)")
	cmd.Flags().IntVar(&top, "top", 10, "How many sender domains to list")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func bump(m map[string]int, key string) {
	if strings.TrimSpace(key) == "" {
		key = "unknown"
	}
	m[key]++
}

func topBuckets(m map[string]int, limit int) []triageBucket {
	out := make([]triageBucket, 0, len(m))
	for k, v := range m {
		out = append(out, triageBucket{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func printBuckets(cmd *cobra.Command, title string, buckets []triageBucket) {
	if len(buckets) == 0 {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", title)
	for _, b := range buckets {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %5d\n", b.Name, b.Count)
	}
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
