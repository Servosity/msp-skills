// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): handfixes.json novel-money-shapes.

package cli

import (
	"io"
	"reflect"
	"strings"
)

// pax8PrintNovelJSON prints a novel command's result keeping, under --agent and
// --compact, every field its row types define. The shared compactor drops a key
// that only some rows carry, and on these commands that key is the one saying a
// figure is partial (subscriptionsWithoutCost, notOwedInvoices) or which
// currency a row is in.
func pax8PrintNovelJSON(w io.Writer, v any, flags *rootFlags) error {
	return printJSONFilteredKeep(w, v, flags, pax8JSONKeys(reflect.TypeOf(v), map[reflect.Type]bool{})...)
}

// pax8JSONKeys lists the JSON names of every exported field reachable from t.
func pax8JSONKeys(t reflect.Type, seen map[reflect.Type]bool) []string {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct || seen[t] {
		return nil
	}
	seen[t] = true
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		keys = append(keys, name)
		keys = append(keys, pax8JSONKeys(f.Type, seen)...)
	}
	return keys
}
