// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionFileName is the per-NAS session cache file the CLI keeps in its data
// directory (sid, SynoToken and the 2FA trusted-device token, mode 0600) for
// an origin (scheme://host:port). The generated session manager
// (HAND-FIX(dsm-login)) and the fleet layer share it, so `auth login` with a
// one-time code and later fleet logins to the same NAS reuse one token. A
// hash of the origin keeps names unique: "nas:5001" and "nas_5001" differ.
func SessionFileName(origin string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(origin)))
	readable := strings.NewReplacer("://", "-", ":", "_", "/", "_", "\\", "_").Replace(origin)
	return "session-" + readable + "-" + hex.EncodeToString(sum[:4]) + ".json"
}

func originOf(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func sessionFilePath(dataDir, baseURL string) string {
	origin := originOf(baseURL)
	if origin == "" || dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, SessionFileName(origin))
}

// CachedDeviceID returns the trusted-device token cached for baseURL's host
// and this account, or "" when none is cached.
func CachedDeviceID(dataDir, baseURL, account string) string {
	return CachedDeviceIDFile(sessionFilePath(dataDir, baseURL), originOf(baseURL), account)
}

// CachedDeviceIDFile is CachedDeviceID for a known session file path. The
// token is returned only for the exact origin and account it was recorded
// for: a file recorded for another origin or for no origin, another account,
// or an unknown (empty) account yields nothing.
func CachedDeviceIDFile(path, origin, account string) string {
	if path == "" || origin == "" || account == "" {
		return ""
	}
	rec := readRecord(path)
	if o, _ := rec["origin"].(string); o != origin {
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
	// #nosec G304 -- path comes from sessionFilePath: this CLI's data dir joined with a sanitized, hashed per-origin file name
	if data, err := os.ReadFile(path); err == nil {
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
	path := sessionFilePath(dataDir, baseURL)
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
	rec["origin"] = originOf(baseURL)
	if _, ok := rec["saved"]; !ok {
		rec["saved"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// ForgetSession clears the cached DSM session (sid, SynoToken, the session
// manager's account and device token) for baseURL's origin, plus account's
// entry in the per-account device-token map; other accounts' tokens stay.
// A file left with nothing to keep is removed.
func ForgetSession(dataDir, baseURL, account string) error {
	path := sessionFilePath(dataDir, baseURL)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	rec := readRecord(path)
	for _, k := range []string{"token", "synotoken", "device_id", "account", "cookies"} {
		delete(rec, k)
	}
	if ids, ok := rec["device_ids"].(map[string]any); ok {
		delete(ids, account)
		if len(ids) == 0 {
			delete(rec, "device_ids")
		}
	}
	if _, keep := rec["device_ids"]; !keep {
		return os.Remove(path)
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

// writeAtomic replaces path with data, mode 0600, through a temp file and a
// rename so a reader never sees a torn file.
// ponytail: read-modify-write is not locked across processes; two logins to
// one host in the same instant can drop one token (the next OTP login re-adds it).
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".session-*.tmp")
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

// Origin renders a base URL as scheme://host for display, dropping any user
// info, path or query.
func Origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "(invalid SYNOLOGY_BASE_URL)"
	}
	return u.Scheme + "://" + u.Host
}
