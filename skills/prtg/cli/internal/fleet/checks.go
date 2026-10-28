// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the pure logic behind the fleet reports (no I/O).

package fleet

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Ref is a PRTG model.ReferencedObject.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// NodeStatusInfo is model.NodeStatusInfo: who set a pause or acknowledgement.
type NodeStatusInfo struct {
	AutoAcknowledged bool   `json:"auto_acknowledged"`
	Message          string `json:"message"`
	TimeStamp        string `json:"time_stamp"`
	ValidUntil       string `json:"valid_until"`
	User             *Ref   `json:"user"`
}

// StatusInfo is model.StatusInfo.
type StatusInfo struct {
	Info *NodeStatusInfo `json:"info"`
}

// Node is the subset of SensorInfo / DeviceInfo / GroupInfo / ProbeInfo /
// TreeNode the fleet reports read. PRTG returns ids as strings.
type Node struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	StatusSince string `json:"status_since"`
	LastDown    string `json:"last_down"`
	// ScanningInterval is SensorInfo.scanning_interval, in seconds.
	ScanningInterval float64     `json:"scanning_interval"`
	Message          string      `json:"message"`
	Priority         int         `json:"priority"`
	SensorKind       string      `json:"sensor_kind"`
	Path             []Ref       `json:"path"`
	StatusInfo       *StatusInfo `json:"status_info"`
	ProbeStatus      string      `json:"probe_status"`
}

// DecodeNodes reads a list body ([]Node). Unknown fields are ignored.
func DecodeNodes(body json.RawMessage) ([]Node, error) {
	var nodes []Node
	if err := json.Unmarshal(body, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// pathRef returns the last path element of the given referenced type
// (REFERENCED_DEVICE, REFERENCED_GROUP, REFERENCED_PROBE), or a zero Ref.
func (n Node) pathRef(refType string) Ref {
	for i := len(n.Path) - 1; i >= 0; i-- {
		if n.Path[i].Type == refType {
			return n.Path[i]
		}
	}
	return Ref{}
}

func (n Node) info() NodeStatusInfo {
	if n.StatusInfo != nil && n.StatusInfo.Info != nil {
		return *n.StatusInfo.Info
	}
	return NodeStatusInfo{}
}

// severity ranks alarm states for sorting: lower is worse.
var severity = map[string]int{
	"DOWN": 0, "DOWN_PARTIAL": 1, "WARNING": 2, "UNUSUAL": 3, "DOWN_ACKNOWLEDGED": 4,
}

func sev(status string) int {
	if s, ok := severity[status]; ok {
		return s
	}
	return 9
}

// alarmStatusGroups maps the --status words to PRTG status values.
var alarmStatusGroups = map[string][]string{
	"down":         {"DOWN", "DOWN_PARTIAL"},
	"warning":      {"WARNING"},
	"unusual":      {"UNUSUAL"},
	"acknowledged": {"DOWN_ACKNOWLEDGED"},
}

// AlarmStatusWords lists the accepted --status words.
func AlarmStatusWords() []string { return []string{"down", "warning", "unusual", "acknowledged"} }

// ParseAlarmStatuses turns "down,warning" into a status set. Empty selects
// every alarm state.
func ParseAlarmStatuses(words []string) (map[string]bool, error) {
	set := map[string]bool{}
	if len(words) == 0 {
		words = AlarmStatusWords()
	}
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		vals, ok := alarmStatusGroups[w]
		if !ok {
			return nil, fmt.Errorf("unknown status %q (use %s)", w, strings.Join(AlarmStatusWords(), ", "))
		}
		for _, v := range vals {
			set[v] = true
		}
	}
	return set, nil
}

// AlarmRow is one sensor in alarm on one core.
type AlarmRow struct {
	Core        string `json:"core"`
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Device      string `json:"device,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	Group       string `json:"group,omitempty"`
	Probe       string `json:"probe,omitempty"`
	Status      string `json:"status,omitempty"`
	StatusSince string `json:"status_since,omitempty"`
	AgeMinutes  *int   `json:"age_minutes,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	Message     string `json:"message,omitempty"`
	AckedBy     string `json:"acked_by,omitempty"`
	AckMessage  string `json:"ack_message,omitempty"`
	Sensors     int    `json:"sensors,omitempty"`
	Error       string `json:"error,omitempty"`
}

// AlarmOptions filter and shape the alarm list.
type AlarmOptions struct {
	Statuses map[string]bool
	Unacked  bool
	MinAge   time.Duration
	Now      time.Time
}

// AlarmRows turns one core's alarm list into rows, applying the filters.
func AlarmRows(core string, nodes []Node, o AlarmOptions) []AlarmRow {
	rows := make([]AlarmRow, 0, len(nodes))
	for _, n := range nodes {
		if len(o.Statuses) > 0 && !o.Statuses[n.Status] {
			continue
		}
		if o.Unacked && n.Status == "DOWN_ACKNOWLEDGED" {
			continue
		}
		r := AlarmRow{Core: core, ID: n.ID, Name: n.Name, Status: n.Status, StatusSince: n.StatusSince,
			Priority: n.Priority, Message: n.Message,
			Device: n.pathRef("REFERENCED_DEVICE").Name, DeviceID: n.pathRef("REFERENCED_DEVICE").ID,
			Group: n.pathRef("REFERENCED_GROUP").Name, Probe: n.pathRef("REFERENCED_PROBE").Name}
		// Without a timestamp the age is unknown; such a row is kept rather
		// than hiding a real alarm behind the age filter.
		if t, ok := ParseTime(n.StatusSince); ok {
			age := o.Now.Sub(t)
			if o.MinAge > 0 && age < o.MinAge {
				continue
			}
			m := int(age.Minutes())
			r.AgeMinutes = &m
		}
		if n.Status == "DOWN_ACKNOWLEDGED" {
			inf := n.info()
			if inf.User != nil {
				r.AckedBy = inf.User.Name
			}
			r.AckMessage = inf.Message
		}
		rows = append(rows, r)
	}
	return rows
}

// SortAlarms orders rows worst first: severity, then priority (high first),
// then the oldest status_since. Error rows go last, by core.
func SortAlarms(rows []AlarmRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Error != "") != (b.Error != "") {
			return a.Error == ""
		}
		if a.Error != "" {
			return a.Core < b.Core
		}
		if sev(a.Status) != sev(b.Status) {
			return sev(a.Status) < sev(b.Status)
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		ta, okA := ParseTime(a.StatusSince)
		tb, okB := ParseTime(b.StatusSince)
		if okA != okB {
			return okA
		}
		if okA && !ta.Equal(tb) {
			return ta.Before(tb)
		}
		if a.Core != b.Core {
			return a.Core < b.Core
		}
		return a.Name < b.Name
	})
}

// RollupByDevice collapses sensor rows into one row per core+device ID: the
// worst status, the highest priority, the oldest status_since and a sensor
// count, so 37 sensors on one dead firewall read as one line. Devices that
// share a name stay apart; a sensor with no device in its path stays its
// own row.
func RollupByDevice(rows []AlarmRow) []AlarmRow {
	type key struct{ core, device string }
	idx := map[key]int{}
	out := make([]AlarmRow, 0, len(rows))
	for _, r := range rows {
		r.Sensors = 1
		if r.Error != "" || r.DeviceID == "" {
			if r.Error != "" {
				r.Sensors = 0
			}
			out = append(out, r)
			continue
		}
		k := key{r.Core, r.DeviceID}
		i, ok := idx[k]
		if !ok {
			// A device with one alarming sensor keeps that sensor's full row
			// (ID, message, acknowledgement); it becomes an aggregate only
			// when a second sensor arrives.
			idx[k] = len(out)
			out = append(out, r)
			continue
		}
		agg := &out[i]
		if agg.Sensors == 1 {
			*agg = AlarmRow{Core: agg.Core, Device: agg.Device, DeviceID: agg.DeviceID, Group: agg.Group, Probe: agg.Probe,
				Status: agg.Status, StatusSince: agg.StatusSince, AgeMinutes: agg.AgeMinutes, Priority: agg.Priority, Sensors: 1}
		}
		agg.Sensors++
		if sev(r.Status) < sev(agg.Status) {
			agg.Status = r.Status
		}
		if r.Priority > agg.Priority {
			agg.Priority = r.Priority
		}
		if t, ok := ParseTime(r.StatusSince); ok {
			if cur, okCur := ParseTime(agg.StatusSince); !okCur || t.Before(cur) {
				agg.StatusSince, agg.AgeMinutes = r.StatusSince, r.AgeMinutes
			}
		}
		agg.Name = fmt.Sprintf("%d sensors", agg.Sensors)
	}
	return out
}

// SensorSummary is SensorStatusSummary (counts by state).
type SensorSummary struct {
	Total              int `json:"total"`
	Up                 int `json:"up"`
	Down               int `json:"down"`
	DownAcknowledged   int `json:"down_acknowledged"`
	DownPartial        int `json:"down_partial"`
	Warning            int `json:"warning"`
	Unusual            int `json:"unusual"`
	Unknown            int `json:"unknown"`
	PausedByUser       int `json:"paused_by_user"`
	PausedByUserUntil  int `json:"paused_by_user_until"`
	PausedByDependency int `json:"paused_by_dependency"`
	PausedBySchedule   int `json:"paused_by_schedule"`
	PausedByLicense    int `json:"paused_by_license"`
	PausedByParent     int `json:"paused_by_parent"`
	ProbeDisconnected  int `json:"probe_disconnected"`
}

// Paused sums every paused state.
func (s SensorSummary) Paused() int {
	return s.PausedByUser + s.PausedByUserUntil + s.PausedByDependency + s.PausedBySchedule + s.PausedByLicense + s.PausedByParent
}

// StatusRow is one core in 'fleet status'.
type StatusRow struct {
	Core              string `json:"core"`
	Server            string `json:"server"`
	Health            string `json:"health"`
	Down              *int   `json:"down,omitempty"`
	DownAcknowledged  *int   `json:"down_acknowledged,omitempty"`
	Warning           *int   `json:"warning,omitempty"`
	Unusual           *int   `json:"unusual,omitempty"`
	Unknown           *int   `json:"unknown,omitempty"`
	Paused            *int   `json:"paused,omitempty"`
	ProbeDisconnected *int   `json:"probe_disconnected,omitempty"`
	Up                *int   `json:"up,omitempty"`
	Total             *int   `json:"total,omitempty"`
	Error             string `json:"error,omitempty"`
}

func ip(v int) *int { return &v }

// HealthState classifies GET /health: a 204 is ok, anything else carries the
// PRTG error code (SERVICE_UNAVAILABLE, LICENSE_INACTIVE, ...) or status.
func HealthState(body json.RawMessage) string {
	if e, ok := RequestError(body); ok {
		if code := errorCode(e.Message); code != "" {
			return code
		}
		if e.Status > 0 {
			return "HTTP " + strconv.Itoa(e.Status)
		}
		return "unreachable"
	}
	return "ok"
}

// errorCode pulls a PRTG error code ("LICENSE_INACTIVE") out of an error
// message that embeds the JSON error body.
func errorCode(msg string) string {
	i := strings.Index(msg, "{")
	if i < 0 {
		return ""
	}
	var e struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(msg[i:]), &e) == nil {
		return e.Code
	}
	return ""
}

// StatusRowFor builds one core's status row from its health and summary
// bodies. A failed summary keeps the row with an error and no counts.
func StatusRowFor(c Core, health, summary json.RawMessage) StatusRow {
	row := StatusRow{Core: c.Name, Server: c.Server, Health: HealthState(health)}
	if e, ok := RequestError(summary); ok {
		row.Error = e.Message
		return row
	}
	var s SensorSummary
	if err := json.Unmarshal(summary, &s); err != nil {
		row.Error = "unexpected sensor-status-summary body"
		return row
	}
	row.Down, row.DownAcknowledged, row.Warning, row.Unusual = ip(s.Down+s.DownPartial), ip(s.DownAcknowledged), ip(s.Warning), ip(s.Unusual)
	row.Unknown, row.Paused, row.ProbeDisconnected, row.Up, row.Total = ip(s.Unknown), ip(s.Paused()), ip(s.ProbeDisconnected), ip(s.Up), ip(s.Total)
	return row
}

// SortStatus orders cores by down count (worst first), then warning, then name.
func SortStatus(rows []StatusRow) {
	val := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Error != "") != (b.Error != "") {
			return a.Error != "" // unreadable cores first: they need attention
		}
		if val(a.Down) != val(b.Down) {
			return val(a.Down) > val(b.Down)
		}
		if val(a.Warning) != val(b.Warning) {
			return val(a.Warning) > val(b.Warning)
		}
		return a.Core < b.Core
	})
}

// Versions is GET /version.
type Versions struct {
	PRTGVersion string `json:"prtg_version"`
}

// License is GET /experimental/license (the key field is never read).
type License struct {
	Edition            string `json:"edition"`
	LicenseType        string `json:"license_type"`
	EndDate            string `json:"end_date"`
	MaintenanceEndDate string `json:"maintenance_end_date"`
	SensorCount        *int   `json:"sensor_count"`
	LicenseSensorCount *int   `json:"license_sensor_count"`
	OverusageEnabled   bool   `json:"overusage_sensors"`              // "overusage is enabled", not "over the limit"
	OverusageLimit     *int   `json:"license_sensor_overusage_limit"` // absolute sensor count when overusage is enabled
	UnlimitedSensors   bool   `json:"unlimited_sensors"`
	UseUnlimited       bool   `json:"use_unlimited_sensors"`
}

// SensorEntitlement is how many sensors the license allows: the absolute
// overusage limit when overusage is enabled, else the license's sensor count
// ("use sensor count if use_unlimited_sensors is false"), else the base count.
// ok is false for unlimited licenses or when no count is known.
func (l License) SensorEntitlement() (int, bool) {
	if l.UnlimitedSensors || l.UseUnlimited {
		return 0, false
	}
	if l.OverusageEnabled && l.OverusageLimit != nil && *l.OverusageLimit > 0 {
		return *l.OverusageLimit, true
	}
	for _, n := range []*int{l.SensorCount, l.LicenseSensorCount} {
		if n != nil && *n > 0 {
			return *n, true
		}
	}
	return 0, false
}

// licenseKeys maps the normalized spelling (lower case, no underscores) of
// every LicenseInfo field drift reads. The schema uses snake_case; Paessler's
// own example uses PascalCase, so both are accepted.
var licenseKeys = map[string]string{
	"edition": "edition", "licensetype": "license_type", "enddate": "end_date",
	"maintenanceenddate": "maintenance_end_date", "sensorcount": "sensor_count",
	"licensesensorcount": "license_sensor_count", "overusagesensors": "overusage_sensors",
	"licensesensoroverusagelimit": "license_sensor_overusage_limit",
	"unlimitedsensors":            "unlimited_sensors", "useunlimitedsensors": "use_unlimited_sensors",
}

// UnmarshalJSON accepts snake_case and PascalCase keys and fails when the
// body carries none of the license fields, so an unexpected shape is
// reported instead of read as "no findings".
func (l *License) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	norm := map[string]json.RawMessage{}
	for k, v := range raw {
		if canon, ok := licenseKeys[strings.ToLower(strings.ReplaceAll(k, "_", ""))]; ok {
			norm[canon] = v
		}
	}
	if len(norm) == 0 {
		return fmt.Errorf("no license fields in the response")
	}
	data, err := json.Marshal(norm)
	if err != nil {
		return err
	}
	type plain License
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*l = License(p)
	return nil
}

// DriftFinding is one hygiene problem on one core.
type DriftFinding struct {
	Core     string `json:"core"`
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Detail   string `json:"detail"`
	Object   string `json:"object,omitempty"`
}

// CoreFacts is what 'fleet drift' learned about one core.
type CoreFacts struct {
	Core            string
	Health          string
	Version         *Versions
	License         *License
	Probes          []Node
	Errors          map[string]string // check -> error message for an unreadable source
	SensorsInUse    *int              // sensor-status-summary total
	PausedByLicense *int              // sensors PRTG paused because the license is exceeded
	AuthFailed      bool              // the API key was rejected (HTTP 401)
	Unread          bool              // the core could not be reached at all
	ReadErr         string
}

// DriftOptions configure the thresholds.
type DriftOptions struct {
	Within time.Duration
	Now    time.Time
}

// CompareVersions compares dotted PRTG versions ("26.3.124.1727"); missing
// or non-numeric parts compare as zero.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(strings.TrimSpace(a), "."), strings.Split(strings.TrimSpace(b), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(strings.TrimSpace(pa[i]))
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(strings.TrimSpace(pb[i]))
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Drift compares every core against the fleet and the thresholds.
func Drift(facts []CoreFacts, o DriftOptions) []DriftFinding {
	newest := ""
	for _, f := range facts {
		if f.Version != nil && f.Version.PRTGVersion != "" && (newest == "" || CompareVersions(f.Version.PRTGVersion, newest) > 0) {
			newest = f.Version.PRTGVersion
		}
	}
	out := []DriftFinding{}
	add := func(core, sevr, check, detail, obj string) {
		out = append(out, DriftFinding{Core: core, Severity: sevr, Check: check, Detail: detail, Object: obj})
	}
	for _, f := range facts {
		if f.Unread {
			add(f.Core, "critical", "unreachable", "core could not be read: "+f.ReadErr, "")
			continue
		}
		if f.AuthFailed {
			add(f.Core, "critical", "auth", "the API key was rejected (HTTP 401); check this core's entry in PRTG_FLEET_KEYS", "")
		}
		if f.Health != "" && f.Health != "ok" {
			add(f.Core, "critical", "health", "application server health is "+f.Health, "")
		}
		if f.Version != nil && f.Version.PRTGVersion != "" && newest != "" && CompareVersions(f.Version.PRTGVersion, newest) < 0 {
			add(f.Core, "warning", "version", fmt.Sprintf("PRTG %s is behind the fleet's newest %s", f.Version.PRTGVersion, newest), "")
		}
		if l := f.License; l != nil {
			for _, d := range []struct{ check, label, value string }{
				{"license_end", "license", l.EndDate},
				{"maintenance_end", "maintenance", l.MaintenanceEndDate},
			} {
				t, ok := ParseTime(d.value)
				if !ok {
					continue
				}
				left := t.Sub(o.Now)
				switch {
				case left < 0:
					add(f.Core, "critical", d.check, fmt.Sprintf("%s ended %s (%d days ago)", d.label, t.Format("2006-01-02"), int(-left.Hours()/24)), "")
				case left <= o.Within:
					add(f.Core, "warning", d.check, fmt.Sprintf("%s ends %s (in %d days)", d.label, t.Format("2006-01-02"), int(math.Ceil(left.Hours()/24))), "")
				}
			}
			if allowed, ok := l.SensorEntitlement(); ok && f.SensorsInUse != nil && *f.SensorsInUse > allowed {
				add(f.Core, "warning", "sensor_overuse", fmt.Sprintf("%d sensors in use, %d allowed by the license", *f.SensorsInUse, allowed), "")
			}
		}
		if f.PausedByLicense != nil && *f.PausedByLicense > 0 {
			add(f.Core, "critical", "paused_by_license", fmt.Sprintf("%d sensors are paused because the license sensor count is exceeded", *f.PausedByLicense), "")
		}
		for _, p := range f.Probes {
			switch p.ProbeStatus {
			case "", "CONNECTED":
			case "DISCONNECTED", "BANNED":
				add(f.Core, "critical", "probe", "probe status is "+p.ProbeStatus, p.Name)
			default:
				// HELLO, INITIALIZING, WAITING, UPDATING, MANUAL, RESTART:
				// usually transient, worth a look but not an outage.
				add(f.Core, "warning", "probe", "probe status is "+p.ProbeStatus, p.Name)
			}
		}
		for _, check := range slices.Sorted(maps.Keys(f.Errors)) {
			add(f.Core, "info", check, "could not read: "+f.Errors[check], "")
		}
	}
	sevRank := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if sevRank[out[i].Severity] != sevRank[out[j].Severity] {
			return sevRank[out[i].Severity] < sevRank[out[j].Severity]
		}
		if out[i].Core != out[j].Core {
			return out[i].Core < out[j].Core
		}
		return out[i].Check < out[j].Check
	})
	return out
}

// HandoffRow is one change inside the handoff window.
type HandoffRow struct {
	Core     string `json:"core"`
	Event    string `json:"event"`
	At       string `json:"at"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Device   string `json:"device,omitempty"`
	Status   string `json:"status,omitempty"`
	LastDown string `json:"last_down,omitempty"`
	// Via is a recovery's path: "direct" (the down sample was the scan right
	// before the switch to up) or "after_gap" (paused, acknowledged or
	// rescheduled in between).
	Via     string `json:"via,omitempty"`
	By      string `json:"by,omitempty"`
	Auto    bool   `json:"auto,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handoffSkew tolerates core clocks running slightly ahead of this machine.
const handoffSkew = 10 * time.Minute

// Handoff buckets one core's sensors into the window [since, now]:
// new_alarm (in alarm now, status_since inside), recovered (up now, the
// switch to up inside the window, and a recorded last_down at or before that
// switch, whatever interrupted the way from down to up: via "direct" when the
// down sample was at most one scanning interval, plus clock skew, before the
// switch, else via "after_gap" (paused and resumed, acknowledged, or scanned
// on another interval in between); a brand-new sensor with no recorded down
// is not a recovery), acknowledged / paused (status_info time stamp inside;
// auto marks an acknowledgement a trigger made). Each sensor reports its
// current state only, so a sensor that went down and recovered twice appears
// once.
func Handoff(core string, nodes []Node, since, now time.Time) []HandoffRow {
	in := func(s string) (time.Time, bool) {
		t, ok := ParseTime(s)
		return t, ok && !t.Before(since) && !t.After(now.Add(handoffSkew))
	}
	rows := []HandoffRow{}
	for _, n := range nodes {
		base := HandoffRow{Core: core, ID: n.ID, Name: n.Name, Device: n.pathRef("REFERENCED_DEVICE").Name, Status: n.Status}
		switch n.Status {
		case "DOWN", "DOWN_PARTIAL", "WARNING", "UNUSUAL":
			if _, ok := in(n.StatusSince); ok {
				r := base
				r.Event, r.At, r.Message = "new_alarm", n.StatusSince, n.Message
				rows = append(rows, r)
			}
		case "UP":
			upAt, ok := in(n.StatusSince)
			downAt, wasDown := ParseTime(n.LastDown)
			if ok && wasDown && !downAt.After(upAt.Add(handoffSkew)) {
				r := base
				r.Event, r.At, r.LastDown, r.Via = "recovered", n.StatusSince, n.LastDown, "direct"
				interval := time.Duration(n.ScanningInterval * float64(time.Second))
				if upAt.Sub(downAt) > interval+handoffSkew {
					r.Via = "after_gap"
				}
				rows = append(rows, r)
			}
		case "DOWN_ACKNOWLEDGED", "PAUSED_BY_USER", "PAUSED_BY_USER_UNTIL":
			inf := n.info()
			at := inf.TimeStamp
			if at == "" {
				at = n.StatusSince
			}
			if _, ok := in(at); ok {
				r := base
				r.Event, r.At, r.Message = "acknowledged", at, inf.Message
				if n.Status != "DOWN_ACKNOWLEDGED" {
					r.Event = "paused"
				}
				if inf.User != nil {
					r.By = inf.User.Name
				}
				r.Auto = inf.AutoAcknowledged
				rows = append(rows, r)
			}
		}
	}
	return rows
}

// SortHandoff orders newest first, error rows last.
func SortHandoff(rows []HandoffRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Error != "") != (b.Error != "") {
			return a.Error == ""
		}
		ta, _ := ParseTime(a.At)
		tb, _ := ParseTime(b.At)
		if !ta.Equal(tb) {
			return ta.After(tb)
		}
		if a.Core != b.Core {
			return a.Core < b.Core
		}
		return a.Name < b.Name
	})
}

// SilencedRow is one object a person paused or acknowledged.
type SilencedRow struct {
	Core       string `json:"core"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Device     string `json:"device,omitempty"`
	Status     string `json:"status,omitempty"`
	By         string `json:"by,omitempty"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
	AgeDays    *int   `json:"age_days,omitempty"`
	Indefinite bool   `json:"indefinite"`
	Stale      bool   `json:"stale"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}

// SilencedStatuses are the states a person set.
var SilencedStatuses = map[string]bool{"PAUSED_BY_USER": true, "PAUSED_BY_USER_UNTIL": true, "DOWN_ACKNOWLEDGED": true}

// SilencedOptions filter the silenced list.
type SilencedOptions struct {
	OlderThan  time.Duration // an indefinite item older than this is stale
	Indefinite bool          // keep only items with no end time
	Now        time.Time
}

// Silenced turns one core's objects into silenced rows. Objects paused only
// because a parent is paused (PAUSED_BY_PARENT) and acknowledgements a
// trigger made (auto_acknowledged) are not listed.
func Silenced(core string, nodes []Node, o SilencedOptions) []SilencedRow {
	rows := []SilencedRow{}
	for _, n := range nodes {
		if !SilencedStatuses[n.Status] {
			continue
		}
		inf := n.info()
		if inf.AutoAcknowledged {
			continue // a trigger acknowledged it, not a person
		}
		r := SilencedRow{Core: core, ID: n.ID, Name: n.Name, Type: firstNonEmpty(n.Type, n.Kind), Status: n.Status,
			Device: n.pathRef("REFERENCED_DEVICE").Name, Message: firstNonEmpty(inf.Message, n.Message),
			Since: firstNonEmpty(inf.TimeStamp, n.StatusSince), Until: inf.ValidUntil}
		if inf.User != nil {
			r.By = inf.User.Name
		}
		if _, ok := ParseTime(r.Until); !ok {
			r.Until = ""
			r.Indefinite = n.Status != "PAUSED_BY_USER_UNTIL"
		}
		if o.Indefinite && !r.Indefinite {
			continue
		}
		if t, ok := ParseTime(r.Since); ok {
			d := int(o.Now.Sub(t).Hours() / 24)
			r.AgeDays = &d
			r.Stale = r.Indefinite && o.OlderThan > 0 && o.Now.Sub(t) >= o.OlderThan
		}
		if o.OlderThan > 0 && !r.Stale && r.Indefinite {
			// An indefinite item younger than the threshold is not news yet.
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// SortSilenced orders stale first, then oldest first.
func SortSilenced(rows []SilencedRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Error != "") != (b.Error != "") {
			return a.Error == ""
		}
		if a.Stale != b.Stale {
			return a.Stale
		}
		ta, okA := ParseTime(a.Since)
		tb, okB := ParseTime(b.Since)
		if okA != okB {
			return okA
		}
		if okA && !ta.Equal(tb) {
			return ta.Before(tb)
		}
		if a.Core != b.Core {
			return a.Core < b.Core
		}
		return a.Name < b.Name
	})
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
