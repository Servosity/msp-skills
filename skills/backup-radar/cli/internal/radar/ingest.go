// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package radar

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxIngestResults is the v3 POST /backups cap on results across one request.
const MaxIngestResults = 10000

// IngestResult is one BackupResultEntry.
type IngestResult struct {
	Time            string   `json:"time"`
	Status          string   `json:"status"`
	Notes           []string `json:"notes,omitempty"`
	DataSelected    *int64   `json:"data_selected,omitempty"`
	DataTransferred *int64   `json:"data_transferred,omitempty"`
}

// IngestEntry is one BackupEntry. Collections are always present (possibly
// empty) so the payload matches the documented example shape.
type IngestEntry struct {
	DeviceName        string            `json:"device_name"`
	JobName           string            `json:"job_name"`
	ClientName        string            `json:"client_name"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Backups           []IngestResult    `json:"backups"`
	Replications      []IngestResult    `json:"replications"`
	Verifications     []IngestResult    `json:"verifications"`
	BootVerifications []IngestResult    `json:"boot_verifications"`
	Alerts            []IngestResult    `json:"alerts"`
}

func (e *IngestEntry) bucket(kind string) *[]IngestResult {
	switch kind {
	case "replications":
		return &e.Replications
	case "verifications":
		return &e.Verifications
	case "boot_verifications":
		return &e.BootVerifications
	case "alerts":
		return &e.Alerts
	default:
		return &e.Backups
	}
}

// FlatRow is one input row before grouping.
type FlatRow struct {
	Client          string
	Device          string
	Job             string
	Kind            string
	Time            string
	Status          string
	Notes           []string
	DataSelected    *int64
	DataTransferred *int64
}

// columnAliases maps accepted input column names to canonical fields.
// Canonical names win; aliases are listed in precedence order.
var columnAliases = map[string][]string{
	"client_name":      {"client_name", "client", "company"},
	"device_name":      {"device_name", "device", "hostname"},
	"job_name":         {"job_name", "job"},
	"type":             {"type", "kind", "result_type"},
	"time":             {"time", "timestamp", "date"},
	"status":           {"status", "result"},
	"notes":            {"notes", "note"},
	"data_selected":    {"data_selected"},
	"data_transferred": {"data_transferred"},
}

var kindAliases = map[string]string{
	"": "backups", "backup": "backups", "backups": "backups",
	"replication": "replications", "replications": "replications",
	"verification": "verifications", "verifications": "verifications",
	"boot_verification": "boot_verifications", "boot_verifications": "boot_verifications", "boot-verification": "boot_verifications",
	"alert": "alerts", "alerts": "alerts",
}

// canonicalColumns fixes the order fields (and their errors) are resolved in.
var canonicalColumns = []string{"client_name", "device_name", "job_name", "type", "time", "status", "notes", "data_selected", "data_transferred"}

// Name limits from the v3 BackupEntry schema.
var nameLimits = map[string]int{"client_name": 256, "device_name": 512, "job_name": 512}

// ParseFlatRows reads CSV (with a header row), JSONL, or a JSON array of
// objects using the column names above and validates every row. All errors
// are returned together so one pass fixes the whole file.
func ParseFlatRows(data []byte) ([]FlatRow, []error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(trimmed) == 0 {
		return nil, []error{fmt.Errorf("input is empty")}
	}
	var records []map[string]any
	var lines []int
	var errs []error
	decode := func(b []byte, v any) error {
		if err := duplicateKeys(b); err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber() // keep byte counts exact
		if err := dec.Decode(v); err != nil {
			return err
		}
		if _, err := dec.Token(); err != io.EOF {
			return fmt.Errorf("unexpected content after the first JSON value")
		}
		return nil
	}
	switch trimmed[0] {
	case '[':
		var arr []map[string]any
		if err := decode(trimmed, &arr); err != nil {
			return nil, []error{fmt.Errorf("parsing JSON array: %w", err)}
		}
		for i, obj := range arr {
			records, lines = append(records, obj), append(lines, i+1)
		}
	case '{':
		sc := bufio.NewScanner(bytes.NewReader(trimmed))
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		n := 0
		for sc.Scan() {
			n++
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var obj map[string]any
			if err := decode([]byte(line), &obj); err != nil {
				errs = append(errs, fmt.Errorf("line %d: invalid JSON: %w", n, err))
				continue
			}
			records, lines = append(records, obj), append(lines, n)
		}
		if err := sc.Err(); err != nil {
			errs = append(errs, err)
		}
	default:
		r := csv.NewReader(bytes.NewReader(trimmed))
		// FieldsPerRecord stays 0: every row must have as many cells as the
		// header, so an unquoted comma in a note is an error, not a lost tail.
		header, err := r.Read()
		if err != nil {
			return nil, []error{fmt.Errorf("reading CSV header: %w", err)}
		}
		seenHeader := map[string]string{}
		for _, h := range header {
			lh := strings.ToLower(strings.TrimSpace(h))
			if prev, dup := seenHeader[lh]; dup {
				return nil, []error{fmt.Errorf("CSV header repeats column %q (also %q); each column may appear once", h, prev)}
			}
			seenHeader[lh] = h
		}
		if missing := missingColumns(header); len(missing) > 0 {
			return nil, []error{fmt.Errorf("CSV header %q is missing required column(s) %s; accepted names: client_name|client|company, device_name|device|hostname, job_name|job, time|timestamp|date, status|result",
				strings.Join(header, ","), strings.Join(missing, ", "))}
		}
		n := 1
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			n++
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d: %w", n, err))
				continue
			}
			m := map[string]any{}
			for i, h := range header {
				if i < len(rec) {
					m[h] = rec[i]
				}
			}
			records, lines = append(records, m), append(lines, n)
		}
	}
	rows := make([]FlatRow, 0, len(records))
	for i, rec := range records {
		row, rowErrs := toFlatRow(rec, lines[i])
		errs = append(errs, rowErrs...)
		if len(rowErrs) == 0 {
			rows = append(rows, row)
		}
	}
	return rows, errs
}

// duplicateKeys rejects a JSON object that repeats a key: decoding into a map
// would keep only the last value and silently change what gets pushed.
func duplicateKeys(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // huge numbers (1e400) must not stop the scan
	type frame struct {
		object bool
		keys   map[string]bool
		expect bool // next string token is a key
	}
	var stack []*frame
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking JSON for repeated keys: %w", err) // fail closed
		}
		top := func() *frame {
			if len(stack) == 0 {
				return nil
			}
			return stack[len(stack)-1]
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				if f := top(); f != nil && f.object {
					f.expect = true
				}
				stack = append(stack, &frame{object: true, keys: map[string]bool{}, expect: true})
			case '[':
				if f := top(); f != nil && f.object {
					f.expect = true
				}
				stack = append(stack, &frame{})
			default:
				stack = stack[:len(stack)-1]
			}
		default:
			f := top()
			if f == nil || !f.object {
				continue
			}
			if f.expect {
				key, _ := t.(string)
				if f.keys[key] {
					return fmt.Errorf("JSON object repeats key %q", key)
				}
				f.keys[key] = true
				f.expect = false
			} else {
				f.expect = true
			}
		}
	}
}

func missingColumns(header []string) []string {
	have := map[string]bool{}
	for _, h := range header {
		have[strings.ToLower(strings.TrimSpace(h))] = true
	}
	var missing []string
	for _, canon := range []string{"client_name", "device_name", "job_name", "time", "status"} {
		found := false
		for _, a := range columnAliases[canon] {
			found = found || have[a]
		}
		if !found {
			missing = append(missing, canon)
		}
	}
	return missing
}

// lowerKeys indexes a record by lowercased key (keys maps back to the
// original spelling), rejecting keys that differ only in case and disagree.
func lowerKeys(rec map[string]any) (lower map[string]any, keys map[string]string, err error) {
	lower, keys = map[string]any{}, map[string]string{}
	for k, v := range rec {
		lk := strings.ToLower(strings.TrimSpace(k))
		if prev, dup := keys[lk]; dup && fmt.Sprint(lower[lk]) != fmt.Sprint(v) {
			return nil, nil, fmt.Errorf("keys %q and %q differ only in case and disagree", prev, k)
		}
		lower[lk], keys[lk] = v, k
	}
	return lower, keys, nil
}

// fieldValue resolves one canonical field from a lowerKeys index, rejecting
// two different non-empty values supplied under different aliases.
func fieldValue(lower map[string]any, keys map[string]string, canon string) (any, error) {
	var val any
	var from string
	for _, alias := range columnAliases[canon] {
		v, ok := lower[alias]
		if !ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
			continue
		}
		if from != "" && fmt.Sprint(v) != fmt.Sprint(val) {
			return val, fmt.Errorf("columns %q and %q disagree for %s", from, keys[alias], canon)
		}
		if from == "" {
			val, from = v, keys[alias]
		}
	}
	return val, nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func toFlatRow(rec map[string]any, line int) (FlatRow, []error) {
	var errs []error
	fail := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf("line %d: "+format, append([]any{line}, a...)...))
	}
	f := map[string]any{}
	lower, keys, err := lowerKeys(rec)
	if err != nil {
		fail("%v", err)
		return FlatRow{}, errs // every field would be ambiguous; report the cause once
	}
	for _, canon := range canonicalColumns {
		v, err := fieldValue(lower, keys, canon)
		if err != nil {
			fail("%v", err)
		}
		f[canon] = v
	}
	row := FlatRow{Client: asString(f["client_name"]), Device: asString(f["device_name"]), Job: asString(f["job_name"])}
	for _, req := range []struct{ name, val string }{{"client_name", row.Client}, {"device_name", row.Device}, {"job_name", row.Job}} {
		if req.val == "" {
			fail("%s is required (empty or whitespace-only values are rejected by the API)", req.name)
		} else if n := utf8.RuneCountInString(req.val); n > nameLimits[req.name] {
			fail("%s is %d characters; the API allows at most %d", req.name, n, nameLimits[req.name])
		}
	}
	kind, ok := kindAliases[strings.ToLower(asString(f["type"]))]
	if !ok {
		fail("type %q must be one of backups, replications, verifications, boot_verifications, alerts", asString(f["type"]))
	}
	row.Kind = kind
	switch st := strings.ToLower(asString(f["status"])); st {
	case "success", "warning", "failure":
		row.Status = st
	default:
		fail("status %q must be success, warning, or failure", asString(f["status"]))
	}
	if t, ok := ParseTime(asString(f["time"])); ok {
		row.Time = t.Format(time.RFC3339Nano)
	} else {
		fail("time %q is not an RFC 3339 or YYYY-MM-DD HH:MM:SS timestamp", asString(f["time"]))
	}
	switch n := f["notes"].(type) {
	case []any: // JSON arrays keep each note intact
		for _, item := range n {
			if s := asString(item); s != "" {
				row.Notes = append(row.Notes, s)
			}
		}
	case nil:
	default: // CSV and plain strings use ';' between notes
		for _, part := range strings.Split(asString(n), ";") {
			if part = strings.TrimSpace(part); part != "" {
				row.Notes = append(row.Notes, part)
			}
		}
	}
	for _, num := range []struct {
		name string
		dst  **int64
	}{{"data_selected", &row.DataSelected}, {"data_transferred", &row.DataTransferred}} {
		if v := asString(f[num.name]); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				fail("%s %q must be a non-negative whole number of bytes", num.name, v)
				continue
			}
			*num.dst = &n
		}
	}
	return row, errs
}

// Chunk is one POST /backups request body.
type Chunk struct {
	Entries []IngestEntry `json:"entries"`
	Results int           `json:"results"`
}

// BuildChunks groups rows by exact client/device/job (the API receives the
// names as given, so "Acme" and "ACME" stay separate) and splits them so no request
// carries more than limit results. An entry larger than limit is split into
// several entries with the same identity across requests.
func BuildChunks(rows []FlatRow, limit int) []Chunk {
	if limit <= 0 || limit > MaxIngestResults {
		limit = MaxIngestResults
	}
	type group struct {
		entry IngestEntry
		rows  []FlatRow
	}
	groups := map[string]*group{}
	order := []string{}
	for _, r := range rows {
		k := r.Client + "\x1f" + r.Device + "\x1f" + r.Job
		if groups[k] == nil {
			groups[k] = &group{entry: IngestEntry{DeviceName: r.Device, JobName: r.Job, ClientName: r.Client}}
			order = append(order, k)
		}
		groups[k].rows = append(groups[k].rows, r)
	}
	chunks := []Chunk{}
	cur := Chunk{}
	flush := func() {
		if cur.Results > 0 {
			chunks = append(chunks, cur)
		}
		cur = Chunk{}
	}
	for _, k := range order {
		g := groups[k]
		pending := g.rows
		for len(pending) > 0 {
			room := limit - cur.Results
			if room == 0 {
				flush()
				room = limit
			}
			take := pending
			if len(take) > room {
				take = pending[:room]
			}
			e := newEntry(g.entry)
			for _, r := range take {
				b := e.bucket(r.Kind)
				*b = append(*b, IngestResult{Time: r.Time, Status: r.Status, Notes: r.Notes, DataSelected: r.DataSelected, DataTransferred: r.DataTransferred})
			}
			cur.Entries = append(cur.Entries, e)
			cur.Results += len(take)
			pending = pending[len(take):]
		}
	}
	flush()
	return chunks
}

func newEntry(id IngestEntry) IngestEntry {
	return IngestEntry{DeviceName: id.DeviceName, JobName: id.JobName, ClientName: id.ClientName,
		Backups: []IngestResult{}, Replications: []IngestResult{}, Verifications: []IngestResult{}, BootVerifications: []IngestResult{}, Alerts: []IngestResult{}}
}
