// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices lookup.
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"apple-business-pp-cli/internal/client"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type lookupRow struct {
	Query              string        `json:"query"`
	MatchedBy          string        `json:"matched_by"`
	Serial             string        `json:"serial"`
	Model              string        `json:"model,omitempty"`
	Family             string        `json:"family,omitempty"`
	Status             string        `json:"status,omitempty"`
	OrderNumber        string        `json:"order_number,omitempty"`
	PurchaseSourceType string        `json:"purchase_source_type,omitempty"`
	AddedToOrg         string        `json:"added_to_org,omitempty"`
	Released           bool          `json:"released"`
	AssignedServerID   string        `json:"assigned_server_id"`
	AssignedServerName string        `json:"assigned_server_name,omitempty"`
	AppleCare          *coverageView `json:"applecare,omitempty"`
	ActivationLock     *lockView     `json:"activation_lock,omitempty"`
	MigrationStatus    string        `json:"mdm_migration_status,omitempty"`
	MigrationDeadline  string        `json:"mdm_migration_deadline,omitempty"`
	Source             string        `json:"source"`
	Note               string        `json:"note,omitempty"`
	Errors             []string      `json:"errors,omitempty"`
}

type lookupFailure struct {
	Query string `json:"query"`
	Error string `json:"error"`
}

type lookupResult struct {
	Devices  []lookupRow     `json:"devices"`
	NotFound []string        `json:"not_found"`
	Failed   []lookupFailure `json:"failed,omitempty"`
}

func newNovelDevicesLookupCmd(flags *rootFlags) *cobra.Command {
	var file, dbPath string
	var concurrency int
	cmd := &cobra.Command{
		Use:   "lookup [serial|mac|imei|eid|order-number...]",
		Short: "One row per device: org record, MDM server, AppleCare and Activation Lock, by serial, MAC, IMEI, EID or order",
		Long: `Use this command for everything about specific devices you can name by serial, MAC, IMEI, EID or order number: org record, assigned MDM server, AppleCare and Activation Lock in one row. Do NOT use this command for fleet-wide lists; use 'devices unassigned', 'devices locked' or 'applecare expiring' instead.

Serials go straight to the API. MAC addresses (any separator), IMEI, MEID, EID and
order numbers are resolved to serials through the synced device inventory, so run
'sync --resources devices' first for those. With --data-source local the answer
comes from the store and 'devices enrich' data only.`,
		Example: strings.Trim(`
  apple-business-cli devices lookup C02XL0ABJGH5 --agent
  apple-business-cli devices lookup aa:bb:cc:dd:ee:ff --json
  apple-business-cli devices lookup --file serials.csv --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "serial=XABC123X0ABC123X0"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices lookup")
			}
			ids, err := readSerials(args, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one serial, MAC, IMEI, EID or order number is required (positional or --file)"))
			}
			ctx := cmd.Context() // each request is bounded by --timeout in the client
			// One config for the store, the ownership check, and the requests.
			c, cur, err := frozenClient(flags)
			if err != nil {
				return err
			}
			local, err := loadLookupIndex(ctx, cmd, storeDBPath(dbPath), flags, cur)
			if err != nil {
				return err
			}
			res := lookupResult{Devices: make([]lookupRow, 0), NotFound: make([]string, 0)}
			type target struct{ query, serial, by string }
			var targets []target
			seenSerial := map[string]bool{}
			for _, q := range ids {
				if serials, by := local.resolve(q); len(serials) > 0 {
					for _, s := range serials {
						if !seenSerial[strings.ToUpper(s)] {
							seenSerial[strings.ToUpper(s)] = true
							targets = append(targets, target{q, s, by})
						}
					}
					continue
				}
				if seenSerial[strings.ToUpper(q)] {
					continue
				}
				seenSerial[strings.ToUpper(q)] = true
				if flags.dataSource == "local" {
					res.NotFound = append(res.NotFound, q)
					continue
				}
				targets = append(targets, target{q, q, "serial"})
			}

			if flags.dataSource == "local" || len(targets) == 0 {
				c = nil
			}
			rows := make([]lookupRow, len(targets))
			errs := make([]error, len(targets))
			for i, t := range targets {
				rows[i] = lookupRow{Query: t.query, MatchedBy: t.by, Serial: t.serial}
			}
			if c == nil {
				for i := range rows {
					local.fill(&rows[i])
				}
			} else if err := fanOut(ctx, flags, c, concurrency, len(rows), func(wc *client.Client, i int) {
				errs[i] = lookupLive(ctx, wc, &rows[i], local, flags.dataSource == "auto")
			}); err != nil {
				if flags.dataSource != "auto" || !apiUnreachable(err) {
					return classifyAPIErrorOnly(err)
				}
				// API unreachable before any lookup ran: answer what the store
				// knows (auto), fail the rest explicitly.
				for i := range rows {
					if _, ok := local.devices[strings.ToUpper(rows[i].Serial)]; ok {
						local.fill(&rows[i])
						rows[i].Note = "API unreachable, showing local data: " + err.Error()
					} else {
						errs[i] = err
					}
				}
			}
			var firstErr error
			partial := 0
			for i, r := range rows {
				if errs[i] != nil {
					res.Failed = append(res.Failed, lookupFailure{Query: r.Query, Error: errs[i].Error()})
					if firstErr == nil {
						firstErr = errs[i]
					}
					continue
				}
				if r.Source == "" {
					res.NotFound = append(res.NotFound, r.Query)
					continue
				}
				if len(r.Errors) > 0 {
					partial++
				}
				res.Devices = append(res.Devices, r)
			}
			if firstErr == nil && partial > 0 {
				firstErr = apiErr(fmt.Errorf("%d device(s) are missing part of their record; see errors", partial))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), res, flags); err != nil {
					return err
				}
				return classifyAPIErrorOnly(firstErr)
			}
			w := cmd.OutOrStdout()
			for _, r := range res.Devices {
				fmt.Fprintf(w, "%s  %s (%s)  status=%s  server=%s\n", r.Serial, r.Model, r.Family, r.Status, firstNonEmpty(r.AssignedServerName, r.AssignedServerID, "-"))
				if r.AppleCare != nil {
					end := "-"
					if r.AppleCare.End != nil {
						end = r.AppleCare.End.Format("2006-01-02")
					}
					fmt.Fprintf(w, "    AppleCare: active=%v %s ends %s\n", r.AppleCare.Active, r.AppleCare.Description, end)
				}
				if r.ActivationLock != nil {
					fmt.Fprintf(w, "    Activation Lock: locked=%v type=%s\n", r.ActivationLock.IsLocked, r.ActivationLock.LockType)
				}
				for _, e := range r.Errors {
					fmt.Fprintf(w, "    warning: %s\n", e)
				}
			}
			if len(res.NotFound) > 0 {
				fmt.Fprintf(w, "not found: %s\n", strings.Join(res.NotFound, ", "))
			}
			return classifyAPIErrorOnly(firstErr)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "File of identifiers, one per line (first CSV column); - reads stdin")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Devices fetched in parallel")
	return cmd
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// lookupIndex resolves identifiers through the synced inventory and supplies
// local fallbacks (server names, enrichment) for the live path.
type lookupIndex struct {
	devices  map[string]orgDevice // serial -> device
	byHW     map[string][]string  // normalized MAC/IMEI/MEID/EID -> serials
	byOrder  map[string][]string
	servers  map[string]string // id -> name
	care     map[string]enrichRow
	lock     map[string]enrichRow
	srvLinks map[string]enrichRow
}

// loadLookupIndex reads the store; a missing store is an empty index, but a
// store that exists and cannot be read, or belongs to another API account,
// is an error rather than a silent "not found".
func loadLookupIndex(ctx context.Context, cmd *cobra.Command, path string, flags *rootFlags, cur string) (lookupIndex, error) {
	ix := lookupIndex{devices: map[string]orgDevice{}, byHW: map[string][]string{}, byOrder: map[string][]string{}, servers: map[string]string{}}
	if _, err := os.Stat(path); err != nil {
		return ix, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return ix, fmt.Errorf("opening local store %s: %w", path, err)
	}
	defer db.Close()
	if err := verifyStoreOwner(ctx, db, path, flagDB(cmd), cur); err != nil {
		return ix, err
	}
	if stored := readMeta(ctx, db, "client_id"); stored != "" && cur != "" && stored != cur {
		return ix, usageErr(fmt.Errorf("store %s belongs to API account %s but the active account is %s", path, stored, cur))
	}
	if !hintIfUnsynced(cmd, db, "devices") {
		hintIfStale(cmd, db, "devices", flags.maxAge)
	}
	devices, err := loadLocalDevices(ctx, db)
	if err != nil {
		return ix, err
	}
	for _, d := range devices {
		s := d.serial()
		ix.devices[strings.ToUpper(s)] = d
		a := d.Attributes
		hw := append([]string{a.WifiMacAddress, a.BluetoothMacAddress, a.EID}, a.EthernetMacAddress...)
		hw = append(hw, a.IMEI...)
		hw = append(hw, a.MEID...)
		for _, h := range hw {
			if k := normalizeHW(h); k != "" {
				ix.byHW[k] = append(ix.byHW[k], s)
			}
		}
		if a.OrderNumber != "" {
			ix.byOrder[a.OrderNumber] = append(ix.byOrder[a.OrderNumber], s)
		}
	}
	servers, _ := loadLocalServers(ctx, db)
	for _, s := range servers {
		ix.servers[s.ID] = s.Attributes.ServerName
	}
	for _, k := range []struct {
		kind string
		dst  *map[string]enrichRow
	}{{"applecare", &ix.care}, {"lock", &ix.lock}, {"server", &ix.srvLinks}} {
		if *k.dst, err = loadEnrichment(ctx, db, k.kind); err != nil {
			return ix, err
		}
	}
	return ix, nil
}

func (ix lookupIndex) resolve(q string) ([]string, string) {
	if d, ok := ix.devices[strings.ToUpper(q)]; ok {
		return []string{d.serial()}, "serial"
	}
	if s := ix.byHW[normalizeHW(q)]; len(s) > 0 {
		return s, "hardware_id"
	}
	if s := ix.byOrder[q]; len(s) > 0 {
		return s, "order_number"
	}
	return nil, ""
}

func (ix lookupIndex) applyDevice(row *lookupRow, d orgDevice) {
	a := d.Attributes
	row.Model, row.Family, row.Status = a.DeviceModel, a.ProductFamily, a.Status
	row.OrderNumber, row.PurchaseSourceType, row.AddedToOrg = a.OrderNumber, a.PurchaseSourceType, a.AddedToOrgDateTime
	row.Released = d.released()
	row.MigrationStatus, row.MigrationDeadline = a.MdmMigrationStatus, a.MdmMigrationDeadlineDateTime
}

// fill answers from the local store only (--data-source local).
func (ix lookupIndex) fill(row *lookupRow) {
	d, ok := ix.devices[strings.ToUpper(row.Serial)]
	if !ok {
		return
	}
	row.Source = "local"
	ix.applyDevice(row, d)
	if e, ok := ix.srvLinks[d.serial()]; ok {
		id := serverLinkID(e)
		row.AssignedServerID, row.AssignedServerName = id, ix.servers[id]
	}
	if e, ok := ix.care[d.serial()]; ok {
		v := coverageSummary(e.Data)
		row.AppleCare = &v
	}
	if e, ok := ix.lock[d.serial()]; ok {
		if l, ok := lockFrom(e); ok {
			row.ActivationLock = &lockView{l.IsLocked, l.LockType}
		}
	}
}

// lookupLive fills row from the API. Only HTTP 404 means "not in this org"
// (row left without a source); any other failure is returned so auth and
// network problems are never reported as not found. When the API is
// unreachable (auto data source) the local store answers, marked local;
// credential and token failures never fall back.
func lookupLive(ctx context.Context, c *client.Client, row *lookupRow, ix lookupIndex, allowLocal bool) error {
	id := row.Serial
	if d, ok := ix.devices[strings.ToUpper(row.Serial)]; ok && d.ID != "" {
		id = d.ID // the resource ID; Apple documents it as the serial
	}
	raw, err := c.Get(ctx, "/orgDevices/"+url.PathEscape(id), nil)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return nil
		}
		if _, ok := ix.devices[strings.ToUpper(row.Serial)]; ok && allowLocal && apiUnreachable(err) {
			ix.fill(row)
			row.Note = "live device fetch failed, showing local data: " + err.Error()
			return nil
		}
		return err
	}
	var env struct {
		Data orgDevice `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("parsing device %s: %w", row.Serial, err)
	}
	row.Source = "live"
	if s := env.Data.serial(); s != "" {
		row.Serial = s
	}
	ix.applyDevice(row, env.Data)
	id = firstNonEmpty(env.Data.ID, id)
	base := "/orgDevices/" + url.PathEscape(id)

	if id, err := assignedServerID(ctx, c, id); err != nil {
		row.Errors = append(row.Errors, "assigned server: "+err.Error())
	} else {
		row.AssignedServerID = id
		if id != "" {
			row.AssignedServerName = ix.servers[id]
			if row.AssignedServerName == "" {
				if sraw, err := c.Get(ctx, "/mdmServers/"+url.PathEscape(id), nil); err == nil {
					var s struct {
						Data mdmServer `json:"data"`
					}
					if json.Unmarshal(sraw, &s) == nil {
						row.AssignedServerName = s.Data.Attributes.ServerName
					}
				}
			}
		}
	}
	if craw, err := fetchEnrichment(ctx, c, "applecare", base+"/appleCareCoverage"); err != nil {
		row.Errors = append(row.Errors, "applecare: "+err.Error())
	} else {
		v := coverageSummary(craw)
		row.AppleCare = &v
	}
	if lraw, err := fetchEnrichment(ctx, c, "lock", base+"/activationLockStatus"); err != nil {
		row.Errors = append(row.Errors, "activation lock: "+err.Error())
	} else if l, ok := lockFrom(enrichRow{Data: lraw}); ok {
		row.ActivationLock = &lockView{l.IsLocked, l.LockType}
	}
	return nil
}
