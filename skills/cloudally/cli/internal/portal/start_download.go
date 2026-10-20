// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
)

// startOperation creates a restore, a download, or an item deletion,
// depending on its type field. This package only ever sends EXPORT.
const startOperation = "startRestoreOrDownload"

// jobTypeExport is the only job type this package will ask for.
//
// The same endpoint also accepts "RESTORE", which writes data back into the
// live mailbox, and "DELETEITEMS", which deletes from the backup. Neither is
// reachable through this package: the type is written here, never taken from
// a caller, so no argument or config can turn a download into either one.
const jobTypeExport = "EXPORT"

// StorageCloudAllyS3 keeps the archive in CloudAlly's own storage, which is
// what makes it downloadable from the portal's Jobs page. The portal labels
// this target "For download".
const StorageCloudAllyS3 = "CLOUDALLY_S3"

// Archive formats. Zip is the default; PST splits large mailboxes unless a
// single file is preferred.
const (
	FormatZip       = "zip"
	FormatPST       = "pst"
	FormatSinglePST = "single-pst"
)

// DownloadRequest describes one archive to build. It deliberately has no
// type field: see jobTypeExport.
type DownloadRequest struct {
	// Entity names the task and mailbox to archive.
	Entity Entity
	// Source is the service to archive, for example MS365 for a whole
	// mailbox or O365MAIL for just its mail.
	Source string
	// DateRaw is the backup to archive, as the compact yyyyMMddHHmmss
	// integer the calendar reports.
	DateRaw int64
	// Format is FormatZip, FormatPST or FormatSinglePST.
	Format string
	// JobLabel is an optional free-text label stored on the job.
	JobLabel string
	// IncludeDeleted and IncludeLegalHold widen what goes in the archive.
	IncludeDeleted   bool
	IncludeLegalHold bool
}

// ErrNeedsPortalRedirect is returned when the portal answers a create with
// a redirect to an interactive step. The job was not created, so the row is
// a clean refusal rather than an unknown outcome.
var ErrNeedsPortalRedirect = errors.New("portal: the download needs interactive authorisation in the portal before it can run")

// startResponse is what the create call returns. A redirect means the portal
// wants the operator to complete something interactively, which a headless
// caller cannot do.
type startResponse struct {
	NeedRedirect bool   `json:"needRedirect"`
	RedirectURL  string `json:"redirectUrl"`
}

// StartDownload creates one download (export) job and reports nothing but
// success: the portal does not return the new job's id. Find it afterwards
// with DownloadJobs, matching on entity, source and snapshot date.
func (s *Session) StartDownload(ctx context.Context, req DownloadRequest) error {
	switch {
	case strings.TrimSpace(req.Entity.TaskID) == "":
		return fmt.Errorf("portal: download needs a backup task id")
	case strings.TrimSpace(req.Entity.AccountID) == "":
		return fmt.Errorf("portal: download needs an entity")
	case strings.TrimSpace(req.Source) == "":
		return fmt.Errorf("portal: download needs a source")
	case req.DateRaw <= 0:
		return fmt.Errorf("portal: download needs a backup date")
	}
	// The date must name a real backup; a bad one would otherwise be
	// accepted and produce an empty or wrong archive.
	if _, err := ParseSnapshotDate(req.DateRaw); err != nil {
		return fmt.Errorf("portal: download date %d: %w", req.DateRaw, err)
	}

	body := map[string]any{
		"type":    jobTypeExport,
		"account": req.Entity.AccountID,
		// The portal sets targetAccount to the same entity for a download.
		"targetAccount": req.Entity.AccountID,
		"source":        req.Source,
		"taskId":        req.Entity.TaskID,
		"date":          req.DateRaw,
		"storageType":   StorageCloudAllyS3,
		// The front-end always sends this; it is the browser's offset, so
		// a server running on UTC is the honest value for a CLI.
		"timeZoneOffset": 0,
	}
	if extID := strings.TrimSpace(req.Entity.ExtID); extID != "" {
		body["extid"] = extID
	}
	if label := strings.TrimSpace(req.JobLabel); label != "" {
		body["jobLabel"] = label
	}
	switch req.Format {
	case FormatPST:
		body["downloadToPst"] = true
	case FormatSinglePST:
		body["downloadToPst"] = true
		body["preferSinglePST"] = "true"
	case FormatZip, "":
		body["downloadToPst"] = false
	default:
		return fmt.Errorf("portal: unknown archive format %q", req.Format)
	}
	// The portal sends these as the strings "true" when set, and omits
	// them otherwise, so that shape is matched exactly.
	if req.IncludeDeleted {
		body["includeDeleted"] = "true"
	}
	if req.IncludeLegalHold {
		body["includeLegalHold"] = "true"
	}

	raw, err := s.Call(ctx, startOperation, body)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var resp startResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		// A successful envelope with an unfamiliar body is not a failure.
		return nil
	}
	if resp.NeedRedirect {
		return ErrNeedsPortalRedirect
	}
	return nil
}

// DownloadJob is one export job as getRestoreDownloadStatus reports it.
// This is the only reliable status source: a running job disappears from the
// documented backup-tasks list within a couple of minutes.
type DownloadJob struct {
	JobID        string         `json:"taskId"`
	Type         string         `json:"type"`
	Status       string         `json:"status"`
	Account      string         `json:"account"`
	Source       string         `json:"source"`
	SubSource    string         `json:"subsource"`
	SnapshotDate int64          `json:"snapshotDate"`
	InitDate     int64          `json:"initDate"`
	BackupAlias  string         `json:"backupAlias"`
	JobLabel     string         `json:"jobLabel"`
	FakeExport   bool           `json:"fakeExport"`
	Chunks       []DownloadFile `json:"chunks"`
}

// Ready reports whether this job's archive can be downloaded.
func (j DownloadJob) Ready() bool {
	return strings.EqualFold(strings.TrimSpace(j.Status), StatusReadyForDownload)
}

// SnapshotTime decodes the backup date this job archived.
func (j DownloadJob) SnapshotTime() (time.Time, error) {
	return ParseSnapshotDate(j.SnapshotDate)
}

// downloadStatusPage is the paginated envelope of getRestoreDownloadStatus.
type downloadStatusPage struct {
	Page       int           `json:"page"`
	TotalPages int           `json:"totalPages"`
	Total      int           `json:"total"`
	Data       []DownloadJob `json:"data"`
}

// downloadStatusOperation lists restore and download jobs with their chunks.
const downloadStatusOperation = "getRestoreDownloadStatus"

// DownloadJobs lists the account's restore and download jobs, newest first
// as the portal returns them. Only EXPORT jobs are returned.
func (s *Session) DownloadJobs(ctx context.Context) ([]DownloadJob, error) {
	var all []DownloadJob
	for page := 1; page <= maxDownloadInfoPages; page++ {
		raw, err := s.Call(ctx, downloadStatusOperation, map[string]any{
			"_search": false,
			"rows":    downloadInfoPageSize,
			"page":    page,
			"sord":    "desc",
		})
		if err != nil {
			return all, err
		}
		var rows []DownloadJob
		if err := json.Unmarshal(raw, &rows); err != nil {
			var pg downloadStatusPage
			if perr := json.Unmarshal(raw, &pg); perr != nil {
				return all, fmt.Errorf("portal: decoding %s rows: %w", downloadStatusOperation, err)
			}
			rows = pg.Data
		}
		for _, r := range rows {
			if r.Type == "" || strings.EqualFold(r.Type, jobTypeExport) {
				all = append(all, r)
			}
		}
		if len(rows) < downloadInfoPageSize {
			break
		}
	}
	return all, nil
}

// ParseSnapshotDate decodes a compact yyyyMMddHHmmss backup date.
func ParseSnapshotDate(v int64) (time.Time, error) {
	return cadate.ParseCompact(v)
}
