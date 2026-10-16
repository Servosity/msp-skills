// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/config"
)

// Shared NSM and MySonicWall reads for the hand-written posture commands. Every
// call goes through the generated client, so auth routing (Bearer for NSM,
// X-API-KEY for MySonicWall), caching, retries and the NSM envelope hook apply.
// Rule throughout: a response that cannot be read is an error or "unknown",
// never an empty list or a pass.

const mswBase = "https://api.mysonicwall.com"

// fleetDevice is the subset of GET /devices/tenant the posture commands use.
type fleetDevice struct {
	SerialNumber    string `json:"serialNumber"`
	FriendlyName    string `json:"friendlyName"`
	FirmwareVersion string `json:"firmwareVersion"`
	IPAddress       string `json:"ipAddress"`
	LiveStatus      *bool  `json:"liveStatus"` // nil when NSM did not report it: unknown, never offline
	TenantID        string `json:"tenantId"`
}

// dogfoodDeviceCap keeps per-device fan-outs inside the live dogfood matrix's
// per-command timeout; real runs walk the whole fleet.
const dogfoodDeviceCap = 3

const (
	devicePageSize = 10000 // NSM's documented default limit
	maxDevicePages = 20
)

// fetchFleet lists every NSM device for the token's tenant, optionally narrowed
// to one NSM tenant id or one serial. It pages until a short page and fails
// rather than reporting on a truncated fleet.
func fetchFleet(ctx context.Context, c *client.Client, tenant, serial string) ([]fleetDevice, error) {
	all := make([]fleetDevice, 0)
	seen := map[string]bool{}
	for page := 1; ; page++ {
		if page > maxDevicePages {
			return nil, fmt.Errorf("NSM returned more than %d devices; refusing to report on a truncated fleet", devicePageSize*maxDevicePages)
		}
		raw, err := c.Get(ctx, "/devices/tenant", map[string]string{"page": strconv.Itoa(page), "limit": strconv.Itoa(devicePageSize)})
		if err != nil {
			return nil, err
		}
		var batch []fleetDevice
		if err := decodeList(raw, &batch); err != nil {
			return nil, fmt.Errorf("parsing /devices/tenant: %w", err)
		}
		added := 0
		for _, d := range batch {
			if strings.TrimSpace(d.SerialNumber) == "" {
				return nil, fmt.Errorf("parsing /devices/tenant: device row without serialNumber")
			}
			key := strings.ToUpper(d.SerialNumber)
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, d)
			added++
		}
		if len(batch) < devicePageSize {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("NSM ignored paging at %d devices; refusing to report on a possibly truncated fleet", len(all))
		}
	}
	out := make([]fleetDevice, 0, len(all))
	for _, d := range all {
		if tenant != "" && !strings.EqualFold(d.TenantID, tenant) {
			continue
		}
		if serial != "" && !strings.EqualFold(d.SerialNumber, serial) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SerialNumber < out[j].SerialNumber })
	return out, nil
}

// capForDogfood trims per-device fan-outs under the live dogfood harness only.
// Never apply it to set comparisons (blind-spots), where a short list invents gaps.
func capForDogfood(devices []fleetDevice) ([]fleetDevice, bool) {
	if cliutil.IsDogfoodEnv() && len(devices) > dogfoodDeviceCap {
		return devices[:dogfoodDeviceCap], true
	}
	return devices, false
}

// interrupted turns a cancelled or timed-out command context into a
// partial-failure exit, so rows already printed are never mistaken for a
// complete audit.
func interrupted(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return partialFailureErr(fmt.Errorf("interrupted before every firewall was checked: %w", err))
	}
	return nil
}

// advisory is one entry of vulnList in GET /devices/vulnerable.
type advisory struct {
	AdvisoryID     string `json:"advisoryID"`
	Title          string `json:"title"`
	Impact         string `json:"impact"`
	HaveWorkaround *bool  `json:"haveWorkaroud"` // sic: NSM spells it haveWorkaroud; nil when not reported
	NoticeLink     string `json:"noticeLink"`
}

type vulnerableDevice struct {
	SerialNumber string      `json:"serialNumber"`
	FriendlyName string      `json:"friendlyName"`
	VulnList     *[]advisory `json:"vulnList"` // nil = missing or null, which fetchVulnerable rejects
}

// advisories returns the validated advisory list.
func (v vulnerableDevice) advisories() []advisory {
	if v.VulnList == nil {
		return nil
	}
	return *v.VulnList
}

func fetchVulnerable(ctx context.Context, c *client.Client) ([]vulnerableDevice, error) {
	raw, err := c.Get(ctx, "/devices/vulnerable", nil)
	if err != nil {
		return nil, err
	}
	out := make([]vulnerableDevice, 0)
	if err := decodeList(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing /devices/vulnerable: %w", err)
	}
	for _, v := range out {
		if strings.TrimSpace(v.SerialNumber) == "" {
			return nil, fmt.Errorf("parsing /devices/vulnerable: row without serialNumber")
		}
		if v.VulnList == nil {
			return nil, fmt.Errorf("parsing /devices/vulnerable: %s has no vulnList", v.SerialNumber)
		}
		for _, a := range *v.VulnList {
			if strings.TrimSpace(a.AdvisoryID) == "" {
				return nil, fmt.Errorf("parsing /devices/vulnerable: advisory without advisoryID on %s", v.SerialNumber)
			}
		}
	}
	return out, nil
}

// advisoriesForFirmware calls GET /devices/vulnerableFirmware for one build.
// sonicOSVersionPattern matches a SonicOS build such as 7.0.1-5035 or 6.5.4.4-44n.
var sonicOSVersionPattern = regexp.MustCompile(`\b\d+\.\d+(\.\d+)*(-[0-9A-Za-z]+)*\b`)

func advisoriesForFirmware(ctx context.Context, c *client.Client, firmware string) ([]advisory, error) {
	// A blank or unusable firmware string (N/A, unknown) would ask NSM about no
	// real build, and its empty answer would then read as "no advisories" (a
	// pass). Only a SonicOS version (digits.digits..., optional -build parts)
	// is looked up; anything else is unknown, never pass.
	version := sonicOSVersionPattern.FindString(strings.TrimSpace(firmware))
	if version == "" {
		return nil, fmt.Errorf("running firmware version not reported by NSM (got %q)", strings.TrimSpace(firmware))
	}
	raw, err := c.Get(ctx, "/devices/vulnerableFirmware", map[string]string{"firmwareVersion": version})
	if err != nil {
		return nil, err
	}
	out := make([]advisory, 0)
	if err := decodeList(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing /devices/vulnerableFirmware: %w", err)
	}
	for _, a := range out {
		if strings.TrimSpace(a.AdvisoryID) == "" {
			return nil, fmt.Errorf("parsing /devices/vulnerableFirmware: advisory without advisoryID")
		}
	}
	return out, nil
}

func advisoryIDs(advs []advisory) []string {
	ids := make([]string, 0, len(advs))
	for _, a := range advs {
		ids = append(ids, a.AdvisoryID)
	}
	return ids
}

// lifecycle is GET /devices/productLifeCycle/{serial}.
type lifecycle struct {
	Model   string `json:"model"`
	EOS     string `json:"eos"`
	LOD     string `json:"lod"`
	ARMB    string `json:"armb"`
	LRMB    string `json:"lrmb"`
	OneYLDO string `json:"oneyldo"`
}

func fetchLifecycle(ctx context.Context, c *client.Client, serial string) (lifecycle, error) {
	var lc lifecycle
	raw, err := c.Get(ctx, "/devices/productLifeCycle/"+url.PathEscape(serial), nil)
	if err != nil {
		return lc, err
	}
	if err := json.Unmarshal(raw, &lc); err != nil {
		return lc, fmt.Errorf("parsing productLifeCycle: %w", err)
	}
	return lc, nil
}

func fetchAvailableVersions(ctx context.Context, c *client.Client, serial string) (json.RawMessage, error) {
	return c.Get(ctx, "/devices/availableVersions/"+url.PathEscape(serial), nil)
}

// sonicOSGet reads one SonicOS passthrough path on the firewall named by serial.
func sonicOSGet(ctx context.Context, c *client.Client, serial, path string) (json.RawMessage, error) {
	return c.GetWithHeaders(ctx, path, nil, map[string]string{"X-DEVICE-ID": serial})
}

// mswServiceInfo reads MySonicWall licence rows for one serial.
func mswServiceInfo(ctx context.Context, c *client.Client, serial string) (json.RawMessage, error) {
	raw, err := c.Get(ctx, mswBase+"/api/product/serviceInfo", map[string]string{"serial": serial})
	if err != nil {
		return nil, err
	}
	if err := mswStatusError(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// mswStatusError reports a MySonicWall application-level failure carried in an
// HTTP-200 body ({"operationStatus":"Failure","msgCode":...,"msgDesc":...}).
func mswStatusError(raw json.RawMessage) error {
	var head struct {
		OperationStatus *string `json:"operationStatus"`
	}
	if json.Unmarshal(raw, &head) != nil || head.OperationStatus == nil {
		return nil // no operationStatus to judge; callers still validate the payload shape
	}
	switch strings.ToLower(strings.TrimSpace(*head.OperationStatus)) {
	case "success", "ok", "succeeded":
		return nil
	}
	// The failure is decided; diagnostics are best effort and never undo it.
	var diag struct {
		MsgCode json.RawMessage `json:"msgCode"`
		MsgDesc json.RawMessage `json:"msgDesc"`
	}
	_ = json.Unmarshal(raw, &diag)
	return fmt.Errorf("MySonicWall operationStatus %q (%s %s)", *head.OperationStatus,
		strings.Trim(string(diag.MsgCode), `"`), strings.Trim(string(diag.MsgDesc), `"`))
}

// mswKeyConfigured reports whether a MySonicWall API key is available from the
// config file or SONICWALL_MSW_API_KEY.
func mswKeyConfigured(flags *rootFlags) bool {
	cfg, err := config.Load(flags.configPath)
	return err == nil && cfg != nil && strings.TrimSpace(cfg.SonicwallMswApiKey) != ""
}

// decodeList decodes an NSM list response. The vendor spec documents a bare
// array; an object carrying the list under devices/data/items/content is also
// accepted. Anything else (null, empty body, a bare object) is an error, so an
// unreadable answer never becomes an empty list.
func decodeList[T any](raw json.RawMessage, out *[]T) error {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal(raw, out)
	}
	if strings.HasPrefix(trimmed, "{") {
		var env map[string]json.RawMessage
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		for _, key := range []string{"devices", "data", "items", "content"} {
			if inner, ok := env[key]; ok && strings.HasPrefix(strings.TrimSpace(string(inner)), "[") {
				return json.Unmarshal(inner, out)
			}
		}
	}
	snippet := trimmed
	if len(snippet) > 80 {
		snippet = snippet[:80] + "..."
	}
	return fmt.Errorf("unexpected response shape, want a JSON array: %q", snippet)
}

// fanoutFailureNote formats FanoutRun failures (cancelled, panicked or erroring
// sources) for stderr; every failed source also gets an "unknown" output row.
func fanoutFailureNote(fails []cliutil.FanoutError, total int) string {
	return fmt.Sprintf("%d of %d firewalls were not checked (cancelled or failed); their rows are marked unknown", len(fails), total)
}

// classifyReadError maps an API error to its exit code and JSON error envelope
// without the shared --idempotent "already exists" no-op, which only makes
// sense for create calls: a failed read must never become a successful no-op.
func classifyReadError(w io.Writer, err error, flags *rootFlags) error {
	if err == nil {
		return nil
	}
	classified := classifyAPIErrorOnly(err)
	writeAPIErrorEnvelope(w, flags, classified, ExitCode(classified))
	return classified
}

// errText shortens an error for a per-row errors field.
func errText(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i > 0 {
		msg = msg[:i]
	}
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return msg
}

// noFirewallsErr refuses a fleet report that matched no NSM firewall: its empty
// answer would read as an all-clear for firewalls nobody checked.
func noFirewallsErr(w io.Writer, flags *rootFlags) error {
	err := notFoundErr(fmt.Errorf("no firewall in NSM matches this token and filter, so nothing was checked (only firewalls onboarded to NSM SaaS are visible; try 'blind-spots')"))
	writeAPIErrorEnvelope(w, flags, err, ExitCode(err))
	return err
}

// printOrderedTable prints exactly these columns in this order. printAutoTable
// keeps six columns and breaks ties in map order, so wider reports lost a
// random column each run.
func printOrderedTable(w io.Writer, cols []string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	tw := newTabWriter(w)
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = bold(strings.ToUpper(c))
	}
	fmt.Fprintln(tw, strings.Join(head, "\t"))
	for _, r := range rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = formatCellValue(r[c])
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}
