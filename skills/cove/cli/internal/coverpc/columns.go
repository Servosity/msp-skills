// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package coverpc

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file embeds the Management Console column-code legend from
// documentation.n-able.com (Cove "Management Console column codes for API").
// EnumerateAccountStatistics addresses every statistic by these codes:
// I-codes are device/profile/company fields, F-codes are per-data-source
// session statistics, D-codes are data sources. Combined codes concatenate
// data source + stat, e.g. D19F20 = M365 Exchange "Last Session User
// Mailboxes Count". D9 is the synthetic "Total" data source.

// SessionStatusNames maps the F00 "Last Session Status" enum to names.
var SessionStatusNames = map[int]string{
	1:  "InProcess",
	2:  "Failed",
	3:  "Aborted",
	5:  "Completed",
	6:  "Interrupted",
	7:  "NotStarted",
	8:  "CompletedWithErrors",
	9:  "InProgressWithFaults",
	10: "OverQuota",
	11: "NoSelection",
	12: "Restarted",
}

// BadSessionStatuses are the adverse F00 values: any of them means the device
// is not protected the way the operator expects. 7 (NotStarted) is handled
// separately because never-run devices carry no session timestamp.
var BadSessionStatuses = map[int]bool{
	2:  true, // Failed
	3:  true, // Aborted
	6:  true, // Interrupted
	8:  true, // CompletedWithErrors
	9:  true, // InProgressWithFaults
	10: true, // OverQuota
	11: true, // NoSelection: nothing is selected for backup
}

// StatusClass is the one classification every command applies to an F00
// value. Only StatusSucceeded is positive evidence of a good backup; a status
// that is not reported, or that this build cannot name, is StatusUnknown and
// is never read as healthy.
type StatusClass string

const (
	StatusUnknown   StatusClass = "unknown"
	StatusSucceeded StatusClass = "succeeded" // 5 Completed
	StatusRunning   StatusClass = "running"   // 1 InProcess, 12 Restarted
	StatusAdverse   StatusClass = "adverse"   // BadSessionStatuses
	StatusNeverRun  StatusClass = "never_run" // 7 NotStarted
)

// ClassifyStatus classifies an F00 value; reported is false when the column
// was absent or unparseable.
func ClassifyStatus(code int64, reported bool) StatusClass {
	if !reported {
		return StatusUnknown
	}
	switch {
	case code == 5:
		return StatusSucceeded
	case code == 7:
		return StatusNeverRun
	case code >= 0 && code <= math.MaxInt32 && BadSessionStatuses[int(code)]:
		return StatusAdverse
	case code == 1 || code == 12:
		return StatusRunning
	}
	return StatusUnknown
}

// Bad reports whether the class is adverse or never-run.
func (s StatusClass) Bad() bool { return s == StatusAdverse || s == StatusNeverRun }

// DataSourceNames maps D-codes to data source names.
var DataSourceNames = map[string]string{
	"D1":  "Files and Folders",
	"D2":  "System State",
	"D3":  "MsSql",
	"D4":  "VssExchange",
	"D5":  "Microsoft 365 SharePoint",
	"D6":  "NetworkShares",
	"D7":  "VssSystemState",
	"D8":  "VMware Virtual Machines",
	"D9":  "Total",
	"D10": "VssMsSql",
	"D11": "VssSharePoint",
	"D12": "Oracle",
	"D14": "Hyper-V",
	"D15": "MySql",
	"D16": "Virtual Disaster Recovery",
	"D17": "Bare Metal Restore",
	"D19": "Microsoft 365 Exchange",
	"D20": "Microsoft 365 OneDrive",
	"D23": "Microsoft 365 Teams",
}

// Well-known column codes used by the hand-built commands.
const (
	ColDeviceID      = "I0"  // Device ID
	ColDeviceName    = "I1"  // Device name
	ColCustomer      = "I8"  // Customer (partner) name
	ColProduct       = "I10" // Product name
	ColUsedStorage   = "I14" // Used storage (bytes)
	ColComputerName  = "I18" // Computer name
	ColOSType        = "I32" // OS type: 1 workstation, 2 server
	ColSKU           = "I57" // Stock Keeping Unit (current month)
	ColPrevSKU       = "I58" // SKU of the previous month
	ColActiveSources = "I78" // Active data sources, e.g. "D1,D2"

	ColTotalLastStatus    = "D9F00" // Total: last session status (enum)
	ColTotalLastErrors    = "D9F06" // Total: last session errors count
	ColTotalLastSuccessTS = "D9F09" // Total: last successful session timestamp (unix)
	ColTotalLastSessionTS = "D9F15" // Total: last session timestamp (unix)

	ColM365ExchangeLicenses = "D19F13" // M365 Exchange: license items count
	ColM365ExchangeMailbox  = "D19F20" // M365 Exchange: user mailboxes count
	ColM365OneDriveLicenses = "D20F13" // M365 OneDrive: license items count
)

// SnapshotColumns is the default column vector the snapshot and fleet
// commands request: the superset the novel commands decode.
var SnapshotColumns = []string{
	ColDeviceID, ColDeviceName, ColCustomer, ColProduct, ColUsedStorage,
	ColComputerName, ColOSType, ColSKU, ColPrevSKU, ColActiveSources,
	ColTotalLastStatus, ColTotalLastErrors, ColTotalLastSuccessTS, ColTotalLastSessionTS,
	ColM365ExchangeLicenses, ColM365ExchangeMailbox, ColM365OneDriveLicenses,
}

// StatusName renders an F00 value as its enum name, falling back to the
// numeric form for unknown values.
func StatusName(code int) string {
	if name, ok := SessionStatusNames[code]; ok {
		return name
	}
	return fmt.Sprintf("Status%d", code)
}

// SettingInt parses an integer-typed column value from a flattened Settings
// map; ok is false when the column is absent or is not exactly an integer.
func SettingInt(settings map[string]string, code string) (int64, bool) {
	raw, ok := settings[code]
	if !ok {
		return 0, false
	}
	return parseInteger(raw)
}

// SettingCount reads a count or byte size (used storage, seats, errors): a
// negative value is not a measurement, so it is not reported either.
func SettingCount(settings map[string]string, code string) (int64, bool) {
	n, ok := SettingInt(settings, code)
	if !ok || n < 0 {
		return 0, false
	}
	return n, true
}

// exactDecimal is the only number grammar a Cove column value is read in:
// optional sign, decimal digits, an optional fraction and an optional
// decimal exponent. No hex, octal, binary, underscores, NaN or Inf.
var exactDecimal = regexp.MustCompile(`^([+-]?)([0-9]*)(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

// parseInteger is the one exact numeric parser for Cove column values. It
// reads a decimal integer, also in the integral decimal or exponent forms a
// JSON encoder may produce ("2e9", "1.792e9", "2000000000.0"), by exact digit
// arithmetic: no float64 conversion, so nothing is ever rounded. A value that
// is not exactly an integer (a fraction such as "4.9999999999999999"),
// outside the int64 range, or not a number at all is not reported (ok false),
// never rounded and never zero.
func parseInteger(raw string) (int64, bool) {
	m := exactDecimal.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil || m[2]+m[3] == "" {
		return 0, false
	}
	sign, intPart, fracPart, expPart := m[1], m[2], m[3], m[4]
	digits := strings.TrimLeft(intPart+fracPart, "0")
	if digits == "" {
		return 0, true // every spelling of zero
	}
	// value = digits * 10^shift, with digits free of trailing zeros.
	exp := int64(0)
	if expPart != "" {
		e, err := strconv.ParseInt(expPart, 10, 32)
		if err != nil {
			return 0, false
		}
		exp = e
	}
	trimmed := strings.TrimRight(digits, "0")
	shift := exp - int64(len(fracPart)) + int64(len(digits)-len(trimmed))
	if shift < 0 {
		return 0, false // a nonzero fractional part remains
	}
	if int64(len(trimmed))+shift > 19 {
		return 0, false // beyond int64
	}
	n, err := strconv.ParseInt(sign+trimmed+strings.Repeat("0", int(shift)), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ColumnText is the one shared parser for a Cove column or field value. A
// string is kept as written; a number keeps its exact decimal form (an
// integral value in e-notation becomes a plain integer); a list keeps every
// element, joined by ","; null is not reported (ok false). Callers decode with
// json.Decoder.UseNumber (Rows does) so numbers arrive exact.
func ColumnText(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case json.Number:
		return normalizeNumber(t.String()), true
	case float64:
		// Only reachable for a value decoded without UseNumber. A float64
		// at or past 2^53 (or NaN/Inf) may not hold the value Cove sent, so it
		// is not reported rather than rendered as a different number.
		if math.IsNaN(t) || math.IsInf(t, 0) || math.Abs(t) >= 1<<53 {
			return "", false
		}
		return normalizeNumber(strconv.FormatFloat(t, 'f', -1, 64)), true
	case bool:
		return strconv.FormatBool(t), true
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			text, ok := ColumnText(e)
			if !ok {
				if e == nil {
					continue // a null element carries nothing
				}
				// A value that cannot be rendered exactly would silently
				// change the list: the whole column is not reported.
				return "", false
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ","), true
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// normalizeNumber renders an exact integer in plain decimal form and keeps
// any other number (a fraction, an out-of-range value) exactly as written, so
// parseInteger later reports it as not an integer instead of a rounded one.
func normalizeNumber(raw string) string {
	if n, ok := parseInteger(raw); ok {
		return strconv.FormatInt(n, 10)
	}
	return raw
}

// RowInt reads an integer field (AccountId, PartnerId, ...) of a decoded row
// through ColumnText.
func RowInt(row map[string]any, key string) (int64, bool) {
	text, ok := ColumnText(row[key])
	if !ok {
		return 0, false
	}
	return parseInteger(text)
}

// RowText reads a field of a decoded row as text through ColumnText; ok is
// false when it is absent or null.
func RowText(row map[string]any, key string) (string, bool) {
	return ColumnText(row[key])
}

// SettingTime parses a unix-seconds column value into a time. ok is false
// for absent, zero, or unparseable values.
func SettingTime(settings map[string]string, code string) (time.Time, bool) {
	n, ok := SettingInt(settings, code)
	if !ok || n <= 0 {
		return time.Time{}, false
	}
	return time.Unix(n, 0).UTC(), true
}
