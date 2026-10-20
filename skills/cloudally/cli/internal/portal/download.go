// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Portal service operations for export (download) jobs. CloudAlly calls the
// job an EXPORT on the documented API and a "Download" in the portal.
const (
	downloadURLOperation  = "getDownloadUrl"
	downloadInfoOperation = "getDownloadInfo"
)

// StatusReadyForDownload is the export task status that means the archive is
// built and a link can be minted.
const StatusReadyForDownload = "READY_FOR_DOWNLOAD"

// downloadInfoPageSize is how many files one getDownloadInfo page returns.
const downloadInfoPageSize = 100

// maxDownloadInfoPages bounds a runaway pagination loop.
const maxDownloadInfoPages = 50

// DownloadFile is one downloadable artifact of an export job. A mailbox
// export is normally a single zip; a PST export splits into chunks.
//
// Size is the portal's own human string ("871.26 MB"), not a byte count.
// DownloadURL is empty on a listing: mint it with DownloadURL().
type DownloadFile struct {
	ExportDataID string `json:"exportDataId"`
	FileName     string `json:"fileName"`
	Size         string `json:"size"`
	StorageType  string `json:"storageType"`
	DownloadURL  string `json:"downloadUrl"`
}

// downloadInfoPage is the paginated envelope getDownloadInfo returns.
type downloadInfoPage struct {
	Page       int            `json:"page"`
	TotalPages int            `json:"totalPages"`
	Total      int            `json:"total"`
	Data       []DownloadFile `json:"data"`
}

// DownloadURL mints a presigned link for ONE file of a finished export job.
//
// exportDataID is the chunk's exportDataId from DownloadFiles, NOT the export
// job's id. The wire parameter is nevertheless named "taskId": the portal
// front-end passes the chunk id under that name, and passing the job id
// instead fails. That failure is reported as "The download link has
// expired." regardless of the real cause, so a wrong id here looks exactly
// like an expired one.
//
// The link is extremely short lived (see LinkExpiry; observed at 60 seconds),
// so it is only useful if the download starts immediately. Minting one to
// paste somewhere for later does not work.
func (s *Session) DownloadURL(ctx context.Context, exportDataID string) (string, error) {
	if strings.TrimSpace(exportDataID) == "" {
		return "", fmt.Errorf("portal: download url needs an export data id")
	}
	raw, err := s.Get(ctx, downloadURLOperation, url.Values{"taskId": {exportDataID}})
	if err != nil {
		return "", err
	}
	// The service answers with the URL as a bare JSON string.
	var link string
	if err := json.Unmarshal(raw, &link); err != nil {
		return "", fmt.Errorf("portal: %s returned %s, not a url string", downloadURLOperation, truncate(string(raw), 120))
	}
	if strings.TrimSpace(link) == "" {
		return "", fmt.Errorf("portal: %s returned an empty url", downloadURLOperation)
	}
	return link, nil
}

// DownloadFiles lists every file an export job produced. Here taskId really
// is the export job's id. The rows carry no link: a link is minted per file
// with DownloadURL(file.ExportDataID).
func (s *Session) DownloadFiles(ctx context.Context, jobID string) ([]DownloadFile, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("portal: download info needs an export job id")
	}
	var all []DownloadFile
	for page := 1; ; page++ {
		if page > maxDownloadInfoPages {
			// A job's file list cut short would download as complete.
			return all, fmt.Errorf("%w: job %s has more than %d files", ErrListingIncomplete, jobID, maxDownloadInfoPages*downloadInfoPageSize)
		}
		raw, err := s.Get(ctx, downloadInfoOperation, url.Values{
			"taskId":  {jobID},
			"_search": {"false"},
			"rows":    {strconv.Itoa(downloadInfoPageSize)},
			"page":    {strconv.Itoa(page)},
			"sord":    {"asc"},
		})
		if err != nil {
			return all, err
		}
		var pg downloadInfoPage
		if err := json.Unmarshal(raw, &pg); err != nil {
			// Some deployments answer with a bare array.
			var rows []DownloadFile
			if jerr := json.Unmarshal(raw, &rows); jerr != nil {
				return all, fmt.Errorf("portal: decoding %s rows: %w", downloadInfoOperation, err)
			}
			pg.Data = rows
		}
		all = append(all, pg.Data...)
		if len(pg.Data) < downloadInfoPageSize || (pg.TotalPages > 0 && page >= pg.TotalPages) {
			return all, nil
		}
	}
}

// LinkExpiry reports when a minted download link stops working, read from
// the link itself. CloudAlly hands out presigned object-storage URLs, and
// every flavour states its own lifetime in the query string:
//
//   - AWS SigV4:  X-Amz-Date + X-Amz-Expires (seconds)
//   - AWS SigV2:  Expires (unix seconds)
//   - Azure SAS:  se (RFC3339 instant)
//   - GCS:        Expires (unix seconds)
//
// ok is false when the URL carries no recognisable expiry, which callers
// must report as unknown rather than assuming the link is durable.
func LinkExpiry(link string) (expiry time.Time, ok bool) {
	u, err := url.Parse(link)
	if err != nil {
		return time.Time{}, false
	}
	q := u.Query()
	// Azure shared access signature: absolute instant.
	if se := strings.TrimSpace(q.Get("se")); se != "" {
		if t, perr := time.Parse(time.RFC3339, se); perr == nil {
			return t.UTC(), true
		}
	}
	// AWS SigV4: signing instant plus a lifetime in seconds.
	if d := strings.TrimSpace(q.Get("X-Amz-Date")); d != "" {
		if secs, serr := strconv.Atoi(strings.TrimSpace(q.Get("X-Amz-Expires"))); serr == nil && secs > 0 {
			if signed, perr := time.Parse("20060102T150405Z", d); perr == nil {
				return signed.Add(time.Duration(secs) * time.Second).UTC(), true
			}
		}
	}
	// AWS SigV2 and GCS: absolute unix seconds.
	if e := strings.TrimSpace(q.Get("Expires")); e != "" {
		if secs, serr := strconv.ParseInt(e, 10, 64); serr == nil && secs > 0 {
			return time.Unix(secs, 0).UTC(), true
		}
	}
	return time.Time{}, false
}

// LinkLifetime reports a link's total validity window, for the AWS SigV4
// form that states it directly. It is the number support quotes to a
// customer ("valid for N hours") without depending on clock skew.
func LinkLifetime(link string) (time.Duration, bool) {
	u, err := url.Parse(link)
	if err != nil {
		return 0, false
	}
	if secs, serr := strconv.Atoi(strings.TrimSpace(u.Query().Get("X-Amz-Expires"))); serr == nil && secs > 0 {
		return time.Duration(secs) * time.Second, true
	}
	return 0, false
}

// truncate shortens a string for an error message.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
