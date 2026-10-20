// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CachedSession is one session cache file on this computer and what it held,
// as auth logout reports it. Values are never shown: only the file, the NAS
// it was recorded for and which kinds of secret it carried. Only a file this
// CLI can prove it wrote is ever a CachedSession (see CachedSessions).
type CachedSession struct {
	File string `json:"file"`
	// NAS is the address the file was recorded for, through DisplayAddress
	// ("" for the legacy origin-less session.json).
	NAS string `json:"nas"`
	// SessionID: the file held a DSM session (session id, SynoToken or
	// session cookies) that a later command could reuse without a password.
	SessionID bool `json:"session_id"`
	// DeviceTokenAccounts are the accounts whose two-factor trusted-device
	// token the file held (a login with that token skips the one-time code).
	DeviceTokenAccounts []string `json:"device_token_accounts"`

	// owner is the session file name the file is (or is the atomic-write
	// temp file of); legacy marks the origin-less session.json. Removal
	// proves the file again against them.
	owner  string
	legacy bool
}

// SkippedFile is a file named like a session cache that this CLI cannot
// prove it wrote. It is listed and kept: never read further, never removed.
type SkippedFile struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// NotWrittenByCLI begins every SkippedFile reason.
const NotWrittenByCLI = "not a session file this CLI wrote"

// SessionDirs are where this CLI keeps DSM session caches.
type SessionDirs struct {
	// Data is the CLI's data directory, the only place it writes a per-NAS
	// session file (session-<scheme>-<host>_<port>[_<path>]-<hash8>.json,
	// SessionFileName) and its atomic-write temp file.
	Data string
	// Config is the directory of the CLI's config file. Early builds kept an
	// origin-less session.json there; that legacy file is the only session
	// cache this CLI ever wrote outside Data ("" or Data itself: nothing
	// extra is looked at).
	Config string
}

// maxSessionRecord bounds what is read from a candidate file: a session
// record (a session id, a few cookies, device tokens) is a few KB.
const maxSessionRecord = 1 << 20

// CachedSessions lists the session cache files this CLI wrote, and the
// session-cache-like files it did not, in dirs. HAND-FIX(dsm-auth-logout):
// a file is listed as this CLI's only when BOTH
//   - its name is exactly the CLI's own scheme: a per-NAS file
//     (SessionFileName) in the data directory, or the legacy session.json in
//     the data or config directory, or the atomic-write temp file of either
//     (".<name>.<digits>.tmp", as writeAtomic creates it); and
//   - its content parses as the session record the CLI writes under that
//     name: for a per-NAS file a record whose recorded origin names exactly
//     this file (SessionFileName(origin) == name), for session.json the
//     legacy origin-less record.
//
// Anything else that looks like a session cache by name (session*.json,
// .session*.tmp) is a SkippedFile. A symbolic link or other non-regular file
// is never followed or read. With baseURL "" every file is listed (auth
// logout --all); otherwise only baseURL's NAS: its per-NAS file, plus any
// skipped file that carries its name or records its origin. The legacy
// session.json names no NAS, so it is listed only for --all. A directory
// that does not exist lists nothing.
func CachedSessions(baseURL string, dirs SessionDirs) ([]CachedSession, []SkippedFile, error) {
	var files []CachedSession
	var skipped []SkippedFile
	own := SessionFileName(baseURL)
	type scan struct {
		dir  string
		data bool
	}
	scans := []scan{{dirs.Data, true}}
	if dirs.Config != "" && !sameDir(dirs.Config, dirs.Data) {
		scans = append(scans, scan{dirs.Config, false})
	}
	for _, sc := range scans {
		if sc.dir == "" {
			continue
		}
		entries, err := os.ReadDir(sc.dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("listing %s: %w", sc.dir, err)
		}
		for _, e := range entries {
			cs, sk, origin := classifySessionEntry(sc.dir, e.Name(), sc.data)
			switch {
			case cs != nil && (baseURL == "" || (!cs.legacy && cs.owner == own)):
				files = append(files, *cs)
			case sk != nil && (baseURL == "" || sessionOwner(e.Name()) == own || SameSession(origin, baseURL)):
				skipped = append(skipped, *sk)
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].File < skipped[j].File })
	return files, skipped, nil
}

// ForgetSessionFiles removes each listed session cache file, whatever it
// holds (every account's session id, SynoToken, cookies and trusted-device
// tokens), after proving it once more: a file that is no longer this CLI's
// session record (replaced since it was listed, now a symbolic link) is kept
// and returned as skipped. It returns the files actually removed; a file
// already gone is neither. The first failure stops it, reported with what was
// removed before it.
func ForgetSessionFiles(files []CachedSession) ([]CachedSession, []SkippedFile, error) {
	var removed []CachedSession
	var skipped []SkippedFile
	for _, f := range files {
		if _, err := os.Lstat(f.File); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if _, reason, _ := proveSessionFile(f.File, f.owner, f.legacy); reason != "" {
			skipped = append(skipped, SkippedFile{File: f.File, Reason: NotWrittenByCLI + ": " + reason + " (changed since it was listed)"})
			continue
		}
		err := os.Remove(f.File) // a symbolic link swapped in after the proof is removed itself, never its target
		switch {
		case err == nil:
			removed = append(removed, f)
		case errors.Is(err, os.ErrNotExist):
		default:
			return removed, skipped, fmt.Errorf("removing %s: %w", f.File, err)
		}
	}
	return removed, skipped, nil
}

// looksLikeSessionCache is every name a session cache of this CLI (or a
// file a user could mistake for one) may carry: session.json,
// session-*.json and .session*.tmp. Only these are considered at all.
func looksLikeSessionCache(name string) bool {
	return name == "session.json" ||
		(strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json")) ||
		(strings.HasPrefix(name, ".session") && strings.HasSuffix(name, ".tmp"))
}

// sessionOwner is the session file name a cache file name belongs to: the
// name itself, or for writeAtomic's temp file ".<name>.<digits>.tmp" the
// name it was being written to; "" when the name fits neither.
func sessionOwner(name string) string {
	if strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp") {
		mid := strings.TrimSuffix(strings.TrimPrefix(name, "."), ".tmp")
		i := strings.LastIndexByte(mid, '.')
		if i <= 0 || !allDigits(mid[i+1:]) {
			return ""
		}
		name = mid[:i]
	}
	if name == "session.json" || (strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json")) {
		return name
	}
	return ""
}

// perNASFileName reports whether name has SessionFileName's shape:
// "session-" + a readable part of [A-Za-z0-9._-] + "-" + 8 hex digits +
// ".json". (Whether the hash is right is checked against the content.)
func perNASFileName(name string) bool {
	stem, ok := strings.CutSuffix(strings.TrimPrefix(name, "session-"), ".json")
	if !ok || !strings.HasPrefix(name, "session-") || len(stem) < 10 || stem[len(stem)-9] != '-' {
		return false
	}
	for _, r := range stem[len(stem)-8:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	for _, r := range stem[:len(stem)-9] {
		if r != '.' && r != '-' && r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// classifySessionEntry decides what one directory entry is: this CLI's
// session file (cs), a session-cache-like file it did not write (skip), or
// neither (both nil: not considered). origin is the address the file's
// content records, when it parsed far enough to tell.
func classifySessionEntry(dir, name string, dataDir bool) (cs *CachedSession, skip *SkippedFile, origin string) {
	if !looksLikeSessionCache(name) {
		return nil, nil, ""
	}
	path := filepath.Join(dir, name)
	notOurs := func(why string) (*CachedSession, *SkippedFile, string) {
		return nil, &SkippedFile{File: path, Reason: NotWrittenByCLI + ": " + why}, origin
	}
	owner := sessionOwner(name)
	switch {
	case owner == "" || (owner != "session.json" && !perNASFileName(owner)):
		origin = recordedOrigin(path) // only to tell which NAS's logout lists it
		return notOurs("the name is not one this CLI gives a session file")
	case owner != "session.json" && !dataDir:
		origin = recordedOrigin(path)
		return notOurs("a per-NAS session file name outside the CLI's data directory")
	}
	legacy := owner == "session.json"
	got, reason, recorded := proveSessionFile(path, owner, legacy)
	origin = recorded
	if reason != "" {
		return notOurs(reason)
	}
	return got, nil, origin
}

// proveSessionFile reads path (never through a symbolic link, at most
// maxSessionRecord bytes) and checks that its content is the session record
// this CLI writes under owner. It returns the description, or the reason the
// file is not this CLI's; origin is the address the content records, if any.
func proveSessionFile(path, owner string, legacy bool) (cs *CachedSession, reason, origin string) {
	data, why := readRegularFile(path)
	if why != "" {
		return nil, why, ""
	}
	rec := map[string]any{}
	if err := json.Unmarshal(data, &rec); err != nil || rec == nil {
		return nil, "its content is not a JSON session record", ""
	}
	origin, _ = rec["origin"].(string)
	if why := checkSessionRecord(rec, legacy); why != "" {
		return nil, why, origin
	}
	if legacy {
		if origin != "" {
			return nil, "a session.json that records a NAS address is not the legacy origin-less record", origin
		}
	} else if SessionFileName(origin) != owner {
		return nil, "the NAS address it records does not name this file", origin
	}
	d := describeSessionRecord(path, origin, rec)
	d.owner, d.legacy = owner, legacy
	return &d, "", origin
}

// recordedOrigin is the origin a session-cache-like file records, or "".
func recordedOrigin(path string) string {
	data, why := readRegularFile(path)
	if why != "" {
		return ""
	}
	var rec struct {
		Origin string `json:"origin"`
	}
	if json.Unmarshal(data, &rec) != nil {
		return ""
	}
	return rec.Origin
}

// readRegularFile reads a regular file without following a symbolic link:
// the entry is checked with Lstat and the opened file must be that same
// file, so a link swapped in meanwhile is refused, not followed.
func readRegularFile(path string) ([]byte, string) {
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, "it could not be inspected"
	}
	switch {
	case lst.Mode()&fs.ModeSymlink != 0:
		return nil, "a symbolic link (not followed)"
	case !lst.Mode().IsRegular():
		return nil, "not a regular file"
	case lst.Size() > maxSessionRecord:
		return nil, "larger than any session record"
	}
	// path is a directory entry of the CLI's own data or config directory,
	// checked above to be a regular file and re-checked below to be the same.
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, "it could not be read"
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(lst, fi) {
		return nil, "it changed while being read"
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSessionRecord+1))
	if err != nil || len(data) > maxSessionRecord {
		return nil, "it could not be read"
	}
	return data, ""
}

// sessionRecordFields are the fields of the record the session manager
// (sessionRecord) and the fleet layer (SaveDeviceID) write, and their JSON
// types. A legacy session.json carries the session manager's fields, never
// device_ids.
var sessionRecordFields = map[string]string{
	"token": "string", "account": "string", "origin": "string", "synotoken": "string",
	"device_id": "string", "saved": "time", "cookies": "cookies", "device_ids": "tokens",
}

// checkSessionRecord returns "" when rec has exactly the session record's
// shape: only its fields, each of its type, and the fields every record of
// that kind carries (a per-NAS record: the origin its file name is derived
// from; the legacy session.json, which has no such binding: token, cookies
// and saved, which its writer always wrote).
func checkSessionRecord(rec map[string]any, legacy bool) string {
	required := []string{"origin"}
	if legacy {
		required = []string{"token", "cookies", "saved"}
	}
	for _, k := range required {
		if _, ok := rec[k]; !ok {
			return fmt.Sprintf("its content is not a session record of this CLI (no %q field)", k)
		}
	}
	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kind, known := sessionRecordFields[k]
		if !known || (legacy && k == "device_ids") {
			return fmt.Sprintf("its content is not a session record of this CLI (unexpected field %q)", k)
		}
		if !fieldHasKind(rec[k], kind) {
			return fmt.Sprintf("its content is not a session record of this CLI (field %q has the wrong type)", k)
		}
	}
	return ""
}

func fieldHasKind(v any, kind string) bool {
	switch kind {
	case "string":
		_, ok := v.(string)
		return ok
	case "time":
		s, ok := v.(string)
		if !ok {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, s)
		return err == nil
	case "tokens":
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for _, t := range m {
			if _, ok := t.(string); !ok {
				return false
			}
		}
		return true
	case "cookies":
		if v == nil {
			return true // the session manager writes a nil cookie list as null
		}
		list, ok := v.([]any)
		if !ok {
			return false
		}
		for _, c := range list {
			m, ok := c.(map[string]any)
			if !ok {
				return false
			}
			for k, f := range m {
				switch k {
				case "url", "name", "value", "path":
					if _, ok := f.(string); !ok {
						return false
					}
				case "expires":
					if !fieldHasKind(f, "time") {
						return false
					}
				default:
					return false
				}
			}
		}
		return true
	}
	return false
}

// sameDir reports whether two directory paths name one directory.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}

func describeSessionRecord(path, origin string, rec map[string]any) CachedSession {
	cs := CachedSession{File: path, DeviceTokenAccounts: []string{}}
	if origin != "" {
		cs.NAS = DisplayAddress(origin)
	}
	tok, _ := rec["token"].(string)
	st, _ := rec["synotoken"].(string)
	cookies, _ := rec["cookies"].([]any)
	cs.SessionID = tok != "" || st != "" || len(cookies) > 0
	accounts := map[string]bool{}
	if did, _ := rec["device_id"].(string); did != "" {
		acct, _ := rec["account"].(string)
		if acct == "" {
			acct = "(no account recorded)"
		}
		accounts[acct] = true
	}
	if ids, ok := rec["device_ids"].(map[string]any); ok {
		for acct, v := range ids {
			if did, _ := v.(string); did != "" {
				accounts[acct] = true
			}
		}
	}
	for a := range accounts {
		cs.DeviceTokenAccounts = append(cs.DeviceTokenAccounts, a)
	}
	sort.Strings(cs.DeviceTokenAccounts)
	return cs
}
