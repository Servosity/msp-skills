package fortios

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// HealthRow is the red/amber/green operational summary for one device.
type HealthRow struct {
	Device        string   `json:"device"`
	Hostname      string   `json:"hostname,omitempty"`
	Model         string   `json:"model,omitempty"`
	Serial        string   `json:"serial,omitempty"`
	Version       string   `json:"version,omitempty"`
	HAMembers     int      `json:"ha_members"`
	HASync        string   `json:"ha_sync"`
	TunnelsDown   int      `json:"ipsec_tunnels_down"`
	TunnelsTotal  int      `json:"ipsec_tunnels_total"`
	SDWANDown     int      `json:"sdwan_members_down"`
	SDWANChecked  int      `json:"sdwan_members_checked"`
	CPUPercent    *float64 `json:"cpu_percent,omitempty"`
	MemPercent    *float64 `json:"memory_percent,omitempty"`
	CertsExpiring []string `json:"certificates_expiring"`
	Unreadable    []string `json:"unreadable,omitempty"`
	// NotEvaluated lists signals the device answered in a shape this CLI does
	// not recognize. The public FortiOS docs do not fix these response shapes,
	// so they are reported rather than graded (no guessing in either direction).
	NotEvaluated []string `json:"not_evaluated,omitempty"`
	Signals      []string `json:"signals"`
	Status       string   `json:"status"`
}

// HAChecksumsInSync compares the `checksum` object of every HA member in
// monitor system/ha-checksums. Returns "n/a" for standalone units and
// "unknown" when the payload carries no checksum objects to compare.
func HAChecksumsInSync(members []Obj) string {
	if len(members) < 2 {
		return "n/a"
	}
	var first string
	for i, m := range members {
		// Only a non-empty checksum object is evidence; null or {} on every
		// member would otherwise compare equal and read as in sync.
		c, ok := m["checksum"].(map[string]any)
		if !ok || !hasChecksumValue(c) {
			return "unknown"
		}
		b, _ := json.Marshal(c)
		if i == 0 {
			first = string(b)
		} else if string(b) != first {
			return "out-of-sync"
		}
	}
	return "in-sync"
}

func hasChecksumValue(c map[string]any) bool {
	for _, v := range c {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// IPsecDown counts tunnels whose phase-2 selectors are all down.
func IPsecDown(tunnels []Obj) (down, total int) {
	for _, t := range tunnels {
		sels := Objs(t["proxyid"])
		if len(sels) == 0 {
			continue
		}
		total++
		up := false
		for _, s := range sels {
			if strings.EqualFold(Str(s, "status"), "up") {
				up = true
			}
		}
		if !up {
			down++
		}
	}
	return down, total
}

// SDWANDown walks monitor virtual-wan/health-check results and counts member
// objects that carry a `status` field, and how many of those are not "up".
func SDWANDown(v any) (down, checked int) {
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			if s, ok := x["status"].(string); ok {
				checked++
				if !strings.EqualFold(s, "up") {
					down++
				}
				return
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
	return down, checked
}

// CurrentUsage reads results.<key>[0].current from monitor
// system/resource/usage (e.g. cpu, mem). ok=false when absent.
func CurrentUsage(results Obj, key string) (float64, bool) {
	arr := Objs(results[key])
	if len(arr) == 0 {
		return 0, false
	}
	return Num(arr[0], "current")
}

// ExpiringCerts lists non-built-in, non-CA certificates whose valid_to is
// within `within` of now (or already past).
func ExpiringCerts(certs []Obj, now time.Time, within time.Duration) []string {
	out := []string{}
	for _, c := range certs {
		if b, _ := c["is_built_in"].(bool); b {
			continue
		}
		if ca, _ := c["is_ca"].(bool); ca {
			continue
		}
		to, ok := Num(c, "valid_to")
		if !ok || to <= 0 {
			continue
		}
		at := time.Unix(int64(to), 0)
		if at.Sub(now) <= within {
			out = append(out, fmt.Sprintf("%s (%s)", Str(c, "name"), at.UTC().Format("2006-01-02")))
		}
	}
	return out
}

// UndatedCerts names the non-built-in, non-CA certificates ExpiringCerts could
// not judge because valid_to is missing or not a Unix time, so the caller can
// report them as not evaluated instead of reading the list as all clear.
func UndatedCerts(certs []Obj) []string {
	var out []string
	for _, c := range certs {
		if b, _ := c["is_built_in"].(bool); b {
			continue
		}
		if ca, _ := c["is_ca"].(bool); ca {
			continue
		}
		if to, ok := Num(c, "valid_to"); !ok || to <= 0 {
			out = append(out, Str(c, "name"))
		}
	}
	return out
}

// Grade fills Signals and Status from the collected values.
func (h *HealthRow) Grade(cpuWarn, memWarn float64) {
	h.Signals = []string{}
	status := Pass
	flag := func(s, msg string) {
		h.Signals = append(h.Signals, msg)
		status = Worst(status, s)
	}
	if h.HASync == "out-of-sync" {
		flag(Fail, "HA configuration out of sync")
	}
	if h.TunnelsDown > 0 {
		flag(Warn, fmt.Sprintf("%d of %d IPsec tunnel(s) down", h.TunnelsDown, h.TunnelsTotal))
	}
	if h.SDWANDown > 0 {
		flag(Warn, fmt.Sprintf("%d SD-WAN member(s) not up", h.SDWANDown))
	}
	if h.CPUPercent != nil && *h.CPUPercent >= cpuWarn {
		flag(Warn, fmt.Sprintf("CPU %.0f%%", *h.CPUPercent))
	}
	if h.MemPercent != nil && *h.MemPercent >= memWarn {
		flag(Warn, fmt.Sprintf("memory %.0f%%", *h.MemPercent))
	}
	if len(h.CertsExpiring) > 0 {
		flag(Warn, fmt.Sprintf("%d certificate(s) expiring", len(h.CertsExpiring)))
	}
	// A signal that could not be read was not checked: never report PASS for it.
	for _, u := range h.Unreadable {
		flag(Unknown, "not checked: "+u)
	}
	h.Status = status
}
