// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CachedSession is one session cache file on this computer and what it held,
// as auth logout reports it. Values are never shown: only the file, the NAS
// it was recorded for and which kinds of secret it carried.
type CachedSession struct {
	File string `json:"file"`
	// NAS is the address the file was recorded for, through DisplayAddress
	// ("" when the file names none, as the legacy session.json may not).
	NAS string `json:"nas"`
	// SessionID: the file held a DSM session (session id, SynoToken or
	// session cookies) that a later command could reuse without a password.
	SessionID bool `json:"session_id"`
	// DeviceTokenAccounts are the accounts whose two-factor trusted-device
	// token the file held (a login with that token skips the one-time code).
	DeviceTokenAccounts []string `json:"device_token_accounts"`
}

// isSessionCacheName matches every file name the CLI has used for a DSM
// session cache: the per-NAS session-<key>.json, the legacy origin-less
// session.json, and an atomic-write temp file (.session-*.tmp) a crash could
// leave behind.
func isSessionCacheName(name string) bool {
	switch {
	case name == "session.json":
		return true
	case strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json"):
		return true
	case strings.HasPrefix(name, ".session-") && strings.HasSuffix(name, ".tmp"):
		return true
	}
	return false
}

// CachedSessions lists the DSM session cache files in dirs (the CLI's data
// directory, and the config directory where early builds kept session.json).
// With baseURL "" it lists every one; otherwise only baseURL's NAS: its own
// file (SessionFilePath) and any other cache file whose recorded origin is
// the same session (SameSession), such as one written under an older file
// name. A directory that does not exist lists nothing.
func CachedSessions(baseURL string, dirs ...string) ([]CachedSession, error) {
	var out []CachedSession
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", dir, err)
		}
		own := SessionFileName(baseURL)
		for _, e := range entries {
			if e.IsDir() || !isSessionCacheName(e.Name()) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			rec := readRecord(path)
			origin, _ := rec["origin"].(string)
			if baseURL != "" && e.Name() != own && !SameSession(origin, baseURL) {
				continue
			}
			seen[path] = true
			out = append(out, describeSessionRecord(path, origin, rec))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
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

// ForgetSessionFiles removes each listed session cache file, whatever it
// holds (every account's session id, SynoToken, cookies and trusted-device
// tokens). It returns the files actually removed; a file already gone is
// neither removed nor an error. The first failure stops it, reported with
// what was removed before it.
func ForgetSessionFiles(files []CachedSession) ([]CachedSession, error) {
	removed := make([]CachedSession, 0, len(files))
	for _, f := range files {
		err := os.Remove(f.File)
		switch {
		case err == nil:
			removed = append(removed, f)
		case errors.Is(err, os.ErrNotExist):
		default:
			return removed, fmt.Errorf("removing %s: %w", f.File, err)
		}
	}
	return removed, nil
}
