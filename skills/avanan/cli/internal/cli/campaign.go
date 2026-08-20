// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `campaign` — group detections into candidate phishing campaigns.
//
// pp:data-source local
//
// The API returns flat rows and offers no grouping primitive, so clustering
// has to happen locally. The grouping key is deliberately mechanical (exact
// sender domain plus a deterministically normalized subject) because the
// output can drive a bulk quarantine: every decision must be explainable and
// reproducible, which a similarity score would not be.

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
		addNovelCommandIfAbsent(root, newNovelCampaignCmd(flags))
	})
}

type campaignGroup struct {
	SenderDomain    string   `json:"sender_domain"`
	SubjectPattern  string   `json:"subject_pattern"`
	SampleSubject   string   `json:"sample_subject"`
	Messages        int      `json:"messages"`
	Recipients      int      `json:"recipients"`
	Scopes          int      `json:"scopes"`
	Unresolved      int      `json:"unresolved"`
	Senders         []string `json:"senders"`
	FirstSeen       string   `json:"first_seen,omitempty"`
	LastSeen        string   `json:"last_seen,omitempty"`
	DetectionTypes  []string `json:"detection_types"`
	SampleEntityIDs []string `json:"sample_entity_ids"`
}

type campaignReport struct {
	Since         string          `json:"since"`
	Scanned       int             `json:"scanned_events"`
	UndatedEvents int             `json:"undated_events"`
	Campaigns     []campaignGroup `json:"campaigns"`
	Note          string          `json:"note,omitempty"`
}

func newNovelCampaignCmd(flags *rootFlags) *cobra.Command {
	var (
		since   string
		minSize int
		limit   int
		dbPath  string
	)

	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Group detections into candidate phishing campaigns by sender domain and normalized subject",
		Long: strings.Trim(`
Collapse a window of detections into candidate campaigns.

Two messages group together when they share a sender domain AND an identical
subject after reply/forward markers are stripped, digit runs are collapsed,
case is folded, and whitespace is normalized. Grouping is exact after that
normalization — there is no similarity scoring, so every group is
reproducible and explainable.

Reads the local mirror only. Run 'mirror --since 7d' first.

Use this command to find multi-recipient phishing campaigns across synced
events and entities. Do NOT use this command for a single-message
investigation; use 'timeline' instead. Do NOT use it as a general free-text
search; use 'search' instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli campaign --since 7d
  avanan-cli campaign --since 7d --agent
  avanan-cli campaign --since 30d --min-size 5 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--since=7d",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "campaign")
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("campaign has no live equivalent: grouping happens over the local mirror; run 'mirror' then retry without --data-source live"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			window := 7 * 24 * time.Hour
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --since %q: %w (try 24h, 7d, or 4w)", since, err))
				}
				window = d
			}
			cutoff := time.Now().Add(-window)

			if minSize <= 0 {
				minSize = 2
			}
			if limit <= 0 {
				limit = 25
			}

			empty := campaignReport{
				Since:     cutoff.UTC().Format(time.RFC3339),
				Campaigns: []campaignGroup{},
			}

			db, _, ok, err := openMirror(cmd, ctx, flags, dbPath, empty)
			if err != nil || !ok {
				return err
			}
			defer db.Close()

			hintMirrorFreshness(cmd, db, avananmirror.ResourceEvents, flags.maxAge)

			// Drain events first, then entities. Two sequential reads, never
			// one nested inside the other's open result set.
			events, err := loadResources(ctx, db, avananmirror.ResourceEvents)
			if err != nil {
				return err
			}
			entities, err := loadResources(ctx, db, avananmirror.ResourceEntities)
			if err != nil {
				return err
			}

			// Index entities so an event that carries only an entity ID can
			// still contribute a subject and recipient.
			entityByID := make(map[string]map[string]any, len(entities))
			for _, e := range entities {
				if e.Obj == nil {
					continue
				}
				if id := str(e.Obj, entityIDKeys...); id != "" {
					entityByID[id] = e.Obj
				} else if e.ID != "" {
					entityByID[e.ID] = e.Obj
				}
			}

			type agg struct {
				group      campaignGroup
				recipients map[string]bool
				scopes     map[string]bool
				senders    map[string]bool
				types      map[string]bool
				first      time.Time
				last       time.Time
			}
			groups := map[string]*agg{}
			report := empty

			for _, ev := range events {
				if ev.Obj == nil {
					continue
				}
				ts := avananTime(ev.Obj)
				if ts.IsZero() {
					// Counted, not silently dropped. If Avanan names the time
					// field something outside timeKeys for a payload, the
					// alternative is reporting "no campaigns" over a full
					// mirror and blaming the window.
					report.UndatedEvents++
					continue
				}
				if ts.Before(cutoff) {
					continue
				}
				report.Scanned++

				obj := ev.Obj
				entityID := str(obj, entityIDKeys...)
				if detail, ok := entityByID[entityID]; ok {
					// Merge is read-only: prefer the event's own fields and
					// fall back to the entity record for anything missing.
					obj = mergeMissing(obj, detail)
				}

				sender := str(obj, senderKeys...)
				domain := emailDomain(sender)
				subject := str(obj, subjectKeys...)
				pattern := normalizeSubject(subject)
				if domain == "" && pattern == "" {
					// Nothing to group on; counting it as its own campaign
					// would be noise.
					continue
				}

				key := domain + "\x00" + pattern
				a, ok := groups[key]
				if !ok {
					a = &agg{
						group: campaignGroup{
							SenderDomain:    domain,
							SubjectPattern:  pattern,
							SampleSubject:   subject,
							Senders:         []string{},
							DetectionTypes:  []string{},
							SampleEntityIDs: []string{},
						},
						recipients: map[string]bool{},
						scopes:     map[string]bool{},
						senders:    map[string]bool{},
						types:      map[string]bool{},
					}
					groups[key] = a
				}

				a.group.Messages++
				if isUnresolved(str(obj, stateKeys...)) {
					a.group.Unresolved++
				}
				if r := str(obj, recipientKeys...); r != "" {
					a.recipients[strings.ToLower(r)] = true
				}
				if s := str(obj, scopeKeys...); s != "" {
					a.scopes[s] = true
				}
				if sender != "" {
					a.senders[strings.ToLower(sender)] = true
				}
				if t := str(obj, eventTypeKeys...); t != "" {
					a.types[t] = true
				}
				if entityID != "" && len(a.group.SampleEntityIDs) < 5 {
					a.group.SampleEntityIDs = append(a.group.SampleEntityIDs, entityID)
				}
				if a.first.IsZero() || ts.Before(a.first) {
					a.first = ts
				}
				if ts.After(a.last) {
					a.last = ts
				}
			}

			out := make([]campaignGroup, 0, len(groups))
			for _, a := range groups {
				if a.group.Messages < minSize {
					continue
				}
				g := a.group
				g.Recipients = len(a.recipients)
				g.Scopes = len(a.scopes)
				g.Senders = sortedKeys(a.senders)
				g.DetectionTypes = sortedKeys(a.types)
				if !a.first.IsZero() {
					g.FirstSeen = a.first.UTC().Format(time.RFC3339)
				}
				if !a.last.IsZero() {
					g.LastSeen = a.last.UTC().Format(time.RFC3339)
				}
				out = append(out, g)
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].Messages != out[j].Messages {
					return out[i].Messages > out[j].Messages
				}
				return out[i].SenderDomain < out[j].SenderDomain
			})
			if len(out) > limit {
				out = out[:limit]
			}
			report.Campaigns = out

			if len(out) == 0 {
				report.Note = fmt.Sprintf(
					"scanned %d mirrored events since %s (%d more had no parseable timestamp); no sender+subject group reached --min-size %d. Widen with --since or lower --min-size, or run 'avanan-cli mirror' for fresher data.",
					report.Scanned, cutoff.UTC().Format(time.RFC3339), report.UndatedEvents, minSize)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), report.Note)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d candidate campaigns from %d events since %s\n\n",
				len(out), report.Scanned, cutoff.UTC().Format(time.RFC3339))
			for i, g := range out {
				fmt.Fprintf(cmd.OutOrStdout(), "%d. %s  —  %q\n", i+1, orDefault(g.SenderDomain, "(unknown sender)"), g.SampleSubject)
				fmt.Fprintf(cmd.OutOrStdout(), "   %d messages, %d recipients, %d scopes, %d unresolved\n",
					g.Messages, g.Recipients, g.Scopes, g.Unresolved)
				if len(g.DetectionTypes) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "   types: %s\n", strings.Join(g.DetectionTypes, ", "))
				}
				if g.FirstSeen != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "   window: %s → %s\n", g.FirstSeen, g.LastSeen)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Window to cluster: 24h, 7d, 4w (default 7d)")
	cmd.Flags().IntVar(&minSize, "min-size", 2, "Minimum messages before a group counts as a campaign")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum campaigns to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// mergeMissing returns a view of primary with any key it lacks filled in from
// fallback. Neither input is mutated.
func mergeMissing(primary, fallback map[string]any) map[string]any {
	out := make(map[string]any, len(primary)+len(fallback))
	for k, v := range fallback {
		out[k] = v
	}
	for k, v := range primary {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
