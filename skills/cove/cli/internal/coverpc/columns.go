// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package coverpc

import (
	"fmt"
	"strconv"
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

// BadSessionStatuses are the F00 values a triage sweep should surface. 7
// (NotStarted) is handled separately because never-run devices carry no
// session timestamp.
var BadSessionStatuses = map[int]bool{
	2:  true, // Failed
	3:  true, // Aborted
	6:  true, // Interrupted
	8:  true, // CompletedWithErrors
	10: true, // OverQuota
}

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
// commands request — the superset the novel commands decode.
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
// map; ok is false when the column is absent or unparseable.
func SettingInt(settings map[string]string, code string) (int64, bool) {
	raw, ok := settings[code]
	if !ok || raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
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
