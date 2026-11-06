// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: brute-force oracle for subscriptions changes previous_seats.
// Every stated previous_seats must be the one value the order history
// allows in every ordering consistent with the order times (adversarial
// round 26 wrote the oracle; it closes the seat-history inference class).

package cli

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"
)

type oracleord struct {
	id, status, cust, res string
	t                     int // day offset; 0 = undated
	qty                   int // -1 unset
	create                bool
}

// oracle: for each order index, the set of possible previous counts ("?" = unknown) over every ordering consistent with times.
func oracleoracle(subs map[string]string, ords []oracleord) []map[string]bool {
	n := len(ords)
	poss := make([]map[string]bool, n)
	for i := range poss {
		poss[i] = map[string]bool{}
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	var permute func(k int)
	valid := func(p []int) bool {
		// dated orders must be non-decreasing in time
		last := -1 << 30
		for _, i := range p {
			if ords[i].t == 0 {
				continue
			}
			if ords[i].t < last {
				return false
			}
			last = ords[i].t
		}
		return true
	}
	eval := func(p []int) {
		count := map[string]string{} // res -> value or "?"
		for r := range subs {
			count[r] = "?"
		}
		for _, i := range p {
			o := ords[i]
			if o.res != "" {
				poss[i][count[o.res]] = true
			}
			if o.status != "COMPLETED" {
				continue
			}
			if o.res == "" {
				if o.create {
					continue // a new purchase changes no existing subscription
				}
				for r, c := range subs {
					if o.cust == "" || c == o.cust {
						count[r] = "?"
					}
				}
				continue
			}
			if o.qty > 0 {
				count[o.res] = fmt.Sprint(o.qty)
			} else {
				count[o.res] = "?"
			}
		}
	}
	permute = func(k int) {
		if k == n {
			if valid(idx) {
				eval(idx)
			}
			return
		}
		for i := k; i < n; i++ {
			idx[k], idx[i] = idx[i], idx[k]
			permute(k + 1)
			idx[k], idx[i] = idx[i], idx[k]
		}
	}
	permute(0)
	return poss
}

// oracleIterations keeps the default run short; raise it locally to hunt.
var oracleIterations = 4000

func TestChangesPreviousSeatsOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(2626))
	// r4 belongs to c2 but is not synced: the book holds no subscription for it.
	subsCust := map[string]string{"r1": "c1", "r2": "c1", "r3": "c2", "r4": "c2"}
	statuses := []string{"COMPLETED", "COMPLETED", "COMPLETED", "ERROR", "CANCELED", "IN_PROGRESS"}
	bad := map[string]int{}
	shown := 0
	for iter := 0; iter < oracleIterations; iter++ {
		// Items, grouped into orders: an order's items share its status,
		// customer and time, and may come in any order among themselves.
		nOrders := 2 + rng.Intn(4)
		var items []oracleord
		var orderOf []int
		for k := 0; k < nOrders; k++ {
			status := statuses[rng.Intn(len(statuses))]
			tt := []int{0, -9, -8, -8, -7, -6, -5, -5, -4}[rng.Intn(9)]
			nItems := 1
			if rng.Intn(4) == 0 {
				nItems = 2
			}
			ocust := []string{"c1", "c2", "", "auto", "auto", "auto"}[rng.Intn(6)]
			for j := 0; j < nItems && len(items) < 6; j++ {
				res := []string{"r1", "r1", "r1", "r2", "r3", "r4", ""}[rng.Intn(7)]
				cust := ocust
				if cust == "auto" {
					cust = ""
					if res != "" {
						cust = subsCust[res]
					}
				}
				q := []int{-1, 0, 1, 2, 3, 3, 4, 5}[rng.Intn(8)]
				items = append(items, oracleord{id: fmt.Sprintf("o%02d", k), status: status, cust: cust, res: res, t: tt, qty: q, create: rng.Intn(5) == 0})
				orderOf = append(orderOf, k)
			}
		}
		// An order's customer is one value; take the first item's.
		for x := range items {
			for y := range items {
				if orderOf[y] == orderOf[x] {
					items[x].cust = items[y].cust
					break
				}
			}
		}
		synced := map[string]float64{"r1": 3, "r2": 2, "r3": 4}
		b := &ionBook{Customers: map[string]ionCustomer{}}
		for _, r := range []string{"r1", "r2", "r3"} {
			b.Subs = append(b.Subs, ionSub{ID: "s" + r, SubscriptionID: r, CustomerID: subsCust[r], Seats: optFloat{V: synced[r], Set: true}})
		}
		for k := 0; k < nOrders; k++ {
			var o *ionOrder
			for x, it := range items {
				if orderOf[x] != k {
					continue
				}
				if o == nil {
					ct := ""
					if it.t != 0 {
						ct = testNow.AddDate(0, 0, it.t).Format(time.RFC3339)
					}
					b.Orders = append(b.Orders, ionOrder{OrderID: it.id, CustomerID: it.cust, Status: it.status, CreateTime: ct})
					o = &b.Orders[len(b.Orders)-1]
				}
				oi := ionOrderItem{Action: "UPDATE", ResourceID: it.res}
				if it.create {
					oi.Action = "CREATE"
				}
				if it.qty >= 0 {
					oi.Quantity = optFloat{V: float64(it.qty), Set: true}
				}
				o.Items = append(o.Items, oi)
			}
		}
		poss := oracleoracle(subsCust, items)
		rows := subscriptionChanges(b, testNow, 3650*24*time.Hour, "", true)
		seen := map[string]int{}
		for _, r := range rows {
			j := seen[r.OrderID]
			seen[r.OrderID]++
			x := -1
			for y, it := range items {
				if it.id == r.OrderID {
					if j == 0 {
						x = y
						break
					}
					j--
				}
			}
			if x < 0 || r.Change != "seats" || r.PreviousSeats == nil {
				continue
			}
			o := items[x]
			got := fmt.Sprint(*r.PreviousSeats)
			var want []string
			kind := "history"
			if o.status == "IN_PROGRESS" {
				kind = "open"
				if v, ok := synced[o.res]; ok {
					want = []string{fmt.Sprint(v)}
				} // an open order on an unsynced subscription states nothing
			} else {
				for k := range poss[x] {
					want = append(want, k)
				}
				sort.Strings(want)
			}
			if len(want) != 1 || want[0] != got {
				bad[kind+"/"+o.status]++
				if shown < 6 {
					shown++
					var sb strings.Builder
					for _, it := range items {
						fmt.Fprintf(&sb, "  %s %s cust=%q res=%s t=%d qty=%d\n", it.id, it.status, it.cust, it.res, it.t, it.qty)
					}
					t.Logf("STATED %s for %s item (%s) but the oracle allows %v\n%s", got, o.id, o.status, want, sb.String())
				}
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("previous_seats stated where the history does not establish it: %v", bad)
	}
}
