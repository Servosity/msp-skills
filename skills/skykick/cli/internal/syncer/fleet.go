// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored fleet-sync orchestration for skykick-cli (not generator-emitted).
//
// Package syncer drives the fleet-sync fan-out: list every backup
// subscription, then pull seven per-subscription facets (settings, retention,
// autodiscover, snapshot stats, mailboxes, sites, alerts) with bounded
// concurrency into the run-versioned fleet store. Every fetch error is
// preserved per-facet — failed facets never become phantom empty rows, and the
// caller gets an exact fetch_failures accounting.
package syncer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"skykick-pp-cli/internal/client"
	"skykick-pp-cli/internal/cliutil"
	"skykick-pp-cli/internal/fleet"
	"skykick-pp-cli/internal/store"
)

// FetchFailure records one failed API call during a fleet sync.
type FetchFailure struct {
	SubscriptionID string `json:"subscription_id"`
	Facet          string `json:"facet"`
	Error          string `json:"error"`
}

// Options tunes a fleet sync.
type Options struct {
	// Limit caps how many subscriptions are synced (0 = all).
	Limit int
	// Workers bounds subscription-level concurrency.
	Workers int
	// TopAlerts is the $top passed to /Alerts/{id} (max 500 upstream).
	TopAlerts int
	// Skip names facets to skip entirely (e.g. {"alerts": true}).
	Skip map[string]bool
}

// Result summarizes a completed fleet sync.
type Result struct {
	RunID             int64          `json:"run_id"`
	Subscriptions     int            `json:"subscriptions_synced"`
	SnapshotRows      int            `json:"snapshot_rows"`
	Mailboxes         int            `json:"mailboxes"`
	Sites             int            `json:"sites"`
	Alerts            int            `json:"alerts"`
	FetchFailures     []FetchFailure `json:"fetch_failures"`
	DurationSeconds   float64        `json:"duration_seconds"`
	SubscriptionsSeen int            `json:"subscriptions_seen"`
}

// facetNames lists the per-subscription facets in fetch order.
var facetNames = []string{"settings", "retention", "autodiscover", "snapshotstats", "mailboxes", "sites", "alerts"}

// RunFleetSync executes the full fan-out. The store must be writable; the
// fleet schema is ensured here. Rate limiting and 429 retries live inside the
// generated client (adaptive limiter); a subscription whose every facet fails
// still records its subscription row so posture queries can show it.
func RunFleetSync(ctx context.Context, c *client.Client, st *store.Store, opts Options) (*Result, error) {
	start := time.Now()
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.TopAlerts <= 0 || opts.TopAlerts > 500 {
		opts.TopAlerts = 500
	}
	if opts.Skip == nil {
		opts.Skip = map[string]bool{}
	}

	if err := st.EnsureFleetSchema(ctx); err != nil {
		return nil, err
	}

	// Subscription list is the spine: if this fails, the sync fails.
	raw, err := c.Get(ctx, "/Backup", nil)
	if err != nil {
		return nil, fmt.Errorf("listing backup subscriptions: %w", err)
	}
	subs := fleet.ParseSubscriptions(raw)
	seen := len(subs)
	if opts.Limit > 0 && len(subs) > opts.Limit {
		subs = subs[:opts.Limit]
	}

	runID, err := st.BeginFleetRun(ctx)
	if err != nil {
		return nil, err
	}

	res := &Result{RunID: runID, SubscriptionsSeen: seen, FetchFailures: []FetchFailure{}}
	var mu sync.Mutex // guards res counters + store writes' failure slice

	sem := make(chan struct{}, opts.Workers)
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		go func(sub fleet.Subscription) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}

			var (
				settings  *fleet.Settings
				retention *fleet.Retention
				auto      *fleet.Autodiscover
				stats     []fleet.MailboxStat
				mailboxes []fleet.Mailbox
				sites     []fleet.Site
				alerts    []fleet.Alert
				failures  []FetchFailure
			)
			fetch := func(facet, path string, params map[string]string) ([]byte, bool) {
				if opts.Skip[facet] {
					return nil, false
				}
				data, err := c.Get(ctx, path, params)
				if err != nil {
					failures = append(failures, FetchFailure{SubscriptionID: sub.ID, Facet: facet, Error: err.Error()})
					return nil, false
				}
				return data, true
			}

			if data, ok := fetch("settings", "/Backup/"+sub.ID+"/subscriptionsettings", nil); ok {
				s := fleet.ParseSettings(sub.ID, data)
				settings = &s
			}
			if data, ok := fetch("retention", "/Backup/"+sub.ID+"/retentionperiod", nil); ok {
				r := fleet.ParseRetention(sub.ID, data)
				retention = &r
			}
			if data, ok := fetch("autodiscover", "/Backup/"+sub.ID+"/autodiscover", nil); ok {
				a := fleet.ParseAutodiscover(sub.ID, data)
				auto = &a
			}
			if data, ok := fetch("snapshotstats", "/Backup/"+sub.ID+"/lastsnapshotstats", nil); ok {
				stats = fleet.ParseMailboxStats(sub.ID, data)
			}
			if data, ok := fetch("mailboxes", "/Backup/"+sub.ID+"/mailboxes", nil); ok {
				mailboxes = fleet.ParseMailboxes(sub.ID, data)
			}
			if data, ok := fetch("sites", "/Backup/"+sub.ID+"/sites", nil); ok {
				sites = fleet.ParseSites(sub.ID, data)
			}
			if data, ok := fetch("alerts", "/Alerts/"+sub.ID, map[string]string{"$top": fmt.Sprintf("%d", opts.TopAlerts)}); ok {
				alerts = fleet.ParseAlerts(sub.ID, data)
			}

			mu.Lock()
			defer mu.Unlock()
			if err := st.InsertFleetState(ctx, runID, sub, settings, retention, auto, stats, mailboxes, sites, alerts); err != nil {
				failures = append(failures, FetchFailure{SubscriptionID: sub.ID, Facet: "store", Error: err.Error()})
			} else {
				res.Subscriptions++
				res.SnapshotRows += len(stats)
				res.Mailboxes += len(mailboxes)
				res.Sites += len(sites)
				res.Alerts += len(alerts)
			}
			res.FetchFailures = append(res.FetchFailures, failures...)
		}(sub)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := st.FinishFleetRun(ctx, runID, res.Subscriptions, res.SubscriptionsSeen, len(res.FetchFailures)); err != nil {
		return nil, err
	}
	res.DurationSeconds = time.Since(start).Seconds()
	return res, nil
}

// DogfoodCurtailed reports whether fleet-sync should shrink its workload to
// fit the live-dogfood per-command timeout, and the subscription cap to use.
func DogfoodCurtailed(requested int) int {
	if cliutil.IsDogfoodEnv() {
		if requested == 0 || requested > 2 {
			return 2
		}
	}
	return requested
}
