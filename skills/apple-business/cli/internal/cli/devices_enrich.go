// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices enrich.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"apple-business-pp-cli/internal/client"
	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type enrichCounts struct {
	Fetched      int `json:"fetched"`
	SkippedFresh int `json:"skipped_fresh"`
	Failed       int `json:"failed"`
}

type enrichResult struct {
	Store          string        `json:"store"`
	Devices        int           `json:"devices_considered"`
	AppleCare      *enrichCounts `json:"applecare,omitempty"`
	ActivationLock *enrichCounts `json:"activation_lock,omitempty"`
	Servers        *struct {
		Servers       int `json:"servers"`
		MappedDevices int `json:"mapped_devices"`
	} `json:"servers,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func newNovelDevicesEnrichCmd(flags *rootFlags) *cobra.Command {
	var doCare, doLock, doServers bool
	var skipFresh, dbPath string
	var concurrency, limit int
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Fetch AppleCare, Activation Lock and the serial-to-server map for the whole fleet into the local store",
		Long: `Use this command to fetch per-device AppleCare coverage, Activation Lock status and MDM server mapping into the local store before running reports. Do NOT use this command to refresh the device list itself; use 'sync --resources devices' instead. Do NOT use it for one device; use 'devices lookup' instead.

AppleCare and Activation Lock are per-device endpoints, so this fans out over
every unreleased device in the synced inventory (bounded by --concurrency; the
client paces 429s). The server map costs one paged call per device management
service, not one per device. --skip-fresh makes reruns resume: devices enriched
within that window are skipped. With no kind flags, all three kinds run.`,
		Example: strings.Trim(`
  apple-business-cli devices enrich --applecare --activation-lock --servers --skip-fresh 7d
  apple-business-cli devices enrich --servers --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices enrich")
			}
			if !doCare && !doLock && !doServers {
				doCare, doLock, doServers = true, true, true
			}
			var fresh time.Duration
			if skipFresh != "" {
				d, err := cliutil.ParseDurationLoose(skipFresh)
				if err != nil || d < 0 {
					return usageErr(fmt.Errorf("--skip-fresh must be a duration such as 24h (0 refetches everything)"))
				}
				fresh = d
			}
			if concurrency < 1 {
				concurrency = 1
			}
			if cliutil.IsDogfoodEnv() && (limit <= 0 || limit > 5) {
				limit = 5
			}
			// Each request is bounded by --timeout inside the generated client;
			// a whole-command deadline would cut a fleet-wide fan-out short.
			ctx := cmd.Context()
			// One config for the store, the ownership checks, and every request.
			c, cur, err := frozenClient(flags)
			if err != nil {
				return err
			}
			path := storeDBPath(dbPath)
			res := enrichResult{Store: path, FetchFailures: make([]fetchFailure, 0)}
			if _, err := os.Stat(path); err != nil {
				res.Note = "no local store yet; run 'apple-business-cli sync --resources devices,mdm_servers' first"
				fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: apple-business-cli sync --resources devices,mdm_servers\n", path)
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			db, err := store.OpenWithContext(ctx, path)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			// Never write one customer's data into another customer's store.
			if err := verifyStoreOwner(ctx, db, path, dbPath, cur); err != nil {
				return err
			}
			if stored := readMeta(ctx, db, "client_id"); stored != "" && cur != "" && stored != cur {
				return usageErr(fmt.Errorf("store %s belongs to API account %s but the active account is %s", path, stored, cur))
			}
			for _, ddl := range []string{enrichmentDDL, metaDDL} {
				if _, err := db.DB().ExecContext(ctx, ddl); err != nil {
					return fmt.Errorf("preparing enrichment tables: %w", err)
				}
			}
			// The default store is scoped to the active account, so only it is
			// safe to label without asking.
			if dbPath == "" && cur != "" {
				_ = writeMeta(ctx, db, "client_id", cur)
			}
			hintIfUnsynced(cmd, db, "devices")
			devices, err := loadLocalDevices(ctx, db)
			if err != nil {
				return err
			}
			serials := make([]string, 0, len(devices))
			// Requests and server linkages use the resource ID; results are
			// keyed by serial. Apple documents the two as the same value.
			idOf := map[string]string{}
			for _, d := range devices {
				if !d.released() {
					serials = append(serials, d.serial())
					idOf[d.serial()] = firstNonEmpty(d.ID, d.serial())
				}
			}
			if limit > 0 && len(serials) > limit {
				serials = serials[:limit]
			}
			res.Devices = len(serials)
			c.NoCache = true // fresh data; and no per-device cache files
			serverPages := 0
			if cliutil.IsDogfoodEnv() {
				serverPages = 1
			}

			if doServers {
				servers, raws, err := liveServersRaw(ctx, c)
				if err != nil {
					res.FetchFailures = append(res.FetchFailures, fetchFailure{Kind: "server", Error: "listing device management services: " + err.Error()})
				}
				if len(raws) > 0 {
					if _, _, err := db.UpsertBatch("mdm_servers", raws); err != nil {
						return fmt.Errorf("storing servers: %w", err)
					}
				}
				link := map[string]string{}
				for _, s := range servers {
					ids, err := listAll(ctx, c, "/mdmServers/"+url.PathEscape(s.ID)+"/relationships/devices", map[string]string{"limit": "1000"}, serverPages)
					if err != nil {
						res.FetchFailures = append(res.FetchFailures, fetchFailure{Kind: "server", Error: s.ID + ": " + err.Error()})
						continue
					}
					for _, r := range ids {
						var ref struct {
							ID string `json:"id"`
						}
						if json.Unmarshal(r, &ref) == nil && ref.ID != "" {
							link[ref.ID] = s.ID
						}
					}
				}
				res.Servers = &struct {
					Servers       int `json:"servers"`
					MappedDevices int `json:"mapped_devices"`
				}{Servers: len(servers), MappedDevices: len(link)}
				// Only trust "no server" for devices when every server listed cleanly.
				complete := len(res.FetchFailures) == 0 && serverPages == 0
				for _, s := range serials {
					id, ok := link[idOf[s]]
					if !ok && !complete {
						continue
					}
					b, _ := json.Marshal(map[string]string{"server_id": id})
					if err := saveEnrichment(ctx, db, s, "server", b); err != nil {
						return fmt.Errorf("storing server map: %w", err)
					}
				}
			}

			type job struct{ serial, kind, path string }
			type done struct {
				job
				data json.RawMessage
				err  error
			}
			var jobs []job
			for _, kind := range []struct {
				on     bool
				kind   string
				suffix string
				counts **enrichCounts
			}{{doCare, "applecare", "/appleCareCoverage", &res.AppleCare}, {doLock, "lock", "/activationLockStatus", &res.ActivationLock}} {
				if !kind.on {
					continue
				}
				*kind.counts = &enrichCounts{}
				existing, err := loadEnrichment(ctx, db, kind.kind)
				if err != nil {
					return err
				}
				for _, s := range serials {
					if e, ok := existing[s]; ok && fresh > 0 && time.Since(e.EnrichedAt) < fresh {
						(*kind.counts).SkippedFresh++
						continue
					}
					jobs = append(jobs, job{s, kind.kind, "/orgDevices/" + url.PathEscape(idOf[s]) + kind.suffix})
				}
			}
			// Results are saved as they arrive (single writer: this goroutine),
			// so an interrupted run keeps its progress for --skip-fresh.
			results := make(chan done, 2*max(1, concurrency))
			fanErr := make(chan error, 1)
			go func() {
				fanErr <- fanOut(ctx, flags, c, concurrency, len(jobs), func(wc *client.Client, i int) {
					raw, err := fetchEnrichment(ctx, wc, jobs[i].kind, jobs[i].path)
					results <- done{jobs[i], raw, err}
				})
				close(results)
			}()
			for r := range results {
				counts := res.AppleCare
				if r.kind == "lock" {
					counts = res.ActivationLock
				}
				if r.err == nil {
					r.err = saveEnrichment(ctx, db, r.serial, r.kind, r.data)
				}
				if r.err != nil {
					counts.Failed++
					res.FetchFailures = append(res.FetchFailures, fetchFailure{Serial: r.serial, Kind: r.kind, Error: r.err.Error()})
					continue
				}
				counts.Fetched++
			}
			if err := <-fanErr; err != nil {
				return classifyAPIErrorOnly(err)
			}
			if n := len(res.FetchFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d fetch(es) failed; rerun with --skip-fresh 24h to retry only what is missing\n", n)
			}
			if len(serials) == 0 {
				res.Note = "no unreleased devices in the local store; run 'apple-business-cli sync --resources devices' first"
			}
			var failed error
			if n := len(res.FetchFailures); n > 0 {
				failed = apiErr(fmt.Errorf("%d enrichment fetch(es) failed; reports built on this store will undercount until a rerun succeeds", n))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), res, flags); err != nil {
					return err
				}
				return failed
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "devices considered: %d\n", res.Devices)
			if res.Servers != nil {
				fmt.Fprintf(w, "servers: %d, devices mapped to a server: %d\n", res.Servers.Servers, res.Servers.MappedDevices)
			}
			for _, k := range []struct {
				name   string
				counts *enrichCounts
			}{{"applecare", res.AppleCare}, {"activation lock", res.ActivationLock}} {
				if k.counts != nil {
					fmt.Fprintf(w, "%s: fetched %d, skipped fresh %d, failed %d\n", k.name, k.counts.Fetched, k.counts.SkippedFresh, k.counts.Failed)
				}
			}
			if res.Note != "" {
				fmt.Fprintln(w, res.Note)
			}
			return failed
		},
	}
	cmd.Flags().BoolVar(&doCare, "applecare", false, "Fetch AppleCare coverage for every device")
	cmd.Flags().BoolVar(&doLock, "activation-lock", false, "Fetch Activation Lock status for every device")
	cmd.Flags().BoolVar(&doServers, "servers", false, "Build the serial-to-server map (one paged call per device management service)")
	cmd.Flags().StringVar(&skipFresh, "skip-fresh", "", "Skip devices enriched within this window (e.g. 7d) so reruns resume")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Per-device requests in flight")
	cmd.Flags().IntVar(&limit, "limit", 0, "Enrich at most this many devices (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	return cmd
}

// fetchEnrichment reads one per-device endpoint. AppleCare coverage is paged
// (every page is read, then stored as {"data":[...]}); for Activation Lock it
// keeps only the attributes and treats a response without them as a failure.
func fetchEnrichment(ctx context.Context, c *client.Client, kind, path string) (json.RawMessage, error) {
	if kind == "applecare" {
		items, err := listAll(ctx, c, path, nil, 0)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"data": items})
	}
	raw, err := c.GetNoCache(ctx, path, nil)
	if err != nil || kind != "lock" {
		return raw, err
	}
	var env struct {
		Data struct {
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parsing activation lock: %w", err)
	}
	if len(env.Data.Attributes) == 0 || string(env.Data.Attributes) == "null" {
		return nil, fmt.Errorf("activation lock response had no attributes")
	}
	return env.Data.Attributes, nil
}
