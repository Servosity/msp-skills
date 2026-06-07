// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Exposure delta: the security-relevant subset of Diff — services/ports that
// became newly exposed and assets that became newly vulnerable since the
// baseline sync, ranked by asset criticality.
// ---------------------------------------------------------------------------

// DeltaService is one newly-exposed service joined to its asset.
type DeltaService struct {
	AssetID     string `json:"asset_id"`
	AssetName   string `json:"asset_name"`
	Address     string `json:"address"`
	Criticality string `json:"criticality"`
	External    bool   `json:"external"`
	Transport   string `json:"transport"`
	Port        int64  `json:"port"`
	Protocol    string `json:"protocol"`
	Product     string `json:"product,omitempty"`
}

// DeltaVuln is one newly-detected vulnerability joined to its asset.
type DeltaVuln struct {
	AssetID     string  `json:"asset_id"`
	AssetName   string  `json:"asset_name"`
	Address     string  `json:"address"`
	Criticality string  `json:"criticality"`
	External    bool    `json:"external"`
	CVE         string  `json:"cve,omitempty"`
	Name        string  `json:"name"`
	Severity    string  `json:"severity"`
	CVSS        float64 `json:"cvss"`
}

// ExposureDeltaResult is the newly-exposed / newly-vulnerable delta between
// the latest sync and a baseline sync.
type ExposureDeltaResult struct {
	BaselineRun  int64          `json:"baseline_run"`
	LatestRun    int64          `json:"latest_run"`
	NewServices  []DeltaService `json:"new_services"`
	NewVulns     []DeltaVuln    `json:"new_vulnerabilities"`
	SyncRunCount int64          `json:"sync_run_count"`
	// StaleEntities names entity tables whose newest rows predate the latest
	// sync run (e.g. after 'inventory sync --only assets'). An empty delta
	// for a stale entity means "not re-synced", not "nothing changed".
	StaleEntities []string `json:"stale_entities,omitempty"`
}

// ExposureDelta reports services that first appeared after the baseline run
// and vulnerabilities first detected after the baseline run, each joined to
// their asset and ordered by asset criticality (highest first). since picks
// the baseline as the most recent run at/before now-since; zero compares the
// two most recent runs. With fewer than two sync runs the result is empty —
// run 'inventory sync' at least twice to establish a baseline.
func ExposureDelta(ctx context.Context, db *sql.DB, since time.Duration) (*ExposureDeltaResult, error) {
	res := &ExposureDeltaResult{NewServices: []DeltaService{}, NewVulns: []DeltaVuln{}}
	var runCount sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_sync_runs`).Scan(&runCount); err == nil && runCount.Valid {
		res.SyncRunCount = runCount.Int64
	}
	latest, _, ok := latestTwoRuns(ctx, db)
	if !ok || res.SyncRunCount < 2 {
		return res, nil
	}
	baseline, _ := baselineForSince(ctx, db, since)
	res.BaselineRun = baseline
	res.LatestRun = latest

	// Surface entities that were not part of the latest sync run (partial
	// 'inventory sync --only ...'): their delta below would be silently
	// empty even when the live surface changed.
	for _, probe := range []struct{ entity, table string }{
		{"services", "inv_services"},
		{"vulnerabilities", "inv_vulnerabilities"},
		{"assets", "inv_assets"},
	} {
		var maxRun sql.NullInt64
		if err := db.QueryRowContext(ctx, `SELECT MAX(last_run) FROM `+probe.table).Scan(&maxRun); err == nil && maxRun.Valid && maxRun.Int64 < latest {
			res.StaleEntities = append(res.StaleEntities, probe.entity)
		}
	}

	svcRows, err := db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.asset_id,''), COALESCE(a.name,''), COALESCE(s.address, a.address, ''),
			COALESCE(a.criticality,''), COALESCE(a.external,0),
			COALESCE(s.transport,''), COALESCE(s.port,0), COALESCE(s.protocol,''), COALESCE(s.product,'')
		FROM inv_services s
		LEFT JOIN inv_assets a ON a.id = s.asset_id
		WHERE s.first_run > ? AND s.last_run = ?
		ORDER BY COALESCE(a.criticality_rank,0) DESC, COALESCE(a.external,0) DESC, s.port ASC`, baseline, latest)
	if err == nil {
		for svcRows.Next() {
			var id string
			var d DeltaService
			var ext int64
			if svcRows.Scan(&id, &d.AssetID, &d.AssetName, &d.Address, &d.Criticality, &ext, &d.Transport, &d.Port, &d.Protocol, &d.Product) == nil {
				d.External = ext != 0
				res.NewServices = append(res.NewServices, d)
			}
		}
		_ = svcRows.Close()
	}

	vulnRows, err := db.QueryContext(ctx, `
		SELECT COALESCE(v.asset_id,''), COALESCE(a.name,''), COALESCE(a.address,''),
			COALESCE(a.criticality,''), COALESCE(a.external,0),
			COALESCE(v.cve,''), COALESCE(v.name,''), COALESCE(v.severity,''), COALESCE(v.cvss,0)
		FROM inv_vulnerabilities v
		LEFT JOIN inv_assets a ON a.id = v.asset_id
		WHERE v.first_run > ? AND v.last_run = ?
		ORDER BY COALESCE(a.criticality_rank,0) DESC, COALESCE(v.severity_rank,0) DESC, v.cvss DESC`, baseline, latest)
	if err == nil {
		for vulnRows.Next() {
			var d DeltaVuln
			var ext int64
			if vulnRows.Scan(&d.AssetID, &d.AssetName, &d.Address, &d.Criticality, &ext, &d.CVE, &d.Name, &d.Severity, &d.CVSS) == nil {
				d.External = ext != 0
				res.NewVulns = append(res.NewVulns, d)
			}
		}
		_ = vulnRows.Close()
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Certificate expiry / weak-crypto watch over the synced certificates table.
// ---------------------------------------------------------------------------

// CertRow is one expiring or weak certificate joined to its asset.
type CertRow struct {
	CertID      string `json:"cert_id"`
	Subject     string `json:"subject"`
	Issuer      string `json:"issuer"`
	NotAfter    string `json:"not_after,omitempty"`
	DaysLeft    int64  `json:"days_left"`
	Expired     bool   `json:"expired"`
	SelfSigned  bool   `json:"self_signed"`
	WeakSig     bool   `json:"weak_signature"`
	AssetID     string `json:"asset_id,omitempty"`
	AssetName   string `json:"asset_name,omitempty"`
	Address     string `json:"address,omitempty"`
	Criticality string `json:"criticality,omitempty"`
}

// CertsExpiring lists certificates from the latest sync that are already
// expired or expire within the next `days` days, plus (with weakOnly) only
// those that are self-signed or carry a weak signature algorithm (MD5/SHA-1,
// detected from the raw cert JSON when present). Rows join to the presenting
// asset and order by asset criticality, then soonest expiry.
func CertsExpiring(ctx context.Context, db *sql.DB, days int, weakOnly bool) ([]CertRow, error) {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := time.Now().Unix()
	rows, err := db.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.subject,''), COALESCE(c.issuer,''), COALESCE(c.not_after,0),
			COALESCE(c.self_signed,0), COALESCE(c.data,''),
			COALESCE(c.asset_id,''), COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,'')
		FROM inv_certificates c
		LEFT JOIN inv_assets a ON a.id = c.asset_id
		WHERE c.last_run = (SELECT MAX(last_run) FROM inv_certificates)
		ORDER BY COALESCE(a.criticality_rank,0) DESC, c.not_after ASC`)
	if err != nil {
		return []CertRow{}, nil // empty/absent store: degrade gracefully
	}
	defer rows.Close()
	out := []CertRow{}
	for rows.Next() {
		var r CertRow
		var notAfter, selfSigned int64
		var data string
		if rows.Scan(&r.CertID, &r.Subject, &r.Issuer, &notAfter, &selfSigned, &data, &r.AssetID, &r.AssetName, &r.Address, &r.Criticality) != nil {
			continue
		}
		r.SelfSigned = selfSigned != 0
		r.WeakSig = weakSignature(data)
		if notAfter > 0 {
			r.NotAfter = time.Unix(notAfter, 0).UTC().Format(time.RFC3339)
			if notAfter >= now {
				r.DaysLeft = (notAfter - now) / 86400
			} else {
				// Floor toward negative so a cert expired <24h ago reports
				// -1, keeping days_left<0 a reliable "already expired" signal.
				r.DaysLeft = -((now - notAfter + 86399) / 86400)
			}
			r.Expired = notAfter <= now
		}
		expiringSoon := notAfter > 0 && notAfter <= cutoff
		weak := r.SelfSigned || r.WeakSig
		if weakOnly {
			if !weak {
				continue
			}
		} else if !expiringSoon {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// weakSignature reports whether the certificate JSON names an MD5 or SHA-1
// based signature algorithm. It decodes the payload and inspects only
// signature-algorithm-shaped fields (keys containing "signature" or "alg"),
// so SHA-1 *fingerprint* fields — which runZero cert exports commonly carry —
// do not false-positive. Detection is best-effort; absent or unrecognized
// fields simply report false.
func weakSignature(data string) bool {
	if data == "" {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err == nil && m != nil {
		return weakSigInFields(m)
	}
	// Non-object payload: fall back to algorithm-value matching on the raw
	// string (full algorithm tokens, or the whole payload being exactly
	// "sha1"/"md5"); fingerprint-shaped strings do not match.
	return weakAlgValue(data)
}

func weakSigInFields(m map[string]any) bool {
	for k, v := range m {
		lk := strings.ToLower(k)
		isAlgKey := strings.Contains(lk, "sig") || strings.Contains(lk, "alg")
		// Fingerprint/thumbprint/hash keys carry digests of the cert, not
		// its signature algorithm.
		if strings.Contains(lk, "fingerprint") || strings.Contains(lk, "thumbprint") || strings.Contains(lk, "hash") {
			isAlgKey = false
		}
		switch val := v.(type) {
		case string:
			if isAlgKey && weakAlgValue(val) {
				return true
			}
		case map[string]any:
			if weakSigInFields(val) {
				return true
			}
		}
	}
	return false
}

// weakAlgValue matches an algorithm *value* (e.g. "SHA1withRSA",
// "ecdsa-with-SHA1", "md5WithRSAEncryption", or a bare "sha1" in an
// algorithm-named field).
func weakAlgValue(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "sha1" || low == "md5" {
		return true
	}
	for _, marker := range []string{"md5withrsa", "sha1withrsa", "md5-rsa", "sha1-rsa", "ecdsa-with-sha1", "dsa-with-sha1"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}
