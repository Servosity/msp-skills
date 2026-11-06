// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	// StaleEntities names entities the latest sync run did not refresh (e.g.
	// after 'inventory sync --only assets'): their delta compares the runs
	// that did refresh them, so it is as of their latest refresh.
	StaleEntities []string `json:"stale_entities,omitempty"`
	// NoBaselineEntities names entities whose latest refresh has no earlier
	// refresh under the same --search filter to compare with; their delta is
	// empty (runzero handfix r1-compare-same-scope).
	NoBaselineEntities []string `json:"no_baseline_entities,omitempty"`
}

// ExposureDelta reports services and vulnerabilities present at their
// entity's latest refresh and absent at its baseline, each joined to its asset
// and ordered by asset criticality (highest first). Each entity is compared
// across its own refreshes under the same --search filter, by presence at the
// two runs, as Diff does (runzero handfix r1-presence-per-refresh: a service
// closed and reopened is newly exposed). since picks the baseline as the most
// recent such refresh at/before now-since; zero compares the two most recent.
// With fewer than two sync runs the result is empty: run 'inventory sync' at
// least twice to establish a baseline.
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

	// Name the entities the latest sync run did not refresh: their delta
	// is as of their own latest refresh.
	for _, entity := range []string{"services", "vulnerabilities", "assets"} {
		if cur := currentRun(ctx, db, entity); cur < latest {
			res.StaleEntities = append(res.StaleEntities, entity)
		}
	}

	// Each row joins its asset only in the current asset snapshot (runzero
	// handfix r2-current-asset-snapshot-joins); Coverage names the rest.
	ra := currentRun(ctx, db, "assets")

	// newRows binds the window for rows of entity present at its latest
	// refresh and not at its baseline, for the constant queries below: the
	// row's last_run (L), a first_run floor (F), and the run its presence is
	// checked at (P, 0 = no presence check). ok false when the entity has
	// nothing comparable.
	newRows := func(entity string) (l, f, p int64, ok bool) {
		c := entityComparison(ctx, db, entity, since)
		if !c.recorded {
			// A store no run of this build wrote: the global runs.
			return latest, baseline, 0, true
		}
		if c.noBaseline {
			res.NoBaselineEntities = append(res.NoBaselineEntities, entity)
		}
		if c.latest == 0 || c.noBaseline {
			return 0, 0, 0, false
		}
		if c.baseline == 0 {
			if !rowsBefore(ctx, db, entity, c.latest) {
				// Its first refresh, and nothing older to compare with: no
				// baseline, not every row new (runzero handfix
				// r2-first-refresh-has-no-baseline).
				res.NoBaselineEntities = append(res.NoBaselineEntities, entity)
				return 0, 0, 0, false
			}
			// Its first recorded refresh over rows an older build wrote:
			// the rows first seen in it are new.
			return c.latest, c.latest - 1, 0, true
		}
		if !presenceRecorded(ctx, db, entity, c.baseline) {
			// The baseline has no presence history (an older build wrote
			// it): rows first seen after it are new (runzero handfix
			// r2-presence-history-per-run).
			return c.latest, c.baseline, 0, true
		}
		return c.latest, 0, c.baseline, true
	}

	if l, f, p, ok := newRows("services"); ok {
		svcRows, err := db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.asset_id,''), COALESCE(a.name,''), COALESCE(s.address, a.address, ''),
			COALESCE(a.criticality,''), COALESCE(a.external,0),
			COALESCE(s.transport,''), COALESCE(s.port,0), COALESCE(s.protocol,''), COALESCE(s.product,'')
		FROM inv_services s
		LEFT JOIN inv_assets a ON a.id = s.asset_id AND a.last_run = ?
		WHERE s.last_run = ? AND s.first_run > ?
			AND NOT EXISTS (SELECT 1 FROM inv_presence pr WHERE pr.entity = 'services' AND pr.id = s.id AND pr.first_run <= ? AND pr.last_run >= ?)
		ORDER BY COALESCE(a.criticality_rank,0) DESC, COALESCE(a.external,0) DESC, s.port ASC`, ra, l, f, p, p)
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
	}

	if l, f, p, ok := newRows("vulnerabilities"); ok {
		vulnRows, err := db.QueryContext(ctx, `
		SELECT COALESCE(v.asset_id,''), COALESCE(a.name,''), COALESCE(a.address,''),
			COALESCE(a.criticality,''), COALESCE(a.external,0),
			COALESCE(v.cve,''), COALESCE(v.name,''), COALESCE(v.severity,''), COALESCE(v.cvss,0)
		FROM inv_vulnerabilities v
		LEFT JOIN inv_assets a ON a.id = v.asset_id AND a.last_run = ?
		WHERE v.last_run = ? AND v.first_run > ?
			AND NOT EXISTS (SELECT 1 FROM inv_presence pr WHERE pr.entity = 'vulnerabilities' AND pr.id = v.id AND pr.first_run <= ? AND pr.last_run >= ?)
		ORDER BY COALESCE(a.criticality_rank,0) DESC, COALESCE(v.severity_rank,0) DESC, v.cvss DESC`, ra, l, f, p, p)
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

// CertsExpiring lists certificates of the current snapshot that are already
// expired or expire within the next `days` days, plus (with weakOnly) only
// those that are self-signed or carry a weak signature algorithm (MD5/SHA-1,
// detected from the raw cert JSON when present). Rows join to the presenting
// asset in the current asset snapshot (runzero handfix
// r2-current-asset-snapshot-joins) and order by asset criticality, then
// soonest expiry.
func CertsExpiring(ctx context.Context, db *sql.DB, days int, weakOnly bool) ([]CertRow, error) {
	if days <= 0 {
		days = 30
	}
	cutoff := clock().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := clock().Unix()
	rows, err := db.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.subject,''), COALESCE(c.issuer,''), COALESCE(c.not_after,0),
			COALESCE(c.self_signed,0), COALESCE(c.data,''),
			COALESCE(c.asset_id,''), COALESCE(a.name,''), COALESCE(a.address,''), COALESCE(a.criticality,'')
		FROM inv_certificates c
		LEFT JOIN inv_assets a ON a.id = c.asset_id AND a.last_run = ?
		WHERE c.last_run = ?
		ORDER BY COALESCE(a.criticality_rank,0) DESC, c.not_after ASC`, currentRun(ctx, db, "assets"), currentRun(ctx, db, "certificates"))
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
