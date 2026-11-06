// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: typed views of the XAPI bodies the fleet commands read.

package fleet

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"threecx-pp-cli/internal/cliutil"
)

// SystemStatus is GET /SystemStatus (fields the fleet commands use).
type SystemStatus struct {
	FQDN                            string `json:"FQDN"`
	Version                         string `json:"Version"`
	ProductCode                     string `json:"ProductCode"`
	LicenseActive                   *bool  `json:"LicenseActive"`
	ExpirationDate                  string `json:"ExpirationDate"`
	MaintenanceExpiresAt            string `json:"MaintenanceExpiresAt"`
	ResellerName                    string `json:"ResellerName"`
	LastBackupDateTime              string `json:"LastBackupDateTime"`
	BackupScheduled                 *bool  `json:"BackupScheduled"`
	AutoUpdateEnabled               *bool  `json:"AutoUpdateEnabled"`
	LastCheckForUpdates             string `json:"LastCheckForUpdates"`
	LastSuccessfulUpdate            string `json:"LastSuccessfulUpdate"`
	Firmware3CXOutdated             *bool  `json:"Firmware3CXOutdated"`
	TrunksRegistered                *int   `json:"TrunksRegistered"`
	TrunksTotal                     *int   `json:"TrunksTotal"`
	ExtensionsRegistered            *int   `json:"ExtensionsRegistered"`
	ExtensionsTotal                 *int   `json:"ExtensionsTotal"`
	UserExtensions                  *int   `json:"UserExtensions"`
	MaxUserExtensions               *int   `json:"MaxUserExtensions"`
	MaxSimCalls                     *int   `json:"MaxSimCalls"`
	CallsActive                     *int   `json:"CallsActive"`
	TotalDiskSpace                  *int64 `json:"TotalDiskSpace"`
	FreeDiskSpace                   *int64 `json:"FreeDiskSpace"`
	HasNotRunningServices           *bool  `json:"HasNotRunningServices"`
	HasUnregisteredSystemExtensions *bool  `json:"HasUnregisteredSystemExtensions"`
	RecordingStopped                *bool  `json:"RecordingStopped"`
	RecordingQuotaReached           *bool  `json:"RecordingQuotaReached"`
	VoicemailStopped                *bool  `json:"VoicemailStopped"`
	VoicemailQuotaReached           *bool  `json:"VoicemailQuotaReached"`
}

// LicenseStatus is GET /LicenseStatus. LicenseKey is deliberately absent.
type LicenseStatus struct {
	CompanyName          string `json:"CompanyName"`
	ProductCode          string `json:"ProductCode"`
	LicenseActive        *bool  `json:"LicenseActive"`
	ExpirationDate       string `json:"ExpirationDate"`
	MaintenanceExpiresAt string `json:"MaintenanceExpiresAt"`
	MaxSimCalls          *int   `json:"MaxSimCalls"`
	ResellerName         string `json:"ResellerName"`
	Version              string `json:"Version"`
}

// UpdateList is GET /GetUpdates(). Every boolean and number a 3CX body
// carries is nullable (vendor-docs/3cx-swagger.yaml), so each is a pointer
// here and in every type below: nil is "not reported", never false or 0
// (threecx: unknown is never healthy; TestNoVendorFactDecodesNullAsAValue).
type UpdateList struct {
	IsMaintananceExpired *bool  `json:"IsMaintananceExpired"` // sic: 3CX spelling
	UpdatesRunning       *bool  `json:"UpdatesRunning"`
	LastSuccessfulUpdate string `json:"LastSuccessfulUpdate"`
	Entries              []struct {
		Name          string `json:"Name"`
		LocalVersion  string `json:"LocalVersion"`
		ServerVersion string `json:"ServerVersion"`
		OutOfDate     *bool  `json:"OutOfDate"`
		Ignore        *bool  `json:"Ignore"`
	} `json:"Entries"`
}

// BackupSettings is GET /Backups/Pbx.GetBackupSettings().
type BackupSettings struct {
	ScheduleEnabled *bool `json:"ScheduleEnabled"`
	Rotation        *int  `json:"Rotation"`
	Schedule        *struct {
		ScheduleType string `json:"ScheduleType"`
		Time         string `json:"Time"`
		Day          string `json:"Day"`
	} `json:"Schedule"`
	Contents *struct {
		Recordings    *bool `json:"Recordings"`
		VoiceMails    *bool `json:"VoiceMails"`
		CallHistory   *bool `json:"CallHistory"`
		EncryptBackup *bool `json:"EncryptBackup"`
	} `json:"Contents"`
}

// Backup is one row of GET /Backups.
type Backup struct {
	FileName     string `json:"FileName"`
	CreationTime string `json:"CreationTime"`
	Size         *int64 `json:"Size"`
}

// Trunk is one row of GET /Trunks with status fields only.
type Trunk struct {
	ID                        *int    `json:"Id"`
	Number                    string  `json:"Number"`
	IsOnline                  *bool   `json:"IsOnline"`
	ConfigurationIssue        *string `json:"ConfigurationIssue"` // nullable: nil is "not reported", not "no issue"
	Direction                 string  `json:"Direction"`
	SimultaneousCalls         *int    `json:"SimultaneousCalls"`
	CertificateExpirationDate string  `json:"CertificateExpirationDate"`
	Gateway                   *struct {
		Name string `json:"Name"`
		Host string `json:"Host"`
	} `json:"Gateway"`
}

// Items returns the rows of a body: an OData collection's "value" array, a
// bare array, or the body itself as a single row. "@odata.*" keys are dropped.
func Items(body json.RawMessage) []map[string]any {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		return rowsOf(arr)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		var scalar any
		if json.Unmarshal(body, &scalar) == nil && scalar != nil {
			return []map[string]any{{"value": scalar}}
		}
		return []map[string]any{}
	}
	if raw, ok := obj["value"]; ok {
		if err := json.Unmarshal(raw, &arr); err == nil {
			return rowsOf(arr)
		}
	}
	return rowsOf([]json.RawMessage{body})
}

func rowsOf(arr []json.RawMessage) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, raw := range arr {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			var scalar any
			if json.Unmarshal(raw, &scalar) == nil {
				out = append(out, map[string]any{"value": scalar})
			}
			continue
		}
		for k := range row {
			if strings.HasPrefix(k, "@odata.") {
				delete(row, k)
			}
		}
		out = append(out, row)
	}
	return out
}

// SecretField reports a property name that looks like a credential (the one
// rule in cliutil, shared with response redaction and the local store).
func SecretField(name string) bool { return cliutil.SecretFieldName(name) }

// Redacted replaces credential-bearing values with "[redacted]".
const Redacted = cliutil.RedactedValue

// Redact walks a decoded JSON value and masks credential fields in place
// (cliutil.RedactSecretFieldsStrict: credential names, numbers under them,
// Concealed, name/value and unnamed scalar Value records), since fleet exec
// reads any XAPI path.
func Redact(v any) any { return cliutil.RedactSecretFieldsStrict(v) }

// RedactJSON masks credential fields in a raw JSON body.
func RedactJSON(body json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return json.Marshal(Redact(v))
}

// ParseTime reads an XAPI timestamp. Empty and year-1 placeholders are unset.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() <= 1 {
				return time.Time{}, false
			}
			return t, true
		}
	}
	return time.Time{}, false
}

// DaysUntil is whole days from now to t (negative once t has passed).
func DaysUntil(t, now time.Time) int {
	d := t.Sub(now)
	days := int(d / (24 * time.Hour))
	if d < 0 && d%(24*time.Hour) != 0 {
		days--
	}
	return days
}

// CompareVersions compares dotted numeric versions like 20.0.10.1479.
// Non-numeric or missing parts compare as 0.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := part(pa, i), part(pb, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func part(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(p[i]))
	if err != nil {
		return 0
	}
	return n
}
