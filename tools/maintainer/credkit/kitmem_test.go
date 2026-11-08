// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// #612: a connector's first sync ran out of memory in a 1 GB container, and
// one large ordinary HubSpot call record was withheld from the store (the
// check ran to its depth limit), which aborted the whole resource.

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// jsonStr is s as a JSON string, HTML characters as written (as a vendor
// API's Python or Ruby encoder writes them).
func jsonStr(s string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// callBody is a call's notes as an Outlook-forwarded HTML email of at least
// size bytes: per paragraph a backup-job note and a SafeLinks-wrapped link
// (the #495 shape), as the #612 fake vendor served them.
func callBody(i, size int) string {
	var b strings.Builder
	b.WriteString("<div>Please see the links below.<br>")
	for j := 0; b.Len() < size; j++ {
		inner := fmt.Sprintf("https://portal.vendor.example/kb/article?id=%d&utm_source=email&utm_campaign=nov%d", 500+j, i)
		g := sha256.Sum256([]byte(fmt.Sprintf("%d %d", i, j)))
		data := fmt.Sprintf("05|02|pat.doe@contoso.example|%s|%s|0|0|6386%014d|Unknown|TWFpbGZsb3d8eyJWIjoiMC4wLjAwMDAiLCJQIjoiV2luMzIiLCJBTiI6Ik1haWwiLCJXVCI6Mn0=|0|||",
			hex.EncodeToString(g[:16]), hex.EncodeToString(g[16:]), i*100+j)
		fmt.Fprintf(&b, `<p>Ticket update %d: the backup job on SRV-FS%02d reported 0x80070005; log C:\ProgramData\Agent\logs\job-%d.log.</p>`, j, j%40, j)
		fmt.Fprintf(&b, `<a href="https://nam02.safelinks.protection.outlook.com/?url=%s&amp;data=%s&amp;sdata=%s&amp;reserved=0" originalsrc="%s">KB %d</a><br>`,
			url.QueryEscape(inner), url.QueryEscape(data), url.QueryEscape(base64.StdEncoding.EncodeToString(g[:])), inner, 500+j)
	}
	b.WriteString("</div>")
	return b.String()
}

// callRecord is HubSpot's /crm/v3/objects/calls record i, its body about
// size bytes (0: a one-line note), as the API returns it.
func callRecord(i, size int) []byte {
	body := fmt.Sprintf("<p>Discussed renewal %d. Next step: send quote.</p>", i)
	if size > 0 {
		body = callBody(i, size)
	}
	return []byte(fmt.Sprintf(`{"id": "%d", "properties": {"hs_object_id": "%d", "hs_createdate": "2026-10-01T09:00:00Z", "hs_lastmodifieddate": "2026-11-%02dT09:00:00Z", "hubspot_owner_id": "%d", "hs_call_title": "Call with Contoso about backups %d", "hs_call_duration": "%d", "hs_call_status": "COMPLETED", "hs_call_body": %s}, "createdAt": "2026-10-01T09:00:00Z", "updatedAt": "2026-11-01T09:00:00Z", "archived": false}`,
		i, i, 1+i%5, 70000+i%9, i, 60000+i, jsonStr(body)))
}

// cfBm is a Cloudflare __cf_bm cookie value as HubSpot's edge sets one on
// every response that carries none: 130 characters, distinct per page.
func cfBm(page int) string {
	g := sha256.Sum256([]byte(fmt.Sprint("cf_bm", page)))
	h := sha256.Sum256(g[:])
	return base64.RawURLEncoding.EncodeToString(append(g[:], h[:]...)) + "-1762444187-1.0.1.1-" + base64.RawURLEncoding.EncodeToString(h[:24])
}

const kitmemInsert = "INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(resource_type, id) DO UPDATE SET data = excluded.data"

// heapPeak samples the heap (objects and the free space the collector has
// not returned) until stop, and reports the highest it saw.
func heapPeak() (stop func() uint64) {
	var peak atomic.Uint64
	quit, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/heap/unused:bytes"}}
		for {
			metrics.Read(s)
			if v := s[0].Value.Uint64() + s[1].Value.Uint64(); v > peak.Load() {
				peak.Store(v)
			}
			select {
			case <-quit:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	return func() uint64 { close(quit); <-done; return peak.Load() }
}

func heapLive() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// kitmemDB is a database whose driver is the kit's SQL wrapper over a
// recorder: what reaches the driver is what the kit let through.
func kitmemDB(t *testing.T) (*sql.DB, *recDriver) {
	t.Helper()
	rec := &recDriver{}
	sqlTestRuns++
	name := fmt.Sprintf("credkit-kitmem-%d", sqlTestRuns)
	sql.Register(name, SQLDriver(rec))
	db, err := sql.Open(name, "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, rec
}

func kitmemRow(db *sql.DB, id string, row []byte) error {
	_, err := db.Exec(kitmemInsert, id, "hubspot-calls-crm", string(row), "2026-11-06T15:00:00Z", "2026-11-06T15:00:00Z")
	return err
}

// A large ordinary call record is stored: its check ended at the depth
// limit (#612, sync_error at hubspot-calls-crm row 32), as the decoders'
// outputs joined into one part were read across each other into longer new
// parts at every level.
func TestKitmemLargeCallRecordIsStored(t *testing.T) {
	holding(t, kitsafeHeld)
	db, rec := kitmemDB(t)
	row := callRecord(32, 20000+(32*7919)%280000) // 277 KB, byte for byte the record the sync withheld
	if raceBuild || testing.Short() {
		t.Skipf("a %d KB check: skipped under -race and -short", len(row)>>10)
	}
	if err := kitmemRow(db, "32", row); err != nil {
		t.Fatalf("an ordinary %d KB call record was not stored: %v", len(row)>>10, err)
	}
	if len(rec.stmts) != 1 {
		t.Fatalf("statements reaching the driver: %d, want 1", len(rec.stmts))
	}
}

// A sync-shaped store write: rows page by page, a new Cloudflare cookie held
// with each page (as HubSpot's edge sets one per response), one large rich
// text record in the middle. Measured with counters: the heap at its peak
// over the large row (sampled), the heap the run leaves behind, and the
// bytes allocated per byte stored.
func TestKitmemSyncWorkloadMemory(t *testing.T) {
	holding(t, kitsafeHeld)
	db, _ := kitmemDB(t)
	pages, large := 400, callRecord(1, 100_000)
	if raceBuild || testing.Short() {
		pages, large = 100, nil
	}
	base := heapLive()
	var alloc, stored uint64
	var peak uint64
	for p := 0; p < pages; p++ {
		Hold("cookie:__cf_bm", cfBm(p))
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		for r := 0; r < 2; r++ {
			row := callRecord(2*p+r, 0)
			if err := kitmemRow(db, fmt.Sprint(2*p+r), row); err != nil {
				t.Fatal(err)
			}
			stored += uint64(len(row))
		}
		runtime.ReadMemStats(&b)
		alloc += b.TotalAlloc - a.TotalAlloc
		if p == pages/2 && large != nil {
			stop := heapPeak()
			err := kitmemRow(db, "large", large)
			peak = stop()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	retained := int64(heapLive()) - int64(base)
	t.Logf("%d pages: retained %d MB, %d bytes allocated per byte stored, large row %d KB peaked %d MB over the start",
		pages, retained>>20, alloc/stored, len(large)>>10, (int64(peak)-int64(base))>>20)
	// The held set holds a cookie per page; the matcher kept about 200 KB
	// for each (101 MB after 400 pages), and the pool the largest dedup sets.
	if retained > 48<<20 {
		t.Errorf("the run left %d MB more heap than it started with: memory grows with the rows synced", retained>>20)
	}
	if alloc/stored > 48<<10 {
		t.Errorf("%d bytes allocated per byte stored (main: 170,000)", alloc/stored)
	}
	if large != nil {
		// Proportional to the row, and far below what the decode budget
		// allows the check (main: 1,394 MB over the start for this row).
		budget := uint64(budgetPerByte*len(large) + budgetBase)
		if over := int64(peak) - int64(base); over > int64(4096*len(large)+64<<20) || uint64(over) > budget/2 {
			t.Errorf("a %d KB row's check peaked %d MB over the start (budget %d MB)", len(large)>>10, over>>20, budget>>20)
		}
	}
}

// A held credential inside a large synced row is still withheld from the
// store: raw in the notes, percent-encoded inside a SafeLinks url parameter,
// and base64 inside the JSON (its sdata), at the start, middle and end.
func TestKitmemHeldInLargeRowIsWithheld(t *testing.T) {
	holding(t, kitsafeHeld)
	db, rec := kitmemDB(t)
	size := 100_000
	if raceBuild || testing.Short() {
		size = 20_000
	}
	row := string(callRecord(7, size))
	b64 := base64.StdEncoding.EncodeToString([]byte(kitsafeHeld))
	for name, spell := range map[string]string{
		"raw":     kitsafeHeld,
		"percent": url.QueryEscape("https://x.example/?t=" + kitsafeHeld),
		"base64":  "sdata=" + url.QueryEscape(b64),
	} {
		for _, at := range []int{strings.Index(row, "<div>") + 5, len(row) / 2, strings.LastIndex(row, "</div>")} {
			unit := row[:at] + " " + spell + " " + row[at:]
			if err := kitmemRow(db, "7", []byte(unit)); !errors.Is(err, ErrWithheld) {
				t.Errorf("%s at %d of %d: not withheld (%v)", name, at, len(unit), err)
			}
		}
	}
	if len(rec.stmts) != 0 {
		t.Errorf("%d statements carrying the credential reached the driver", len(rec.stmts))
	}
}
