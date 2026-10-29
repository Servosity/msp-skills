// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): vendor routing for serials.

// Package warranty holds the vendor-neutral warranty logic: which vendor a
// serial belongs to, the normalized coverage-line shape every vendor response
// is parsed into, the client import file format, and coverage events.
package warranty

import (
	"strings"
	"unicode"
)

// Vendors this connector can look up.
const (
	Dell   = "dell"
	Lenovo = "lenovo"
	Apple  = "apple"
)

// Vendors lists the supported vendors in display order.
var Vendors = []string{Dell, Lenovo, Apple}

// Vendor sources: how a device's vendor was decided.
const (
	SourceDeclared = "declared" // --vendor, or a manufacturer column
	SourceDetected = "detected" // serial-length heuristic
)

// NormalizeVendor maps a vendor flag or a manufacturer string from an RMM
// export ("Dell Inc.", "LENOVO", "Apple Inc.") to a vendor constant. ok is
// false for anything else, including HP (its public warranty API is gone).
func NormalizeVendor(s string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "":
		return "", false
	case v == Dell || strings.HasPrefix(v, "dell"):
		return Dell, true
	case v == Lenovo || strings.HasPrefix(v, "lenovo"):
		return Lenovo, true
	case v == Apple || strings.HasPrefix(v, "apple"):
		return Apple, true
	}
	return "", false
}

// NormalizeSerial trims and upper-cases a serial. Lenovo's SERIAL.MACHINETYPE
// form keeps its dot.
func NormalizeSerial(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// ValidSerial reports whether s looks like a serial at all: 4 to 40 letters,
// digits, dots or dashes. It keeps arbitrary text out of vendor requests.
func ValidSerial(s string) bool {
	if len(s) < 4 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-') {
			return false
		}
	}
	return true
}

// DetectVendor guesses a vendor from the serial's shape, the way PSWarranty's
// Get-WarrantyInfo routes them: 7 characters is a Dell service tag, 8 is a
// Lenovo serial (SERIAL.MACHINETYPE is Lenovo too), 12 non-numeric characters
// is an Apple serial. Ten characters is ambiguous (Apple since 2021, and HP),
// so it is not guessed. ok is false when the shape names no vendor; the caller
// then needs --vendor or a manufacturer column.
func DetectVendor(serial string) (string, bool) {
	s := NormalizeSerial(serial)
	if strings.Count(s, ".") == 1 {
		sn, mt, _ := strings.Cut(s, ".")
		if sn != "" && len(mt) >= 4 && alnum(sn) && alnum(mt) {
			return Lenovo, true
		}
		return "", false
	}
	if !alnum(s) {
		return "", false
	}
	switch len(s) {
	case 7:
		return Dell, true
	case 8:
		return Lenovo, true
	case 12:
		if !allDigits(s) {
			return Apple, true
		}
	}
	return "", false
}

func alnum(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
