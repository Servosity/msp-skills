// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"bitdefender-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type rmmDevice struct {
	Hostname string   `json:"hostname"`
	MACs     []string `json:"macs,omitempty"`
}

type gapEndpoint struct {
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	EndpointID  string `json:"endpoint_id"`
	Name        string `json:"name"`
	RMMHostname string `json:"rmm_hostname,omitempty"`
}

type ambiguousMatch struct {
	Device     rmmDevice     `json:"device"`
	Candidates []gapEndpoint `json:"candidates"`
}

type rmmGapView struct {
	DevicesRead       int              `json:"devices_read"`
	EndpointsCompared int              `json:"endpoints_compared"`
	Source            string           `json:"source"`
	TakenAt           string           `json:"taken_at,omitempty"`
	InventoryComplete bool             `json:"inventory_complete"`
	MissingInGZ       []rmmDevice      `json:"missing_in_gravityzone"`
	UnmanagedInGZ     []gapEndpoint    `json:"unmanaged_in_gravityzone"`
	MissingInRMM      []gapEndpoint    `json:"missing_in_rmm"`
	Ambiguous         []ambiguousMatch `json:"ambiguous"`
	Summary           map[string]int   `json:"summary"`
	FetchFailures     []fetchFailure   `json:"fetch_failures"`
	Note              string           `json:"note,omitempty"`
}

func newNovelFleetRmmGapCmd(flags *rootFlags) *cobra.Command {
	var devicesPath string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "rmm-gap",
		Short: "Compare an RMM device export (--devices CSV or JSON) with GravityZone inventory across every company",
		Long: `Reads an RMM or PSA device export (--devices: a CSV with a hostname column, or a JSON
array of objects, e.g. another connector's --json output) and joins it to GravityZone
inventory, unmanaged endpoints included. A device matches by MAC address when one of its MACs
is known to GravityZone, otherwise by short hostname; a hostname that matches more than one
endpoint (in one company or several) is reported under ambiguous instead of being guessed.
Returns three lists:
  missing_in_gravityzone    RMM devices with no GravityZone record at all
  unmanaged_in_gravityzone  RMM devices GravityZone discovered but that have no agent
  missing_in_rmm            managed GravityZone endpoints the RMM does not know
Inventory comes from the latest 'fleet health' snapshot, or a live fan-out when there is no
snapshot or --data-source live is set. Recognized columns/keys: hostname, name, device,
computer, systemName, dnsName, displayName; mac, macAddress, macs.

Use this command to compare an RMM or PSA device list against GravityZone inventory. Do NOT
use this command for GravityZone-only protection status without an external device list; use
'fleet health' instead.`,
		Example:     "  bitdefender-cli fleet rmm-gap --devices ninja-devices.csv --agent\n  bitdefender-cli fleet rmm-gap --devices datto-devices.json --data-source live --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "--devices=example-value"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet rmm-gap")
			}
			if devicesPath == "" {
				_ = cmd.Usage()
				return fleetUsageErr(cmd, flags, fmt.Errorf("--devices is required (a CSV or JSON device export)"))
			}
			devices, err := readRMMDevices(devicesPath)
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			inv, err := fleetInventory(ctx, func() (*client.Client, error) { return flags.newClient() }, s.DB(), flags.dataSource, flags.maxAge, maxPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := compareRMM(devices, inv.Endpoints)
			if inv.Source == "snapshot" && inv.TakenAt == "" {
				// Nothing to compare against: never report every device as missing.
				view = compareRMM(nil, nil)
				view.DevicesRead = len(devices)
			}
			view.Source, view.TakenAt, view.InventoryComplete = inv.Source, inv.TakenAt, inv.Complete
			flags.agentSource = inventoryAgentSource(inv)
			view.FetchFailures = append(make([]fetchFailure, 0), inv.Failures...)
			switch {
			case inv.Source == "snapshot" && inv.TakenAt == "":
				view.Note = "no fleet snapshot yet; run 'bitdefender-cli fleet health' or use --data-source live"
			case !inv.Complete:
				view.Note = "inventory is incomplete (see fetch_failures): devices in unfetched companies show up under missing_in_gravityzone"
			case inv.Stale:
				view.Note = "snapshot from " + inv.TakenAt + " is older than --max-age; run 'fleet health' to refresh"
			}
			if inv.Note != "" {
				view.Note = strings.TrimSpace(view.Note + " " + inv.Note)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			humanNote(cmd, view.Note)
			rows := make([]map[string]any, 0)
			for _, d := range view.MissingInGZ {
				rows = append(rows, map[string]any{"gap": "missing_in_gravityzone", "company": "", "machine": d.Hostname})
			}
			for _, e := range view.UnmanagedInGZ {
				rows = append(rows, map[string]any{"gap": "unmanaged_in_gravityzone", "company": e.CompanyName, "machine": e.Name})
			}
			for _, e := range view.MissingInRMM {
				rows = append(rows, map[string]any{"gap": "missing_in_rmm", "company": e.CompanyName, "machine": e.Name})
			}
			for _, a := range view.Ambiguous {
				rows = append(rows, map[string]any{"gap": "ambiguous", "company": fmt.Sprintf("%d companies", len(a.Candidates)), "machine": a.Device.Hostname})
			}
			if inv.Source == "snapshot" && inv.TakenAt == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing compared: no GravityZone inventory was read (see the note above).")
				return nil
			}
			if len(rows) == 0 && !view.InventoryComplete {
				// Hand-wired (handfixes.json: rmm-gap-incomplete-never-clean).
				fmt.Fprintf(cmd.OutOrStdout(), "INCOMPLETE: no gaps among the %d endpoints that were read, but %d GravityZone fetch(es) failed, so devices in those companies were not compared (run with --json and read fetch_failures).\n", view.EndpointsCompared, len(view.FetchFailures))
				return nil
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No gaps: %d devices and %d endpoints line up.\n", view.DevicesRead, view.EndpointsCompared)
				return nil
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&devicesPath, "devices", "", "RMM/PSA device export: CSV with a hostname column, or a JSON array of objects")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum getEndpointsList pages per company on a live fan-out")
	return cmd
}

var hostKeys = []string{"hostname", "host", "name", "device", "devicename", "computer", "computername", "systemname", "dnsname", "displayname"}
var macKeys = []string{"mac", "macaddress", "macs", "macaddresses"}

func normKey(k string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(k))
}

func readRMMDevices(path string) ([]rmmDevice, error) {
	b, err := readBoundedFile("--devices", path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(string(b)), "\ufeff")
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		return parseRMMJSON([]byte(trimmed))
	}
	cr := csv.NewReader(strings.NewReader(strings.TrimPrefix(trimmed, "\ufeff")))
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("--devices: %w", err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("--devices: need a header row and at least one device")
	}
	hi, mi := -1, -1
	for _, want := range hostKeys {
		for i, h := range recs[0] {
			if hi < 0 && normKey(h) == want {
				hi = i
			}
		}
	}
	for _, want := range macKeys {
		for i, h := range recs[0] {
			if mi < 0 && normKey(h) == want {
				mi = i
			}
		}
	}
	if hi < 0 {
		return nil, fmt.Errorf("--devices: no hostname column (expected one of %s)", strings.Join(hostKeys, ", "))
	}
	out := make([]rmmDevice, 0, len(recs)-1)
	for n, rec := range recs[1:] {
		if strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue // blank line
		}
		if hi >= len(rec) || strings.TrimSpace(rec[hi]) == "" {
			return nil, fmt.Errorf("--devices: record %d has no hostname", n+2)
		}
		d := rmmDevice{Hostname: strings.TrimSpace(rec[hi])}
		if mi >= 0 && mi < len(rec) {
			for _, m := range strings.FieldsFunc(rec[mi], func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
				d.MACs = append(d.MACs, m)
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func parseRMMJSON(b []byte) ([]rmmDevice, error) {
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		var wrapped map[string]json.RawMessage
		if json.Unmarshal(b, &wrapped) != nil {
			return nil, fmt.Errorf("--devices: not a JSON array of device objects")
		}
		found := false
		for _, k := range []string{"results", "items", "devices", "data"} {
			if raw, ok := wrapped[k]; ok && strings.HasPrefix(strings.TrimSpace(string(raw)), "[") && json.Unmarshal(raw, &arr) == nil {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("--devices: JSON object has no results/items/devices/data array")
		}
	}
	out := make([]rmmDevice, 0, len(arr))
	for n, obj := range arr {
		fields := map[string]any{}
		for k, v := range obj {
			fields[normKey(k)] = v
		}
		var d rmmDevice
		for _, k := range hostKeys {
			if v, ok := fields[k].(string); ok && strings.TrimSpace(v) != "" {
				d.Hostname = strings.TrimSpace(v)
				break
			}
		}
		for _, k := range macKeys {
			switch v := fields[k].(type) {
			case string:
				d.MACs = append(d.MACs, v)
			case []any:
				for _, m := range v {
					if s, ok := m.(string); ok {
						d.MACs = append(d.MACs, s)
					}
				}
			}
		}
		if d.Hostname == "" {
			return nil, fmt.Errorf("--devices: JSON record %d has no hostname (expected one of %s)", n+1, strings.Join(hostKeys, ", "))
		}
		out = append(out, d)
	}
	return out, nil
}

// compareRMM joins devices to endpoints: by MAC when any device MAC is known,
// else by short hostname. A hostname matching several endpoints is ambiguous.
func compareRMM(devices []rmmDevice, eps []snapEndpoint) rmmGapView {
	view := rmmGapView{DevicesRead: len(devices), EndpointsCompared: len(eps), MissingInGZ: make([]rmmDevice, 0),
		UnmanagedInGZ: make([]gapEndpoint, 0), MissingInRMM: make([]gapEndpoint, 0), Ambiguous: make([]ambiguousMatch, 0), Summary: map[string]int{}}
	byHost := map[string][]int{}
	byMAC := map[string][]int{}
	for i, e := range eps {
		for _, h := range []string{e.Name, e.FQDN} {
			if sh := shortHost(h); sh != "" {
				byHost[sh] = append(byHost[sh], i)
			}
		}
		own := map[string]bool{}
		for _, m := range e.MACs {
			if nm := normalizeMAC(m); identifyingMAC(nm) && !own[nm] {
				own[nm] = true
				byMAC[nm] = append(byMAC[nm], i)
			}
		}
	}
	// A MAC carried by more than one endpoint identifies none of them.
	for m, idx := range byMAC {
		if len(idx) > 1 {
			delete(byMAC, m)
		}
	}
	matched := map[int]bool{}
	toGap := func(e snapEndpoint, host string) gapEndpoint {
		return gapEndpoint{CompanyID: e.CompanyID, CompanyName: e.CompanyName, EndpointID: e.EndpointID, Name: e.Name, RMMHostname: host}
	}
	for _, d := range devices {
		hits := map[int]bool{}
		for _, m := range d.MACs {
			for _, i := range byMAC[normalizeMAC(m)] {
				hits[i] = true
			}
		}
		if len(hits) == 0 {
			for _, i := range byHost[shortHost(d.Hostname)] {
				hits[i] = true
			}
		}
		{
			if len(hits) > 1 {
				a := ambiguousMatch{Device: d}
				for i := range hits {
					matched[i] = true // claimed by an RMM device, so never "missing in RMM"
					a.Candidates = append(a.Candidates, toGap(eps[i], d.Hostname))
				}
				sort.Slice(a.Candidates, func(x, y int) bool { return a.Candidates[x].CompanyName < a.Candidates[y].CompanyName })
				view.Ambiguous = append(view.Ambiguous, a)
				continue
			}
		}
		if len(hits) == 0 {
			view.MissingInGZ = append(view.MissingInGZ, d)
			continue
		}
		anyManaged := false
		for i := range hits {
			matched[i] = true
			if eps[i].IsManaged {
				anyManaged = true
			}
		}
		if !anyManaged {
			for i := range hits {
				view.UnmanagedInGZ = append(view.UnmanagedInGZ, toGap(eps[i], d.Hostname))
			}
		}
	}
	for i, e := range eps {
		if e.IsManaged && !matched[i] {
			view.MissingInRMM = append(view.MissingInRMM, gapEndpoint{CompanyID: e.CompanyID, CompanyName: e.CompanyName, EndpointID: e.EndpointID, Name: e.Name})
		}
	}
	sort.SliceStable(view.UnmanagedInGZ, func(i, j int) bool { return view.UnmanagedInGZ[i].CompanyName < view.UnmanagedInGZ[j].CompanyName })
	sort.SliceStable(view.MissingInRMM, func(i, j int) bool {
		if view.MissingInRMM[i].CompanyName != view.MissingInRMM[j].CompanyName {
			return view.MissingInRMM[i].CompanyName < view.MissingInRMM[j].CompanyName
		}
		return view.MissingInRMM[i].Name < view.MissingInRMM[j].Name
	})
	view.Summary["missing_in_gravityzone"] = len(view.MissingInGZ)
	view.Summary["unmanaged_in_gravityzone"] = len(view.UnmanagedInGZ)
	view.Summary["missing_in_rmm"] = len(view.MissingInRMM)
	view.Summary["ambiguous"] = len(view.Ambiguous)
	return view
}
