// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBillingUsageDecodesBillingColumns(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "Settings": settingsList(map[string]string{
			"I1": "srv-01", "I8": "Acme", "I10": "Server Backup",
			"I57": "SRV", "I58": "SRV", "I14": "2000000000",
		})},
		{"AccountId": 2, "Settings": settingsList(map[string]string{
			"I1": "m365-acme", "I8": "Acme", "I10": "M365",
			"I57": "M365", "D19F13": "25", "D19F20": "23", "D20F13": "25",
		})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "billing", "usage", "--json")
	if err != nil {
		t.Fatalf("billing usage: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName       string  `json:"device_name"`
			SKU              string  `json:"sku"`
			UsedStorageGB    float64 `json:"used_storage_gb"`
			M365ExchangeSeat int64   `json:"m365_exchange_licenses"`
			M365ExchangeBox  int64   `json:"m365_exchange_mailboxes"`
			M365OneDriveSeat int64   `json:"m365_onedrive_licenses"`
		} `json:"items"`
		TotalDevices   int     `json:"total_devices"`
		TotalStorageGB float64 `json:"total_storage_gb"`
		TotalM365Seats int64   `json:"total_m365_licenses"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalDevices != 2 {
		t.Fatalf("expected 2 devices, got %+v", view)
	}
	// Storage decode: 2e9 bytes = 2.0 GB.
	if view.Items[0].DeviceName != "m365-acme" && view.Items[1].DeviceName != "m365-acme" {
		t.Fatalf("missing m365 row: %+v", view.Items)
	}
	var srvRow, m365Row int
	for i, it := range view.Items {
		if it.DeviceName == "srv-01" {
			srvRow = i
		} else {
			m365Row = i
		}
	}
	if view.Items[srvRow].UsedStorageGB != 2.0 || view.Items[srvRow].SKU != "SRV" {
		t.Fatalf("storage/SKU decode wrong: %+v", view.Items[srvRow])
	}
	if view.Items[m365Row].M365ExchangeSeat != 25 || view.Items[m365Row].M365ExchangeBox != 23 || view.Items[m365Row].M365OneDriveSeat != 25 {
		t.Fatalf("M365 seat decode wrong: %+v", view.Items[m365Row])
	}
	if view.TotalStorageGB != 2.0 || view.TotalM365Seats != 50 {
		t.Fatalf("totals wrong: storage=%v seats=%d", view.TotalStorageGB, view.TotalM365Seats)
	}
}

func TestBillingUsageCSVEmitsRows(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "Settings": settingsList(map[string]string{
			"I1": "srv-01", "I8": "Acme", "I57": "SRV", "I14": "1000000000",
		})},
	})
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "billing", "usage", "--csv")
	if err != nil {
		t.Fatalf("billing usage --csv: %v\n%s", err, out)
	}
	if !strings.Contains(out, "srv-01") || !strings.Contains(out, ",") {
		t.Fatalf("expected CSV rows containing the device, got:\n%s", out)
	}
	if strings.Contains(out, "scanned_devices") {
		t.Fatalf("CSV mode should emit item rows, not the wrapper envelope:\n%s", out)
	}
}
