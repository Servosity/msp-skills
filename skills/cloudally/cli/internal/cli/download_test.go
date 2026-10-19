// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"cloudally-pp-cli/internal/portal"
)

func TestDownloadGroupWires(t *testing.T) {
	cmd := RootCmd()
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "download" {
			found = true
			names := map[string]bool{}
			for _, sub := range c.Commands() {
				names[sub.Name()] = true
			}
			for _, want := range []string{"start", "list", "watch", "link"} {
				if !names[want] {
					t.Fatalf("download is missing %q: %v", want, names)
				}
			}
		}
	}
	if !found {
		t.Fatal("download group is not attached to the root")
	}
}

func TestExportJobStatusClassification(t *testing.T) {
	if !exportJobReady("READY_FOR_DOWNLOAD") || !exportJobReady(" ready_for_download ") {
		t.Fatal("READY_FOR_DOWNLOAD should be ready")
	}
	for _, s := range []string{"INPROCESS", "QUEUED", "PENDING", "DISPATCHED"} {
		if exportJobReady(s) {
			t.Fatalf("%s is not ready", s)
		}
		if exportJobFinished(s) {
			t.Fatalf("%s is not finished", s)
		}
	}
	for _, s := range []string{"FAILED", "CANCELLED", "PARTIALLY_SUCCEEDED", "READY_FOR_DOWNLOAD"} {
		if !exportJobFinished(s) {
			t.Fatalf("%s should be finished", s)
		}
	}
	// A failed job is finished but must never count as ready, or the link
	// step would run against an archive that does not exist.
	if exportJobReady("FAILED") {
		t.Fatal("FAILED must not be ready")
	}
}

func TestExportBodyShape(t *testing.T) {
	ent := taskEntity{Entity: "jane.doe@contoso.com", ExtID: "ext-jane", Status: "ACTIVE"}
	body := exportBody("task-1", "MS365", ent, 20251231043522, downloadFormatZip, false, false)
	if body["taskId"] != "task-1" || body["account"] != "jane.doe@contoso.com" ||
		body["source"] != "MS365" || body["date"] != int64(20251231043522) {
		t.Fatalf("body = %+v", body)
	}
	if body["extid"] != "ext-jane" {
		t.Fatalf("extid = %v", body["extid"])
	}
	// zip is the default, so no PST switches are sent at all.
	if _, ok := body["downloadToPst"]; ok {
		t.Fatalf("zip must not set downloadToPst: %+v", body)
	}
	// Storage type is left unset so the archive stays in CloudAlly's own
	// storage, which is what makes it downloadable.
	for _, k := range []string{"storageType", "storageBucketName", "storageKey", "storageSecretKey"} {
		if _, ok := body[k]; ok {
			t.Fatalf("download must not set %s: %+v", k, body)
		}
	}

	pst := exportBody("task-1", "MS365", ent, 20251231043522, downloadFormatPST, true, true)
	if pst["downloadToPst"] != true || pst["includeDeleted"] != true || pst["includeLegalHold"] != true {
		t.Fatalf("pst body = %+v", pst)
	}
	if _, ok := pst["preferSinglePST"]; ok {
		t.Fatalf("chunked pst must not ask for a single file: %+v", pst)
	}
	single := exportBody("task-1", "MS365", ent, 20251231043522, downloadFormatSinglePST, false, false)
	if single["downloadToPst"] != true || single["preferSinglePST"] != true {
		t.Fatalf("single pst body = %+v", single)
	}
	// An entity with no external id omits the field rather than sending "".
	noExt := exportBody("task-1", "MS365", taskEntity{Entity: "x@contoso.com"}, 1, downloadFormatZip, false, false)
	if _, ok := noExt["extid"]; ok {
		t.Fatalf("empty extid should be omitted: %+v", noExt)
	}
}

// TestDownloadStartDryRunCreatesNothing proves --dry-run stops before any
// network call, to the API or the portal.
func TestDownloadStartDryRunCreatesNothing(t *testing.T) {
	var apiHits, portalHits int
	api := countingServer(t, &apiHits)
	portalSrv := countingServer(t, &portalHits)

	out, _, err := runSnapshotCLI(t, api.URL,
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--date", "2025-12-31",
		"--portal-url", portalSrv.URL, "--dry-run", "--json", "--yes")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if apiHits != 0 || portalHits != 0 {
		t.Fatalf("dry-run made requests: api=%d portal=%d", apiHits, portalHits)
	}
	if !strings.Contains(out, "dry_run") {
		t.Fatalf("output = %s", out)
	}
	// The dry run must say where it would post and that the archive stays
	// in CloudAlly storage.
	if !strings.Contains(out, "recovery/export") || !strings.Contains(out, downloadDestinationOwned) {
		t.Fatalf("dry-run should name the endpoint and destination: %s", out)
	}
}

func TestDownloadStartValidatesFormatAndEntity(t *testing.T) {
	_, _, err := runSnapshotCLI(t, "http://127.0.0.1:9",
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane@contoso.com", "--date", "2025-12-31", "--format", "tar", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--format") {
		t.Fatalf("want a format usage error, got %v", err)
	}
	// One mailbox per run: --entity is required, so a missing one cannot
	// quietly become "every mailbox on the task".
	_, _, err = runSnapshotCLI(t, "http://127.0.0.1:9",
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--date", "2025-12-31", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--entity") {
		t.Fatalf("want an entity usage error, got %v", err)
	}
	// The portal is required to resolve a date, so --account is required.
	_, _, err = runSnapshotCLI(t, "http://127.0.0.1:9",
		"download", "start", "--task", "task-1",
		"--entity", "jane@contoso.com", "--date", "2025-12-31", "--yes")
	if err == nil {
		t.Fatal("want an error without --account")
	}
}

func TestDownloadLinkRequiresJobAndAccount(t *testing.T) {
	_, _, err := runSnapshotCLI(t, "http://127.0.0.1:9", "download", "link", "--account", "acct-1")
	if err == nil || !strings.Contains(err.Error(), "--job") {
		t.Fatalf("want a --job usage error, got %v", err)
	}
	_, _, err = runSnapshotCLI(t, "http://127.0.0.1:9", "download", "link", "--job", "job-1")
	if err == nil {
		t.Fatal("want an error without --account")
	}
}

func TestDownloadJobRowFromDecodesTheSnapshotDate(t *testing.T) {
	row := downloadJobRowFrom(portal.DownloadJob{
		JobID: "job-1", Type: "EXPORT", Status: portal.StatusReadyForDownload,
		Account: "jane@contoso.com", Source: "MS365", SubSource: "O365MAIL",
		SnapshotDate: 20251231043522, InitDate: 20261015184000,
		JobLabel: "SRV#6840424",
		Chunks:   []portal.DownloadFile{{ExportDataID: "d1", FileName: "jane.zip", Size: "871.26 MB"}},
	})
	if row.JobID != "job-1" || !row.Ready {
		t.Fatalf("row = %+v", row)
	}
	if row.SnapshotDate != "2025-12-31T04:35:22Z" || row.SnapshotRaw != 20251231043522 {
		t.Fatalf("snapshot = %q / %d", row.SnapshotDate, row.SnapshotRaw)
	}
	if row.CreatedAt != "2026-10-15T18:40:00Z" {
		t.Fatalf("created_at = %q", row.CreatedAt)
	}
	if row.SubSource != "O365MAIL" || row.Files != 1 || row.JobLabel != "SRV#6840424" {
		t.Fatalf("row = %+v", row)
	}
	// A job that is not ready says so, so nobody mints a link too early.
	notReady := downloadJobRowFrom(portal.DownloadJob{JobID: "job-2", Status: "INPROCESS"})
	if notReady.Ready {
		t.Fatalf("INPROCESS should not be ready: %+v", notReady)
	}
}
func TestDownloadFileOutReportsExpiryHonestly(t *testing.T) {
	withExpiry := downloadFileOut(portal.DownloadFile{
		FileName:    "jane.zip",
		Size:        "1.00 MB",
		DownloadURL: "https://s3/jane.zip?X-Amz-Date=20261015T183000Z&X-Amz-Expires=3600",
	})
	if !withExpiry.ExpiryKnown || withExpiry.ExpiresAt != "2026-10-15T19:30:00Z" {
		t.Fatalf("with expiry = %+v", withExpiry)
	}
	if withExpiry.ValidFor != "1h0m0s" {
		t.Fatalf("valid_for = %q", withExpiry.ValidFor)
	}
	// No stated expiry must come back unknown, never as a blank that reads
	// like "does not expire".
	without := downloadFileOut(portal.DownloadFile{DownloadURL: "https://s3/jane.zip"})
	if without.ExpiryKnown || without.ExpiresAt != "" {
		t.Fatalf("without expiry = %+v", without)
	}
}

func TestExportPathPerScope(t *testing.T) {
	if got := (restoreScope{account: "a1"}).exportPath(); got != "/v2/distributors/accounts/a1/recovery/export" {
		t.Fatalf("distributor export path = %q", got)
	}
	if got := (restoreScope{account: "a1", reseller: true}).exportPath(); got != "/v2/resellers/accounts/a1/recovery/export" {
		t.Fatalf("reseller export path = %q", got)
	}
	if got := (restoreScope{}).exportPath(); got != "/v2/recovery/export" {
		t.Fatalf("account export path = %q", got)
	}
}

// TestPlanLatestBatchUsesEachSubSourceOwnDate covers the --latest set: it
// must reach mailboxes whose last backup was years ago, and each row must
// carry that sub-source's own timestamp.
func TestPlanLatestBatchUsesEachSubSourceOwnDate(t *testing.T) {
	task := &taskWithEntities{
		ID:     "task-1",
		Source: "MS365",
		Entities: []taskEntity{
			{Entity: "active@contoso.com", ExtID: "ext-a", Status: "ACTIVE",
				LastBackupDates: map[string]flexInt{
					"O365MAIL": 20261015042344, "O365CALENDAR": 20261015042341,
				}},
			// Last backed up in 2022: an --on-date run for any recent year
			// would miss this entirely.
			{Entity: "ancient@contoso.com", ExtID: "ext-b", Status: "ARCHIVED",
				LastBackupDates: map[string]flexInt{"O365MAIL": 20221011043115}},
			// No recorded backup at all.
			{Entity: "nothing@contoso.com", ExtID: "ext-c", Status: "ON_PREMISE"},
			// No external id, so it cannot be archived.
			{Entity: "noext@contoso.com", Status: "ARCHIVED",
				LastBackupDates: map[string]flexInt{"O365MAIL": 20240101040000}},
		},
	}
	sizes := map[string]portal.TaskEntityInfo{
		"active@contoso.com":  {Size: "1.79 GB", SizeBytes: 1 << 30, Status: "Active"},
		"ancient@contoso.com": {Size: "14.00 GB", SizeBytes: 14 << 30, Status: "Archived"},
		"nothing@contoso.com": {Size: "0 Bytes", SizeBytes: 0, Status: "Not Accessible"},
		"noext@contoso.com":   {Size: "2.19 MB", SizeBytes: 2 << 20, Status: "Archived"},
	}

	batch := planLatestBatch(task, nil, nil, sizes, 0)
	if batch.OnDate != latestLabel {
		t.Fatalf("label = %q", batch.OnDate)
	}
	if len(batch.Rows) != 3 {
		t.Fatalf("want 3 rows (2 active + 1 ancient), got %d: %+v", len(batch.Rows), batch.Rows)
	}
	byKey := map[string]downloadRow{}
	for _, r := range batch.Rows {
		byKey[r.Entity+"/"+r.SubSource] = r
	}
	// Each sub-source keeps its own timestamp.
	if got := byKey["active@contoso.com/O365MAIL"].SnapshotRaw; got != 20261015042344 {
		t.Fatalf("mail snapshot = %d", got)
	}
	if got := byKey["active@contoso.com/O365CALENDAR"].SnapshotRaw; got != 20261015042341 {
		t.Fatalf("calendar snapshot = %d", got)
	}
	// A mailbox last backed up in 2022 is still included.
	anc := byKey["ancient@contoso.com/O365MAIL"]
	if anc.SnapshotRaw != 20221011043115 || anc.Snapshot != "2022-10-11T04:31:15Z" {
		t.Fatalf("ancient row = %+v", anc)
	}
	if anc.EntityStatus != "ARCHIVED" {
		t.Fatalf("archived status should be carried: %+v", anc)
	}
	// The unusable ones are skipped with a stated reason, not silently.
	reasons := map[string]string{}
	for _, s := range batch.Skipped {
		reasons[s.Entity] = s.Reason
	}
	if len(batch.Skipped) != 2 {
		t.Fatalf("skipped = %+v", batch.Skipped)
	}
	if !strings.Contains(reasons["nothing@contoso.com"], "Not Accessible") {
		t.Fatalf("nothing@ reason = %q", reasons["nothing@contoso.com"])
	}
	if !strings.Contains(reasons["noext@contoso.com"], "no external id") {
		t.Fatalf("noext@ reason = %q", reasons["noext@contoso.com"])
	}
}

func TestPlanLatestBatchHonoursSubSourceAndSizeFilters(t *testing.T) {
	task := &taskWithEntities{
		ID: "task-1", Source: "MS365",
		Entities: []taskEntity{
			{Entity: "big@contoso.com", ExtID: "ext-a", Status: "ACTIVE",
				LastBackupDates: map[string]flexInt{
					"O365MAIL": 20261015042344, "O365NOTES": 20261015042351,
				}},
			{Entity: "tiny@contoso.com", ExtID: "ext-b", Status: "ACTIVE",
				LastBackupDates: map[string]flexInt{"O365MAIL": 20261015042600}},
		},
	}
	sizes := map[string]portal.TaskEntityInfo{
		"big@contoso.com":  {Size: "1.79 GB", SizeBytes: 1 << 31, Status: "Active"},
		"tiny@contoso.com": {Size: "224.04 KB", SizeBytes: 229416, Status: "Active"},
	}
	batch := planLatestBatch(task, nil, map[string]bool{"O365MAIL": true}, sizes, 10<<20)
	if len(batch.Rows) != 1 {
		t.Fatalf("want only big@ mail, got %+v", batch.Rows)
	}
	if batch.Rows[0].Entity != "big@contoso.com" || batch.Rows[0].SubSource != "O365MAIL" {
		t.Fatalf("row = %+v", batch.Rows[0])
	}
	// tiny@ is below the floor and says so.
	var found bool
	for _, s := range batch.Skipped {
		if s.Entity == "tiny@contoso.com" && strings.Contains(s.Reason, "below --min-size") {
			found = true
		}
	}
	if !found {
		t.Fatalf("skipped = %+v", batch.Skipped)
	}
}
