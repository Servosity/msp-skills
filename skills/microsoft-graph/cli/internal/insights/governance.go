// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// governance.go holds the 2026-06 reprint additions to the insights package:
// the per-SKU license consumer map and the group sprawl/risk audit. Same
// contract as insights.go — pure functions over already-fetched JSON rows so
// everything is unit-testable without a live tenant.

package insights

import (
	"encoding/json"
	"sort"
	"strings"
)

// ---- licenses map ------------------------------------------------------------

// LicenseConsumer is one user currently holding the mapped SKU.
type LicenseConsumer struct {
	UserPrincipalName string   `json:"userPrincipalName"`
	DisplayName       string   `json:"displayName"`
	AccountEnabled    bool     `json:"accountEnabled"`
	UserType          string   `json:"userType,omitempty"`
	Flags             []string `json:"flags,omitempty"`
}

// LicenseMapResult is the consumer map for one subscribed SKU.
type LicenseMapResult struct {
	Query            string            `json:"query"`
	SkuPartNumber    string            `json:"skuPartNumber,omitempty"`
	SkuID            string            `json:"skuId,omitempty"`
	EnabledUnits     int               `json:"enabledUnits"`
	ConsumedUnits    int               `json:"consumedUnits"`
	Consumers        []LicenseConsumer `json:"consumers"`
	ReclaimableSeats int               `json:"reclaimableSeats"`
	Note             string            `json:"note,omitempty"`
}

// LicenseMap resolves `query` against the subscribedSkus (matching
// skuPartNumber or skuId, case-insensitively) and returns every user holding
// that SKU with their account state. ReclaimableSeats counts consumers flagged
// disabled or guest — the seats 'licenses orphans' would also surface, here
// scoped to one SKU so a reassignment plan can be drawn per renewal line item.
// ok=false when no subscribed SKU matches the query.
func LicenseMap(users, skus []json.RawMessage, query string) (LicenseMapResult, bool) {
	res := LicenseMapResult{Query: query, Consumers: []LicenseConsumer{}}
	var matched *subscribedSku
	for _, raw := range skus {
		var s subscribedSku
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if strings.EqualFold(s.SkuPartNumber, query) || strings.EqualFold(s.SkuID, query) {
			matched = &s
			break
		}
	}
	if matched == nil {
		return res, false
	}
	res.SkuPartNumber = matched.SkuPartNumber
	res.SkuID = matched.SkuID
	res.EnabledUnits = matched.PrepaidUnits.Enabled
	res.ConsumedUnits = matched.ConsumedUnits

	for _, raw := range users {
		var u userLite
		if err := json.Unmarshal(raw, &u); err != nil {
			continue
		}
		holds := false
		for _, l := range u.AssignedLicenses {
			if strings.EqualFold(l.SkuID, matched.SkuID) {
				holds = true
				break
			}
		}
		if !holds {
			continue
		}
		var flagsList []string
		disabled := u.AccountEnabled != nil && !*u.AccountEnabled
		guest := strings.EqualFold(u.UserType, "Guest")
		if disabled {
			flagsList = append(flagsList, "disabled")
		}
		if guest {
			flagsList = append(flagsList, "guest")
		}
		if disabled || guest {
			res.ReclaimableSeats++
		}
		res.Consumers = append(res.Consumers, LicenseConsumer{
			UserPrincipalName: u.UserPrincipalName,
			DisplayName:       u.DisplayName,
			AccountEnabled:    u.AccountEnabled != nil && *u.AccountEnabled,
			UserType:          u.UserType,
			Flags:             flagsList,
		})
	}
	// Reclaimable (flagged) consumers first, then stable by UPN, so the
	// reassignment candidates lead the list.
	sort.SliceStable(res.Consumers, func(i, j int) bool {
		fi, fj := len(res.Consumers[i].Flags) > 0, len(res.Consumers[j].Flags) > 0
		if fi != fj {
			return fi
		}
		return res.Consumers[i].UserPrincipalName < res.Consumers[j].UserPrincipalName
	})
	return res, true
}

// ---- groups risk ---------------------------------------------------------------

// RiskGroup is one group flagged by the sprawl/risk audit.
type RiskGroup struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"displayName"`
	Members      int      `json:"members"`
	GuestMembers int      `json:"guestMembers"`
	Owners       int      `json:"owners"`
	Reasons      []string `json:"reasons"`
}

// GroupsRiskResult is the audit envelope: flagged groups plus scan accounting,
// so an empty result can be told apart from "no membership data was synced".
type GroupsRiskResult struct {
	Groups              []RiskGroup `json:"groups"`
	ScannedGroups       int         `json:"scannedGroups"`
	MissingAssociations int         `json:"missingAssociationData,omitempty"`
	GuestRatio          float64     `json:"guestRatioThreshold"`
	Note                string      `json:"note,omitempty"`
}

type ownerRef struct {
	ID string `json:"id"`
}

// groupLite uses pointers for the embedded association arrays so a group row
// synced without them (e.g. via the generic `sync` instead of `pull`) is
// distinguishable from a group that genuinely has zero members or owners.
type groupLite struct {
	ID          string        `json:"id"`
	DisplayName string        `json:"displayName"`
	Members     *[]roleMember `json:"members"`
	Owners      *[]ownerRef   `json:"owners"`
}

// GroupsRisk flags ownerless, empty, and guest-heavy groups. A group is
// guest-heavy when guest members / total members >= guestRatio (and it has at
// least one member). Groups whose rows carry no embedded members/owners data
// are counted in MissingAssociations and never flagged — absence of evidence
// is reported, not treated as risk. Flagged groups sort most-reasons-first.
func GroupsRisk(groups []json.RawMessage, guestRatio float64) GroupsRiskResult {
	res := GroupsRiskResult{Groups: []RiskGroup{}, GuestRatio: guestRatio}
	for _, raw := range groups {
		var g groupLite
		if err := json.Unmarshal(raw, &g); err != nil {
			continue
		}
		res.ScannedGroups++
		if g.Members == nil && g.Owners == nil {
			res.MissingAssociations++
			continue
		}
		var reasons []string
		memberCount, guestCount, ownerCount := 0, 0, 0
		if g.Members != nil {
			memberCount = len(*g.Members)
			for _, m := range *g.Members {
				if strings.EqualFold(m.UserType, "Guest") {
					guestCount++
				}
			}
			if memberCount == 0 {
				reasons = append(reasons, "empty")
			} else if guestRatio > 0 && float64(guestCount)/float64(memberCount) >= guestRatio {
				reasons = append(reasons, "guest-heavy")
			}
		}
		if g.Owners != nil {
			ownerCount = len(*g.Owners)
			if ownerCount == 0 {
				reasons = append(reasons, "ownerless")
			}
		}
		if len(reasons) == 0 {
			continue
		}
		res.Groups = append(res.Groups, RiskGroup{
			ID:           g.ID,
			DisplayName:  g.DisplayName,
			Members:      memberCount,
			GuestMembers: guestCount,
			Owners:       ownerCount,
			Reasons:      reasons,
		})
	}
	if res.MissingAssociations > 0 {
		res.Note = "some group rows lack embedded members/owners data; run 'microsoft-graph-cli pull --only groups' to embed associations"
	}
	sort.SliceStable(res.Groups, func(i, j int) bool {
		if len(res.Groups[i].Reasons) != len(res.Groups[j].Reasons) {
			return len(res.Groups[i].Reasons) > len(res.Groups[j].Reasons)
		}
		return res.Groups[i].DisplayName < res.Groups[j].DisplayName
	})
	return res
}
