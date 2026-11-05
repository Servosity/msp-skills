// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): the device import format.

package warranty

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ImportRow is one device from an import file.
type ImportRow struct {
	Serial       string `json:"serial"`
	Vendor       string `json:"vendor,omitempty"`
	VendorSource string `json:"vendor_source,omitempty"`
	Customer     string `json:"customer,omitempty"`
	Model        string `json:"model,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
}

// ImportSkip is a row the import could not use.
type ImportSkip struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

// Column aliases, matched after lower-casing and dropping spaces, dashes and
// underscores. These cover the device exports of the RMMs MSPs feed this from
// (Datto RMM, NinjaOne and N-central all export "Serial Number" and
// "Manufacturer"). An asset tag is not a serial and is never read as one.
var columnAliases = map[string]string{
	"serial": "serial", "serialnumber": "serial", "serialno": "serial", "sn": "serial",
	"servicetag": "serial", "biosserialnumber": "serial", "deviceserial": "serial",
	"manufacturer": "vendor", "vendor": "vendor", "make": "vendor", "oem": "vendor",
	"systemmanufacturer": "vendor",
	"model":              "model", "devicemodel": "model", "modelnumber": "model", "systemmodel": "model",
	"hostname": "hostname", "computername": "hostname", "devicename": "hostname",
	"name": "hostname", "systemname": "hostname",
	"customer": "customer", "client": "customer", "clientname": "customer",
	"customername": "customer", "company": "customer", "organization": "customer",
	"organisation": "customer", "organizationname": "customer", "site": "customer",
	"sitename": "customer",
}

func canonicalColumn(h string) string {
	k := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
	k = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(k)
	return columnAliases[k]
}

// MaxImportBytes caps an import file (a 10 MB CSV is ~200k devices).
const MaxImportBytes = 10 << 20

// ParseImport reads a CSV or JSON device file. format is "csv", "json" or ""
// (sniffed from the first non-space byte). customer, when set, overrides any
// per-row customer column. Rows without a usable serial are skipped with a
// reason; a manufacturer naming a vendor this CLI does not cover (HP) is kept
// with no vendor so unresolved can report it.
func ParseImport(r io.Reader, format, customer string) ([]ImportRow, []ImportSkip, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxImportBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > MaxImportBytes {
		return nil, nil, fmt.Errorf("import file is larger than %d MB", MaxImportBytes>>20)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if format == "" {
		t := bytes.TrimSpace(data)
		if len(t) > 0 && (t[0] == '[' || t[0] == '{') {
			format = "json"
		} else {
			format = "csv"
		}
	}
	var records []map[string]string
	switch format {
	case "csv":
		records, err = csvRecords(data)
	case "json":
		records, err = jsonRecords(data)
	default:
		return nil, nil, fmt.Errorf("unknown format %q: use csv or json", format)
	}
	if err != nil {
		return nil, nil, err
	}
	rows := make([]ImportRow, 0, len(records))
	skips := make([]ImportSkip, 0)
	seen := map[string]int{}
	for i, rec := range records {
		rowNum := i + 1
		serial := NormalizeSerial(rec["serial"])
		if serial == "" {
			skips = append(skips, ImportSkip{Row: rowNum, Reason: "no serial"})
			continue
		}
		if !ValidSerial(serial) {
			skips = append(skips, ImportSkip{Row: rowNum, Reason: "serial is not 4 to 40 letters, digits, dots or dashes"})
			continue
		}
		if prev, dup := seen[serial]; dup {
			skips = append(skips, ImportSkip{Row: rowNum, Reason: fmt.Sprintf("serial repeats row %d", prev)})
			continue
		}
		seen[serial] = rowNum
		row := ImportRow{
			Serial:   serial,
			Customer: strings.TrimSpace(rec["customer"]),
			Model:    strings.TrimSpace(rec["model"]),
			Hostname: strings.TrimSpace(rec["hostname"]),
		}
		if customer != "" {
			row.Customer = customer
		}
		if v, ok := NormalizeVendor(rec["vendor"]); ok {
			row.Vendor, row.VendorSource = v, SourceDeclared
		} else if strings.TrimSpace(rec["vendor"]) == "" {
			if v, ok := DetectVendor(serial); ok {
				row.Vendor, row.VendorSource = v, SourceDetected
			}
		}
		rows = append(rows, row)
	}
	return rows, skips, nil
}

func csvRecords(data []byte) ([]map[string]string, error) {
	cr := csv.NewReader(bytes.NewReader(data))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err == io.EOF {
		return []map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}
	cols := make([]string, len(header))
	hasSerial := false
	for i, h := range header {
		cols[i] = canonicalColumn(h)
		hasSerial = hasSerial || cols[i] == "serial"
	}
	if !hasSerial {
		return nil, fmt.Errorf("CSV has no serial column (accepted: Serial, Serial Number, Service Tag, SN)")
	}
	out := []map[string]string{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV: %w", err)
		}
		m := map[string]string{}
		for i, v := range rec {
			if i < len(cols) && cols[i] != "" && m[cols[i]] == "" {
				m[cols[i]] = v
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func jsonRecords(data []byte) ([]map[string]string, error) {
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		var wrapped struct {
			Devices []map[string]any `json:"devices"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 != nil || wrapped.Devices == nil {
			return nil, fmt.Errorf("JSON import must be an array of device objects or {\"devices\": [...]}: %w", err)
		}
		raw = wrapped.Devices
	}
	out := make([]map[string]string, 0, len(raw))
	for _, obj := range raw {
		m := map[string]string{}
		for k, v := range obj {
			col := canonicalColumn(k)
			if col == "" || m[col] != "" {
				continue
			}
			switch t := v.(type) {
			case string:
				m[col] = t
			case float64, bool:
				m[col] = fmt.Sprint(t)
			}
		}
		out = append(out, m)
	}
	return out, nil
}
