// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (NOT generated): the one shared reader of a maintenance action's
// outcome. Every command that counts, filters or shows an action outcome scans
// immyActionOutcomeColumns and parses them here, so no command can read the
// outcome from the wrong field.
//
// The vendor's documented shape (GetMaintenanceActionResponse in the vendored
// swagger, BezaluLLC/Bezalu.ImmyBot.Client Ref/swagger.json @7b1b55c):
//   - result / resultName: MaintenanceActionResult, the OUTCOME. Pending = 0,
//     Success = 1, Failed = 2, Cancelled = 3, Indeterminable = 4, Resolved = 5;
//     "the string or integer value may be used". resultName is readOnly and
//     nullable, so the integer may arrive alone.
//   - status / statusName: MaintenanceActionStatus, the LIFECYCLE. NotStarted = 0
//     ... Complete = 4 ... Skipped = 7, PendingExecution = 8. It never says
//     Failed or Success; it stays a signal only for rows that put the
//     conventional failure vocabulary there.
//   - resultReasonMessage / resultReason: the cause of the result.
//   - reason: MaintenanceActionReason, why the action was planned.
//
// pp:data-source local

package cli

import (
	"database/sql"
	"strconv"
	"strings"
)

var (
	immyActionResultNames = []string{"Pending", "Success", "Failed", "Cancelled", "Indeterminable", "Resolved"}
	immyActionStatusNames = []string{"NotStarted", "Downloading", "PerformingAction", "DeterminingResult",
		"Complete", "WaitingForPrerequisite", "DeterminingAction", "Skipped", "PendingExecution"}
	immyActionResultReasonNames = []string{"NoLicenseAvailable", "SoftwareNotFound", "DetectionActionFailed",
		"TaskNotFound", "TaskNotCompliant", "TaskCompliant", "TaskSetIsMissing", "TaskTestIsMissing",
		"TaskGetIsMissing", "SoftwareConfigurationTaskMissing", "TaskModeIsMissing", "DeviceOffline",
		"RebootsSuppressed"}
	immyActionReasonNames = []string{"SoftwareMissing", "UpdateAvailable", "UpToDate", "NotAssigned", "TestFailed",
		"SoftwareDetected", "OlderVersionRequired", "NoApplicableSoftwareVersion", "NoAction", "Undetermined",
		"TaskEnforce", "TaskAudit", "TaskMonitor", "TaskNotCompliant", "TaskCompliant", "DeviceOffline",
		"RepairTriggered"}
)

// immyActionFrom is the FROM/WHERE fragment every action reader uses: all three
// stores that mirror GetMaintenanceActionResponse rows. One action can be
// mirrored by more than one of them, so readers select the id first and keep
// the first row per id (immyActionSeen); the dx store's row sorts first.
const immyActionFrom = `FROM resources
				WHERE resource_type IN ('maintenance-actions','maintenance-actions-maintenance-item','maintenance-actions-version')`

// immyActionOrder sorts the dx store's copy of an action first.
const immyActionOrder = ` ORDER BY CASE resource_type WHEN 'maintenance-actions' THEN 0 ELSE 1 END`

// immyActionSeen reports whether an action id was already read, recording it.
// A row without an id is never a duplicate.
func immyActionSeen(seen map[string]struct{}, id sql.NullString) bool {
	if !id.Valid || id.String == "" {
		return false
	}
	if _, ok := seen[id.String]; ok {
		return true
	}
	seen[id.String] = struct{}{}
	return false
}

// immyActionOutcomeColumns is the SELECT fragment every outcome reader uses, in
// the order immyActionOutcomeRow.dest scans it.
const immyActionOutcomeColumns = `json_extract(data,'$.resultName'), json_extract(data,'$.result'),
				       json_extract(data,'$.statusName'), json_extract(data,'$.status'),
				       json_extract(data,'$.resultReasonMessage'), json_extract(data,'$.resultReason'),
				       json_extract(data,'$.reason')`

// immyActionOutcomeRow holds the scanned outcome columns of one action row.
type immyActionOutcomeRow struct {
	resultName, result, statusName, status, resultReasonMessage, resultReason, reason sql.NullString
}

// dest returns the scan destinations for immyActionOutcomeColumns.
func (r *immyActionOutcomeRow) dest() []any {
	return []any{&r.resultName, &r.result, &r.statusName, &r.status,
		&r.resultReasonMessage, &r.resultReason, &r.reason}
}

// outcome parses the scanned columns.
func (r *immyActionOutcomeRow) outcome() immyActionOutcome {
	return immyParseActionOutcome(nullStr(r.resultName), nullStr(r.result), nullStr(r.statusName), nullStr(r.status),
		nullStr(r.resultReasonMessage), nullStr(r.resultReason), nullStr(r.reason))
}

// immyActionOutcome is one action's outcome, read from every signal the row
// carries.
type immyActionOutcome struct {
	// Label is what a command shows: the failing signal's name when the action
	// failed, otherwise the result name, otherwise the lifecycle name.
	Label string
	// Result is the MaintenanceActionResult name ("" when the row has none).
	Result string
	// Lifecycle is the MaintenanceActionStatus name, or the raw status text.
	Lifecycle string
	// Cause is resultReasonMessage, else the resultReason name, else the
	// planning reason name.
	Cause     string
	Failed    bool
	Succeeded bool
	signals   []string
}

// immyEnumName resolves a vendor enum value sent as its name or its integer
// ("the string or integer value may be used") to the canonical name. A value
// outside the enum is returned trimmed and unchanged, so a legacy or unknown
// spelling is still visible to the failure vocabulary.
func immyEnumName(raw string, names []string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n >= 0 && n < len(names) {
			return names[n]
		}
		return s
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f == float64(int(f)) {
		if n := int(f); n >= 0 && n < len(names) {
			return names[n]
		}
		return s
	}
	for _, name := range names {
		if strings.EqualFold(name, s) {
			return name
		}
	}
	return s
}

// immyIsSuccessStatus reports whether an outcome name reads as a success.
func immyIsSuccessStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "succeeded":
		return true
	}
	return false
}

// immyParseActionOutcome classifies one maintenance action. Any failure signal
// wins: the result (name or integer) and the lifecycle field are all checked
// against the failure vocabulary, and a row is a success only when no signal
// reads as a failure.
func immyParseActionOutcome(resultName, result, statusName, status, resultReasonMessage, resultReason, reason string) immyActionOutcome {
	rn := immyEnumName(resultName, immyActionResultNames)
	rv := immyEnumName(result, immyActionResultNames)
	sn := immyEnumName(statusName, immyActionStatusNames)
	sv := immyEnumName(status, immyActionStatusNames)
	o := immyActionOutcome{
		Result:    firstNonEmpty(rn, rv),
		Lifecycle: firstNonEmpty(sn, sv),
		Cause: firstNonEmpty(strings.TrimSpace(resultReasonMessage),
			immyEnumName(resultReason, immyActionResultReasonNames),
			immyEnumName(reason, immyActionReasonNames)),
		signals: []string{rn, rv, sn, sv,
			strings.TrimSpace(resultName), strings.TrimSpace(result),
			strings.TrimSpace(statusName), strings.TrimSpace(status)},
	}
	for _, s := range []string{rn, rv, sn, sv} {
		if immyIsFailureStatus(s) {
			o.Failed = true
			if o.Label == "" {
				o.Label = s
			}
		}
	}
	if !o.Failed {
		for _, s := range []string{rn, rv, sn, sv} {
			if immyIsSuccessStatus(s) {
				o.Succeeded = true
				break
			}
		}
		o.Label = firstNonEmpty(o.Result, o.Lifecycle)
	}
	return o
}

// Matches reports whether an explicit --status value names this action's
// outcome or its lifecycle, in either the name or the integer form.
func (o immyActionOutcome) Matches(want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, s := range o.signals {
		if s != "" && strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// Selected reports whether an action is in a command's selection: the actions
// an explicit --status names, else (no --status) the failures. Selection is not
// failure: --status Success selects successes, so a caller counts what it
// selected and, separately, only the selected actions with Failed set.
func (o immyActionOutcome) Selected(wantStatus string) bool {
	if strings.TrimSpace(wantStatus) != "" {
		return o.Matches(wantStatus)
	}
	return o.Failed
}
