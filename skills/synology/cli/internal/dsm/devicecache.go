// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionKey is the one canonical key of a NAS's cached session: the lower-case
// scheme and host with the default port written out (https -> 443, http ->
// 80), plus the path prefix with no trailing slash (a DSM behind a reverse
// proxy at https://proxy/nas1 is a different NAS from https://proxy/nas2), or
// "" for an address that is not an http(s) URL with a host and no user info.
// It is CanonicalAddress, the key VerifyDSM remembers, so the session file
// name, the origin recorded in it, every comparison against that origin and
// the trusted-device token cache all agree: HTTPS://NAS:443/nas1/ and
// https://nas/nas1 are one session, https://nas and http://nas never are.
// The generated session manager (HAND-FIX(dsm-login)) and the fleet client
// both key the cache through it.
func SessionKey(baseURL string) string {
	return CanonicalAddress(baseURL)
}

// SameSession reports whether two addresses (a configured base URL, an origin
// recorded in a session file) name the same cached session: equal, non-empty
// SessionKeys.
func SameSession(a, b string) bool {
	ka := SessionKey(a)
	return ka != "" && ka == SessionKey(b)
}

// SessionFileName is the per-NAS session cache file the CLI keeps in its data
// directory (sid, SynoToken and the 2FA trusted-device tokens, mode 0600) for
// baseURL's SessionKey, or "" when the address has no key. The generated
// session manager and the fleet layer share it, so `auth login` with a
// one-time code and later fleet logins to the same NAS reuse one token. The
// readable part keeps only [A-Za-z0-9.-] (safe on Windows, never a path
// separator) and is capped; a hash of the key keeps names unique ("nas:5001"
// and "nas_5001" differ).
func SessionFileName(baseURL string) string {
	key := SessionKey(baseURL)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	readable := strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, strings.Replace(key, "://", "-", 1))
	if len(readable) > 96 {
		readable = readable[:96]
	}
	return "session-" + readable + "-" + hex.EncodeToString(sum[:4]) + ".json"
}

// SessionFilePath is baseURL's session cache file in dataDir ("" without a
// data directory or a usable address).
func SessionFilePath(dataDir, baseURL string) string {
	name := SessionFileName(baseURL)
	if name == "" || dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, name)
}

// CachedDeviceID returns the trusted-device token cached for baseURL's NAS
// and this account, or "" when none is cached.
func CachedDeviceID(dataDir, baseURL, account string) string {
	return CachedDeviceIDFile(SessionFilePath(dataDir, baseURL), baseURL, account)
}

// CachedDeviceIDFile is CachedDeviceID for a known session file path. The
// token is returned only when the file's recorded origin is the same session
// as baseURL (SameSession) and for the account it was recorded for: a file
// recorded for another NAS or for no origin, another account, or an unknown
// (empty) account yields nothing.
func CachedDeviceIDFile(path, baseURL, account string) string {
	if path == "" || account == "" {
		return ""
	}
	rec := readRecord(path)
	if o, _ := rec["origin"].(string); !SameSession(o, baseURL) {
		return ""
	}
	if ids, ok := rec["device_ids"].(map[string]any); ok {
		if did, _ := ids[account].(string); did != "" {
			return did
		}
	}
	// The session manager's own record (auth login) for the same account.
	if acct, _ := rec["account"].(string); acct == account {
		did, _ := rec["device_id"].(string)
		return did
	}
	return ""
}

func readRecord(path string) map[string]any {
	rec := map[string]any{}
	// path is SessionFilePath: this CLI's data dir joined with a sanitized,
	// hashed per-NAS file name.
	if data, err := os.ReadFile(filepath.Clean(path)); err == nil {
		if json.Unmarshal(data, &rec) != nil || rec == nil {
			rec = map[string]any{} // unreadable or JSON null: start over
		}
	}
	return rec
}

// SaveDeviceID records a trusted-device token for account in baseURL's
// session cache file (a per-account map, so two accounts on one host never
// overwrite each other), keeping the session manager's fields, mode 0600.
func SaveDeviceID(dataDir, baseURL, account, deviceID string) error {
	path := SessionFilePath(dataDir, baseURL)
	if path == "" || deviceID == "" {
		return errors.New("no session cache path or device token")
	}
	rec := readRecord(path)
	ids, _ := rec["device_ids"].(map[string]any)
	if ids == nil {
		ids = map[string]any{}
	}
	ids[account] = deviceID
	rec["device_ids"] = ids
	rec["origin"] = SessionKey(baseURL)
	if _, ok := rec["saved"]; !ok {
		rec["saved"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// WriteSessionRecord writes the session manager's JSON record to path,
// carrying over the per-account device_ids map already in the file (plus this
// account's token), so `auth login` never discards a token a fleet login cached.
func WriteSessionRecord(path string, data []byte, account, deviceID string) error {
	rec := map[string]any{}
	if err := json.Unmarshal(data, &rec); err != nil || rec == nil {
		return errors.New("session record is not a JSON object")
	}
	ids, _ := readRecord(path)["device_ids"].(map[string]any)
	if ids == nil {
		ids = map[string]any{}
	}
	if account != "" && deviceID != "" {
		ids[account] = deviceID
	}
	if len(ids) > 0 {
		rec["device_ids"] = ids
	}
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, out)
}

// atomicTempPattern is writeAtomic's os.CreateTemp pattern for path:
// ".<file name>.<digits>.tmp", the only temp file name sessionOwner maps back
// to its session file.
func atomicTempPattern(path string) string {
	return "." + filepath.Base(path) + ".*.tmp"
}

// writeAtomic replaces path with data, mode 0600, through a temp file and a
// rename so a reader never sees a torn file. The temp file is named
// ".<file name>.<digits>.tmp", so a crash leaves one that names the session
// file it belongs to (auth logout removes it only as that file's temp file).
// ponytail: read-modify-write is not locked across processes; two logins to
// one host in the same instant can drop one token (the next OTP login re-adds it).
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), atomicTempPattern(path))
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after a successful rename
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path) // CreateTemp files are already 0600
}
