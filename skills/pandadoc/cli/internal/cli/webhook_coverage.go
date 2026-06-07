// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// pandadocEventCatalog is the set of webhook event types (triggers) PandaDoc
// documents. Derived from the event tokens present in the official OpenAPI
// spec. webhook-coverage compares active subscriptions against this catalog.
var pandadocEventCatalog = []string{
	"document_state_changed",
	"document_updated",
	"document_deleted",
	"document_completed_pdf_ready",
	"document_creation_failed",
	"document_section_added",
	"recipient_completed",
}

type coverageEvent struct {
	Event         string   `json:"event"`
	Covered       bool     `json:"covered"`
	ByActive      bool     `json:"by_active_subscription"`
	Subscriptions []string `json:"subscriptions,omitempty"`
}

type webhookCoverageReport struct {
	Catalog         []string        `json:"catalog"`
	Subscriptions   int             `json:"subscriptions"`
	ActiveSubs      int             `json:"active_subscriptions"`
	Events          []coverageEvent `json:"events"`
	UncoveredEvents []string        `json:"uncovered_events"`
	Note            string          `json:"note,omitempty"`
}

// newNovelWebhookCoverageCmd implements the "webhook-coverage" transcendence
// command: compares active webhook subscriptions against the documented event
// catalog to surface monitoring gaps.
// pp:data-source local
func newNovelWebhookCoverageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "webhook-coverage",
		Short:       "Compare your active webhook subscriptions against the full event catalog to find gaps.",
		Long:        "Compare the event types your webhook subscriptions listen for against the documented PandaDoc event catalog. Surfaces events nobody is subscribed to and counts inactive subscriptions.\n\nReads the local store — run `sync` first. Joins subscribed triggers against the known catalog, a coverage view the PandaDoc API never reports as a gap list.",
		Example:     "  pandadoc-cli webhook-coverage --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, _, err := openNovelStore(cmd, flags, dbPath, "webhook-subscriptions")
			if err != nil {
				return err
			}
			defer db.Close()
			// The syncer persists under "webhook-subscriptions" (hyphen); older
			// local stores from the 4.19 print used the underscore form. Read
			// the canonical name first and fall back so legacy DBs keep working.
			raws, err := db.List("webhook-subscriptions", 1000000)
			if err != nil {
				return err
			}
			if len(raws) == 0 {
				if legacy, lerr := db.List("webhook_subscriptions", 1000000); lerr == nil && len(legacy) > 0 {
					raws = legacy
				}
			}

			// event -> list of subscription names covering it (active only)
			coverActive := map[string][]string{}
			activeCount := 0
			for _, raw := range raws {
				var m map[string]json.RawMessage
				if err := json.Unmarshal(raw, &m); err != nil {
					continue
				}
				name := jsonStr(m, "name")
				if name == "" {
					name = jsonStr(m, "uuid")
				}
				var active bool
				if aRaw, ok := m["active"]; ok {
					_ = json.Unmarshal(aRaw, &active)
				}
				if active {
					activeCount++
				}
				trigRaw, ok := m["triggers"]
				if !ok {
					continue
				}
				var triggers []string
				if json.Unmarshal(trigRaw, &triggers) != nil {
					continue
				}
				for _, t := range triggers {
					if active {
						coverActive[t] = append(coverActive[t], name)
					}
				}
			}

			report := webhookCoverageReport{
				Catalog:         pandadocEventCatalog,
				Subscriptions:   len(raws),
				ActiveSubs:      activeCount,
				Events:          make([]coverageEvent, 0, len(pandadocEventCatalog)),
				UncoveredEvents: make([]string, 0),
			}
			for _, ev := range pandadocEventCatalog {
				subs := coverActive[ev]
				covered := len(subs) > 0
				report.Events = append(report.Events, coverageEvent{
					Event: ev, Covered: covered, ByActive: covered, Subscriptions: subs,
				})
				if !covered {
					report.UncoveredEvents = append(report.UncoveredEvents, ev)
				}
			}
			sort.Strings(report.UncoveredEvents)
			if len(raws) == 0 {
				report.Note = "no webhook subscriptions in local store — run `pandadoc-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			fmt.Fprintf(w, "Webhook coverage — %d subscriptions (%d active)\n\n", report.Subscriptions, report.ActiveSubs)
			for _, e := range report.Events {
				mark := "MISSING"
				if e.Covered {
					mark = "ok"
				}
				fmt.Fprintf(w, "  %-30s %s\n", e.Event, mark)
			}
			if len(report.UncoveredEvents) > 0 {
				fmt.Fprintf(w, "\n  %d uncovered event(s): %v\n", len(report.UncoveredEvents), report.UncoveredEvents)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
